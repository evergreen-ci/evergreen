package cloudnew

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/hostutil"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

func (m *ec2Manager) spot(h *host.Host) bool {
	return h.Distro.Provider == evergreen.ProviderNameEc2Spot
}
func (m *ec2Manager) onDemand(h *host.Host) bool {
	return h.Distro.Provider == evergreen.ProviderNameEc2OnDemand
}

// EC2ProviderSettings describes properties of managed instances.
type EC2ProviderSettings struct {
	// AMI is the AMI ID.
	AMI string `mapstructure:"ami" json:"ami,omitempty" bson:"ami,omitempty"`

	// InstanceType is the EC2 instance type.
	InstanceType string `mapstructure:"instance_type" json:"instance_type,omitempty" bson:"instance_type,omitempty"`

	// KeyName is the AWS SSH key name.
	KeyName string `mapstructure:"key_name" json:"key_name,omitempty" bson:"key_name,omitempty"`

	// MountPoints are the disk mount points for EBS volumes.
	MountPoints []cloud.MountPoint `mapstructure:"mount_points" json:"mount_points,omitempty" bson:"mount_points,omitempty"`
	// SecurityGroup is the security group name in EC2 classic and the security group ID in a VPC.
	SecurityGroup string `mapstructure:"security_group" json:"security_group,omitempty" bson:"security_group,omitempty"`
	// SubnetId is only set in a VPC.
	SubnetId string `mapstructure:"subnet_id" json:"subnet_id,omitempty" bson:"subnet_id,omitempty"`

	// IsVpc is set to true if the security group is part of a VPC.
	IsVpc bool `mapstructure:"is_vpc" json:"is_vpc,omitempty" bson:"is_vpc,omitempty"`

	// BidPrice is the price we are willing to pay for a spot instance.
	BidPrice float64 `mapstructure:"bid_price" json:"bid_price,omitempty" bson:"bid_price,omitempty"`
}

// Validate that essential EC2ProviderSettings fields are not empty.
func (s *EC2ProviderSettings) Validate() error {
	if s.AMI == "" || s.InstanceType == "" || s.SecurityGroup == "" || s.KeyName == "" {
		return errors.New("AMI, instance type, security group, and key name must not be empty")
	}
	if s.BidPrice < 0 {
		return errors.New("Bid price must not be negative")
	}
	if _, err := makeBlockDeviceMappings(s.MountPoints); err != nil {
		return errors.Wrap(err, "block device mappings invalid")
	}
	return nil
}

// EC2ManagerOptions are used to construct a new ec2Manager.
type EC2ManagerOptions struct {
	// client is the client library for communicating with AWS.
	client AWSClient
}

// ec2Manager starts and configures instances in EC2.
type ec2Manager struct {
	*EC2ManagerOptions
	credentials *credentials.Credentials
}

// NewEC2Manager creates a new manager of EC2 spot and on-demand instances.
func NewEC2Manager(opts *EC2ManagerOptions) cloud.CloudManager {
	return &ec2Manager{opts, nil}
}

// GetSettings returns a pointer to the manager's configuration settings struct.
func (m *ec2Manager) GetSettings() cloud.ProviderSettings {
	return &EC2ProviderSettings{}
}

// Configure loads credentials or other settings from the config file.
func (m *ec2Manager) Configure(settings *evergreen.Settings) error {
	if settings.Providers.AWS.Id == "" || settings.Providers.AWS.Secret == "" {
		return errors.New("AWS ID and Secret must not be blank")
	}

	m.credentials = credentials.NewStaticCredentialsFromCreds(credentials.Value{
		AccessKeyID:     settings.Providers.AWS.Id,
		SecretAccessKey: settings.Providers.AWS.Secret,
	})

	return nil
}

