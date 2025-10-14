package util

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// metadataBaseURL is the URL to make requests for instance-specific metadata on
// EC2 instances.
const metadataBaseURL = "http://169.254.169.254/latest/meta-data"

// getEC2InstanceID returns the instance ID from the metadata endpoint if it's
// an EC2 instance.
func getEC2InstanceID(ctx context.Context) (string, error) {
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

// getEC2Hostname returns the public host name from the metadata endpoint if
// it's an EC2 instance.
func getEC2Hostname(ctx context.Context) (string, error) {
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

// getEC2AvailabilityZone returns the availability zone from the metadata endpoint.
func getEC2AvailabilityZone(ctx context.Context) (string, error) {
	return getEC2Metadata(ctx, "placement/availability-zone", func(resp *http.Response) (string, error) {
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", errors.Wrap(err, "reading response body")
		}
		return string(b), nil
	})
}

// getEC2PublicIPv4 returns the public IPv4 address from the metadata endpoint.
func getEC2PublicIPv4(ctx context.Context) (string, error) {
	return getEC2Metadata(ctx, "public-ipv4", func(resp *http.Response) (string, error) {
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", errors.Wrap(err, "reading response body")
		}
		return string(b), nil
	})
}

// getEC2PrivateIPv4 returns the private IPv4 address from the metadata endpoint.
func getEC2PrivateIPv4(ctx context.Context) (string, error) {
	return getEC2Metadata(ctx, "local-ipv4", func(resp *http.Response) (string, error) {
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", errors.Wrap(err, "reading response body")
		}
		return string(b), nil
	})
}

// getEC2IPv6 returns the IPv6 address from the metadata endpoint if available.
func getEC2IPv6(ctx context.Context) (string, error) {
	ipv6, err := getEC2Metadata(ctx, "ipv6", func(resp *http.Response) (string, error) {
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", errors.Wrap(err, "reading response body")
		}
		return string(b), nil
	})
	if err != nil {
		return "", nil
	}
	return ipv6, nil
}

// getEC2LaunchTime returns the instance launch time from the metadata endpoint.
func getEC2LaunchTime(ctx context.Context) (time.Time, error) {
	return getEC2Metadata(ctx, "instance-launch-time", func(resp *http.Response) (time.Time, error) {
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return time.Time{}, errors.Wrap(err, "reading response body")
		}
		t, err := time.Parse(time.RFC3339, string(b))
		if err != nil {
			return time.Time{}, errors.Wrap(err, "parsing launch time")
		}
		return t, nil
	})
}

// GetEC2Metadata fetches necessary EC2 metadata needed for the needed for
// the /hosts/{host_id}/is_up endpoint.
func GetEC2Metadata(ctx context.Context) (*model.APIHostIsUpOptions, error) {
	metadata := &model.APIHostIsUpOptions{}

	instanceID, err := getEC2InstanceID(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "fetching EC2 instance ID")
	}
	metadata.EC2InstanceID = instanceID

	hostname, err := getEC2Hostname(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "fetching EC2 hostname")
	}
	metadata.Hostname = hostname

	if zone, err := getEC2AvailabilityZone(ctx); err == nil {
		metadata.Zone = zone
	}

	if launchTime, err := getEC2LaunchTime(ctx); err == nil {
		metadata.LaunchTime = launchTime
	}

	if publicIPv4, err := getEC2PublicIPv4(ctx); err == nil {
		metadata.PublicIPv4 = publicIPv4
	}

	if privateIPv4, err := getEC2PrivateIPv4(ctx); err == nil {
		metadata.PrivateIPv4 = privateIPv4
	}

	if ipv6, err := getEC2IPv6(ctx); err == nil {
		metadata.IPv6 = ipv6
	}

	return metadata, nil
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
