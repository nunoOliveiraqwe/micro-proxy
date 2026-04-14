package abuseipdb

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
)

const baseURL = "https://api.abuseipdb.com/api/v2"

type Client struct {
	httpClient *http.Client
	apiKey     string
}

func NewClient(apiKey string) *Client {
	zap.S().Debugf("Initializing AbuseIPDB client with base URL %s", baseURL)
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		apiKey:     apiKey,
	}
}

func (c *Client) do(req *http.Request) (io.ReadCloser, error) {
	req.Header.Set("Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("abuseipdb: executing request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("abuseipdb: unexpected status %d", resp.StatusCode)
	}

	return resp.Body, nil
}
