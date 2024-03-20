package cloud

import (
	"context"
	"encoding/base64"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const launchTemplateExpiration = 24 * time.Hour

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
		return nil, errors.Wrap(err, "getting supported AZs")
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
		LocationType: types.LocationTypeAvailabilityZone,
		Filters: []types.Filter{
			{Name: aws.String("instance-type"), Values: []string{instanceRegion.instanceType}},
		},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "getting instance types for filter '%s' in region '%s'", instanceRegion.instanceType, instanceRegion.region)
	}
	if output == nil {
		return nil, errors.Errorf("DescribeInstanceTypeOfferings returned nil output for instance type filter '%s' in '%s'", instanceRegion.instanceType, instanceRegion.region)
	}
	supportingAZs := make([]string, 0, len(output.InstanceTypeOfferings))
	for _, offering := range output.InstanceTypeOfferings {
		if offering.Location != nil {
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
	settings *evergreen.Settings
	env      evergreen.Environment
}

func (m *ec2FleetManager) Configure(ctx context.Context, settings *evergreen.Settings) error {
	m.settings = settings

	if m.region == "" {
		m.region = evergreen.DefaultEC2Region
	}

	return nil
}

func (m *ec2FleetManager) SpawnHost(ctx context.Context, h *host.Host) (*host.Host, error) {
	if h.Distro.Provider != evergreen.ProviderNameEc2Fleet {
		return nil, errors.Errorf("can't spawn instance for distro '%s': distro provider is '%s'", h.Distro.Id, h.Distro.Provider)
	}

	if err := m.client.Create(ctx, m.region); err != nil {
		return nil, errors.Wrap(err, "creating client")
	}
	defer m.client.Close()

	ec2Settings := &EC2ProviderSettings{}
	if err := ec2Settings.FromDistroSettings(h.Distro, ""); err != nil {
		return nil, errors.Wrap(err, "getting EC2 settings")
	}
	if err := ec2Settings.Validate(); err != nil {
		return nil, errors.Wrapf(err, "invalid EC2 settings in distro '%s': %+v", h.Distro.Id, ec2Settings)
	}

	if ec2Settings.KeyName == "" && !h.UserHost {
		if !h.SpawnOptions.SpawnedByTask {
			return nil, errors.New("key name must not be empty")
		}
		k, err := m.client.GetKey(ctx, h)
		if err != nil {
			return nil, errors.Wrap(err, "getting public key")
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
	return errors.New("can't modify instances for EC2 fleet provider")
}

func (m *ec2FleetManager) GetInstanceStatuses(ctx context.Context, hosts []host.Host) (map[string]CloudStatus, error) {
	instanceIDs := make([]string, 0, len(hosts))
	for _, h := range hosts {
		instanceIDs = append(instanceIDs, h.Id)
	}

	if err := m.client.Create(ctx, m.region); err != nil {
		return nil, errors.Wrap(err, "creating client")
	}
	defer m.client.Close()

	describeInstancesOutput, err := m.client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: instanceIDs,
	})

	if err != nil {
		return nil, errors.Wrap(err, "describing instances")
	}

	if err = validateEc2DescribeInstancesOutput(describeInstancesOutput); err != nil {
		return nil, errors.Wrap(err, "invalid describe instances response")
	}

	statuses := map[string]CloudStatus{}
	instanceMap := map[string]*types.Instance{}
	for i := range describeInstancesOutput.Reservations {
		instanceMap[*describeInstancesOutput.Reservations[i].Instances[0].InstanceId] = &describeInstancesOutput.Reservations[i].Instances[0]
		instanceInfo := describeInstancesOutput.Reservations[i].Instances[0]
		instanceID := *instanceInfo.InstanceId
		status := ec2StatusToEvergreenStatus(instanceInfo.State.Name)
		statuses[instanceID] = status
	}

	hostsToCache := make([]hostInstancePair, 0, len(hosts))
	hostIDsToCache := make([]string, 0, len(hosts))
	for i := range hosts {
		h := hosts[i]
		status, ok := statuses[h.Id]
		if !ok {
			statuses[h.Id] = StatusNonExistent
		}
		if status == StatusRunning {
			pair := hostInstancePair{host: &h, instance: instanceMap[h.Id]}
			hostsToCache = append(hostsToCache, pair)
			hostIDsToCache = append(hostIDsToCache, h.Id)
		}
	}

	// Cache instance information so we can make fewer calls to AWS's API.
	grip.Error(message.WrapError(cacheAllHostData(ctx, m.env, m.client, hostsToCache...), message.Fields{
		"message":   "error bulk updating cached host data",
		"num_hosts": len(hostIDsToCache),
		"host_ids":  hostIDsToCache,
	}))

	return statuses, nil
}

func (m *ec2FleetManager) GetInstanceStatus(ctx context.Context, h *host.Host) (CloudStatus, error) {
	status := StatusUnknown

	if err := m.client.Create(ctx, m.region); err != nil {
		return status, errors.Wrap(err, "creating client")
	}
	defer m.client.Close()

	instance, err := m.client.GetInstanceInfo(ctx, h.Id)
	if err != nil {
		if isEC2InstanceNotFound(err) {
			return StatusNonExistent, nil
		}
		grip.Error(message.WrapError(err, message.Fields{
			"message":       "error getting instance info",
			"host_id":       h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		return status, errors.Wrap(err, "getting instance info")
	}

	if instance.State == nil || instance.State.Name == "" {
		return status, errors.New("state name is missing")
	}
	status = ec2StatusToEvergreenStatus(instance.State.Name)
	if status == StatusRunning {
		// Cache instance information so we can make fewer calls to AWS's API.
		pair := hostInstancePair{
			host:     h,
			instance: instance,
		}
		grip.Error(message.WrapError(cacheAllHostData(ctx, m.env, m.client, pair), message.Fields{
			"message": "can't update host cached data",
			"host_id": h.Id,
		}))
	}

	return status, nil
}

func (m *ec2FleetManager) SetPortMappings(context.Context, *host.Host, *host.Host) error {
	return errors.New("can't set port mappings with EC2 fleet provider")
}

func (m *ec2FleetManager) CheckInstanceType(ctx context.Context, instanceType string) error {
	if err := m.client.Create(ctx, m.region); err != nil {
		return errors.Wrap(err, "creating client")
	}
	defer m.client.Close()
	output, err := m.client.DescribeInstanceTypeOfferings(ctx, &ec2.DescribeInstanceTypeOfferingsInput{})
	if err != nil {
		return errors.Wrapf(err, "describing instance types offered for region '%s", m.region)
	}
	validTypes := []string{}
	for _, availableType := range output.InstanceTypeOfferings {
		if availableType.InstanceType == types.InstanceType(instanceType) {
			return nil
		}
		validTypes = append(validTypes, string(availableType.InstanceType))
	}
	return errors.Errorf("available types for region '%s' are: %s", m.region, validTypes)
}

func (m *ec2FleetManager) TerminateInstance(ctx context.Context, h *host.Host, user, reason string) error {
	if h.Status == evergreen.HostTerminated {
		return errors.Errorf("cannot terminate host '%s' because it's already marked as terminated", h.Id)
	}
	if err := m.client.Create(ctx, m.region); err != nil {
		return errors.Wrap(err, "creating client")
	}
	defer m.client.Close()

	resp, err := m.client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: []string{h.Id},
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
		if stateChange.InstanceId == nil {
			grip.Error(message.Fields{
				"message":       "state change missing instance ID",
				"user":          user,
				"host_provider": h.Distro.Provider,
				"host_id":       h.Id,
				"distro":        h.Distro.Id,
			})
			return errors.New("TerminateInstances response did not contain an instance ID")
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

	return errors.Wrap(h.Terminate(ctx, user, reason), "terminating instance in DB")
}

// StopInstance should do nothing for EC2 Fleet.
func (m *ec2FleetManager) StopInstance(context.Context, *host.Host, bool, string) error {
	return errors.New("can't stop instances for EC2 fleet provider")
}

// StartInstance should do nothing for EC2 Fleet.
func (m *ec2FleetManager) StartInstance(context.Context, *host.Host, string) error {
	return errors.New("can't start instances for EC2 fleet provider")
}

func (m *ec2FleetManager) Cleanup(ctx context.Context) error {
	if err := m.client.Create(ctx, m.region); err != nil {
		return errors.Wrap(err, "creating client")
	}
	defer m.client.Close()

	launchTemplates, err := m.client.GetLaunchTemplates(ctx, &ec2.DescribeLaunchTemplatesInput{
		Filters: []types.Filter{
			{Name: aws.String("tag-key"), Values: []string{evergreen.TagDistro}},
		},
	})
	if err != nil {
		return errors.Wrap(err, "getting launch templates")
	}

	catcher := grip.NewBasicCatcher()
	deletedCount := 0
	for _, template := range launchTemplates {
		if template.CreateTime != nil && template.CreateTime.Before(time.Now().Add(-launchTemplateExpiration)) {
			_, err := m.client.DeleteLaunchTemplate(ctx, &ec2.DeleteLaunchTemplateInput{LaunchTemplateId: template.LaunchTemplateId})
			catcher.Add(err)
			if err == nil {
				deletedCount++
			}
		}
	}

	grip.InfoWhen(deletedCount > 0, message.Fields{
		"message":       "removed launch templates",
		"deleted_count": deletedCount,
		"provider":      evergreen.ProviderNameEc2Fleet,
		"region":        m.region,
	})

	return catcher.Resolve()
}

func (m *ec2FleetManager) AttachVolume(context.Context, *host.Host, *host.VolumeAttachment) error {
	return errors.New("can't attach volume with EC2 fleet provider")
}

func (m *ec2FleetManager) DetachVolume(context.Context, *host.Host, string) error {
	return errors.New("can't detach volume with EC2 fleet provider")
}

func (m *ec2FleetManager) CreateVolume(context.Context, *host.Volume) (*host.Volume, error) {
	return nil, errors.New("can't create volume with EC2 fleet provider")
}

func (m *ec2FleetManager) DeleteVolume(context.Context, *host.Volume) error {
	return errors.New("can't delete volume with EC2 fleet provider")
}

func (m *ec2FleetManager) ModifyVolume(context.Context, *host.Volume, *model.VolumeModifyOptions) error {
	return errors.New("can't modify volume with EC2 fleet provider")
}

func (m *ec2FleetManager) GetVolumeAttachment(context.Context, string) (*VolumeAttachment, error) {
	return nil, errors.New("can't get volume attachment with EC2 fleet provider")
}

func (m *ec2FleetManager) GetDNSName(ctx context.Context, h *host.Host) (string, error) {
	if err := m.client.Create(ctx, m.region); err != nil {
		return "", errors.Wrap(err, "creating client")
	}
	defer m.client.Close()

	return m.client.GetPublicDNSName(ctx, h)
}

func (m *ec2FleetManager) TimeTilNextPayment(h *host.Host) time.Duration {
	return timeTilNextEC2Payment(h)
}

func (m *ec2FleetManager) spawnFleetSpotHost(ctx context.Context, h *host.Host, ec2Settings *EC2ProviderSettings) error {
	// Cleanup
	defer func() {
		_, err := m.client.DeleteLaunchTemplate(ctx, &ec2.DeleteLaunchTemplateInput{LaunchTemplateName: aws.String(cleanLaunchTemplateName(h.Tag))})
		grip.Error(message.WrapError(err, message.Fields{
			"message":  "can't delete launch template",
			"host_id":  h.Id,
			"host_tag": h.Tag,
		}))
	}()

	if err := m.uploadLaunchTemplate(ctx, h, ec2Settings); err != nil {
		return errors.Wrapf(err, "unable to upload launch template for host '%s'", h.Id)
	}

	instanceID, err := m.requestFleet(ctx, h, ec2Settings)
	if err != nil {
		return errors.Wrapf(err, "requesting fleet")
	}
	h.Id = instanceID

	return nil
}

func (m *ec2FleetManager) uploadLaunchTemplate(ctx context.Context, h *host.Host, ec2Settings *EC2ProviderSettings) error {
	blockDevices, err := makeBlockDeviceMappingsTemplate(ec2Settings.MountPoints)
	if err != nil {
		return errors.Wrap(err, "making block device mappings")
	}

	launchTemplate := &types.RequestLaunchTemplateData{
		ImageId:             aws.String(ec2Settings.AMI),
		KeyName:             aws.String(ec2Settings.KeyName),
		InstanceType:        types.InstanceType(ec2Settings.InstanceType),
		BlockDeviceMappings: blockDevices,
		TagSpecifications:   makeTagTemplate(makeTags(h)),
	}

	if ec2Settings.IAMInstanceProfileARN != "" {
		launchTemplate.IamInstanceProfile = &types.LaunchTemplateIamInstanceProfileSpecificationRequest{Arn: aws.String(ec2Settings.IAMInstanceProfileARN)}
	}

	if ec2Settings.IsVpc {
		launchTemplate.NetworkInterfaces = []types.LaunchTemplateInstanceNetworkInterfaceSpecificationRequest{
			{
				AssociatePublicIpAddress: aws.Bool(true),
				DeviceIndex:              aws.Int32(0),
				Groups:                   ec2Settings.SecurityGroupIDs,
				SubnetId:                 aws.String(ec2Settings.SubnetId),
			},
		}
		if ec2Settings.IPv6 {
			launchTemplate.NetworkInterfaces[0].Ipv6AddressCount = aws.Int32(1)
			launchTemplate.NetworkInterfaces[0].AssociatePublicIpAddress = aws.Bool(false)
		}
	} else {
		launchTemplate.SecurityGroups = ec2Settings.SecurityGroupIDs
	}

	userData, err := makeUserData(ctx, m.env, h, ec2Settings.UserData, ec2Settings.MergeUserDataParts)
	if err != nil {
		return errors.Wrap(err, "making user data")
	}
	ec2Settings.UserData = userData

	if ec2Settings.UserData != "" {
		var expanded string
		expanded, err = expandUserData(ec2Settings.UserData, m.settings.Expansions)
		if err != nil {
			return errors.Wrap(err, "expanding user data")
		}
		if err = validateUserDataSize(expanded, h.Distro.Id); err != nil {
			return errors.WithStack(err)
		}
		userData := base64.StdEncoding.EncodeToString([]byte(expanded))
		launchTemplate.UserData = &userData
	}

	_, err = m.client.CreateLaunchTemplate(ctx, &ec2.CreateLaunchTemplateInput{
		LaunchTemplateData: launchTemplate,
		LaunchTemplateName: aws.String(cleanLaunchTemplateName(h.Tag)),
		TagSpecifications: []types.TagSpecification{{
			ResourceType: types.ResourceTypeLaunchTemplate,
			Tags:         []types.Tag{{Key: aws.String(evergreen.TagDistro), Value: aws.String(h.Distro.Id)}}},
		},
	})
	if err != nil {
		if errors.Cause(err) == ec2TemplateNameExistsError {
			grip.Info(message.Fields{
				"message":  "template already exists for host",
				"host_id":  h.Id,
				"host_tag": h.Tag,
			})
		} else {
			return errors.Wrap(err, "uploading config template to AWS")
		}
	}

	return nil
}

func (m *ec2FleetManager) requestFleet(ctx context.Context, h *host.Host, ec2Settings *EC2ProviderSettings) (string, error) {
	var overrides []types.FleetLaunchTemplateOverridesRequest
	var err error
	if ec2Settings.VpcName != "" {
		overrides, err = m.makeOverrides(ctx, ec2Settings)
		if err != nil {
			return "", errors.Wrapf(err, "making overrides for VPC '%s'", ec2Settings.VpcName)
		}
	}

	// Create a fleet with a single spot instance from the launch template
	createFleetInput := &ec2.CreateFleetInput{
		LaunchTemplateConfigs: []types.FleetLaunchTemplateConfigRequest{
			{
				LaunchTemplateSpecification: &types.FleetLaunchTemplateSpecificationRequest{
					LaunchTemplateName: aws.String(h.Tag),
					Version:            aws.String("$Latest"),
				},
				Overrides: overrides,
			},
		},
		TargetCapacitySpecification: &types.TargetCapacitySpecificationRequest{
			TotalTargetCapacity:       aws.Int32(1),
			DefaultTargetCapacityType: ec2Settings.FleetOptions.awsTargetCapacityType(),
		},
		Type: types.FleetTypeInstant,
	}

	if allocationStrategy := ec2Settings.FleetOptions.awsAllocationStrategy(); allocationStrategy != "" {
		createFleetInput.SpotOptions = &types.SpotOptionsRequest{AllocationStrategy: allocationStrategy}
	}

	createFleetResponse, err := m.client.CreateFleet(ctx, createFleetInput)
	if err != nil {
		return "", errors.Wrap(err, "creating fleet")
	}
	return createFleetResponse.Instances[0].InstanceIds[0], nil
}

func (m *ec2FleetManager) makeOverrides(ctx context.Context, ec2Settings *EC2ProviderSettings) ([]types.FleetLaunchTemplateOverridesRequest, error) {
	subnets := m.settings.Providers.AWS.Subnets
	if len(subnets) == 0 {
		return nil, errors.New("no AWS subnets were configured")
	}

	supportingSubnets, err := typeCache.subnetsWithInstanceType(ctx, m.settings, m.client, instanceRegionPair{instanceType: ec2Settings.InstanceType, region: ec2Settings.getRegion()})
	if err != nil {
		return nil, errors.Wrapf(err, "getting AZs supporting instance type '%s'", ec2Settings.InstanceType)
	}
	if len(supportingSubnets) == 0 || (len(supportingSubnets) == 1 && supportingSubnets[0].SubnetID == ec2Settings.SubnetId) {
		return nil, nil
	}
	overrides := make([]types.FleetLaunchTemplateOverridesRequest, 0, len(subnets))
	for _, subnet := range supportingSubnets {
		overrides = append(overrides, types.FleetLaunchTemplateOverridesRequest{SubnetId: aws.String(subnet.SubnetID)})
	}

	return overrides, nil
}

func (m *ec2FleetManager) AddSSHKey(ctx context.Context, pair evergreen.SSHKeyPair) error {
	if err := m.client.Create(ctx, m.region); err != nil {
		return errors.Wrap(err, "creating client")
	}
	defer m.client.Close()

	return errors.Wrap(addSSHKey(ctx, m.client, pair), "adding public SSH key")
}
