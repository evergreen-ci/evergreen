package cloud

import (
	"context"
	"encoding/base64"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type instanceTypeSubnetCache map[instanceRegionPair][]evergreen.Subnet

type instanceRegionPair struct {
	instanceType string
	region       string
}

var typeCache instanceTypeSubnetCache

func init() {
	typeCache = make(map[instanceRegionPair][]evergreen.Subnet)
}

func (c instanceTypeSubnetCache) subnetsWithInstanceType(ctx context.Context, settings *evergreen.Settings, client AWSClient, instanceRegion instanceRegionPair) ([]evergreen.Subnet, error) {
	if subnets, ok := c[instanceRegion]; ok {
		return subnets, nil
	}

	supportingAZs, err := c.getAZs(ctx, settings, client, instanceRegion)
	if err != nil {
		return nil, errors.Wrap(err, "can't get supporting AZs")
	}

	subnets := make([]evergreen.Subnet, 0, len(supportingAZs))
	for _, subnet := range settings.Providers.AWS.Subnets {
		if utility.StringSliceContains(supportingAZs, subnet.AZ) {
			subnets = append(subnets, subnet)
		}
	}
	c[instanceRegion] = subnets

	return subnets, nil
}

func (c instanceTypeSubnetCache) getAZs(ctx context.Context, settings *evergreen.Settings, client AWSClient, instanceRegion instanceRegionPair) ([]string, error) {
	// DescribeInstanceTypeOfferings only returns AZs in the client's region
	output, err := client.DescribeInstanceTypeOfferings(ctx, &ec2.DescribeInstanceTypeOfferingsInput{
		LocationType: aws.String(ec2.LocationTypeAvailabilityZone),
		Filters: []*ec2.Filter{
			{Name: aws.String("instance-type"), Values: []*string{aws.String(instanceRegion.instanceType)}},
		},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "can't get instance types for '%s' in '%s'", instanceRegion.instanceType, instanceRegion.region)
	}
	if output == nil {
		return nil, errors.Errorf("DescribeInstanceTypeOfferings returned nil output for instance type '%s' in '%s'", instanceRegion.instanceType, instanceRegion.region)
	}
	supportingAZs := make([]string, 0, len(output.InstanceTypeOfferings))
	for _, offering := range output.InstanceTypeOfferings {
		if offering != nil && offering.Location != nil {
			supportingAZs = append(supportingAZs, *offering.Location)
		}
	}

	return supportingAZs, nil
}

type EC2FleetManagerOptions struct {
	client         AWSClient
	region         string
	providerKey    string
	providerSecret string
}

type ec2FleetManager struct {
	*EC2FleetManagerOptions
	credentials *credentials.Credentials
	settings    *evergreen.Settings
	env         evergreen.Environment
}

func (m *ec2FleetManager) GetSettings() ProviderSettings {
	return &EC2ProviderSettings{}
}

func (m *ec2FleetManager) Configure(ctx context.Context, settings *evergreen.Settings) error {
	m.settings = settings

	if m.region == "" {
		m.region = evergreen.DefaultEC2Region
	}

	var err error
	m.providerKey, m.providerSecret, err = GetEC2Key(settings)
	if err != nil {
		return errors.Wrap(err, "Problem getting EC2 keys")
	}
	if m.providerKey == "" || m.providerSecret == "" {
		return errors.New("provider key/secret can't be empty")
	}

	m.credentials = credentials.NewStaticCredentialsFromCreds(credentials.Value{
		AccessKeyID:     m.providerKey,
		SecretAccessKey: m.providerSecret,
	})

	return nil
}

func (m *ec2FleetManager) SpawnHost(ctx context.Context, h *host.Host) (*host.Host, error) {
	if h.Distro.Provider != evergreen.ProviderNameEc2Fleet {
		return nil, errors.Errorf("Can't spawn instance for distro %s: provider is %s",
			h.Distro.Id, h.Distro.Provider)
	}

	if err := m.client.Create(m.credentials, m.region); err != nil {
		return nil, errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	ec2Settings := &EC2ProviderSettings{}
	if err := ec2Settings.FromDistroSettings(h.Distro, ""); err != nil {
		return nil, errors.Wrap(err, "error getting EC2 settings")
	}
	if err := ec2Settings.Validate(); err != nil {
		return nil, errors.Wrapf(err, "Invalid EC2 settings in distro %s: %+v", h.Distro.Id, ec2Settings)
	}

	if ec2Settings.KeyName == "" && !h.UserHost {
		if !h.SpawnOptions.SpawnedByTask {
			return nil, errors.New("key name must not be empty")
		}
		k, err := m.client.GetKey(ctx, h)
		if err != nil {
			return nil, errors.Wrap(err, "not spawning host, problem creating key")
		}
		ec2Settings.KeyName = k
	}

	if err := m.spawnFleetSpotHost(ctx, h, ec2Settings); err != nil {
		msg := "error spawning spot host with Fleet"
		grip.Error(message.WrapError(err, message.Fields{
			"message":       msg,
			"host_id":       h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		return nil, errors.Wrap(err, msg)
	}
	grip.Debug(message.Fields{
		"message":       "spawned spot host with Fleet",
		"host_id":       h.Id,
		"host_provider": h.Distro.Provider,
		"distro":        h.Distro.Id,
	})

	return h, nil
}

func (m *ec2FleetManager) ModifyHost(context.Context, *host.Host, host.HostModifyOptions) error {
	return errors.New("can't modify instances for ec2 fleet provider")
}

func (m *ec2FleetManager) GetInstanceStatuses(ctx context.Context, hosts []host.Host) ([]CloudStatus, error) {
	instanceIDs := make([]*string, 0, len(hosts))
	for _, h := range hosts {
		instanceIDs = append(instanceIDs, aws.String(h.Id))
	}

	if err := m.client.Create(m.credentials, m.region); err != nil {
		return nil, errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	startAt := time.Now()
	describeInstancesOutput, err := m.client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: instanceIDs,
	})
	grip.Debug(message.Fields{
		"message":       "finished getting describe instances",
		"num_hosts":     len(hosts),
		"duration_secs": time.Since(startAt).Seconds(),
	})

	if err != nil {
		return nil, errors.Wrap(err, "error describing instances")
	}

	if err = validateEc2DescribeInstancesOutput(describeInstancesOutput); err != nil {
		return nil, errors.Wrap(err, "invalid describe instances response")
	}

	if len(instanceIDs) != len(describeInstancesOutput.Reservations) {
		return nil, errors.Errorf("AWS returned %d statuses for %d hosts", len(describeInstancesOutput.Reservations), len(instanceIDs))
	}

	instanceMap := map[string]*ec2.Instance{}
	for i := range describeInstancesOutput.Reservations {
		instanceMap[*describeInstancesOutput.Reservations[i].Instances[0].InstanceId] = describeInstancesOutput.Reservations[i].Instances[0]
	}

	// Return as an ordered slice of statuses
	runningHosts := 0
	statuses := []CloudStatus{}
	startAt = time.Now()
	for _, h := range hosts {
		status := ec2StatusToEvergreenStatus(*instanceMap[h.Id].State.Name)
		if status == StatusRunning {
			// cache instance information so we can make fewer calls to AWS's API
			grip.Error(message.WrapError(cacheHostData(ctx, &h, instanceMap[h.Id], m.client), message.Fields{
				"message": "can't update host cached data",
				"host_id": h.Id,
			}))
			runningHosts += 1
		}
		statuses = append(statuses, status)
	}
	grip.Debug(message.Fields{
		"message":           "finished caching host data",
		"num_hosts_running": runningHosts,
		"duration_secs":     time.Since(startAt).Seconds(),
	})
	return statuses, nil
}

func (m *ec2FleetManager) GetInstanceStatus(ctx context.Context, h *host.Host) (CloudStatus, error) {
	status := StatusUnknown

	if err := m.client.Create(m.credentials, m.region); err != nil {
		return status, errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	instance, err := m.client.GetInstanceInfo(ctx, h.Id)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":       "error getting instance info",
			"host_id":       h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		return status, errors.Wrap(err, "error getting instance info")
	}

	if instance.State == nil || instance.State.Name == nil || *instance.State.Name == "" {
		return status, errors.New("state name is missing")
	}
	status = ec2StatusToEvergreenStatus(*instance.State.Name)
	if status == StatusRunning {
		// cache instance information so we can make fewer calls to AWS's API
		grip.Error(message.WrapError(cacheHostData(ctx, h, instance, m.client), message.Fields{
			"message": "can't update host cached data",
			"host_id": h.Id,
		}))
	}

	return status, nil
}

func (m *ec2FleetManager) SetPortMappings(context.Context, *host.Host, *host.Host) error {
	return errors.New("can't set port mappings with ec2 fleet provider")
}

func (m *ec2FleetManager) CheckInstanceType(ctx context.Context, instanceType string) error {
	if err := m.client.Create(m.credentials, m.region); err != nil {
		return errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()
	output, err := m.client.DescribeInstanceTypeOfferings(ctx, &ec2.DescribeInstanceTypeOfferingsInput{})
	if err != nil {
		return errors.Wrapf(err, "error describe instance types offered for region '%s", m.region)
	}
	validTypes := []string{}
	for _, availableType := range output.InstanceTypeOfferings {
		if availableType.InstanceType != nil && (*availableType.InstanceType) == instanceType {
			return nil
		}
		validTypes = append(validTypes, *availableType.InstanceType)
	}
	return errors.Errorf("available types for region '%s' are: %v", m.region, validTypes)
}

func (m *ec2FleetManager) TerminateInstance(ctx context.Context, h *host.Host, user, reason string) error {
	if h.Status == evergreen.HostTerminated {
		return errors.Errorf("Can not terminate %s - already marked as terminated!", h.Id)
	}
	if err := m.client.Create(m.credentials, m.region); err != nil {
		return errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	resp, err := m.client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: []*string{aws.String(h.Id)},
	})
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":       "error terminating instance",
			"user":          user,
			"host_id":       h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		return err
	}

	for _, stateChange := range resp.TerminatingInstances {
		if stateChange == nil || stateChange.InstanceId == nil {
			grip.Error(message.Fields{
				"message":       "state change missing instance ID",
				"user":          user,
				"host_provider": h.Distro.Provider,
				"host_id":       h.Id,
				"distro":        h.Distro.Id,
			})
			return errors.New("invalid terminate instances response")
		}
		grip.Info(message.Fields{
			"message":       "terminated instance",
			"user":          user,
			"host_provider": h.Distro.Provider,
			"instance_id":   *stateChange.InstanceId,
			"host_id":       h.Id,
			"distro":        h.Distro.Id,
		})
	}

	return errors.Wrap(h.Terminate(user, reason), "failed to terminate instance in db")
}

// StopInstance should do nothing for EC2 Fleet.
func (m *ec2FleetManager) StopInstance(context.Context, *host.Host, string) error {
	return errors.New("can't stop instances for ec2 fleet provider")
}

// StartInstance should do nothing for EC2 Fleet.
func (m *ec2FleetManager) StartInstance(context.Context, *host.Host, string) error {
	return errors.New("can't start instances for ec2 fleet provider")
}

func (m *ec2FleetManager) IsUp(ctx context.Context, h *host.Host) (bool, error) {
	status, err := m.GetInstanceStatus(ctx, h)
	if err != nil {
		return false, errors.Wrap(err, "error checking if instance is up")
	}
	if status == StatusRunning {
		return true, nil
	}
	return false, nil
}

// OnUp is a noop for Fleet
func (m *ec2FleetManager) OnUp(ctx context.Context, h *host.Host) error {
	return nil
}

func (m *ec2FleetManager) AttachVolume(context.Context, *host.Host, *host.VolumeAttachment) error {
	return errors.New("can't attach volume with ec2 fleet provider")
}

func (m *ec2FleetManager) DetachVolume(context.Context, *host.Host, string) error {
	return errors.New("can't detach volume with ec2 fleet provider")
}

func (m *ec2FleetManager) CreateVolume(context.Context, *host.Volume) (*host.Volume, error) {
	return nil, errors.New("can't create volume with ec2 fleet provider")
}

func (m *ec2FleetManager) DeleteVolume(context.Context, *host.Volume) error {
	return errors.New("can't delete volume with ec2 fleet provider")
}

func (m *ec2FleetManager) ModifyVolume(context.Context, *host.Volume, *model.VolumeModifyOptions) error {
	return errors.New("can't modify volume with ec2 fleet provider")
}

func (m *ec2FleetManager) GetVolumeAttachment(context.Context, string) (*host.VolumeAttachment, error) {
	return nil, errors.New("can't get volume attachment with ec2 fleet provider")
}

func (m *ec2FleetManager) GetDNSName(ctx context.Context, h *host.Host) (string, error) {
	if err := m.client.Create(m.credentials, m.region); err != nil {
		return "", errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	return m.client.GetPublicDNSName(ctx, h)
}

func (m *ec2FleetManager) TimeTilNextPayment(h *host.Host) time.Duration {
	return timeTilNextEC2Payment(h)
}

func (m *ec2FleetManager) spawnFleetSpotHost(ctx context.Context, h *host.Host, ec2Settings *EC2ProviderSettings) error {
	// Cleanup
	var templateID *string
	defer func() {
		if templateID != nil {
			// Delete launch template
			_, err := m.client.DeleteLaunchTemplate(ctx, &ec2.DeleteLaunchTemplateInput{LaunchTemplateId: templateID})
			grip.Error(message.WrapError(err, message.Fields{
				"message":  "can't delete launch template",
				"host_id":  h.Id,
				"template": *templateID,
			}))
		}
	}()

	var err error
	var templateVersion *int64
	templateID, templateVersion, err = m.uploadLaunchTemplate(ctx, h, ec2Settings)
	if err != nil {
		return errors.Wrapf(err, "unable to upload launch template for '%s'", h.Id)
	}

	instanceID, err := m.requestFleet(ctx, ec2Settings, templateID, templateVersion)
	if err != nil {
		return errors.Wrapf(err, "can't request fleet")
	}
	h.Id = *instanceID

	return nil
}

func (m *ec2FleetManager) uploadLaunchTemplate(ctx context.Context, h *host.Host, ec2Settings *EC2ProviderSettings) (*string, *int64, error) {
	blockDevices, err := makeBlockDeviceMappingsTemplate(ec2Settings.MountPoints)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error making block device mappings")
	}

	launchTemplate := &ec2.RequestLaunchTemplateData{
		ImageId:             aws.String(ec2Settings.AMI),
		KeyName:             aws.String(ec2Settings.KeyName),
		InstanceType:        aws.String(ec2Settings.InstanceType),
		BlockDeviceMappings: blockDevices,
		TagSpecifications:   makeTagTemplate(makeTags(h)),
	}

	if ec2Settings.IsVpc {
		launchTemplate.NetworkInterfaces = []*ec2.LaunchTemplateInstanceNetworkInterfaceSpecificationRequest{
			{
				AssociatePublicIpAddress: aws.Bool(true),
				DeviceIndex:              aws.Int64(0),
				Groups:                   ec2Settings.getSecurityGroups(),
				SubnetId:                 aws.String(ec2Settings.SubnetId),
			},
		}
		if ec2Settings.IPv6 {
			launchTemplate.NetworkInterfaces[0].SetIpv6AddressCount(1).SetAssociatePublicIpAddress(false)
		}
	} else {
		launchTemplate.SecurityGroups = ec2Settings.getSecurityGroups()
	}

	settings := *m.settings
	// Use the latest service flags instead of those cached in the environment.
	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		return nil, nil, errors.Wrap(err, "getting service flags")
	}
	settings.ServiceFlags = *flags
	userData, err := makeUserData(ctx, m.settings, h, ec2Settings.UserData, ec2Settings.MergeUserDataParts)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not make user data")
	}
	ec2Settings.UserData = userData

	if ec2Settings.UserData != "" {
		var expanded string
		expanded, err = expandUserData(ec2Settings.UserData, m.settings.Expansions)
		if err != nil {
			return nil, nil, errors.Wrap(err, "problem expanding user data")
		}
		if err = validateUserDataSize(expanded, h.Distro.Id); err != nil {
			return nil, nil, errors.WithStack(err)
		}
		userData := base64.StdEncoding.EncodeToString([]byte(expanded))
		launchTemplate.UserData = &userData
	}

	createTemplateResponse, err := m.client.CreateLaunchTemplate(ctx, &ec2.CreateLaunchTemplateInput{
		LaunchTemplateData: launchTemplate,
		// mandatory field may only contain letters, numbers, and the following characters: - ( ) . / _
		LaunchTemplateName: aws.String(utility.RandomString()),
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "can't upload config template to AWS")
	}
	err = validateEc2CreateTemplateResponse(createTemplateResponse)
	if err != nil {
		return nil, nil, errors.Wrap(err, "invalid create template response")
	}

	return createTemplateResponse.LaunchTemplate.LaunchTemplateId, createTemplateResponse.LaunchTemplate.LatestVersionNumber, nil
}

func (m *ec2FleetManager) requestFleet(ctx context.Context, ec2Settings *EC2ProviderSettings, templateID *string, templateVersion *int64) (*string, error) {
	var overrides []*ec2.FleetLaunchTemplateOverridesRequest
	var err error
	if ec2Settings.VpcName != "" {
		overrides, err = m.makeOverrides(ctx, ec2Settings)
		if err != nil {
			return nil, errors.Wrapf(err, "can't make overrides for VPC '%s'", ec2Settings.VpcName)
		}
	}

	// Create a fleet with a single spot instance from the launch template
	createFleetInput := &ec2.CreateFleetInput{
		LaunchTemplateConfigs: []*ec2.FleetLaunchTemplateConfigRequest{
			{
				LaunchTemplateSpecification: &ec2.FleetLaunchTemplateSpecificationRequest{
					LaunchTemplateId: templateID,
					Version:          aws.String(strconv.Itoa(int(*templateVersion))),
				},
				Overrides: overrides,
			},
		},
		TargetCapacitySpecification: &ec2.TargetCapacitySpecificationRequest{
			TotalTargetCapacity:       aws.Int64(1),
			DefaultTargetCapacityType: aws.String(ec2.DefaultTargetCapacityTypeSpot),
		},
		Type: aws.String(ec2.FleetTypeInstant),
	}
	createFleetResponse, err := m.client.CreateFleet(ctx, createFleetInput)
	if err != nil {
		return nil, errors.Wrap(err, "error creating fleet")
	}
	return createFleetResponse.Instances[0].InstanceIds[0], nil
}

func (m *ec2FleetManager) makeOverrides(ctx context.Context, ec2Settings *EC2ProviderSettings) ([]*ec2.FleetLaunchTemplateOverridesRequest, error) {
	subnets := m.settings.Providers.AWS.Subnets
	if len(subnets) == 0 {
		return nil, errors.New("no AWS subnets were configured")
	}

	supportingSubnets, err := typeCache.subnetsWithInstanceType(ctx, m.settings, m.client, instanceRegionPair{instanceType: ec2Settings.InstanceType, region: ec2Settings.getRegion()})
	if err != nil {
		return nil, errors.Wrapf(err, "can't get AZs supporting instance type '%s'", ec2Settings.InstanceType)
	}
	if len(supportingSubnets) == 0 || (len(supportingSubnets) == 1 && supportingSubnets[0].SubnetID == ec2Settings.SubnetId) {
		return nil, nil
	}
	overrides := make([]*ec2.FleetLaunchTemplateOverridesRequest, 0, len(subnets))
	for _, subnet := range supportingSubnets {
		overrides = append(overrides, &ec2.FleetLaunchTemplateOverridesRequest{SubnetId: aws.String(subnet.SubnetID)})
	}

	return overrides, nil
}

func subnetMatchesAz(subnet *ec2.Subnet) bool {
	for _, tag := range subnet.Tags {
		if tag == nil || tag.Key == nil || tag.Value == nil {
			continue
		}
		if *tag.Key == "Name" && strings.HasSuffix(*tag.Value, strings.Split(*subnet.AvailabilityZone, "-")[2]) {
			return true
		}
	}

	return false
}

func (m *ec2FleetManager) AddSSHKey(ctx context.Context, pair evergreen.SSHKeyPair) error {
	if err := m.client.Create(m.credentials, m.region); err != nil {
		return errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	return errors.Wrap(addSSHKey(ctx, m.client, pair), "could not add SSH key")
}
