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
// TODO: Remove nolint:unused when DEVPROD-6983 is resolved.
//
//nolint:unused
func (c *RuntimeEnvironmentsClient) getImageNames(ctx context.Context) ([]string, error) {
	apiURL := fmt.Sprintf("%s/rest/api/v1/imageList", c.BaseURL)
	request, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	request = request.WithContext(ctx)
	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("api-key", c.APIKey)
	result, err := c.Client.Do(request)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if result.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(result.Body)
		return nil, errors.Errorf("HTTP request returned unexpected status `%v`: %v", result.Status, string(msg))
	}
	var imageList []string
	if err := json.NewDecoder(result.Body).Decode(&imageList); err != nil {
		return nil, errors.Wrap(err, "Unable to decode http body")
	}
	var filteredImageList []string // filter out empty values
	for _, img := range imageList {
		if img != "" {
			filteredImageList = append(filteredImageList, img)
		}
	}
	return filteredImageList, nil
}