func (m *ec2Manager) spawnOnDemandHost(h *host.Host, ec2Settings *EC2ProviderSettings, blockDevices []*ec2.BlockDeviceMapping) ([]*string, error) {
	input := &ec2.RunInstancesInput{
		MinCount:            makeInt64Ptr(1),
		MaxCount:            makeInt64Ptr(1),
		ImageId:             &ec2Settings.AMI,
		KeyName:             &ec2Settings.KeyName,
		InstanceType:        &ec2Settings.InstanceType,
		BlockDeviceMappings: blockDevices,
	}

	if ec2Settings.IsVpc {
		input.SecurityGroupIds = []*string{&ec2Settings.SecurityGroup}
		input.SubnetId = &ec2Settings.SubnetId
	} else {
		input.SecurityGroups = []*string{&ec2Settings.SecurityGroup}
	}

	reservation, err := m.client.RunInstances(input)
	if err != nil || reservation == nil {
		grip.Error(errors.Wrapf(h.Remove(), "error removing intent host %s", h.Id))
		return nil, errors.Wrap(err, "RunInstances API call returned an error")
	}

	if len(reservation.Instances) < 1 {
		return nil, errors.New("reservation has no instances")
	}

	instance := reservation.Instances[0]
	grip.Debug(message.Fields{
		"msg":      "started ec2 instance",
		"id":       *instance.InstanceId,
		"key_name": *instance.KeyName,
	})

	max := 5
	var resp *ec2.DescribeInstancesOutput
	for i := 0; i <= max; i++ {
		resp, err = m.client.DescribeInstances(&ec2.DescribeInstancesInput{
			InstanceIds: []*string{instance.InstanceId},
		})
		if err != nil {
			if i == max {
				err = errors.Wrap(err, "error querying for instance info and retries exhausted")
				grip.Error(message.Fields{
					"error":    err,
					"instance": *instance.InstanceId,
				})
				return nil, err
			}
			time.Sleep(30 * time.Second)
			continue
		}
		break
	}

	grip.Debug(message.Fields{
		"msg":      "describe instances returned data",
		"host_id":  h.Id,
		"response": resp,
	})

	if len(resp.Reservations) < 1 {
		return nil, errors.New("describe instances response has no reservations")
	}

	instance = resp.Reservations[0].Instances[0]
	h.Id = *instance.InstanceId
	resources := []*string{instance.InstanceId}
	for _, vol := range instance.BlockDeviceMappings {
		if *vol.DeviceName == "" {
			continue
		}

		resources = append(resources, vol.Ebs.VolumeId)
	}
	return resources, nil
}

func (m *ec2Manager) spawnSpotHost(h *host.Host, ec2Settings *EC2ProviderSettings, blockDevices []*ec2.BlockDeviceMapping) ([]*string, error) {
	spotRequest := &ec2.RequestSpotInstancesInput{
		SpotPrice:     makeStringPtr(fmt.Sprintf("%v", ec2Settings.BidPrice)),
		InstanceCount: makeInt64Ptr(1),
		LaunchSpecification: &ec2.RequestSpotLaunchSpecification{
			ImageId:             makeStringPtr(ec2Settings.AMI),
			KeyName:             makeStringPtr(ec2Settings.KeyName),
			InstanceType:        makeStringPtr(ec2Settings.InstanceType),
			BlockDeviceMappings: blockDevices,
		},
	}

	if ec2Settings.IsVpc {
		spotRequest.LaunchSpecification.SecurityGroupIds = []*string{&ec2Settings.SecurityGroup}
		spotRequest.LaunchSpecification.SubnetId = &ec2Settings.SubnetId
	} else {
		spotRequest.LaunchSpecification.SecurityGroups = []*string{&ec2Settings.SecurityGroup}
	}

	spotResp, err := m.client.RequestSpotInstances(spotRequest)
	if err != nil {
		grip.Error(errors.Wrapf(h.Remove(), "error removing intent host %s", h.Id))
		return nil, errors.Wrap(err, "RequestSpotInstances API call returned an error")
	}

	spotReqRes := spotResp.SpotInstanceRequests[0]
	if *spotReqRes.State != cloud.SpotStatusOpen && *spotReqRes.State != cloud.SpotStatusActive {
		err = errors.Errorf("Spot request %s was found in state %s on intent host %s",
			*spotReqRes.SpotInstanceRequestId, *spotReqRes.State, h.Id)
		grip.Error(err)
		return nil, err
	}

	h.Id = *spotReqRes.SpotInstanceRequestId
	resources := []*string{spotReqRes.SpotInstanceRequestId}
	return resources, nil
}

