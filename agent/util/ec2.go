package util

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// metadataBaseURL is the URL to make requests for instance-specific metadata on
// EC2 instances.
const metadataBaseURL = "http://169.254.169.254/latest/meta-data"

// dynamicDataBaseURL is the URL to make requests for dynamic instance data on
// EC2 instances.
const dynamicDataBaseURL = "http://169.254.169.254/latest/dynamic"

// instanceIdentityDocument is intended to extract the pendingTime field from the EC2 dynamic
// data response. AWS docs: https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/instance-identity-documents.html
type instanceIdentityDocument struct {
	PendingTime string `json:"pendingTime"`
}

func readBodyAsString(resp *http.Response) (string, error) {
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "reading response body")
	}
	return string(b), nil
}

// getEC2InstanceID returns the instance ID from the metadata endpoint if it's
// an EC2 instance.
func getEC2InstanceID(ctx context.Context) (string, error) {
	return getEC2Metadata(ctx, "instance-id", func(resp *http.Response) (string, error) {
		instanceID, err := readBodyAsString(resp)
		if err != nil {
			return "", err
		}
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
		hostname, err := readBodyAsString(resp)
		if err != nil {
			return "", err
		}
		if hostname == "" {
			return "", errors.New("hostname from response is empty")
		}
		return hostname, nil
	})
}

// getEC2AvailabilityZone returns the availability zone from the metadata endpoint.
func getEC2AvailabilityZone(ctx context.Context) (string, error) {
	return getEC2Metadata(ctx, "placement/availability-zone", readBodyAsString)
}

// getEC2PublicIPv4 returns the public IPv4 address from the metadata endpoint.
func getEC2PublicIPv4(ctx context.Context) (string, error) {
	return getEC2Metadata(ctx, "public-ipv4", readBodyAsString)
}

// getEC2PrivateIPv4 returns the private IPv4 address from the metadata endpoint.
func getEC2PrivateIPv4(ctx context.Context) (string, error) {
	return getEC2Metadata(ctx, "local-ipv4", readBodyAsString)
}

// getEC2IPv6 returns the IPv6 address from the metadata endpoint if available.
func getEC2IPv6(ctx context.Context) (string, error) {
	ipv6, err := getEC2Metadata(ctx, "ipv6", readBodyAsString)
	if err != nil {
		return "", nil
	}
	return ipv6, nil
}

// getEC2InstanceStartTime returns the instance start time from an EC2 dynamic data response
func getEC2InstanceStartTime(ctx context.Context) (time.Time, error) {
	return getEC2DynamicData(ctx, "instance-identity/document", func(resp *http.Response) (time.Time, error) {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return time.Time{}, errors.Wrap(err, "reading instance identity response body")
		}

		var doc instanceIdentityDocument
		if err := json.Unmarshal(body, &doc); err != nil {
			return time.Time{}, errors.Wrap(err, "unmarshaling instance identity")
		}

		if doc.PendingTime == "" {
			return time.Time{}, errors.New("pendingTime field is empty")
		}

		startTime, err := time.Parse(time.RFC3339, doc.PendingTime)
		if err != nil {
			return time.Time{}, errors.Wrapf(err, "parsing pendingTime: %s", doc.PendingTime)
		}

		return startTime, nil
	})
}

// getEC2BlockDeviceMappings returns all block device mappings from the metadata endpoint.
func getEC2BlockDeviceMappings(ctx context.Context) ([]host.VolumeAttachment, error) {
	deviceList, err := getEC2Metadata(ctx, "block-device-mapping/", readBodyAsString)
	if err != nil {
		return nil, nil
	}

	deviceNames := strings.Fields(deviceList)
	if len(deviceNames) == 0 {
		return nil, nil
	}

	var volumes []host.VolumeAttachment
	for _, deviceName := range deviceNames {
		if strings.HasPrefix(deviceName, "ephemeral") {
			continue
		}
		volumeID, err := getEC2Metadata(ctx, fmt.Sprintf("block-device-mapping/%s", deviceName), func(resp *http.Response) (string, error) {
			body, err := readBodyAsString(resp)
			if err != nil {
				return "", err
			}
			return body, nil
		})
		if err != nil {
			continue
		}
		volumes = append(volumes, host.VolumeAttachment{
			VolumeID:   volumeID,
			DeviceName: deviceName,
		})
	}

	return volumes, nil
}

// GetEC2Metadata fetches necessary EC2 metadata needed for the needed for
// the /hosts/{host_id}/is_up endpoint.
func GetEC2Metadata(ctx context.Context) (host.HostMetadataOptions, error) {
	metadata := host.HostMetadataOptions{}

	instanceID, err := getEC2InstanceID(ctx)
	if err != nil {
		return metadata, errors.Wrapf(err, "fetching EC2 instance ID")
	}
	metadata.EC2InstanceID = instanceID

	hostname, err := getEC2Hostname(ctx)
	if err != nil {
		return metadata, errors.Wrapf(err, "fetching EC2 host name")
	}
	metadata.PublicDNS = hostname

	zone, err := getEC2AvailabilityZone(ctx)
	if err != nil {
		return metadata, errors.Wrapf(err, "fetching EC2 availability zone")
	}
	metadata.Zone = zone

	publicIPv4, err := getEC2PublicIPv4(ctx)
	if err != nil {
		return metadata, errors.Wrapf(err, "fetching EC2 public ipv4")
	}
	metadata.PublicIPv4 = publicIPv4

	privateIPv4, err := getEC2PrivateIPv4(ctx)
	if err != nil {
		return metadata, errors.Wrapf(err, "fetching EC2 private ipv4")
	}
	metadata.PrivateIPv4 = privateIPv4

	ipv6, err := getEC2IPv6(ctx)
	if err != nil {
		return metadata, errors.Wrapf(err, "fetching EC2 ipv6")
	}
	metadata.IPv6 = ipv6

	volumes, err := getEC2BlockDeviceMappings(ctx)
	if err != nil {
		return metadata, errors.Wrapf(err, "fetching EC2 volume attachments")
	}
	metadata.Volumes = volumes

	startTime, err := getEC2InstanceStartTime(ctx)
	if err != nil {
		return metadata, errors.Wrapf(err, "fetching EC2 instance start time")
	}
	metadata.StartedAt = startTime

	return metadata, nil
}

// getEC2Metadata gets the EC2 metadata for the subpath.
func getEC2Metadata[Output any](ctx context.Context, metadataSubpath string, parseOutput func(resp *http.Response) (Output, error)) (Output, error) {
	return getEC2Data(ctx, metadataBaseURL, metadataSubpath, parseOutput)
}

// getEC2DynamicData gets the EC2 dynamic data for the subpath.
func getEC2DynamicData[Output any](ctx context.Context, dynamicSubpath string, parseOutput func(resp *http.Response) (Output, error)) (Output, error) {
	return getEC2Data(ctx, dynamicDataBaseURL, dynamicSubpath, parseOutput)
}

func getEC2Data[Output any](ctx context.Context, baseURL, subpath string, parseOutput func(resp *http.Response) (Output, error)) (Output, error) {
	c := utility.GetHTTPClient()
	defer utility.PutHTTPClient(c)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	url := fmt.Sprintf("%s/%s", baseURL, subpath)

	var zeroOutput Output
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return zeroOutput, errors.Wrap(err, "creating EC2 data request")
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
		return zeroOutput, errors.Wrapf(err, "requesting EC2 data from %s", baseURL)
	}

	return parseOutput(resp)
}
