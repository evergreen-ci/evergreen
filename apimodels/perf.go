package apimodels

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// GetCedarPerfCountOptions represents the arguments for getting a count
// of perf results for a given task id from Cedar.
type GetCedarPerfCountOptions struct {
	BaseURL   string `json:"-"`
	TaskID    string `json:"-"`
	Execution int    `json:"_"`
}

// CedarPerfCount holds one element, NumberOfResults, matching the json returned by
// Cedar's perf count rest route.
type CedarPerfCount struct {
	NumberOfResults int `json:"number_of_results"`
}

// CedarPerfResultsCount queries SPS for the number of perf results attached to a task.
func CedarPerfResultsCount(ctx context.Context, opts GetCedarPerfCountOptions) (*CedarPerfCount, error) {
	client := utility.GetHTTPClient()
	defer utility.PutHTTPClient(client)

	requestURL := fmt.Sprintf("%s/raw_perf_results/tasks/%s/%d/count", opts.BaseURL, url.PathEscape(opts.TaskID), opts.Execution)
	response, err := client.Get(requestURL)
	if err != nil {
		return nil, errors.Wrap(err, "sending request to get perf results count")
	}
	if response.StatusCode != http.StatusOK {
		body, err := io.ReadAll(response.Body)
		if err == nil {
			return nil, errors.Errorf("unexpected status code: %d: %s", response.StatusCode, body)
		} else {
			return nil, errors.Wrapf(err, "unexpected status code: %d", response.StatusCode)
		}
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading response body when getting perf results count")
	}
	testCount := &CedarPerfCount{}
	if err = json.Unmarshal(body, testCount); err != nil {
		return nil, errors.Wrap(err, "unmarshaling test results from Cedar")
	}

	return testCount, nil
}
