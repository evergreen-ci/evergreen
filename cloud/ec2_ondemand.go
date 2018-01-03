package cloud

import (
	"context"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/hostutil"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/goamz/goamz/aws"
	"github.com/goamz/goamz/ec2"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// ec2OnDemandManager implements the CloudManager interface for Amazon EC2
type ec2OnDemandManager struct {
	awsCredentials *aws.Auth
}

//Valid values for EC2 instance states:
//pending | running | shutting-down | terminated | stopping | stopped
//see http://goo.gl/3OrCGn
const (
	EC2StatusPending      = "pending"
	EC2StatusRunning      = "running"
	EC2StatusShuttingdown = "shutting-down"
	EC2StatusTerminated   = "terminated"
	EC2StatusStopped      = "stopped"
	EC2ErrorNotFound      = "InvalidInstanceID.NotFound"
)

type EC2ProviderSettings struct {
	AMI          string       `mapstructure:"ami" json:"ami,omitempty" bson:"ami,omitempty"`
	InstanceType string       `mapstructure:"instance_type" json:"instance_type,omitempty" bson:"instance_type,omitempty"`
	KeyName      string       `mapstructure:"key_name" json:"key_name,omitempty" bson:"key_name,omitempty"`
	MountPoints  []MountPoint `mapstructure:"mount_points" json:"mount_points,omitempty" bson:"mount_points,omitempty"`

	// this is the security group name in EC2 classic and the security group ID in VPC (eg. sg-xxxx)
	SecurityGroup string `mapstructure:"security_group" json:"security_group,omitempty" bson:"security_group,omitempty"`
	// only set in VPC (eg. subnet-xxxx)
	SubnetId string `mapstructure:"subnet_id" json:"subnet_id,omitempty" bson:"subnet_id,omitempty"`
	// this is set to true if the security group is part of a vpc
	IsVpc bool `mapstructure:"is_vpc" json:"is_vpc,omitempty" bson:"is_vpc,omitempty"`
}

func (self *EC2ProviderSettings) Validate() error {
	if self.AMI == "" {
		return errors.New("AMI must not be blank")
	}

	if self.InstanceType == "" {
		return errors.New("Instance size must not be blank")
	}

	if self.SecurityGroup == "" {
		return errors.New("Security group must not be blank")
	}

	if self.KeyName == "" {
		return errors.New("Key name must not be blank")
	}

	_, err := makeBlockDeviceMappings(self.MountPoints)

	return errors.WithStack(err)
}

//Configure loads necessary credentials or other settings from the global config
//object.
func (cloudManager *ec2OnDemandManager) Configure(settings *evergreen.Settings) error {
	if settings.Providers.AWS.Id == "" || settings.Providers.AWS.Secret == "" {
		return errors.New("AWS ID/Secret must not be blank")
	}

	cloudManager.awsCredentials = &aws.Auth{
		AccessKey: settings.Providers.AWS.Id,
		SecretKey: settings.Providers.AWS.Secret,
	}
	return nil
}

func (cloudManager *ec2OnDemandManager) GetSSHOptions(h *host.Host, keyPath string) ([]string, error) {
	return getEC2KeyOptions(h, keyPath)
}

func (cloudManager *ec2OnDemandManager) IsSSHReachable(host *host.Host, keyPath string) (bool, error) {
	sshOpts, err := cloudManager.GetSSHOptions(host, keyPath)
	if err != nil {
		return false, err
	}
	return hostutil.CheckSSHResponse(context.TODO(), host, sshOpts)
}

func (cloudManager *ec2OnDemandManager) GetInstanceStatus(host *host.Host) (CloudStatus, error) {
	ec2Handle, client := getUSEast(*cloudManager.awsCredentials)
	defer util.PutHttpClient(client)
	instanceInfo, err := GetInstanceInfo(ec2Handle, host.Id)
	if err != nil {
		return StatusUnknown, err
	}
	return ec2StatusToEvergreenStatus(instanceInfo.State.Name), nil
}

func (cloudManager *ec2OnDemandManager) CanSpawn() (bool, error) {
	return true, nil
}

func (*ec2OnDemandManager) GetSettings() ProviderSettings {
	return &EC2ProviderSettings{}
}

//GetInstanceName returns a name to be used for an instance
func (*ec2OnDemandManager) GetInstanceName(d *distro.Distro) string {
	return d.GenerateName()
}

//SpawnHost will spawn an on-demand EC2 host
func (cloudManager *ec2OnDemandManager) SpawnHost(h *host.Host) (*host.Host, error) {
	if h.Distro.Provider != evergreen.ProviderNameEc2OnDemand {
		return nil, errors.Errorf("Can't spawn instance of %v for distro %v: provider is %v",
			evergreen.ProviderNameEc2OnDemand, h.Distro.Id, h.Distro.Provider)
	}

	ec2Handle, client := getUSEast(*cloudManager.awsCredentials)
	defer util.PutHttpClient(client)

	//Decode and validate the ProviderSettings into the ec2-specific ones.
	ec2Settings := &EC2ProviderSettings{}
	if err := mapstructure.Decode(h.Distro.ProviderSettings, ec2Settings); err != nil {
		return nil, errors.Wrapf(err, "Error decoding params for distro %v: %v", h.Distro.Id)
	}

	if err := ec2Settings.Validate(); err != nil {
		return nil, errors.Wrapf(err, "Invalid EC2 settings in distro %#v: and %#v", h.Distro, ec2Settings)
	}

	blockDevices, err := makeBlockDeviceMappings(ec2Settings.MountPoints)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// the document is updated later in hostinit, rather than here
	h.InstanceType = ec2Settings.InstanceType

	options := ec2.RunInstancesOptions{
		MinCount:       1,
		MaxCount:       1,
		ImageId:        ec2Settings.AMI,
		KeyName:        ec2Settings.KeyName,
		InstanceType:   ec2Settings.InstanceType,
		SecurityGroups: ec2.SecurityGroupNames(ec2Settings.SecurityGroup),
		BlockDevices:   blockDevices,
	}

	// if it's a Vpc override the options to be the correct VPC settings.
	if ec2Settings.IsVpc {
		options.SecurityGroups = ec2.SecurityGroupIds(ec2Settings.SecurityGroup)
		options.AssociatePublicIpAddress = true
		options.SubnetId = ec2Settings.SubnetId
	}

	// start the instance - starting an instance does not mean you can connect
	// to it immediately you have to use GetInstanceStatus to ensure that
	// it's actually running
	newHost, resp, err := startEC2Instance(ec2Handle, &options, h)
	grip.Debugf("id=%s, intentHost=%s, starResp=%+v, newHost=%+v",
		h.Id, h.Id, resp, newHost)

	if err != nil {
		err = errors.Wrapf(err, "could not start new instance for distro '%v.'"+
			"Accompanying host record is '%v'", h.Distro.Id, h.Id)
		grip.Error(err)
		return nil, err
	}

	instance := resp.Instances[0]
	grip.Debug(message.Fields{
		"instance": h.Id,
		"object":   instance,
	})

	// create some tags based on user, hostname, owner, time, etc.
	tags := makeTags(h)

	resources := []string{
		instance.InstanceId,
	}

	for _, vol := range instance.BlockDevices {
		if vol.DeviceName == "" {
			continue
		}

		resources = append(resources, vol.EBS.VolumeId)
	}

	// attach the tags to this instance
	if err = attachTagsToResources(ec2Handle, tags, resources); err != nil {
		err = errors.Wrapf(err, "unable to attach tags for %s", instance.InstanceId)
		grip.Error(err)
		return nil, err
	}

	grip.Debugf("attached tags for '%s'", instance.InstanceId)
	event.LogHostStarted(newHost.Id)

	return newHost, nil
}

func (cloudManager *ec2OnDemandManager) IsUp(host *host.Host) (bool, error) {
	ec2Handle, client := getUSEast(*cloudManager.awsCredentials)
	defer util.PutHttpClient(client)

	instanceInfo, err := GetInstanceInfo(ec2Handle, host.Id)
	if err != nil {
		return false, errors.WithStack(err)
	}
	if instanceInfo.State.Name == EC2StatusRunning {
		return true, nil
	}
	return false, nil
}

func (cloudManager *ec2OnDemandManager) OnUp(host *host.Host) error {
	//Not currently needed since we can set the tags immediately
	return nil
}

func (cloudManager *ec2OnDemandManager) GetDNSName(host *host.Host) (string, error) {
	ec2Handle, client := getUSEast(*cloudManager.awsCredentials)
	defer util.PutHttpClient(client)

	instanceInfo, err := GetInstanceInfo(ec2Handle, host.Id)
	if err != nil {
		return "", err
	}
	return instanceInfo.DNSName, nil
}

func (cloudManager *ec2OnDemandManager) TerminateInstance(host *host.Host) error {
	// terminate the instance
	if host.Status == evergreen.HostTerminated {
		err := errors.Errorf("Can not terminate %v - already marked as "+
			"terminated!", host.Id)
		grip.Error(err)
		return err
	}

	ec2Handle, client := getUSEast(*cloudManager.awsCredentials)
	defer util.PutHttpClient(client)

	resp, err := ec2Handle.TerminateInstances([]string{host.Id})
	if err != nil {
		return err
	}

	for _, stateChange := range resp.StateChanges {
		grip.Infoln("Terminated", stateChange.InstanceId)
	}

	// set the host status as terminated and update its termination time
	return host.Terminate()
}

// determine how long until a payment is due for the host
func (cloudManager *ec2OnDemandManager) TimeTilNextPayment(host *host.Host) time.Duration {
	return timeTilNextEC2Payment(host)
}

func startEC2Instance(ec2Handle *ec2.EC2, options *ec2.RunInstancesOptions,
	intentHost *host.Host) (*host.Host, *ec2.RunInstancesResp, error) {
	// start the instance
	resp, err := ec2Handle.RunInstances(options)

	if err != nil {
		// remove the intent host document
		rmErr := intentHost.Remove()
		if rmErr != nil {
			grip.Errorf("Could not remove intent host '%s': %+v", intentHost.Id, rmErr)
		}

		err = errors.Wrap(err, "EC2 RunInstances API call returned error")
		grip.Error(err)
		return nil, nil, err

	}

	grip.Debugf("Spawned %d instance", len(resp.Instances))

	// the instance should have been successfully spawned
	instance := resp.Instances[0]
	grip.Debugln("Started", instance.InstanceId)
	grip.Debugln("Key name:", options.KeyName)

	// update this host's ID
	intentHost.Id = instance.InstanceId

	var infoResp *ec2.DescribeInstancesResp
	instanceInfoRetryCount := 0
	instanceInfoMaxRetries := 5
	for {
		infoResp, err = ec2Handle.DescribeInstances([]string{instance.InstanceId}, nil)
		if err != nil {
			instanceInfoRetryCount++
			if instanceInfoRetryCount == instanceInfoMaxRetries {
				err = errors.WithStack(err)
				grip.Error(message.Fields{
					"message": "There was an error querying for the instance's " +
						"information and retries are exhausted. The instance may be up.",
					"error":    err,
					"instance": instance.InstanceId,
				})
				return nil, resp, err
			}
			grip.Debugf("There was an error querying for the instance's information. "+
				"Retrying in 30 seconds. Error: %v", err)
			time.Sleep(30 * time.Second)
			continue
		}
		break
	}

	reservations := infoResp.Reservations
	if len(reservations) < 1 {
		return nil, resp, errors.New("Reservation was returned as nil, you " +
			"may have to check manually")
	}

	instancesInfo := reservations[0].Instances
	if len(instancesInfo) < 1 {
		return nil, resp, errors.New("Reservation appears to have no " +
			"associated instances")
	}
	return intentHost, resp, nil
}

// CostForDuration returns the cost of running a host between the given start and end times
func (cloudManager *ec2OnDemandManager) CostForDuration(h *host.Host, start, end time.Time) (float64, error) {
	// sanity check
	if end.Before(start) || util.IsZeroTime(start) || util.IsZeroTime(end) {
		return 0, errors.New("task timing data is malformed")
	}
	// grab instance details from EC2
	ec2Handle, client := getUSEast(*cloudManager.awsCredentials)
	defer util.PutHttpClient(client)

	instance, err := GetInstanceInfo(ec2Handle, h.Id)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	os := osLinux
	if strings.Contains(h.Distro.Arch, "windows") {
		os = osWindows
	}
	dur := end.Sub(start)
	region := azToRegion(instance.AvailabilityZone)
	iType := instance.InstanceType

	ebsCost, err := blockDeviceCosts(ec2Handle, instance.BlockDevices, dur)
	if err != nil {
		return 0, errors.Wrap(err, "calculating block device costs")
	}
	hostCost, err := onDemandCost(&pkgOnDemandPriceFetcher, os, iType, region, dur)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	return hostCost + ebsCost, nil
}
