package ec2

import (
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/hostutil"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/ec2"
	"github.com/mitchellh/mapstructure"
	"time"
)

// EC2Manager implements the CloudManager interface for Amazon EC2
type EC2Manager struct {
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
)

type EC2ProviderSettings struct {
	AMI           string       `mapstructure:"ami" json:"ami,omitempty" bson:"ami,omitempty"`
	InstanceType  string       `mapstructure:"instance_type" json:"instance_type,omitempty" bson:"instance_type,omitempty"`
	SecurityGroup string       `mapstructure:"security_group" json:"security_group,omitempty" bson:"security_group,omitempty"`
	KeyName       string       `mapstructure:"key_name" json:"key_name,omitempty" bson:"key_name,omitempty"`
	MountPoints   []MountPoint `mapstructure:"mount_points" json:"mount_points,omitempty" bson:"mount_points,omitempty"`
}

func (self *EC2ProviderSettings) Validate() error {
	if self.AMI == "" {
		return fmt.Errorf("AMI must not be blank")
	}

	if self.InstanceType == "" {
		return fmt.Errorf("Instance size must not be blank")
	}

	if self.SecurityGroup == "" {
		return fmt.Errorf("Security group must not be blank")
	}

	if self.KeyName == "" {
		return fmt.Errorf("Key name must not be blank")
	}

	_, err := makeBlockDeviceMappings(self.MountPoints)
	if err != nil {
		return err
	}

	return nil
}

//Configure loads necessary credentials or other settings from the global config
//object.
func (cloudManager *EC2Manager) Configure(mciSettings *evergreen.MCISettings) error {
	if mciSettings.Providers.AWS.Id == "" || mciSettings.Providers.AWS.Secret == "" {
		return fmt.Errorf("AWS ID/Secret must not be blank")
	}

	cloudManager.awsCredentials = &aws.Auth{
		AccessKey: mciSettings.Providers.AWS.Id,
		SecretKey: mciSettings.Providers.AWS.Secret,
	}
	return nil
}

func (cloudManager *EC2Manager) GetSSHOptions(h *host.Host, keyPath string) ([]string, error) {
	return getEC2KeyOptions(h, keyPath)
}

func (cloudManager *EC2Manager) IsSSHReachable(host *host.Host, keyPath string) (bool, error) {
	sshOpts, err := cloudManager.GetSSHOptions(host, keyPath)
	if err != nil {
		return false, err
	}
	return hostutil.CheckSSHResponse(host, sshOpts)
}

func (cloudManager *EC2Manager) GetInstanceStatus(host *host.Host) (cloud.CloudStatus, error) {
	ec2Handle := getUSEast(*cloudManager.awsCredentials)
	instanceInfo, err := getInstanceInfo(ec2Handle, host.Id)
	if err != nil {
		return cloud.StatusUnknown, err
	}
	return ec2StatusToMCIStatus(instanceInfo.State.Name), nil
}

func (cloudManager *EC2Manager) CanSpawn() (bool, error) {
	return true, nil
}

func (_ *EC2Manager) GetSettings() cloud.ProviderSettings {
	return &EC2ProviderSettings{}
}

