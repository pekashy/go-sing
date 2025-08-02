package config

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// Fetcher handles configuration fetching operations
type Fetcher struct {
	client *http.Client
}

// NewFetcher creates a new configuration fetcher with HTTPS support
func NewFetcher() *Fetcher {
	return &Fetcher{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// FetchConfig fetches configuration from the given URL (supports HTTP and HTTPS)
func (f *Fetcher) FetchConfig(url string) (string, error) {
	resp, err := f.client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}