package thirdparty

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

type RuntimeEnvironmentsClient struct {
	Client  *http.Client
	BaseURL string
	APIKey  string
}

type Package struct {
	Name    string
	Version string
	Manager string
}

func NewRuntimeEnvironmentsClient(baseURL string, apiKey string) *RuntimeEnvironmentsClient {
	c := RuntimeEnvironmentsClient{
		Client:  &http.Client{},
		BaseURL: baseURL,
		APIKey:  apiKey,
	}
	return &c
}

// getImageNames returns a list of strings containing the names of all images from the runtime environments API.
func (c *RuntimeEnvironmentsClient) getImageNames(ctx context.Context) ([]string, error) {
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

// getPackages returns a list of packages changes (name and version) from the corresponding AMI id.
func (c *RuntimeEnvironmentsClient) getPackages(ctx context.Context, amiId string, page string, limit string) ([]Package, error) {
	apiURL := fmt.Sprintf("%s/rest/api/v1/image", c.BaseURL)
	params := url.Values{}
	params.Set("ami", amiId)
	params.Set("page", page)
	params.Set("limit", limit)
	params.Set("type", "Packages")
	reqURL, err := url.Parse(apiURL)
	if err != nil {
		return nil, errors.Wrap(err, "parsing API url")
	}
	reqURL.RawQuery = params.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
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
	var decodedPackages []Package
	if err := gimlet.GetJSON(resp.Body, &decodedPackages); err != nil {
		return nil, errors.Wrap(err, "decoding http body")
	}
	return decodedPackages, nil
}
