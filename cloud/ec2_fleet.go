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
	"go.mongodb.org/mongo-driver/bson/primitive"
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

	supportingAZs, err := c.getAZs(ctx, client, instanceRegion)
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

func (c instanceTypeSubnetCache) getAZs(ctx context.Context, client AWSClient, instanceRegion instanceRegionPair) ([]string, error) {
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
	client  AWSClient
	region  string
	account string
	role    string
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

	role, err := getRoleForAccount(settings, m.account)
	if err != nil {
		return errors.Wrap(err, "getting role for account")
	}
	m.role = role

	return nil
}

func (m *ec2FleetManager) setupClient(ctx context.Context) error {
	return m.client.Create(ctx, m.role, m.region)
}

func (m *ec2FleetManager) SpawnHost(ctx context.Context, h *host.Host) (*host.Host, error) {
	if h.Distro.Provider != evergreen.ProviderNameEc2Fleet {
		return nil, errors.Errorf("can't spawn instance for distro '%s': distro provider is '%s'", h.Distro.Id, h.Distro.Provider)
	}

	if err := m.setupClient(ctx); err != nil {
		return nil, errors.Wrap(err, "creating client")
	}

	ec2Settings := &EC2ProviderSettings{}
	if err := ec2Settings.FromDistroSettings(h.Distro, ""); err != nil {
		return nil, errors.Wrap(err, "getting EC2 settings")
	}
	if err := ec2Settings.Validate(); err != nil {
		return nil, errors.Wrapf(err, "invalid EC2 settings in distro '%s': %+v", h.Distro.Id, ec2Settings)
	}

	var err error
	ec2Settings.KeyName, err = getKeyName(ctx, h, m.settings, m.client)
	if err != nil {
		return nil, errors.Wrap(err, "getting key name")
	}

	if err := m.spawnFleetHost(ctx, h, ec2Settings); err != nil {
		msg := "error spawning host with Fleet"
		grip.Error(message.WrapError(err, message.Fields{
			"message":       msg,
			"host_id":       h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		return nil, errors.Wrap(err, msg)
	}
	grip.Debug(message.Fields{
		"message":       "spawned host with Fleet",
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

	if err := m.setupClient(ctx); err != nil {
		return nil, errors.Wrap(err, "creating client")
	}

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
		status := ec2StateToEvergreenStatus(instanceInfo.State)
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

// GetInstanceState returns a universal status code representing the state
// of an ec2 fleet and a state reason if available. The state reason should not be
// used to determine the status of the ec2 fleet but rather to provide additional
// context about the state of the ec2 state.
// For more information about ec2 state's, look in to the instance.StateReason field.
func (m *ec2FleetManager) GetInstanceState(ctx context.Context, h *host.Host) (CloudInstanceState, error) {
	info := CloudInstanceState{Status: StatusUnknown}

	if err := m.setupClient(ctx); err != nil {
		return info, errors.Wrap(err, "creating client")
	}

	instance, err := m.client.GetInstanceInfo(ctx, h.Id)
	if err != nil {
		if isEC2InstanceNotFound(err) {
			info.Status = StatusNonExistent
			return info, nil
		}
		grip.Error(message.WrapError(err, message.Fields{
			"message":       "error getting instance info",
			"host_id":       h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		return info, errors.Wrap(err, "getting instance info")
	}

	if info.Status = ec2StateToEvergreenStatus(instance.State); info.Status == StatusRunning {
		// Cache instance information so we can make fewer calls to AWS's API.
		pair := hostInstancePair{host: h, instance: instance}
		grip.Error(message.WrapError(cacheAllHostData(ctx, m.env, m.client, pair), message.Fields{
			"message": "can't update host cached data",
			"type":    "ec2 fleet",
			"host_id": h.Id,
		}))
	}

	if instance.StateReason != nil {
		info.StateReason = utility.FromStringPtr(instance.StateReason.Message)
	}

	return info, nil
}

func (m *ec2FleetManager) SetPortMappings(context.Context, *host.Host, *host.Host) error {
	return errors.New("can't set port mappings with EC2 fleet provider")
}

func (m *ec2FleetManager) CheckInstanceType(ctx context.Context, instanceType string) error {
	if err := m.setupClient(ctx); err != nil {
		return errors.Wrap(err, "creating client")
	}
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
	if err := m.setupClient(ctx); err != nil {
		return errors.Wrap(err, "creating client")
	}

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
		grip.Info(message.Fields{
			"message":       "terminated instance",
			"user":          user,
			"host_provider": h.Distro.Provider,
			"instance_id":   aws.ToString(stateChange.InstanceId),
			"host_id":       h.Id,
			"distro":        h.Distro.Id,
		})
	}

	grip.Error(message.WrapError(releaseIPAddressForHost(ctx, h), message.Fields{
		"message":        "could not release elastic IP address from host",
		"provider":       h.Distro.Provider,
		"host_id":        h.Id,
		"association_id": h.IPAssociationID,
		"allocation_id":  h.IPAllocationID,
	}))

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

// TODO (DEVPROD-17195): remove temporary method once all elastic IPs are
// allocated into collection.
func (m *ec2FleetManager) AllocateIP(ctx context.Context) (*host.IPAddress, error) {
	if err := m.setupClient(ctx); err != nil {
		return nil, errors.Wrap(err, "creating client")
	}

	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting service flags")
	}
	if flags.ElasticIPsDisabled {
		return nil, errors.Errorf("elastic IP features are disabled, cannot allocate IP")
	}
	allocationID, err := allocateIPAddress(ctx, m.client, m.settings.Providers.AWS.IPAMPoolID)
	if err != nil {
		return nil, errors.Wrap(err, "allocating IP address")
	}
	ipAddr := &host.IPAddress{
		ID:           primitive.NewObjectID().Hex(),
		AllocationID: allocationID,
	}

	return ipAddr, nil
}

// AssociateIP associates the host with its allocated IP address.
func (m *ec2FleetManager) AssociateIP(ctx context.Context, h *host.Host) error {
	if h.IPAllocationID == "" {
		return nil
	}
	if err := m.setupClient(ctx); err != nil {
		return errors.Wrap(err, "creating client")
	}
	return errors.Wrapf(associateIPAddressForHost(ctx, m.client, h), "associating allocated IP address '%s' with host '%s'", h.IPAllocationID, h.Id)
}

// CleanupIP releases the host's IP address.
func (m *ec2FleetManager) CleanupIP(ctx context.Context, h *host.Host) error {
	return releaseIPAddressForHost(ctx, h)
}

func (m *ec2FleetManager) Cleanup(ctx context.Context) error {
	if err := m.setupClient(ctx); err != nil {
		return errors.Wrap(err, "creating client")
	}

	catcher := grip.NewBasicCatcher()
	catcher.Wrap(m.cleanupStaleLaunchTemplates(ctx), "cleaning up stale launch templates")
	// TODO (DEVPROD-17195): remove this cleanup if pre-allocated elastic IPs
	// created in DEVPROD-17313 work.
	// catcher.Wrap(m.cleanupIdleElasticIPs(ctx), "cleaning up idle elastic IPs")
	catcher.Wrap(m.cleanupStaleIPAddresses(ctx), "cleaning up stale IP addresses")

	return catcher.Resolve()
}

// cleanupStaleLaunchTemplates cleans up launch templates that are older than
// launchTemplateExpiration.
func (m *ec2FleetManager) cleanupStaleLaunchTemplates(ctx context.Context) error {
	launchTemplates, err := m.client.GetLaunchTemplates(ctx, &ec2.DescribeLaunchTemplatesInput{
		Filters: []types.Filter{
			{Name: aws.String(filterTagKey), Values: []string{evergreen.TagDistro}},
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
			catcher.Wrapf(err, "deleting launch template '%s'", aws.ToString(template.LaunchTemplateId))
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

// cleanupStaleIPAddresses cleans up IP addresses that are assigned to a host
// but whose host is already terminated.
// kim: TODO: add unit test
func (m *ec2FleetManager) cleanupStaleIPAddresses(ctx context.Context) error {
	staleIPAddrs, err := host.FindStaleIPAddresses(ctx)
	if err != nil {
		return errors.Wrap(err, "finding stale IP addresses")
	}

	ipAddrIDs := make([]string, 0, len(staleIPAddrs))
	for _, ipAddr := range staleIPAddrs {
		ipAddrIDs = append(ipAddrIDs, ipAddr.ID)
	}
	if err := host.IPAddressUnsetHostTags(ctx, ipAddrIDs...); err != nil {
		return errors.Wrapf(err, "unsetting host tags for %d IP addresses", len(ipAddrIDs))
	}

	grip.InfoWhen(len(staleIPAddrs) > 0, message.Fields{
		"message":        "cleaned up stale IP addresses",
		"num_cleaned_up": len(staleIPAddrs),
		"provider":       evergreen.ProviderNameEc2Fleet,
	})

	return nil
}

// cleanupIdleElasticIPs checks for any elastic IP addresses that are not
// being actively used and releases them. This is a very slow operation and can
// take several minutes.
//
//nolint:unused
func (m *ec2FleetManager) cleanupIdleElasticIPs(ctx context.Context) error {
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return errors.Wrap(err, "getting service flags")
	}
	if flags.ElasticIPsDisabled {
		return nil
	}

	ctx, span := tracer.Start(ctx, "cleanupIdleElasticIPs")
	defer span.End()

	idleAddrAllocationIDs, err := m.getIdleElasticIPs(ctx)
	if err != nil {
		return errors.Wrap(err, "getting idle elastic IP addresses for initial check")
	}
	if len(idleAddrAllocationIDs) == 0 {
		return nil
	}

	// Wait for a few minutes and check again to see which of the idle addresses
	// is still idle. Waiting like this is slightly hacky but important because
	// the AWS API doesn't provide information on when an address was last
	// allocated or associated. That makes it hard to tell if the address is
	// idle because it's truly unused or if it was just recently allocated and
	// is waiting to be associated with a host.
	const recheckIdleAddrsAfter = 5 * time.Minute
	timer := time.NewTimer(recheckIdleAddrsAfter)
	defer timer.Stop()

	select {
	case <-timer.C:
		idleAddrAllocationIDsRecheck, err := m.getIdleElasticIPs(ctx)
		if err != nil {
			return errors.Wrap(err, "getting idle elastic IP addresses for recheck")
		}

		idleAddrs := utility.StringSliceIntersection(idleAddrAllocationIDs, idleAddrAllocationIDsRecheck)
		if len(idleAddrs) == 0 {
			return nil
		}

		catcher := grip.NewBasicCatcher()
		for _, idleAddr := range idleAddrs {
			if _, err := m.client.ReleaseAddress(ctx, &ec2.ReleaseAddressInput{
				AllocationId: aws.String(idleAddr),
			}); err != nil {
				catcher.Wrapf(err, "releasing idle elastic IP address with allocation ID '%s'", idleAddr)
			}
		}

		grip.Info(message.Fields{
			"message":      "cleaned up idle elastic IP addresses",
			"num_released": len(idleAddrs),
			"provider":     evergreen.ProviderNameEc2Fleet,
		})

		return catcher.Resolve()
	case <-ctx.Done():
		return errors.Wrap(ctx.Err(), "context was done while waiting for idle addresses")
	}
}

// getIdleElasticIPs gets all elastic IPs that are not currently associated with
// any host.
//
//nolint:unused
func (m *ec2FleetManager) getIdleElasticIPs(ctx context.Context) ([]string, error) {
	descAddrOut, err := m.client.DescribeAddresses(ctx, &ec2.DescribeAddressesInput{
		Filters: []types.Filter{
			{Name: aws.String(filterTagKey), Values: []string{evergreen.TagEvergreenService}},
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "describing elastic IP addresses")
	}

	var idleAddrAllocationIDs []string
	for _, addr := range descAddrOut.Addresses {
		if aws.ToString(addr.AssociationId) == "" && aws.ToString(addr.AllocationId) != "" {
			idleAddrAllocationIDs = append(idleAddrAllocationIDs, aws.ToString(addr.AllocationId))
		}
	}
	return idleAddrAllocationIDs, nil
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
	if err := m.setupClient(ctx); err != nil {
		return "", errors.Wrap(err, "creating client")
	}

	return m.client.GetPublicDNSName(ctx, h)
}

func (m *ec2FleetManager) TimeTilNextPayment(h *host.Host) time.Duration {
	return timeTilNextEC2Payment(h)
}

func (m *ec2FleetManager) spawnFleetHost(ctx context.Context, h *host.Host, ec2Settings *EC2ProviderSettings) error {
	defer func() {
		// Cleanup
		_, err := m.client.DeleteLaunchTemplate(ctx, &ec2.DeleteLaunchTemplateInput{LaunchTemplateName: aws.String(cleanLaunchTemplateName(h.Tag))})
		grip.Error(message.WrapError(err, message.Fields{
			"message":  "can't delete launch template",
			"host_id":  h.Id,
			"host_tag": h.Tag,
		}))
	}()

	ctx, span := tracer.Start(ctx, "spawnFleetHost")
	defer span.End()

	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return errors.Wrap(err, "getting admin settings")
	}
	useElasticIP := shouldAssignPublicIPv4Address(h, ec2Settings) && canUseElasticIP(settings, ec2Settings, m.account, h)
	if useElasticIP && h.IPAllocationID == "" {
		// If the host can't be allocated an IP address, continue on error
		// because the host should fall back to using an AWS-provided IP
		// address. Using an elastic IP address is a best-effort attempt to save
		// money.
		// This must be done before creating the launch template because
		// allocating the address is only a best-effort attempt and isn't
		// guaranteed to succeed. For example, if the IPAM pool has no addresses
		// available currently, Evergreen still needs a usable host, so the
		// launch template has to fall back to using an AWS-managed IP address.
		grip.Notice(message.WrapError(allocateIPAddressForHost(ctx, h), message.Fields{
			"message": "could not allocate elastic IP address for host, falling back to using AWS-managed IP",
			"host_id": h.Id,
		}))
	}

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
		InstanceType:        types.InstanceType(ec2Settings.InstanceType),
		BlockDeviceMappings: blockDevices,
		TagSpecifications:   makeTagTemplate(makeTags(h)),
	}

	if ec2Settings.IAMInstanceProfileARN != "" {
		launchTemplate.IamInstanceProfile = &types.LaunchTemplateIamInstanceProfileSpecificationRequest{Arn: aws.String(ec2Settings.IAMInstanceProfileARN)}
	}

	assignPublicIPv4 := shouldAssignPublicIPv4Address(h, ec2Settings)
	if assignPublicIPv4 {
		// Only set an SSH key for the host if the host actually has a public
		// IPv4 address. Hosts that don't have a public IPv4 address aren't
		// reachable with SSH even if a key is set.
		launchTemplate.KeyName = aws.String(ec2Settings.KeyName)
	}
	if ec2Settings.IsVpc {
		// Fall back to using an AWS-provided IPv4 address if this host needs a
		// public IPv4 address and it hasn't been allocated an elastic IP.
		useAWSIPv4Addr := assignPublicIPv4 && h.IPAllocationID == ""
		launchTemplate.NetworkInterfaces = []types.LaunchTemplateInstanceNetworkInterfaceSpecificationRequest{
			{
				AssociatePublicIpAddress: aws.Bool(useAWSIPv4Addr),
				DeviceIndex:              aws.Int32(0),
				Groups:                   ec2Settings.SecurityGroupIDs,
				SubnetId:                 aws.String(ec2Settings.SubnetId),
			},
		}
		if ec2Settings.IPv6 {
			launchTemplate.NetworkInterfaces[0].Ipv6AddressCount = aws.Int32(1)
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

	// Create a fleet with a single instance from the launch template
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
			DefaultTargetCapacityType: types.DefaultTargetCapacityTypeOnDemand,
		},
		Type: types.FleetTypeInstant,
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