func (cloudManager *EC2Manager) SpawnInstance(d *distro.Distro, owner string, userHost bool) (*host.Host, error) {
	if d.Provider != OnDemandProviderName {
		return nil, fmt.Errorf("Can't spawn instance of %v for distro %v: provider is %v", OnDemandProviderName, d.Id, d.Provider)
	}
	ec2Handle := getUSEast(*cloudManager.awsCredentials)

	//Decode and validate the ProviderSettings into the ec2-specific ones.
	ec2Settings := &EC2ProviderSettings{}
	if err := mapstructure.Decode(d.ProviderSettings, ec2Settings); err != nil {
		return nil, fmt.Errorf("Error decoding params for distro %v: %v", d.Id, err)
	}

	if err := ec2Settings.Validate(); err != nil {
		return nil, fmt.Errorf("Invalid EC2 settings in distro %#v: %v and %#v", d, err, ec2Settings)
	}

	blockDevices, err := makeBlockDeviceMappings(ec2Settings.MountPoints)
	if err != nil {
		return nil, err
	}

	instanceName := generateName(d.Id)

	// proactively write all possible information pertaining
	// to the host we want to create. this way, if we are unable
	// to start it or record its instance id, we have a way of knowing
	// something went wrong - and what
	intentHost := &host.Host{
		Id:               instanceName,
		User:             d.User,
		Distro:           *d,
		Tag:              instanceName,
		CreationTime:     time.Now(),
		Status:           evergreen.HostUninitialized,
		TerminationTime:  model.ZeroTime,
		TaskDispatchTime: model.ZeroTime,
		Provider:         evergreen.HostTypeEC2,
		InstanceType:     ec2Settings.InstanceType,
		StartedBy:        owner,
		UserHost:         userHost,
	}

	// record this 'intent host'
	if err := intentHost.Insert(); err != nil {
		return nil, evergreen.Logger.Errorf(slogger.ERROR, "Could not insert intent "+
			"host “%v”: %v", intentHost.Id, err)
	}

	evergreen.Logger.Logf(slogger.DEBUG, "Successfully inserted intent host “%v” "+
		"for distro “%v” to signal cloud instance spawn intent", instanceName,
		d.Id)

	options := ec2.RunInstances{
		MinCount:       1,
		MaxCount:       1,
		ImageId:        ec2Settings.AMI,
		KeyName:        ec2Settings.KeyName,
		InstanceType:   ec2Settings.InstanceType,
		SecurityGroups: ec2.SecurityGroupNames(ec2Settings.SecurityGroup),
		BlockDevices:   blockDevices,
	}

	// start the instance - starting an instance does not mean you can connect
	// to it immediately you have to use GetInstanceStatus below to ensure that
	// it's actually running
	newHost, resp, err := startEC2Instance(ec2Handle, &options, intentHost)

	if err != nil {
		return nil, evergreen.Logger.Errorf(slogger.ERROR, "Could not start new "+
			"instance for distro “%v”. Accompanying host record is “%v”: %v",
			d.Id, intentHost.Id, err)
	}

	instance := resp.Instances[0]

	// create some tags based on user, hostname, owner, time, etc.
	tags := makeTags(intentHost)

	// attach the tags to this instance
	err = attachTags(ec2Handle, tags, instance.InstanceId)

	if err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "Unable to attach tags for %v: %v",
			instance.InstanceId, err)
	} else {
		evergreen.Logger.Logf(slogger.DEBUG, "Attached tag name “%v” for “%v”",
			instanceName, instance.InstanceId)
	}
	return newHost, nil
}

func (cloudManager *EC2Manager) IsUp(host *host.Host) (bool, error) {
	ec2Handle := getUSEast(*cloudManager.awsCredentials)
	instanceInfo, err := getInstanceInfo(ec2Handle, host.Id)
	if err != nil {
		return false, err
	}
	if instanceInfo.State.Name == EC2StatusRunning {
		return true, nil
	}
	return false, nil
}

func (cloudManager *EC2Manager) OnUp(host *host.Host) error {
	//Not currently needed since we can set the tags immediately
	return nil
}

func (cloudManager *EC2Manager) GetDNSName(host *host.Host) (string, error) {
	ec2Handle := getUSEast(*cloudManager.awsCredentials)
	instanceInfo, err := getInstanceInfo(ec2Handle, host.Id)
	if err != nil {
		return "", err
	}
	return instanceInfo.DNSName, nil
}

func (cloudManager *EC2Manager) StopInstance(host *host.Host) error {
	ec2Handle := getUSEast(*cloudManager.awsCredentials)
	// stop the instance
	resp, err := ec2Handle.StopInstances(host.Id)

	if err != nil {
		return err
	}

	for _, stateChange := range resp.StateChanges {
		evergreen.Logger.Logf(slogger.INFO, "Stopped %v", stateChange.InstanceId)
	}

	err = host.ClearRunningTask()

	if err != nil {
		return err
	}

	return nil
}

