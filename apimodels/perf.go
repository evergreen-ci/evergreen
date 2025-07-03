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

// GetPerfCountOptions represents the arguments for getting a count
// of perf results for a given task id.
type GetPerfCountOptions struct {
	SPSBaseURL string `json:"-"`
	TaskID     string `json:"-"`
	Execution  int    `json:"-"`
}

// PerfCount holds one element, NumberOfResults, matching the json returned by the perf count rest route.
type PerfCount struct {
	NumberOfResults int `json:"number_of_results"`
}

// PerfResultsCount queries SPS for the number of perf results attached to a task.
func PerfResultsCount(ctx context.Context, opts GetPerfCountOptions) (*PerfCount, error) {
	client := utility.GetHTTPClient()
	defer utility.PutHTTPClient(client)

	requestURL := fmt.Sprintf("%s/raw_perf_results/tasks/%s/%d/count", opts.SPSBaseURL, url.PathEscape(opts.TaskID), opts.Execution)
	response, err := client.Get(requestURL)
	if err != nil {
		return nil, errors.Wrap(err, "sending request to get perf results count")
	}
	defer response.Body.Close()

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
	testCount := &PerfCount{}
	if err = json.Unmarshal(body, testCount); err != nil {
		return nil, errors.Wrap(err, "unmarshaling perf results")
	}

	return testCount, nil
}
