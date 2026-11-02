package agent

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/septi0/dockvmap/internal/agentapi"
)

const controllerRequestTimeout = 30 * time.Second

type controllerClient struct {
	http      *http.Client
	base      *url.URL
	token     string
	userAgent string
}

type controllerError struct {
	status  int
	message string
}

func (e *controllerError) Error() string {
	if e.status == http.StatusUnauthorized {
		return "dockvmap rejected the agent token; regenerate it on this host's page in dockvmap and update DOCKVMAP_AGENT_TOKEN"
	}

	return fmt.Sprintf("dockvmap answered %d: %s", e.status, e.message)
}

func newControllerClient(cfg Config, version string) *controllerClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: cfg.RootCAs, MinVersion: tls.VersionTLS12}

	return &controllerClient{
		http:      &http.Client{Transport: transport, Timeout: controllerRequestTimeout},
		base:      cfg.URL,
		token:     cfg.Token,
		userAgent: "dockvmap-agent/" + version,
	}
}

func (c *controllerClient) config(ctx context.Context) (agentapi.Config, error) {
	var config agentapi.Config

	return config, c.do(ctx, http.MethodGet, agentapi.ConfigPath, nil, &config)
}

func (c *controllerClient) report(ctx context.Context, report agentapi.Report) error {
	body, err := json.Marshal(report)

	if err != nil {
		return fmt.Errorf("encoding report: %w", err)
	}

	return c.do(ctx, http.MethodPost, agentapi.ReportPath, body, nil)
}

func (c *controllerClient) heartbeat(ctx context.Context, heartbeat agentapi.Heartbeat) error {
	body, err := json.Marshal(heartbeat)

	if err != nil {
		return fmt.Errorf("encoding heartbeat: %w", err)
	}

	return c.do(ctx, http.MethodPost, agentapi.HeartbeatPath, body, nil)
}

func (c *controllerClient) do(ctx context.Context, method, path string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.base.JoinPath(path).String(), bytes.NewReader(body))

	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", c.userAgent)

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)

	if err != nil {
		return c.describe(err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var apiErr struct{ Error string }
		_ = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&apiErr)

		return &controllerError{status: resp.StatusCode, message: apiErr.Error}
	}

	if out == nil {
		return nil
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *controllerClient) describe(err error) error {
	var unknownAuthority x509.UnknownAuthorityError
	var invalidCert x509.CertificateInvalidError
	var hostnameErr x509.HostnameError

	switch {
	case errors.As(err, &unknownAuthority), errors.As(err, &invalidCert):
		return fmt.Errorf("the certificate of %s is not trusted; set DOCKVMAP_CA_FILE to the CA certificate that signed it: %w", c.base.Host, err)

	case errors.As(err, &hostnameErr):
		return fmt.Errorf("the certificate of %s does not match its address; use the name the certificate was issued for in DOCKVMAP_URL: %w", c.base.Host, err)
	}

	var urlErr *url.Error

	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}

	return fmt.Errorf("cannot reach dockvmap at %s: %w", c.base.Redacted(), err)
}
