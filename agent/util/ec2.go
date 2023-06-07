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
	url := fmt.Sprintf("%s/instance-id", metadataBaseURL)
	c := utility.GetHTTPClient()
	defer utility.PutHTTPClient(c)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", errors.Wrap(err, "creating metadata request")
	}

	resp, err := c.Do(req)
	if err != nil {
		return "", errors.Wrapf(err, "making metadata request")
	}
	defer resp.Body.Close()
	instanceID, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "reading response body")
	}

	return string(instanceID), nil
}