// SpawnHost spawns a new host.
func (m *ec2Manager) SpawnHost(h *host.Host) (*host.Host, error) {
	if h.Distro.Provider != evergreen.ProviderNameEc2OnDemand && h.Distro.Provider != evergreen.ProviderNameEc2Spot {
		return nil, errors.Errorf("Can't spawn instance of %s for distro %s: provider is %s",
			evergreen.ProviderNameEc2OnDemand, h.Distro.Id, h.Distro.Provider)
	}

	ec2Settings := &EC2ProviderSettings{}
	if err := mapstructure.Decode(h.Distro.ProviderSettings, ec2Settings); err != nil {
		return nil, errors.Wrapf(err, "Error decoding params for distro %+v: %+v", h.Distro.Id, ec2Settings)
	}

	if err := ec2Settings.Validate(); err != nil {
		return nil, errors.Wrapf(err, "Invalid EC2 settings in distro %s: and %+v", h.Distro.Id, ec2Settings)
	}

	blockDevices, err := makeBlockDeviceMappings(ec2Settings.MountPoints)
	if err != nil {
		return nil, errors.Wrap(err, "error making block device mappings")
	}
	h.InstanceType = ec2Settings.InstanceType

	if err := m.client.Create(m.credentials); err != nil {
		return nil, errors.Wrap(err, "error creating client")
	}

	var resources []*string
	if m.onDemand(h) {
		resources, err = m.spawnOnDemandHost(h, ec2Settings, blockDevices)
		if err != nil {
			return nil, errors.Wrap(err, "error spawning on-demand host")
		}
	} else if m.spot(h) {
		resources, err = m.spawnSpotHost(h, ec2Settings, blockDevices)
		if err != nil {
			return nil, errors.Wrap(err, "error spawning spot host")
		}
	} else {
		return nil, errors.New("can only spawn on-demand or spot")
	}

	tags := makeTags(h)
	tagSlice := []*ec2.Tag{}
	for tag := range tags {
		key := tag
		val := tags[tag]
		tagSlice = append(tagSlice, &ec2.Tag{Key: &key, Value: &val})
	}
	if err = m.client.CreateTags(&ec2.CreateTagsInput{
		Resources: resources,
		Tags:      tagSlice,
	}); err != nil {
		err = errors.Wrapf(err, "failed to attach tags for %s", h.Id)
		grip.Error(err)
		return nil, err
	}

	grip.Debugf("attached tags for '%s'", h.Id)
	event.LogHostStarted(h.Id)

	return nil, nil
}

// CanSpawn indicates if a host can be spawned.
func (m *ec2Manager) CanSpawn() (bool, error) {
	return true, nil
}

// GetInstanceStatus returns the current status of an EC2 instance.
func (m *ec2Manager) GetInstanceStatus(h *host.Host) (cloud.CloudStatus, error) {
	if err := m.client.Create(m.credentials); err != nil {
		return cloud.StatusUnknown, errors.Wrap(err, "error creating client")
	}
	if m.onDemand(h) {
		info, err := m.getInstanceInfo(h.Id)
		if err != nil {
			return cloud.StatusUnknown, err
		}
		return ec2StatusToEvergreenStatus(*info.State.Name), nil
	} else if m.spot(h) {
		return m.getSpotInstanceStatus(h.Id)
	}
	return cloud.StatusUnknown, errors.New("type must be on-demand or spot")
}

// TerminateInstance terminates the EC2 instance.
func (m *ec2Manager) TerminateInstance(h *host.Host) error {
	// terminate the instance
	if h.Status == evergreen.HostTerminated {
		err := errors.Errorf("Can not terminate %s - already marked as "+
			"terminated!", h.Id)
		grip.Error(err)
		return err
	}

	if err := m.client.Create(m.credentials); err != nil {
		return errors.Wrap(err, "error creating client")
	}

	if m.spot(h) {
		canTerminate, err := m.cancelSpotRequest(h.Id)
		if err != nil {
			return errors.Wrap(err, "error canceling spot request")
		}
		// the spot request wasn't fulfilled, so don't attempt to terminate
		if !canTerminate {
			return nil
		}
	}

	resp, err := m.client.TerminateInstances(&ec2.TerminateInstancesInput{
		InstanceIds: []*string{&h.Id},
	})
	if err != nil {
		return err
	}

	for _, stateChange := range resp.TerminatingInstances {
		grip.Info(message.Fields{
			"msg":         "terminated instance",
			"instance_id": stateChange.InstanceId,
		})
	}

	return h.Terminate()
}

