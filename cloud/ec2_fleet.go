package cloud

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type ec2FleetManager struct {
	client      AWSClient
	credentials *credentials.Credentials
	settings    *evergreen.Settings
}

func (m *ec2FleetManager) GetSettings() ProviderSettings {
	return &EC2ProviderSettings{}
}

func (m *ec2FleetManager) Configure(ctx context.Context, settings *evergreen.Settings) error {
	if m.client == nil {
		m.client = &awsClientImpl{}
	}

	m.settings = settings

	if settings.Providers.AWS.EC2Key == "" || settings.Providers.AWS.EC2Secret == "" {
		return errors.New("AWS ID and Secret must not be blank")
	}

	m.credentials = credentials.NewStaticCredentialsFromCreds(credentials.Value{
		AccessKeyID:     settings.Providers.AWS.EC2Key,
		SecretAccessKey: settings.Providers.AWS.EC2Secret,
	})

	return nil
}

func (m *ec2FleetManager) SpawnHost(ctx context.Context, h *host.Host) (*host.Host, error) {
	if h.Distro.Provider != evergreen.ProviderNameEc2Fleet {
		return nil, errors.Errorf("Can't spawn instance for distro %s: provider is %s",
			h.Distro.Id, h.Distro.Provider)
	}

	ec2Settings := &EC2ProviderSettings{}
	if h.Distro.ProviderSettings != nil {
		if err := mapstructure.Decode(h.Distro.ProviderSettings, ec2Settings); err != nil {
			return nil, errors.Wrapf(err, "Error decoding params for distro %s: %+v", h.Distro.Id, ec2Settings)
		}
	}
	grip.Debug(message.Fields{
		"message":   "mapstructure comparison",
		"input":     *h.Distro.ProviderSettings,
		"output":    *ec2Settings,
		"inputraw":  fmt.Sprintf("%#v", *h.Distro.ProviderSettings),
		"outputraw": fmt.Sprintf("%#v", *ec2Settings),
	})
	if err := ec2Settings.Validate(); err != nil {
		return nil, errors.Wrapf(err, "Invalid EC2 settings in distro %s: and %+v", h.Distro.Id, ec2Settings)
	}

	if ec2Settings.AWSKeyID != "" {
		m.credentials = credentials.NewStaticCredentialsFromCreds(credentials.Value{
			AccessKeyID:     ec2Settings.AWSKeyID,
			SecretAccessKey: ec2Settings.AWSSecret,
		})
	}

	if ec2Settings.KeyName == "" && !h.UserHost {
		if !h.SpawnOptions.SpawnedByTask {
			return nil, errors.New("key name must not be empty")
		}
		k, err := getKey(ctx, m.client, m.credentials, h)
		if err != nil {
			return nil, errors.Wrap(err, "not spawning host, problem creating key")
		}
		ec2Settings.KeyName = k
	}

	blockDevices, err := makeBlockDeviceMappingsTemplate(ec2Settings.MountPoints)
	if err != nil {
		return nil, errors.Wrap(err, "error making block device mappings")
	}

	r, err := getRegion(h)
	if err != nil {
		return nil, errors.Wrap(err, "problem getting region from host")
	}
	if err = m.client.Create(m.credentials, r); err != nil {
		return nil, errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	resources, err := m.spawnFleetSpotHost(ctx, h, ec2Settings, blockDevices)
	if err != nil {
		msg := "error spawning spot host with Fleet"
		grip.Error(message.WrapError(err, message.Fields{
			"message":       msg,
			"host":          h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		return nil, errors.Wrap(err, msg)
	}
	grip.Debug(message.Fields{
		"message":       "spawned spot host with Fleet",
		"host":          h.Id,
		"host_provider": h.Distro.Provider,
		"distro":        h.Distro.Id,
	})

	if err = setTags(ctx, resources, h, m.client, m.credentials); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":       "error attaching tags",
			"host":          h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		return nil, errors.Wrapf(err, "failed to attach tags for %s", h.Id)
	}
	grip.Debug(message.Fields{
		"message":       "attached tags for host",
		"host":          h.Id,
		"host_provider": h.Distro.Provider,
		"distro":        h.Distro.Id,
	})

	event.LogHostStarted(h.Id)
	return h, nil
}

func (m *ec2FleetManager) GetInstanceStatuses(ctx context.Context, hosts []host.Host) ([]CloudStatus, error) {
	instanceIDs := make([]*string, 0, len(hosts))
	for _, h := range hosts {
		instanceIDs = append(instanceIDs, aws.String(h.Id))
	}

	if err := m.client.Create(m.credentials, defaultRegion); err != nil {
		return nil, errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	describeInstancesOutput, err := m.client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: instanceIDs,
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

	statusMap := map[string]CloudStatus{}
	for i := range describeInstancesOutput.Reservations {
		statusMap[*describeInstancesOutput.Reservations[i].Instances[0].InstanceId] = ec2StatusToEvergreenStatus(*describeInstancesOutput.Reservations[i].Instances[0].State.Name)
	}

	// Return as an ordered slice of statuses
	statuses := []CloudStatus{}
	for _, h := range hosts {
		statuses = append(statuses, statusMap[h.Id])
	}
	return statuses, nil
}

func (m *ec2FleetManager) GetInstanceStatus(ctx context.Context, h *host.Host) (CloudStatus, error) {
	r, err := getRegion(h)
	if err != nil {
		return StatusUnknown, errors.Wrap(err, "problem getting region from host")
	}
	if err = m.client.Create(m.credentials, r); err != nil {
		return StatusUnknown, errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	instance, err := m.getInstance(ctx, h)
	if err != nil {
		return StatusUnknown, errors.Wrapf(err, "can't get instance status")
	}

	return ec2StatusToEvergreenStatus(*instance.State.Name), nil
}

func (m *ec2FleetManager) TerminateInstance(ctx context.Context, h *host.Host, user string) error {
	if h.Status == evergreen.HostTerminated {
		return errors.Errorf("Can not terminate %s - already marked as terminated!", h.Id)
	}
	r, err := getRegion(h)
	if err != nil {
		return errors.Wrap(err, "problem getting region from host")
	}
	if err = m.client.Create(m.credentials, r); err != nil {
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
			"host":          h.Id,
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
				"host":          h.Id,
				"distro":        h.Distro.Id,
			})
			return errors.New("invalid terminate instances response")
		}
		grip.Info(message.Fields{
			"message":       "terminated instance",
			"user":          user,
			"host_provider": h.Distro.Provider,
			"instance_id":   *stateChange.InstanceId,
			"host":          h.Id,
			"distro":        h.Distro.Id,
		})
	}

	return errors.Wrap(h.Terminate(user), "failed to terminate instance in db")
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

// OnUp is a noop for ec2Fleet
func (m *ec2FleetManager) OnUp(context.Context, *host.Host) error {
	return nil
}

func (m *ec2FleetManager) GetDNSName(ctx context.Context, h *host.Host) (string, error) {
	r, err := getRegion(h)
	if err != nil {
		return "", errors.Wrap(err, "problem getting region from host")
	}
	if err = m.client.Create(m.credentials, r); err != nil {
		return "", errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

	instance, err := m.getInstance(ctx, h)
	if err != nil {
		return "", errors.Wrapf(err, "can't get instance information for '%s", h.Id)
	}

	// Cache availability zone, launch time, and volume size in host document, since we
	// have access to this information now. Cost jobs will use this
	// information later.
	h.Zone = *instance.Placement.AvailabilityZone
	h.StartTime = *instance.LaunchTime
	h.VolumeTotalSize, err = getVolumeSize(ctx, m.client, h)
	if err != nil {
		return "", errors.Wrapf(err, "error getting volume size for host %s", h.Id)
	}
	if err := h.CacheHostData(); err != nil {
		return "", errors.Wrap(err, "error updating host document in db")
	}

	// set IPv6 address, if applicable
	for _, networkInterface := range instance.NetworkInterfaces {
		if len(networkInterface.Ipv6Addresses) > 0 {
			err = h.SetIPv6Address(*networkInterface.Ipv6Addresses[0].Ipv6Address)
			if err != nil {
				return "", errors.Wrap(err, "error setting ipv6 address")
			}
			break
		}
	}
	return *instance.PublicDnsName, nil
}

func (m *ec2FleetManager) GetSSHOptions(h *host.Host, keyName string) ([]string, error) {
	return getEc2SSHOptions(h, keyName)
}

func (m *ec2FleetManager) TimeTilNextPayment(h *host.Host) time.Duration {
	return timeTilNextEC2Payment(h)
}

func (m *ec2FleetManager) getInstance(ctx context.Context, h *host.Host) (*ec2.Instance, error) {
	instance, err := m.client.GetInstanceInfo(ctx, h.Id)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":       "error getting instance info",
			"host":          h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		return nil, errors.Wrap(err, "error getting instance info")
	}

	if err = validateEc2InstanceInfoResponse(instance); err != nil {
		return nil, errors.Wrap(err, "invalid instance info response")
	}

	return instance, nil
}

func (m *ec2FleetManager) spawnFleetSpotHost(ctx context.Context, h *host.Host, ec2Settings *EC2ProviderSettings, blockDevices []*ec2.LaunchTemplateBlockDeviceMappingRequest) ([]*string, error) {
	// Cleanup
	var templateID *string
	defer func() {
		if templateID != nil {
			// Delete launch template
			_, err := m.client.DeleteLaunchTemplate(ctx, &ec2.DeleteLaunchTemplateInput{LaunchTemplateId: templateID})
			grip.Error(message.WrapError(err, message.Fields{
				"message":  "can't delete launch template",
				"host":     h.Id,
				"template": *templateID,
			}))
		}
	}()

	var err error
	var templateVersion *int64
	templateID, templateVersion, err = m.uploadLaunchTemplate(ctx, h, ec2Settings, blockDevices)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to upload launch template for '%s'", h.Id)
	}

	instanceID, err := m.requestFleet(ctx, ec2Settings, templateID, templateVersion)
	if err != nil {
		return nil, errors.Wrapf(err, "can't request fleet")
	}
	h.Id = *instanceID

	instanceInfo, err := m.client.GetInstanceInfo(ctx, h.Id)
	if err != nil {
		return nil, errors.Wrap(err, "can't get instance descriptions")
	}

	// return a slice of resources to be tagged
	resources := []*string{instanceID}
	for _, vol := range instanceInfo.BlockDeviceMappings {
		if *vol.DeviceName == "" {
			continue
		}

		resources = append(resources, vol.Ebs.VolumeId)
	}

	return resources, nil
}

func (m *ec2FleetManager) uploadLaunchTemplate(ctx context.Context, h *host.Host, ec2Settings *EC2ProviderSettings, blockDevices []*ec2.LaunchTemplateBlockDeviceMappingRequest) (*string, *int64, error) {
	launchTemplate := &ec2.RequestLaunchTemplateData{
		ImageId:             aws.String(ec2Settings.AMI),
		KeyName:             aws.String(ec2Settings.KeyName),
		InstanceType:        aws.String(ec2Settings.InstanceType),
		BlockDeviceMappings: blockDevices,
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

	userData, err := bootstrapUserData(ctx, evergreen.GetEnvironment(), h, ec2Settings.UserData)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not add bootstrap script to user data")
	}
	ec2Settings.UserData = userData

	if ec2Settings.UserData != "" {
		var expanded string
		expanded, err = expandUserData(ec2Settings.UserData, m.settings.Expansions)
		if err != nil {
			return nil, nil, errors.Wrap(err, "problem expanding user data")
		}
		userData := base64.StdEncoding.EncodeToString([]byte(expanded))
		launchTemplate.UserData = &userData
	}

	createTemplateResponse, err := m.client.CreateLaunchTemplate(ctx, &ec2.CreateLaunchTemplateInput{
		LaunchTemplateData: launchTemplate,
		// mandatory field may only contain letters, numbers, and the following characters: - ( ) . / _
		LaunchTemplateName: aws.String(fmt.Sprintf("%s", util.RandomString())),
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
		overrides, err = m.makeOverrides(ctx, ec2Settings.VpcName)
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
	err = validateEc2CreateFleetResponse(createFleetResponse)
	if err != nil {
		return nil, errors.Wrap(err, "invalid create fleet response")
	}

	return createFleetResponse.Instances[0].InstanceIds[0], nil
}

func (m *ec2FleetManager) makeOverrides(ctx context.Context, vpcName string) ([]*ec2.FleetLaunchTemplateOverridesRequest, error) {
	describeVpcsOutput, err := m.client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{
				Name: aws.String("tag:Name"),
				Values: []*string{
					aws.String(vpcName),
				},
			},
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "error finding vpc id")
	}
	err = validateEc2DescribeVpcsOutput(describeVpcsOutput)
	if err != nil {
		return nil, errors.Wrap(err, "invalid describe VPCs response")
	}

	describeSubnetsOutput, err := m.client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{
				Name:   aws.String("vpc-id"),
				Values: []*string{describeVpcsOutput.Vpcs[0].VpcId},
			},
		},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "can't get subnets for vpc '%s'", vpcName)
	}
	err = validateEc2DescribeSubnetsOutput(describeSubnetsOutput)
	if err != nil {
		return nil, errors.Wrap(err, "invalid describe subnets response")
	}

	overrides := make([]*ec2.FleetLaunchTemplateOverridesRequest, 0, len(describeSubnetsOutput.Subnets))
	for _, subnet := range describeSubnetsOutput.Subnets {
		overrides = append(overrides, &ec2.FleetLaunchTemplateOverridesRequest{SubnetId: subnet.SubnetId})
	}

	return overrides, nil
}
