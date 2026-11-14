package replication

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/septi0/dockvmap/internal/agentapi"
	"github.com/septi0/dockvmap/internal/ocicopy"
	"github.com/septi0/dockvmap/internal/replicationapi"
	"github.com/septi0/dockvmap/internal/service"
)

const (
	maxStateBytes = 64 << 20
	maxErrorBytes = 64 << 10
)

type Client struct {
	primary   string
	secret    string
	userAgent string
	http      *http.Client
}

func NewClient(primary, token, version string) (*Client, error) {
	fingerprint, secret, err := replicationapi.ParseToken(token)

	if err != nil {
		return nil, err
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionTLS13,
			VerifyConnection: func(state tls.ConnectionState) error {
				if len(state.PeerCertificates) == 0 || agentapi.Fingerprint(state.PeerCertificates[0]) != fingerprint {
					return replicationapi.ErrIdentityChanged
				}

				return nil
			},
		},
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: time.Minute,
		IdleConnTimeout:       90 * time.Second,
	}

	return &Client{primary: primary, secret: secret, userAgent: userAgentPrefix + version, http: &http.Client{Transport: transport}}, nil
}

func (c *Client) header() http.Header {
	return http.Header{"Authorization": {"Bearer " + c.secret}, "User-Agent": {c.userAgent}}
}

func (c *Client) Registry() ocicopy.Registry {
	return ocicopy.Registry{BaseURL: "https://" + c.primary + replicationapi.RegistryPrefix, Client: c.http, Header: c.header()}
}

// send adds the token and maps a refused token and a network failure to errors that say what to check
func (c *Client) send(ctx context.Context, method, path string, query url.Values, body io.Reader, header http.Header) (*http.Response, error) {
	target := "https://" + c.primary + path

	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, target, body)

	if err != nil {
		return nil, err
	}

	req.Header = c.header()

	for key, values := range header {
		req.Header[key] = values
	}

	resp, err := c.http.Do(req)

	if err != nil {
		return nil, c.describe(err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		return nil, replicationapi.ErrTokenRefused
	}

	return resp, nil
}

func unexpectedAnswer(resp *http.Response) error {
	var apiErr replicationapi.Error
	json.NewDecoder(io.LimitReader(resp.Body, maxErrorBytes)).Decode(&apiErr)

	if apiErr.Error == "" {
		return fmt.Errorf("the primary answered %s", resp.Status)
	}

	return fmt.Errorf("the primary answered %s: %s", resp.Status, apiErr.Error)
}

// State returns nil and nil when nothing changed during the primary's wait
func (c *Client) State(ctx context.Context, req service.StateRequest, status string) (*replicationapi.StateResponse, *replicationapi.Conflict, error) {
	query := url.Values{}

	if req.HasVersion {
		query.Set(replicationapi.ParamVersion, strconv.FormatInt(req.Version, 10))
		query.Set(replicationapi.ParamTag, req.Tag)
	}

	if req.Cluster != "" {
		query.Set(replicationapi.ParamCluster, req.Cluster)
	}

	if req.Full {
		query.Set(replicationapi.ParamFull, "1")
	}

	resp, err := c.send(ctx, http.MethodGet, replicationapi.StatePath, query, nil, http.Header{replicationapi.FollowerStatusHeader: {status}})

	if err != nil {
		return nil, nil, err
	}

	defer resp.Body.Close()

	body := io.LimitReader(resp.Body, maxStateBytes)

	switch resp.StatusCode {
	case http.StatusNotModified:
		return nil, nil, nil

	case http.StatusConflict:
		var conflict replicationapi.Conflict

		if err := json.NewDecoder(body).Decode(&conflict); err != nil {
			return nil, nil, fmt.Errorf("reading the primary's answer: %w", err)
		}

		return nil, &conflict, nil

	case http.StatusOK:
		var state replicationapi.StateResponse

		if err := json.NewDecoder(body).Decode(&state); err != nil {
			return nil, nil, fmt.Errorf("reading the primary's state: %w", err)
		}

		if state.APIVersion > replicationapi.APIVersion {
			return nil, nil, replicationapi.ErrNewerAPI
		}

		return &state, nil, nil
	}

	return nil, nil, unexpectedAnswer(resp)
}

func (c *Client) Changes(ctx context.Context, repository string, after int64) (*replicationapi.ChangesResponse, error) {
	query := url.Values{replicationapi.ParamRepository: {repository}, replicationapi.ParamAfter: {strconv.FormatInt(after, 10)}}
	resp, err := c.send(ctx, http.MethodGet, replicationapi.ChangesPath, query, nil, nil)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reading the changes of %s: %w", repository, unexpectedAnswer(resp))
	}

	var changes replicationapi.ChangesResponse

	if err := json.NewDecoder(io.LimitReader(resp.Body, maxStateBytes)).Decode(&changes); err != nil {
		return nil, fmt.Errorf("reading the changes of %s: %w", repository, err)
	}

	return &changes, nil
}

func (c *Client) ReportPulls(ctx context.Context, report replicationapi.PullReport) error {
	body, err := json.Marshal(report)

	if err != nil {
		return err
	}

	resp, err := c.send(ctx, http.MethodPost, replicationapi.PullsPath, nil, bytes.NewReader(body), http.Header{"Content-Type": {"application/json"}})

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("reporting pulls: %w", unexpectedAnswer(resp))
	}

	return nil
}

func (c *Client) describe(err error) error {
	if errors.Is(err, replicationapi.ErrIdentityChanged) {
		return replicationapi.ErrIdentityChanged
	}

	var netErr net.Error

	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("the primary at %s didn't answer in time; check follower.primary and that this host can reach its replication port", c.primary)
	}

	var opErr *net.OpError

	if errors.As(err, &opErr) && opErr.Op == "dial" {
		return fmt.Errorf("the primary at %s can't be reached; check follower.primary and that replication is on there", c.primary)
	}

	return err
}
