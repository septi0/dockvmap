package webhook

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"
)

const sendTimeout = 15 * time.Second

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{httpClient: &http.Client{}}
}

func (c *Client) Send(ctx context.Context, url string, payload []byte) error {
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)

	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))

	if err != nil {
		return fmt.Errorf("creating webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)

	if err != nil {
		return fmt.Errorf("sending webhook: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("webhook returned %s", resp.Status)
	}

	return nil
}
