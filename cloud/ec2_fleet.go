package cloud

import (
	"context"
	"encoding/base64"
	"fmt"
	"sync"
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

// instanceTypeSubnetCache caches the subnets that can run a particular instance
// type.
type instanceTypeSubnetCache struct {
	mu    sync.Mutex
	cache map[instanceTypeCacheKey]cachedSubnets
}

type instanceTypeCacheKey struct {
	account      string
	region       string
	instanceType string
}

// cachedSubnets is the result of looking up the subnets for a single
// instance type in a particular account.
type cachedSubnets struct {
	subnets []evergreen.Subnet
	// err is cached if subnets cannot be looked up for the instance type (to
	// avoid repeating failing lookups).
	err error
}

// typeCache caches the subnets that can run a particular instance type within a
// particular region and AWS account.
var typeCache *instanceTypeSubnetCache

func init() {
	typeCache = &instanceTypeSubnetCache{cache: map[instanceTypeCacheKey]cachedSubnets{}}
}

// subnetsWithInstanceType returns the subnets that Evergreen can use for the
// given instance type, region, and AWS account. Subnets for the default account
// come from the admin settings, while subnets for any other account are
// discovered from AWS by their tag.
func (c *instanceTypeSubnetCache) subnetsWithInstanceType(ctx context.Context, settings *evergreen.Settings, client AWSClient, cacheKey instanceTypeCacheKey) ([]evergreen.Subnet, error) {
	if cached, ok := c.get(cacheKey); ok {
		return cached.subnets, cached.err
	}

	var candidateSubnets []evergreen.Subnet
	if cacheKey.account != "" {
		discovered, err := c.discoverSubnets(ctx, settings, client)
		if err != nil {
			c.set(cacheKey, cachedSubnets{err: err})
			return nil, errors.Wrapf(err, "discovering subnets for account '%s'", cacheKey.account)
		}
		candidateSubnets = discovered
	} else {
		candidateSubnets = settings.Providers.AWS.Subnets
	}

	supportingAZs, err := c.getAZs(ctx, client, cacheKey)
	if err != nil {
		c.set(cacheKey, cachedSubnets{err: err})
		return nil, errors.Wrap(err, "getting supported AZs")
	}

	// Filter subnets to those that are in AZs that support the instance type.
	subnets := make([]evergreen.Subnet, 0, len(candidateSubnets))
	for _, subnet := range candidateSubnets {
		if utility.StringSliceContains(supportingAZs, subnet.AZ) {
			subnets = append(subnets, subnet)
		}
	}

	c.set(cacheKey, cachedSubnets{subnets: subnets})

	return subnets, nil
}

func (c *instanceTypeSubnetCache) get(key instanceTypeCacheKey) (cachedSubnets, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	cached, ok := c.cache[key]
	return cached, ok
}

func (c *instanceTypeSubnetCache) set(key instanceTypeCacheKey, result cachedSubnets) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache[key] = result
}

// discoverSubnets returns the subnets in the client's AWS account and region
// that are tagged as usable by Evergreen.
func (c *instanceTypeSubnetCache) discoverSubnets(ctx context.Context, settings *evergreen.Settings, client AWSClient) ([]evergreen.Subnet, error) {
	tagName := settings.Providers.AWS.SubnetTagName
	tagValue := settings.Providers.AWS.SubnetTagValue
	if tagName == "" || tagValue == "" {
		return nil, errors.New("subnet tag name and value must both be set to discover subnets")
	}

	output, err := client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		Filters: []types.Filter{
			{
				Name:   aws.String(fmt.Sprintf("tag:%s", tagName)),
				Values: []string{tagValue},
			},
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "describing subnets")
	}
	if output == nil {
		return nil, errors.New("DescribeSubnets returned nil output")
	}

	subnets := make([]evergreen.Subnet, 0, len(output.Subnets))
	for _, subnet := range output.Subnets {
		az := utility.FromStringPtr(subnet.AvailabilityZone)
		subnetID := utility.FromStringPtr(subnet.SubnetId)
		if az == "" || subnetID == "" {
			continue
		}
		subnets = append(subnets, evergreen.Subnet{AZ: az, SubnetID: subnetID})
	}
	if len(subnets) == 0 {
		return nil, errors.Errorf("no subnets are tagged with '%s: %s'", tagName, tagValue)
	}

	return subnets, nil
}

