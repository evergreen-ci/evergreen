package apimodels

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/evergreen-ci/timber"
	"github.com/evergreen-ci/timber/perf"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// GetPerfCountOptions represents the arguments for getting a count
// of perf results for a given task id from Cedar.
type GetPerfCountOptions struct {
	SPSBaseURL   string `json:"-"`
	CedarBaseURL string `json:"-"`
	TaskID       string `json:"-"`
	Execution    int    `json:"-"`
}

// PerfCount holds one element, NumberOfResults, matching the json returned by
// Cedar's perf count rest route.
type PerfCount struct {
	NumberOfResults int `json:"number_of_results"`
}

// PerfResultsCount queries SPS for the number of perf results attached to a task.
func PerfResultsCount(ctx context.Context, opts GetPerfCountOptions) (*PerfCount, error) {
	// If the SPS base URL is set, use it to get the perf results count.
	if opts.SPSBaseURL != "" {
		client := utility.GetHTTPClient()
		defer utility.PutHTTPClient(client)

		requestURL := fmt.Sprintf("%s/raw_perf_results/tasks/%s/%d/count", opts.SPSBaseURL, url.PathEscape(opts.TaskID), opts.Execution)
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
		testCount := &PerfCount{}
		if err = json.Unmarshal(body, testCount); err != nil {
			return nil, errors.Wrap(err, "unmarshaling perf results")
		}

		return testCount, nil
	}
	// Otherwise, use Cedar to get the perf results count.
	data, err := perf.Get(ctx, opts.convert())
	if err != nil {
		return nil, errors.Wrap(err, "getting test results from Cedar")
	}

	testCount := &PerfCount{}
	if err = json.Unmarshal(data, testCount); err != nil {
		return nil, errors.Wrap(err, "unmarshaling test results from Cedar")
	}

	return testCount, nil
}

// LEGACY CEDAR CODE BELOW, ONLY AROUND TO HELP WITH MIGRATION
func (opts GetPerfCountOptions) convert() perf.GetOptions {
	return perf.GetOptions{
		Cedar: timber.GetOptions{
			BaseURL: fmt.Sprintf("https://%s", opts.CedarBaseURL),
		},
		TaskID:    opts.TaskID,
		Execution: utility.ToIntPtr(opts.Execution),
		Count:     true,
	}
}