func (m *ec2Manager) cancelSpotRequest(id string) (bool, error) {
	spotDetails, err := m.getSpotInstanceStatus(id)
	if err != nil {
		return false, errors.Wrap(err, "failed to get spot instance status")
	}
	grip.Info(message.Fields{
		"msg":     "canceling spot request",
		"spot_id": id,
	})
	if err = m.client.CancelSpotInstanceRequests(&ec2.CancelSpotInstanceRequestsInput{
		SpotInstanceRequestIds: []*string{makeStringPtr(id)},
	}); err != nil {
		grip.Error(message.Fields{
			"msg":     "failed to cancel spot request",
			"spot_id": id,
			"err":     err,
		})
		return false, errors.Wrapf(err, "Failed to cancel spot request for host %s", id)
	}
	if spotDetails == cloud.StatusRunning || spotDetails == cloud.StatusPending {
		return true, nil
	}
	return false, nil
}

// IsUp returns whether a host is up.
func (m *ec2Manager) IsUp(h *host.Host) (bool, error) {
	if err := m.client.Create(m.credentials); err != nil {
		return false, errors.Wrap(err, "error creating client")
	}
	info, err := m.getInstanceInfo(h.Id)
	if err != nil {
		return false, err
	}
	if ec2StatusToEvergreenStatus(*info.State.Name).String() == cloud.EC2StatusRunning {
		return true, nil
	}
	return false, nil
}

// OnUp is called when the host is up.
func (m *ec2Manager) OnUp(*host.Host) error {
	// Unused since we set tags in SpawnHost()
	return nil
}

// IsSSHReachable returns whether or not the host can be reached via SSH.
func (m *ec2Manager) IsSSHReachable(h *host.Host, keyName string) (bool, error) {
	opts, err := m.GetSSHOptions(h, keyName)
	if err != nil {
		return false, err
	}
	return hostutil.CheckSSHResponse(context.TODO(), h, opts)
}

// GetDNSName returns the DNS name for the host.
func (m *ec2Manager) GetDNSName(h *host.Host) (string, error) {
	info, err := m.getInstanceInfo(h.Id)
	if err != nil {
		return "", err
	}
	return *info.PublicDnsName, nil
}

// GetSSHOptions returns the command-line args to pass to SSH.
func (m *ec2Manager) GetSSHOptions(h *host.Host, keyName string) ([]string, error) {
	if keyName == "" {
		return nil, errors.New("No key specified for EC2 host")
	}
	opts := []string{"-i", keyName}
	hasKnownHostsFile := false

	for _, opt := range h.Distro.SSHOptions {
		opt = strings.Trim(opt, " \t")
		opts = append(opts, "-o", opt)
		if strings.HasPrefix(opt, "UserKnownHostsFile") {
			hasKnownHostsFile = true
		}
	}

	if !hasKnownHostsFile {
		opts = append(opts, "-o", "UserKnownHostsFile=/dev/null")
	}
	return opts, nil
}

// TimeTilNextPayment returns how long until the next payment is due for a host.
func (m *ec2Manager) TimeTilNextPayment(host *host.Host) time.Duration {
	return timeTilNextEC2Payment(host)
}

// GetInstanceName returns the name of an instance.
func (m *ec2Manager) GetInstanceName(d *distro.Distro) string {
	return d.GenerateName()
}

// makeInt64Ptr is necessary because Go does not allow you to write `&int64(1)`.
func makeInt64Ptr(i int64) *int64 {
	return &i
}

// makeStringPtr is necessary because Go does not allow you to write `&"foo"`.
func makeStringPtr(s string) *string {
	return &s
}

func (m *ec2Manager) getInstanceInfo(id string) (*ec2.Instance, error) {
	resp, err := m.client.DescribeInstances(&ec2.DescribeInstancesInput{
		InstanceIds: []*string{makeStringPtr(id)},
	})
	if err != nil {
		return nil, errors.Wrap(err, "EC2 API returns error for DescribeInstances")
	}
	reservation := resp.Reservations
	if len(reservation) == 0 {
		err = errors.Errorf("No reservation found for instance id: %s", id)
		grip.Error(err)
		return nil, err
	}

	instances := reservation[0].Instances
	if len(instances) == 0 {
		err = errors.Errorf("'%s' was not found in reservation '%s'",
			id, *resp.Reservations[0].ReservationId)
		grip.Error(err)
		return nil, err
	}

	return instances[0], nil
}