// getAZsf returns the availability zones that support a particular instance
// type.
func (c *instanceTypeSubnetCache) getAZs(ctx context.Context, client AWSClient, cacheKey instanceTypeCacheKey) ([]string, error) {
	// DescribeInstanceTypeOfferings only returns AZs in the client's region
	output, err := client.DescribeInstanceTypeOfferings(ctx, &ec2.DescribeInstanceTypeOfferingsInput{
		LocationType: types.LocationTypeAvailabilityZone,
		Filters: []types.Filter{
			{Name: aws.String("instance-type"), Values: []string{cacheKey.instanceType}},
		},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "getting instance types for filter '%s' in region '%s'", cacheKey.instanceType, cacheKey.region)
	}
	if output == nil {
		return nil, errors.Errorf("DescribeInstanceTypeOfferings returned nil output for instance type filter '%s' in '%s'", cacheKey.instanceType, cacheKey.region)
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
	// ec2Mgr handles host management operations (ModifyHost, Stop/Start/Reboot, volumes).
	ec2Mgr *ec2Manager
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

	m.ec2Mgr = &ec2Manager{
		EC2ManagerOptions: &EC2ManagerOptions{
			client:  m.client,
			region:  m.region,
			account: m.account,
			role:    m.role,
		},
		settings: settings,
		env:      m.env,
	}

	return nil
}

func (m *ec2FleetManager) setupClient(ctx context.Context) error {
	return m.client.Create(ctx, m.role, m.region)
}

func (m *ec2FleetManager) SpawnHost(ctx context.Context, h *host.Host) (*host.Host, error) {
	if !evergreen.IsEc2Provider(h.Distro.Provider) {
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
		return nil, errors.Wrapf(err, "invalid EC2 settings in distro '%s'", h.Distro.Id)
	}

	var err error
	ec2Settings.KeyName, err = getKeyName(ctx, h, m.settings, m.client)
	if err != nil {
		return nil, errors.Wrap(err, "getting key name")
	}

	if err := m.spawnFleetHost(ctx, h, ec2Settings); err != nil {
		msg := "error spawning host with Fleet"
		grip.Error(ctx, message.WrapError(err, message.Fields{
			"message":       msg,
			"host_id":       h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		return nil, errors.Wrap(err, msg)
	}
	grip.Debug(ctx, message.Fields{
		"message":       "spawned host with Fleet",
		"host_id":       h.Id,
		"host_provider": h.Distro.Provider,
		"distro":        h.Distro.Id,
	})

	return h, nil
}

// ModifyHost modifies an existing host's properties (tags, instance type, sleep schedule, expiration).
func (m *ec2FleetManager) ModifyHost(ctx context.Context, h *host.Host, opts host.HostModifyOptions) error {
	if opts.ExtendExpireOnByDay {
		if err := m.setupClient(ctx); err != nil {
			return errors.Wrap(err, "creating client")
		}
		return errors.Wrap(extendExpireOnByDay(ctx, m.client, h), "extending expire-on tag by one day")
	}
	return m.ec2Mgr.ModifyHost(ctx, h, opts)
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
	grip.Error(ctx, message.WrapError(cacheAllHostData(ctx, m.env, m.client, hostsToCache...), message.Fields{
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
		grip.Error(ctx, message.WrapError(err, message.Fields{
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
		grip.Error(ctx, message.WrapError(cacheAllHostData(ctx, m.env, m.client, pair), message.Fields{
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

func (m *ec2FleetManager) CheckInstanceType(ctx context.Context, instanceType string) error {
	if err := m.setupClient(ctx); err != nil {
		return errors.Wrap(err, "creating client")
	}
	output, err := m.client.DescribeInstanceTypeOfferings(ctx, &ec2.DescribeInstanceTypeOfferingsInput{})
	if err != nil {
		return errors.Wrapf(err, "describing instance types offered for region '%s'", m.region)
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

	instanceID := h.Id
	if !IsEC2InstanceID(instanceID) {
		instance, err := m.client.GetInstanceInfo(ctx, h.Id)
		if err != nil {
			if isEC2InstanceNotFound(err) {
				return errors.Wrap(h.Terminate(ctx, user, fmt.Sprintf("no cloud instance found for host '%s'", h.Id)), "terminating instance in DB")
			}
			return errors.Wrapf(err, "finding cloud instance for host '%s'", h.Id)
		}
		instanceID = aws.ToString(instance.InstanceId)
	}

	// Any host that has been unexpirable will have been given a DNS name, which we need to clean up.
	if h.PersistentDNSName != "" {
		dnsName := h.PersistentDNSName
		err := deleteHostPersistentDNSName(ctx, m.env, h, m.client)
		grip.Error(ctx, message.WrapError(err, message.Fields{
			"message":    "could not delete host's persistent DNS name",
			"op":         "delete",
			"dashboard":  "evergreen sleep schedule health",
			"host_id":    h.Id,
			"started_by": h.StartedBy,
			"dns_name":   dnsName,
		}))
		grip.InfoWhen(ctx, err == nil, message.Fields{
			"message":    "deleted host's persistent DNS name",
			"op":         "delete",
			"dashboard":  "evergreen sleep schedule health",
			"host_id":    h.Id,
			"started_by": h.StartedBy,
			"dns_name":   dnsName,
		})
	}

	resp, err := m.client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: []string{instanceID},
	})
	if err != nil {
		grip.Error(ctx, message.WrapError(err, message.Fields{
			"message":       "error terminating instance",
			"user":          user,
			"host_id":       h.Id,
			"host_provider": h.Distro.Provider,
			"distro":        h.Distro.Id,
		}))
		return err
	}

	for _, stateChange := range resp.TerminatingInstances {
		grip.Info(ctx, message.Fields{
			"message":       "terminated instance",
			"user":          user,
			"host_provider": h.Distro.Provider,
			"instance_id":   aws.ToString(stateChange.InstanceId),
			"host_id":       h.Id,
			"distro":        h.Distro.Id,
		})
	}

	grip.Error(ctx, message.WrapError(releaseIPAddressForHost(ctx, h), message.Fields{
		"message":        "could not release elastic IP address from host",
		"provider":       h.Distro.Provider,
		"host_id":        h.Id,
		"association_id": h.IPAssociationID,
		"allocation_id":  h.IPAllocationID,
	}))

	for _, vol := range h.Volumes {
		volDB, err := host.FindVolumeByID(ctx, vol.VolumeID)
		if err != nil {
			return errors.Wrap(err, "finding volumes for host")
		}
		if volDB == nil {
			continue
		}

		if volDB.Expiration.Before(time.Now().Add(evergreen.UnattachedVolumeExpiration)) {
			if err = m.ec2Mgr.modifyVolumeExpiration(ctx, volDB, time.Now().Add(evergreen.UnattachedVolumeExpiration)); err != nil {
				grip.Error(ctx, message.WrapError(err, message.Fields{
					"message": "error updating volume expiration",
					"user":    user,
					"host_id": h.Id,
					"volume":  volDB.ID,
				}))
				return errors.Wrapf(err, "updating expiration for volume '%s'", volDB.ID)
			}
		}

		if err = host.UnsetVolumeHost(ctx, volDB.ID); err != nil {
			grip.Error(ctx, message.WrapError(err, message.Fields{
				"host_id":   h.Id,
				"volume_id": volDB.ID,
				"op":        "terminating host",
				"message":   "problem un-setting host info on volume records",
			}))
		}
	}

	return errors.Wrap(h.Terminate(ctx, user, reason), "terminating instance in DB")
}

// StopInstance stops a running host instance.
func (m *ec2FleetManager) StopInstance(ctx context.Context, h *host.Host, shouldKeepOff bool, user string) error {
	return m.ec2Mgr.StopInstance(ctx, h, shouldKeepOff, user)
}

// StartInstance starts a stopped host instance.
func (m *ec2FleetManager) StartInstance(ctx context.Context, h *host.Host, user string) error {
	return m.ec2Mgr.StartInstance(ctx, h, user)
}

// RebootInstance reboots a running host instance.
func (m *ec2FleetManager) RebootInstance(ctx context.Context, h *host.Host, user string) error {
	return m.ec2Mgr.RebootInstance(ctx, h, user)
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

	grip.InfoWhen(ctx, deletedCount > 0, message.Fields{
		"message":       "removed launch templates",
		"deleted_count": deletedCount,
		"provider":      evergreen.ProviderNameEc2Fleet,
		"region":        m.region,
	})

	return catcher.Resolve()
}

// cleanupStaleIPAddresses cleans up IP addresses that are assigned to a host
// but whose host is already terminated.
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

	grip.InfoWhen(ctx, len(staleIPAddrs) > 0, message.Fields{
		"message":        "cleaned up stale IP addresses",
		"num_cleaned_up": len(staleIPAddrs),
		"provider":       evergreen.ProviderNameEc2Fleet,
	})

	return nil
}

// AttachVolume attaches an EBS volume to a host.
func (m *ec2FleetManager) AttachVolume(ctx context.Context, h *host.Host, attachment *host.VolumeAttachment) error {
	return m.ec2Mgr.AttachVolume(ctx, h, attachment)
}

// DetachVolume detaches an EBS volume from a host.
func (m *ec2FleetManager) DetachVolume(ctx context.Context, h *host.Host, volumeID string) error {
	return m.ec2Mgr.DetachVolume(ctx, h, volumeID)
}

// CreateVolume creates a new EBS volume for attaching to a host.
func (m *ec2FleetManager) CreateVolume(ctx context.Context, volume *host.Volume) (*host.Volume, error) {
	return m.ec2Mgr.CreateVolume(ctx, volume)
}

// DeleteVolume deletes an EBS volume.
func (m *ec2FleetManager) DeleteVolume(ctx context.Context, volume *host.Volume) error {
	return m.ec2Mgr.DeleteVolume(ctx, volume)
}

// ModifyVolume modifies an existing EBS volume's properties.
func (m *ec2FleetManager) ModifyVolume(ctx context.Context, volume *host.Volume, opts *model.VolumeModifyOptions) error {
	return m.ec2Mgr.ModifyVolume(ctx, volume, opts)
}

// GetVolumeAttachment returns the attachment information for a volume.
func (m *ec2FleetManager) GetVolumeAttachment(ctx context.Context, volumeID string) (*VolumeAttachment, error) {
	return m.ec2Mgr.GetVolumeAttachment(ctx, volumeID)
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
		grip.Error(ctx, message.WrapError(err, message.Fields{
			"message":  "can't delete launch template",
			"host_id":  h.Id,
			"host_tag": h.Tag,
		}))
	}()

	ctx, span := tracer.Start(ctx, "spawnFleetHost")
	defer span.End()

	if err := terminatePreexistingInstance(ctx, m.client, h.Id); err != nil {
		return errors.Wrap(err, "terminating pre-existing instance from prior attempt")
	}

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
		grip.Notice(ctx, message.WrapError(allocateIPAddressForHost(ctx, h), message.Fields{
			"message": "could not allocate elastic IP address for host, falling back to using AWS-managed IP",
			"host_id": h.Id,
		}))
	}

	if err := m.uploadLaunchTemplate(ctx, h, ec2Settings); err != nil {
		return errors.Wrapf(err, "uploading launch template for host '%s'", h.Id)
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
	if ec2Settings.EnableNestedVirtualization {
		launchTemplate.CpuOptions = &types.LaunchTemplateCpuOptionsRequest{
			NestedVirtualization: types.NestedVirtualizationSpecificationEnabled,
		}
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
			grip.Info(ctx, message.Fields{
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
	overrides, err := m.makeSubnetOverrides(ctx, h, ec2Settings)
	if err != nil {
		return "", errors.Wrap(err, "making launch template overrides")
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

// makeSubnetOverrides returns the subnets that the fleet request can choose
// between to launch the host. Returning no subnet overrides means the fleet
// request is limited to the distro's default subnet.
// Allowing multiple subnets allows an instance to spawn in different
// availability zones. Multiple subnets have a better chance of successfully
// spawning the host in case one AZ is out of capacity for the instance type.
func (m *ec2FleetManager) makeSubnetOverrides(ctx context.Context, h *host.Host, ec2Settings *EC2ProviderSettings) ([]types.FleetLaunchTemplateOverridesRequest, error) {
	// An EC2 fleet request can only use different subnets if:
	// 1. Hosts are being started in a non-default account (i.e. account is
	//    non-empty) or
	// 2. In the legacy case, hosts in the default AWS account use VPC name
	//    as a feature flag to indicate they have multiple subnets
	//    available.
	if m.account == "" && ec2Settings.VpcName == "" {
		return nil, nil
	}

	supportingSubnets, err := typeCache.subnetsWithInstanceType(ctx, m.settings, m.client, instanceTypeCacheKey{
		instanceType: ec2Settings.InstanceType,
		region:       ec2Settings.getRegion(),
		account:      m.account,
	})
	if err != nil {
		// Not being able to determine the subnets shouldn't stop the host from
		// spawning, but it does mean that this distro can't fall back to other
		// AZs, which reduces resilience during times of low instance capacity.
		// Ideally this should not consistently error since it likely indicates
		// a configuration problem (e.g. AWS permissions issues).
		grip.Warning(ctx, message.WrapError(err, message.Fields{
			"message":          "could not determine which subnets are available to the host, so it can only use the default distro subnet (and AZ)",
			"host_id":          h.Id,
			"host_provider":    h.Distro.Provider,
			"distro":           h.Distro.Id,
			"provider_account": m.account,
			"instance_type":    ec2Settings.InstanceType,
			"region":           ec2Settings.getRegion(),
		}))
		return nil, nil
	}

	if len(supportingSubnets) == 0 || (len(supportingSubnets) == 1 && supportingSubnets[0].SubnetID == ec2Settings.SubnetId) {
		// No other subnets are available, so don't bother with making explicit
		// overrides.
		return nil, nil
	}

	overrides := make([]types.FleetLaunchTemplateOverridesRequest, 0, len(supportingSubnets))
	for _, subnet := range supportingSubnets {
		overrides = append(overrides, types.FleetLaunchTemplateOverridesRequest{SubnetId: aws.String(subnet.SubnetID)})
	}

	return overrides, nil
}
