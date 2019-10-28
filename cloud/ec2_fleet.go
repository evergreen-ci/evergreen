package cloud

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type EC2FleetManagerOptions struct {
	client AWSClient
	region string
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

	key, secret, err := GetEC2Key(m.EC2FleetManagerOptions.region, settings)
	if err != nil {
		return errors.Wrap(err, "Problem getting EC2 keys")
	}

	m.credentials = credentials.NewStaticCredentialsFromCreds(credentials.Value{
		AccessKeyID:     key,
		SecretAccessKey: secret,
	})

	return nil
}

func (m *ec2FleetManager) SpawnHost(ctx context.Context, h *host.Host) (*host.Host, error) {
	if h.Distro.Provider != evergreen.ProviderNameEc2Fleet {
		return nil, errors.Errorf("Can't spawn instance for distro %s: provider is %s",
			h.Distro.Id, h.Distro.Provider)
	}

	ec2Settings := &EC2ProviderSettings{}
	err := ec2Settings.fromDistroSettings(h.Distro)
	if err != nil {
		return nil, errors.Wrap(err, "error getting EC2 settings")
	}

	if ec2Settings.AWSKeyID != "" {
		m.credentials = credentials.NewStaticCredentialsFromCreds(credentials.Value{
			AccessKeyID:     ec2Settings.AWSKeyID,
			SecretAccessKey: ec2Settings.AWSSecret,
		})
	}

	if err = m.client.Create(m.credentials, ec2Settings.getRegion()); err != nil {
		return nil, errors.Wrap(err, "error creating client")
	}
	defer m.client.Close()

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

	blockDevices, err := makeBlockDeviceMappingsTemplate(ec2Settings.MountPoints)
	if err != nil {
		return nil, errors.Wrap(err, "error making block device mappings")
	}

	if err := m.spawnFleetSpotHost(ctx, h, ec2Settings, blockDevices); err != nil {
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

	event.LogHostStarted(h.Id)
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

	instanceMap := map[string]*ec2.Instance{}
	for i := range describeInstancesOutput.Reservations {
		instanceMap[*describeInstancesOutput.Reservations[i].Instances[0].InstanceId] = describeInstancesOutput.Reservations[i].Instances[0]
	}

	// Return as an ordered slice of statuses
	statuses := []CloudStatus{}
	for _, h := range hosts {
		status := ec2StatusToEvergreenStatus(*instanceMap[h.Id].State.Name)
		if status == StatusRunning {
			// cache instance information so we can make fewer calls to AWS's API
			if err = cacheHostData(ctx, &h, instanceMap[h.Id], m.client); err != nil {
				return nil, errors.Wrapf(err, "can't cache host data for '%s'", h.Id)
			}
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func (m *ec2FleetManager) GetInstanceStatus(ctx context.Context, h *host.Host) (CloudStatus, error) {
	status := StatusUnknown

	if err := m.configureClient(h); err != nil {
		return status, errors.Wrapf(err, "can't configure client for '%s'", h.Id)
	}
	defer m.client.Close()

	instance, err := m.getInstance(ctx, h)
	if err != nil {
		return status, errors.Wrapf(err, "can't get instance status")
	}

	status = ec2StatusToEvergreenStatus(*instance.State.Name)
	if status == StatusRunning {
		// cache instance information so we can make fewer calls to AWS's API
		if err = cacheHostData(ctx, h, instance, m.client); err != nil {
			return status, errors.Wrapf(err, "can't update host '%s'", h.Id)
		}
	}

	return status, nil
}

func (m *ec2FleetManager) TerminateInstance(ctx context.Context, h *host.Host, user, reason string) error {
	if h.Status == evergreen.HostTerminated {
		return errors.Errorf("Can not terminate %s - already marked as terminated!", h.Id)
	}
	if err := m.configureClient(h); err != nil {
		return errors.Wrapf(err, "can't configure client for '%s'", h.Id)
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

// OnUp sets tags on the instance created by Fleet
func (m *ec2FleetManager) OnUp(ctx context.Context, h *host.Host) error {
	if err := m.configureClient(h); err != nil {
		return errors.Wrapf(err, "can't configure client for '%s'", h.Id)
	}
	defer m.client.Close()

	resources := []string{h.Id}
	volumeIDs, err := m.client.GetVolumeIDs(ctx, h)
	if err != nil {
		return errors.Wrapf(err, "can't get volume IDs for '%s'", h.Id)
	}
	resources = append(resources, volumeIDs...)

	if err := m.client.SetTags(ctx, resources, h); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":       "error attaching tags",
			"host":          h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		return errors.Wrapf(err, "failed to attach tags for %s", h.Id)
	}
	grip.Debug(message.Fields{
		"message":       "attached tags for host",
		"host":          h.Id,
		"host_provider": h.Distro.Provider,
		"distro":        h.Distro.Id,
	})

	return nil
}

func (m *ec2FleetManager) GetDNSName(ctx context.Context, h *host.Host) (string, error) {
	if err := m.configureClient(h); err != nil {
		return "", errors.Wrapf(err, "can't configure client for '%s'", h.Id)
	}
	defer m.client.Close()

	return m.client.GetPublicDNSName(ctx, h)
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

func (m *ec2FleetManager) spawnFleetSpotHost(ctx context.Context, h *host.Host, ec2Settings *EC2ProviderSettings, blockDevices []*ec2.LaunchTemplateBlockDeviceMappingRequest) error {
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
		return errors.Wrapf(err, "unable to upload launch template for '%s'", h.Id)
	}

	instanceID, err := m.requestFleet(ctx, ec2Settings, templateID, templateVersion)
	if err != nil {
		return errors.Wrapf(err, "can't request fleet")
	}
	h.Id = *instanceID

	return nil
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

	userData, err := bootstrapUserData(ctx, m.env, h, ec2Settings.UserData)
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
	err = validateEc2CreateFleetResponse(createFleetResponse)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":  "invalid create fleet response",
			"request":  createFleetInput,
			"response": createFleetResponse,
		}))
		return nil, errors.Wrap(err, "invalid create fleet response")
	}

	return createFleetResponse.Instances[0].InstanceIds[0], nil
}

func (m *ec2FleetManager) makeOverrides(ctx context.Context, ec2Settings *EC2ProviderSettings) ([]*ec2.FleetLaunchTemplateOverridesRequest, error) {
	describeVpcsOutput, err := m.client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String("tag:Name"),
				Values: []*string{
					aws.String(ec2Settings.VpcName),
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
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{describeVpcsOutput.Vpcs[0].VpcId},
			},
		},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "can't get subnets for vpc '%s'", ec2Settings.VpcName)
	}
	err = validateEc2DescribeSubnetsOutput(describeSubnetsOutput)
	if err != nil {
		return nil, errors.Wrap(err, "invalid describe subnets response")
	}

	AZSet := make(map[string]bool)
	overrides := make([]*ec2.FleetLaunchTemplateOverridesRequest, 0, len(describeSubnetsOutput.Subnets))
	for _, subnet := range describeSubnetsOutput.Subnets {
		// AWS only allows one override per AZ
		if !AZSet[*subnet.AvailabilityZone] && subnetMatchesAz(subnet) {
			overrides = append(overrides, &ec2.FleetLaunchTemplateOverridesRequest{SubnetId: subnet.SubnetId})
			AZSet[*subnet.AvailabilityZone] = true
		}
	}

	return overrides, nil
}

func (m *ec2FleetManager) configureClient(h *host.Host) error {
	ec2Settings := &EC2ProviderSettings{}
	err := ec2Settings.fromDistroSettings(h.Distro)
	if err != nil {
		return errors.Wrap(err, "problem getting settings from host")
	}
	if err = m.client.Create(m.credentials, ec2Settings.getRegion()); err != nil {
		return errors.Wrap(err, "error creating client")
	}

	return nil
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
