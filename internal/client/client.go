package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/johann/ib/internal/backup"
	"github.com/johann/ib/internal/config"
)

// Client is an HTTP client for the backup server
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// New creates a new client from config
func New(cfg *config.ClientConfig) (*Client, error) {
	if cfg.ServerURL == "" {
		return nil, fmt.Errorf("server URL not configured. Run 'ib login <server-url>'")
	}

	return &Client{
		baseURL: cfg.ServerURL,
		token:   cfg.Token,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}, nil
}

// BlockExists checks if a block exists on the server
func (c *Client) BlockExists(ctx context.Context, cid string) (bool, error) {
	req, err := c.newRequest(ctx, "POST", fmt.Sprintf("/api/blocks/%s/exists", cid), nil)
	if err != nil {
		return false, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}

	return false, fmt.Errorf("unexpected status: %d", resp.StatusCode)
}

// UploadBlock uploads a block to the server
func (c *Client) UploadBlock(ctx context.Context, cid string, data []byte, originalSize int64) error {
	req, err := c.newRequest(ctx, "POST", "/api/blocks", bytes.NewReader(data))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Block-CID", cid)
	req.Header.Set("X-Original-Size", fmt.Sprintf("%d", originalSize))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed: %d - %s", resp.StatusCode, string(body))
	}

	return nil
}

// DownloadBlock downloads a block from the server
func (c *Client) DownloadBlock(ctx context.Context, cid string) ([]byte, error) {
	req, err := c.newRequest(ctx, "GET", fmt.Sprintf("/api/blocks/%s", cid), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// GetLatestManifest retrieves the latest manifest matching the given tags
func (c *Client) GetLatestManifest(ctx context.Context, tags map[string]string) (*backup.Manifest, error) {
	u, err := url.Parse(c.baseURL + "/api/manifests/latest")
	if err != nil {
		return nil, err
	}

	q := u.Query()
	for k, v := range tags {
		q.Set("tag."+k, v)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get manifest: %d", resp.StatusCode)
	}

	var manifest backup.Manifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}

// GetManifest retrieves a manifest by ID
func (c *Client) GetManifest(ctx context.Context, id string) (*backup.Manifest, error) {
	req, err := c.newRequest(ctx, "GET", fmt.Sprintf("/api/manifests/%s", id), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get manifest: %d", resp.StatusCode)
	}

	var manifest backup.Manifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}

// UploadManifest uploads a manifest to the server
func (c *Client) UploadManifest(ctx context.Context, manifest *backup.Manifest) error {
	data, err := json.Marshal(manifest)
	if err != nil {
		return err
	}

	req, err := c.newRequest(ctx, "POST", "/api/manifests", bytes.NewReader(data))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("manifest upload failed: %d - %s", resp.StatusCode, string(body))
	}

	return nil
}

// ListManifests lists available manifests
func (c *Client) ListManifests(ctx context.Context, tags map[string]string) ([]ManifestInfo, error) {
	u, err := url.Parse(c.baseURL + "/api/manifests")
	if err != nil {
		return nil, err
	}

	q := u.Query()
	for k, v := range tags {
		q.Set("tag."+k, v)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list manifests: %d", resp.StatusCode)
	}

	var manifests []ManifestInfo
	if err := json.NewDecoder(resp.Body).Decode(&manifests); err != nil {
		return nil, err
	}

	return manifests, nil
}

func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	return req, nil
}

// ManifestInfo contains basic manifest information
type ManifestInfo struct {
	ID        string            `json:"id"`
	Tags      map[string]string `json:"tags"`
	CreatedAt time.Time         `json:"created_at"`
}
