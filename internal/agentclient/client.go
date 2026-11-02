package agentclient

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/septi0/dockvmap/internal/agentapi"
)

const maxReportBytes = 8 << 20

var (
	ErrIdentityChanged = errors.New("the agent's identity changed")
	ErrPairedElsewhere = errors.New("the agent is paired with another DockVMap")
	ErrNotAdopted      = errors.New("the agent is waiting to be adopted")
)

type Client struct {
	identity  tls.Certificate
	userAgent string
}

func New(identity tls.Certificate, version string) *Client {
	return &Client{identity: identity, userAgent: "dockvmap/" + version}
}

func (c *Client) Adopt(ctx context.Context, address, pinned string) (agentapi.Adoption, string, error) {
	var adoption agentapi.Adoption

	resp, seen, err := c.do(ctx, http.MethodPost, address, agentapi.AdoptPath, pinned, nil)

	if err != nil {
		return adoption, seen, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return adoption, seen, statusError(resp)
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&adoption); err != nil {
		return adoption, seen, fmt.Errorf("reading the agent's answer: %w", err)
	}

	return adoption, seen, nil
}

func (c *Client) Snapshot(ctx context.Context, address, pinned, fingerprint, proxyHost string) (*agentapi.Report, error) {
	header := http.Header{agentapi.ProxyHostHeader: {proxyHost}}

	if fingerprint != "" {
		header.Set("If-None-Match", `"`+fingerprint+`"`)
	}

	resp, _, err := c.do(ctx, http.MethodGet, address, agentapi.SnapshotPath, pinned, header)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotModified:
		return nil, nil

	case http.StatusOK:
		var report agentapi.Report

		if err := json.NewDecoder(io.LimitReader(resp.Body, maxReportBytes)).Decode(&report); err != nil {
			return nil, fmt.Errorf("reading the agent's snapshot: %w", err)
		}

		return &report, nil
	}

	return nil, statusError(resp)
}

func (c *Client) do(ctx context.Context, method, address, path, pinned string, header http.Header) (*http.Response, string, error) {
	var seen string

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			Certificates:       []tls.Certificate{c.identity},
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionTLS13,
			VerifyConnection: func(state tls.ConnectionState) error {
				if len(state.PeerCertificates) == 0 {
					return errors.New("the agent sent no certificate")
				}

				seen = agentapi.Fingerprint(state.PeerCertificates[0])

				if pinned != "" && seen != pinned {
					return ErrIdentityChanged
				}

				return nil
			},
		},
		DialContext:         (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableKeepAlives:   true,
	}

	defer transport.CloseIdleConnections()

	req, err := http.NewRequestWithContext(ctx, method, "https://"+address+path, nil)

	if err != nil {
		return nil, "", err
	}

	for key, values := range header {
		req.Header[key] = values
	}

	req.Header.Set("User-Agent", c.userAgent)

	resp, err := (&http.Client{Transport: transport}).Do(req)

	if err != nil {
		return nil, seen, describe(err)
	}

	return resp, seen, nil
}

func statusError(resp *http.Response) error {
	var body agentapi.Error

	_ = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&body)

	switch resp.StatusCode {
	case http.StatusForbidden:
		return ErrPairedElsewhere
	case http.StatusConflict:
		return ErrNotAdopted
	}

	if body.Error == "" {
		return fmt.Errorf("the agent answered %s", resp.Status)
	}

	return fmt.Errorf("the agent answered %s: %s", resp.Status, body.Error)
}

func describe(err error) error {
	if errors.Is(err, ErrIdentityChanged) {
		return ErrIdentityChanged
	}

	var netErr net.Error

	if errors.As(err, &netErr) && netErr.Timeout() {
		return errors.New("the agent didn't answer in time; check the address and that DockVMap can reach the port")
	}

	var urlErr *url.Error

	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}

	return err
}
