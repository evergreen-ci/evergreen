package util

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// metadataBaseURL is the URL to make requests for instance-specific metadata on
// EC2 instances.
const metadataBaseURL = "http://169.254.169.254/latest/meta-data"

// GetEC2InstanceID returns the instance ID from the metadata endpoint if it's
// an EC2 instance.
func GetEC2InstanceID(ctx context.Context) (string, error) {
	return getEC2Metadata(ctx, "instance-id", func(resp *http.Response) (string, error) {
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", errors.Wrap(err, "reading response body")
		}
		instanceID := string(b)
		if instanceID == "" {
			return "", errors.New("instance ID from response is empty")
		}
		return instanceID, nil
	})
}

// GetEC2Hostname returns the public host name from the metadata endpoint if
// it's an EC2 instance.
func GetEC2Hostname(ctx context.Context) (string, error) {
	return getEC2Metadata(ctx, "public-hostname", func(resp *http.Response) (string, error) {
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", errors.Wrap(err, "reading response body")
		}

		hostname := string(b)
		if hostname == "" {
			return "", errors.New("hostname from response is empty")
		}
		return hostname, nil
	})
}

// getEC2Metadata gets the EC2 metadata for the subpath.
func getEC2Metadata[Output any](ctx context.Context, metadataSubpath string, parseOutput func(resp *http.Response) (Output, error)) (Output, error) {
	c := utility.GetHTTPClient()
	defer utility.PutHTTPClient(c)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	url := fmt.Sprintf("%s/%s", metadataBaseURL, metadataSubpath)

	var zeroOutput Output
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return zeroOutput, errors.Wrap(err, "creating metadata request")
	}

	const (
		maxAttempts = 20
		minDelay    = time.Second
		maxDelay    = 10 * time.Second
	)
	resp, err := utility.RetryRequest(ctx, req, utility.RetryRequestOptions{
		RetryOptions: utility.RetryOptions{
			MaxAttempts: maxAttempts,
			MinDelay:    minDelay,
			MaxDelay:    maxDelay,
		},
	})
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return zeroOutput, errors.Wrap(err, "requesting EC2 instance ID from metadata endpoint")
	}

	return parseOutput(resp)
}
