package util

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// metadataBaseURL is the URL to make requests for instance-specific metadata on
// EC2 instances.
const metadataBaseURL = "http://169.254.169.254/latest/meta-data"

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
			return strings.TrimSpace(body), nil
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

	catcher := grip.NewBasicCatcher()
	instanceID, err := getEC2InstanceID(ctx)
	catcher.Wrapf(err, "fetching EC2 instance ID")
	metadata.EC2InstanceID = instanceID

	hostname, err := getEC2Hostname(ctx)
	catcher.Wrapf(err, "fetching EC2 host name")
	metadata.Hostname = hostname

	zone, err := getEC2AvailabilityZone(ctx)
	catcher.Wrapf(err, "fetching EC2 availability zone")
	metadata.Zone = zone

	publicIPv4, err := getEC2PublicIPv4(ctx)
	catcher.Wrapf(err, "fetching EC2 public ipv4")
	metadata.PublicIPv4 = publicIPv4

	privateIPv4, err := getEC2PrivateIPv4(ctx)
	catcher.Wrapf(err, "fetching EC2 private ipv4")
	metadata.PrivateIPv4 = privateIPv4

	ipv6, err := getEC2IPv6(ctx)
	catcher.Wrapf(err, "fetching EC2 ipv6")
	metadata.IPv6 = ipv6

	volumes, err := getEC2BlockDeviceMappings(ctx)
	catcher.Wrapf(err, "fetching EC2 volume attachments")
	metadata.Volumes = volumes

	metadata.LaunchTime = time.Now()

	if catcher.HasErrors() {
		return metadata, catcher.Resolve()
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
