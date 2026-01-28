package thirdparty

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// SageClient is a client for communicating with the Sage API service.
type SageClient struct {
	httpClient *http.Client
	baseURL    string
}

// NewSageClient creates a new Sage API client.
func NewSageClient(baseURL string) (*SageClient, error) {
	if baseURL == "" {
		return nil, errors.New("Sage base URL is not configured")
	}
	return &SageClient{
		httpClient: utility.GetHTTPClient(),
		baseURL:    baseURL,
	}, nil
}

// Close releases resources used by the client.
func (c *SageClient) Close() {
	utility.PutHTTPClient(c.httpClient)
}

// SetCursorAPIKeyResponse is the response from the Sage API when setting a Cursor API key.
type SetCursorAPIKeyResponse struct {
	Success     bool   `json:"success"`
	KeyLastFour string `json:"keyLastFour,omitempty"`
}

// DeleteCursorAPIKeyResponse is the response from the Sage API when deleting a Cursor API key.
type DeleteCursorAPIKeyResponse struct {
	Success bool `json:"success"`
}

// CursorAPIKeyStatusResponse is the response from the Sage API for the key status.
type CursorAPIKeyStatusResponse struct {
	HasKey      bool   `json:"hasKey"`
	KeyLastFour string `json:"keyLastFour,omitempty"`
}

// SetCursorAPIKey submits a user's Cursor API key to Sage.
func (c *SageClient) SetCursorAPIKey(ctx context.Context, userID, apiKey string) (*SetCursorAPIKeyResponse, error) {
	requestBody := map[string]string{
		"apiKey": apiKey,
	}
	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, errors.Wrap(err, "marshalling request body")
	}

	url := fmt.Sprintf("%s/pr-bot/user/cursor-key", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, errors.Wrap(err, "creating request")
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Evergreen-User-ID", userID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "executing request to Sage")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading response body")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("Sage API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result SetCursorAPIKeyResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, errors.Wrap(err, "unmarshalling response")
	}

	return &result, nil
}

// DeleteCursorAPIKey removes a user's Cursor API key from Sage.
func (c *SageClient) DeleteCursorAPIKey(ctx context.Context, userID string) (*DeleteCursorAPIKeyResponse, error) {
	url := fmt.Sprintf("%s/pr-bot/user/cursor-key", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "creating request")
	}

	req.Header.Set("X-Evergreen-User-ID", userID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "executing request to Sage")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, errors.Errorf("Sage API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result DeleteCursorAPIKeyResponse
	if err := utility.ReadJSON(resp.Body, &result); err != nil {
		return nil, errors.Wrap(err, "reading JSON response")
	}

	return &result, nil
}

// GetCursorAPIKeyStatus retrieves the status of a user's Cursor API key from Sage.
func (c *SageClient) GetCursorAPIKeyStatus(ctx context.Context, userID string) (*CursorAPIKeyStatusResponse, error) {
	url := fmt.Sprintf("%s/pr-bot/user/cursor-key", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "creating request")
	}

	req.Header.Set("X-Evergreen-User-ID", userID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "executing request to Sage")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, errors.Errorf("Sage API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result CursorAPIKeyStatusResponse
	if err := utility.ReadJSON(resp.Body, &result); err != nil {
		return nil, errors.Wrap(err, "reading JSON response")
	}

	return &result, nil
}