func (cloudManager *EC2Manager) TerminateInstance(host *host.Host) error {
	// terminate the instance
	if host.Status == evergreen.HostTerminated {
		errMsg := fmt.Errorf("Can not terminate %v - already marked as "+
			"terminated!", host.Id)
		evergreen.Logger.Errorf(slogger.ERROR, errMsg.Error())
		return errMsg
	}

	ec2Handle := getUSEast(*cloudManager.awsCredentials)
	resp, err := ec2Handle.TerminateInstances([]string{host.Id})

	if err != nil {
		return err
	}

	for _, stateChange := range resp.StateChanges {
		evergreen.Logger.Logf(slogger.INFO, "Terminated %v", stateChange.InstanceId)
	}

	// set the host status as terminated and update its termination time
	return host.Terminate()
}

// determine how long until a payment is due for the host
func (cloudManager *EC2Manager) TimeTilNextPayment(host *host.Host) time.Duration {
	return timeTilNextEC2Payment(host)
}

func startEC2Instance(ec2Handle *ec2.EC2, options *ec2.RunInstances,
	intentHost *host.Host) (*host.Host, *ec2.RunInstancesResp, error) {
	// start the instance
	resp, err := ec2Handle.RunInstances(options)

	if err != nil {
		// remove the intent host document
		rmErr := intentHost.Remove()
		if rmErr != nil {
			evergreen.Logger.Errorf(slogger.ERROR, "Could not remove intent host "+
				"“%v”: %v", intentHost.Id, rmErr)
		}
		return nil, nil, evergreen.Logger.Errorf(slogger.ERROR,
			"EC2 RunInstances API call returned error: %v", err)
	}

	evergreen.Logger.Logf(slogger.DEBUG, "Spawned %v instance", len(resp.Instances))

	// the instance should have been successfully spawned
	instance := resp.Instances[0]
	evergreen.Logger.Logf(slogger.DEBUG, "Started %v", instance.InstanceId)
	evergreen.Logger.Logf(slogger.DEBUG, "Key name: %v", string(options.KeyName))

	// find old intent host
	host, err := host.FindOne(host.ById(intentHost.Id))
	if host == nil {
		return nil, nil, evergreen.Logger.Errorf(slogger.ERROR, "Can't locate "+
			"record inserted for intended host “%v”", intentHost.Id)
	}
	if err != nil {
		return nil, nil, evergreen.Logger.Errorf(slogger.ERROR, "Can't locate "+
			"record inserted for intended host “%v” due to error: %v",
			intentHost.Id, err)
	}

	// we found the old document now we can insert the new one
	host.Id = instance.InstanceId
	err = host.Insert()
	if err != nil {
		return nil, nil, evergreen.Logger.Errorf(slogger.ERROR, "Could not insert "+
			"updated host information for “%v” with “%v”: %v", intentHost.Id,
			host.Id, err)
	}

	// remove the intent host document
	err = intentHost.Remove()
	if err != nil {
		return nil, nil, evergreen.Logger.Errorf(slogger.ERROR, "Could not remove "+
			"insert host “%v” (replaced by “%v”): %v", intentHost.Id, host.Id,
			err)
	}

	var infoResp *ec2.InstancesResp
	instanceInfoRetryCount := 0
	instanceInfoMaxRetries := 5

	for {
		infoResp, err = ec2Handle.Instances([]string{instance.InstanceId}, nil)
		if err != nil {
			instanceInfoRetryCount++
			if instanceInfoRetryCount == instanceInfoMaxRetries {
				evergreen.Logger.Errorf(slogger.ERROR, "There was an error querying for the "+
					"instance's information and retries are exhausted. The insance may "+
					"be up.")
				return nil, resp, err
			}

			evergreen.Logger.Errorf(slogger.DEBUG, "There was an error querying for the "+
				"instance's information. Retrying in 30 seconds. Error: %v", err)
			time.Sleep(30 * time.Second)
			continue
		}
		break
	}

	reservations := infoResp.Reservations
	if len(reservations) < 1 {
		return nil, resp, fmt.Errorf("Reservation was returned as nil, you " +
			"may have to check manually")
	}

	instancesInfo := reservations[0].Instances
	if len(instancesInfo) < 1 {
		return nil, resp, fmt.Errorf("Reservation appears to have no " +
			"associated instances")
	}
	return host, resp, nil
}
