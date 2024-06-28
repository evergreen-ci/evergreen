package thirdparty

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/pkg/errors"
)

type RuntimeEnvironmentsClient struct {
	Client  *http.Client
	BaseURL string
	APIKey  string
}

// getImageNames returns a list of strings containing the names of all images from the runtime environments api
func (c *RuntimeEnvironmentsClient) getImageNames(ctx context.Context) ([]string, error) {
	apiURL := fmt.Sprintf("%s/rest/api/v1/imageList", c.BaseURL)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	request = request.WithContext(ctx)
	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("Api-key", c.APIKey)
	result, err := c.Client.Do(request)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if result.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(result.Body)
		return nil, errors.Errorf("HTTP request returned unexpected status `%v`: %v", result.Status, string(msg))
	}
	var images []string
	if err := json.NewDecoder(result.Body).Decode(&images); err != nil {
		return nil, errors.Wrap(err, "Unable to decode http body")
	}
	var filteredImages []string // filter out empty values
	for _, img := range images {
		if img != "" {
			filteredImages = append(filteredImages, img)
		}
	}
	return filteredImages, nil
}
