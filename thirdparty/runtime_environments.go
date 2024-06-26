package thirdparty

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// getImageNames returns a list of strings containing the names of all images from the runtime environments api
func getImageNames(ctx context.Context, base_url string, api_key string) ([]string, error) {
	apiURL := base_url + "/rest/api/v1/imageList"
	request, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	request = request.WithContext(ctx)
	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("api-key", api_key)
	client := utility.GetHTTPClient()
	result, err := client.Do(request)
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
