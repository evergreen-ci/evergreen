package cloud

import (
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/pkg/errors"
)

func (m *ec2Manager) costForDurationOnDemand(h *host.Host, start, end time.Time) (float64, error) {
	instance, err := m.getInstanceInfo(h.Id)
	if err != nil {
		return 0, errors.Wrap(err, "error getting instance info")
	}
	os := osLinux
	if strings.Contains(h.Distro.Arch, "windows") {
		os = osWindows
	}

	dur := end.Sub(start)
	region := azToRegion(*instance.Placement.AvailabilityZone)
	iType := instance.InstanceType

	ebsCost, err := m.blockDeviceCosts(instance.BlockDeviceMappings, dur)
	if err != nil {
		return 0, errors.Wrap(err, "calculating block device costs")
	}
	hostCost, err := onDemandCost(&pkgOnDemandPriceFetcher, os, *iType, region, dur)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	return hostCost + ebsCost, nil
}

func (m *ec2Manager) costForDurationSpot(h *host.Host, start, end time.Time) (float64, error) {
	spotDetails, err := m.client.DescribeSpotInstanceRequests(&ec2.DescribeSpotInstanceRequestsInput{
		SpotInstanceRequestIds: []*string{makeStringPtr(h.Id)},
	})
	if err != nil {
		return 0, err
	}
	instance, err := m.getInstanceInfo(*spotDetails.SpotInstanceRequests[0].InstanceId)
	if err != nil {
		return 0, err
	}
	os := osLinux
	if strings.Contains(h.Distro.Arch, "windows") {
		os = osWindows
	}
	ebsCost, err := m.blockDeviceCosts(instance.BlockDeviceMappings, end.Sub(start))
	if err != nil {
		return 0, errors.Wrap(err, "calculating block device costs")
	}
	spotCost, err := m.calculateSpotCost(instance, os, start, end)
	if err != nil {
		return 0, errors.Wrap(err, "error calculating spot cost")
	}
	return spotCost + ebsCost, nil
}

func (m *ec2Manager) blockDeviceCosts(devices []*ec2.InstanceBlockDeviceMapping, dur time.Duration) (float64, error) {
	cost := 0.0
	if len(devices) > 0 {
		volumeIds := []*string{}
		for i := range devices {
			volumeIds = append(volumeIds, devices[i].Ebs.VolumeId)
		}
		vols, err := m.client.DescribeVolumes(&ec2.DescribeVolumesInput{
			VolumeIds: volumeIds,
		})
		if err != nil {
			return 0, err
		}
		for _, v := range vols.Volumes {
			// an amazon region is just the availability zone minus the final letter
			region := azToRegion(*v.AvailabilityZone)
			p, err := ebsCost(&pkgEBSFetcher, region, *v.Size, dur)
			if err != nil {
				return 0, errors.Wrapf(err, "EBS volume %v", v.VolumeId)
			}
			cost += p
		}
	}
	return cost, nil
}

// calculateSpotCost is a helper for fetching spot price history and computing the
// cost of a task across a host's billing cycles.
func (m *ec2Manager) calculateSpotCost(
	i *ec2.Instance, os osType, start, end time.Time) (float64, error) {
	rates, err := m.describeHourlySpotPriceHistory(hourlySpotPriceHistoryInput{
		iType: *i.InstanceType,
		zone:  *i.Placement.AvailabilityZone,
		os:    os,
		start: *i.LaunchTime,
		end:   end,
	})
	if err != nil {
		return 0, err
	}
	return spotCostForRange(start, end, rates), nil
}

type hourlySpotPriceHistoryInput struct {
	iType string
	zone  string
	os    osType
	start time.Time
	end   time.Time
}

// describeHourlySpotPriceHistory talks to Amazon to get spot price history, then
// simplifies that history into hourly billing rates starting from the supplied
// start time. Returns a slice of hour-separated spot prices or any errors that occur.
func (m *ec2Manager) describeHourlySpotPriceHistory(input hourlySpotPriceHistoryInput) ([]spotRate, error) {
	// expand times to contain the full runtime of the host
	startFilter, endFilter := input.start.Add(-time.Hour), input.end.Add(time.Hour)
	osStr := string(input.os)
	filter := &ec2.DescribeSpotPriceHistoryInput{
		InstanceTypes:       []*string{&input.iType},
		ProductDescriptions: []*string{&osStr},
		AvailabilityZone:    &input.zone,
		StartTime:           &startFilter,
		EndTime:             &endFilter,
	}
	// iterate through all pages of results (the helper that does this for us appears to be broken)
	history := []*ec2.SpotPrice{}
	for {
		h, err := m.client.DescribeSpotPriceHistory(filter)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		history = append(history, h.SpotPriceHistory...)
		if *h.NextToken != "" {
			filter.NextToken = h.NextToken
		} else {
			break
		}
	}
	// this loop samples the spot price history (which includes updates for every few minutes)
	// into hourly billing periods. The price we are billed for an hour of spot time is the
	// current price at the start of the hour. Amazon returns spot price history sorted in
	// decreasing time order. We iterate backwards through the list to
	// pretend the ordering to increasing time.
	prices := []spotRate{}
	i := len(history) - 1
	for i >= 0 {
		// add the current hourly price if we're in the last result bucket
		// OR our billing hour starts the same time as the data (very rare)
		// OR our billing hour starts after the current bucket but before the next one
		if i == 0 || input.start.Equal(*history[i].Timestamp) ||
			input.start.After(*history[i].Timestamp) && input.start.Before(*history[i-1].Timestamp) {
			price, err := strconv.ParseFloat(*history[i].SpotPrice, 64)
			if err != nil {
				return nil, errors.Wrap(err, "parsing spot price")
			}
			prices = append(prices, spotRate{Time: input.start, Price: price})
			// we increment the hour but stay on the same price history index
			// in case the current spot price spans more than one hour
			input.start = input.start.Add(time.Hour)
			if input.start.After(input.end) {
				break
			}
		} else {
			// continue iterating through our price history whenever we
			// aren't matching the next billing hour
			i--
		}
	}
	return prices, nil
}