func (m *ec2Manager) getSpotInstanceStatus(id string) (cloud.CloudStatus, error) {
	spotDetails, err := m.client.DescribeSpotInstanceRequests(&ec2.DescribeSpotInstanceRequestsInput{
		SpotInstanceRequestIds: []*string{makeStringPtr(id)},
	})
	if err != nil {
		err = errors.Wrapf(err, "failed to get spot request info for %s", id)
		grip.Error(err)
		return cloud.StatusUnknown, err
	}

	spotInstance := spotDetails.SpotInstanceRequests[0]
	//Spot request has been fulfilled, so get status of the instance itself
	if *spotInstance.InstanceId != "" {
		instanceInfo, err := m.getInstanceInfo(*spotInstance.InstanceId)
		if err != nil {
			err = errors.Wrap(err, "Got an error checking spot details")
			grip.Error(err)
			return cloud.StatusUnknown, err
		}
		return ec2StatusToEvergreenStatus(*instanceInfo.State.Name), nil
	}

	//Spot request is not fulfilled. Either it's failed/closed for some reason,
	//or still pending evaluation
	switch *spotInstance.State {
	case cloud.SpotStatusOpen:
		return cloud.StatusPending, nil
	case cloud.SpotStatusActive:
		return cloud.StatusPending, nil
	case cloud.SpotStatusClosed:
		return cloud.StatusTerminated, nil
	case cloud.SpotStatusCanceled:
		return cloud.StatusTerminated, nil
	case cloud.SpotStatusFailed:
		return cloud.StatusFailed, nil
	default:
		grip.Errorf("Unexpected status code in spot req: %v", spotInstance.State)
		return cloud.StatusUnknown, nil
	}
}

func (m *ec2Manager) CostForDuration(h *host.Host, start, end time.Time) (float64, error) {
	if end.Before(start) || util.IsZeroTime(start) || util.IsZeroTime(end) {
		return 0, errors.New("task timing data is malformed")
	}
	if err := m.client.Create(m.credentials); err != nil {
		return 0, errors.Wrap(err, "error creating client")
	}

	if m.onDemand(h) {
		return m.costForDurationOnDemand(h, start, end)
	} else if m.spot(h) {
		return m.costForDurationSpot(h, start, end)
	}
	return 0, errors.New("type must be on-demand or spot")
}

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
		return 0, err
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
	rates, err := m.describeHourlySpotPriceHistory(
		*i.InstanceType, *i.Placement.AvailabilityZone, os, *i.LaunchTime, end)
	if err != nil {
		return 0, err
	}
	return spotCostForRange(start, end, rates), nil
}

// describeHourlySpotPriceHistory talks to Amazon to get spot price history, then
// simplifies that history into hourly billing rates starting from the supplied
// start time. Returns a slice of hour-separated spot prices or any errors that occur.
func (m *ec2Manager) describeHourlySpotPriceHistory(
	iType string, zone string, os osType, start, end time.Time) ([]spotRate, error) {
	// expand times to contain the full runtime of the host
	startFilter, endFilter := start.Add(-5*time.Hour), end.Add(time.Hour)
	osStr := string(os)
	filter := &ec2.DescribeSpotPriceHistoryInput{
		InstanceTypes:       []*string{&iType},
		ProductDescriptions: []*string{&osStr},
		AvailabilityZone:    &zone,
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
		if i == 0 || start.Equal(*history[i].Timestamp) ||
			start.After(*history[i].Timestamp) && start.Before(*history[i-1].Timestamp) {
			price, err := strconv.ParseFloat(*history[i].SpotPrice, 64)
			if err != nil {
				return nil, errors.Wrap(err, "parsing spot price")
			}
			prices = append(prices, spotRate{Time: start, Price: price})
			// we increment the hour but stay on the same price history index
			// in case the current spot price spans more than one hour
			start = start.Add(time.Hour)
			if start.After(end) {
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
