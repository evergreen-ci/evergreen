package thirdparty

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

type RuntimeEnvironmentsClient struct {
	Client  *http.Client
	BaseURL string
	APIKey  string
}

// OSInfo stores operating system information.
type OSInfo struct {
	Version string
	Name    string
}

// NewRuntimeEnvironmentsClient creates a new client with the API base URL and API key fields.
func NewRuntimeEnvironmentsClient(baseURL string, apiKey string) *RuntimeEnvironmentsClient {
	c := RuntimeEnvironmentsClient{
		Client:  &http.Client{},
		BaseURL: baseURL,
		APIKey:  apiKey,
	}
	return &c
}

// GetImageNames returns a list of strings containing the names of all images from the runtime environments API.
func (c *RuntimeEnvironmentsClient) GetImageNames(ctx context.Context) ([]string, error) {
	apiURL := fmt.Sprintf("%s/rest/api/v1/imageList", c.BaseURL)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("Api-Key", c.APIKey)
	resp, err := c.Client.Do(request)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return nil, errors.Errorf("HTTP request returned unexpected status '%s': %s", resp.Status, string(msg))
	}
	var images []string
	if err := gimlet.GetJSON(resp.Body, &images); err != nil {
		return nil, errors.Wrap(err, "decoding http body")
	}
	if len(images) == 0 {
		return nil, errors.New("No corresponding images")
	}
	var filteredImages []string
	for _, img := range images {
		if img != "" {
			filteredImages = append(filteredImages, img)
		}
	}
	return filteredImages, nil
}

// getOSInfo returns a list of operating system information for an AMI.
func (c *RuntimeEnvironmentsClient) getOSInfo(ctx context.Context, amiID string, page, limit int) ([]OSInfo, error) {
	params := url.Values{}
	params.Set("ami", amiID)
	params.Set("page", strconv.Itoa(page))
	params.Set("limit", strconv.Itoa(limit))
	params.Set("type", "OS")
	apiURL := fmt.Sprintf("%s/rest/api/v1/image?%s", c.BaseURL, params.Encode())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("Api-Key", c.APIKey)
	resp, err := c.Client.Do(request)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return nil, errors.Errorf("HTTP request returned unexpected status '%s': %s", resp.Status, string(msg))
	}
	var osInfo []OSInfo
	if err := gimlet.GetJSON(resp.Body, &osInfo); err != nil {
		return nil, errors.Wrap(err, "decoding http body")
	}
	return osInfo, nil
}
