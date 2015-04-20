package ec2

import (
	"10gen.com/mci"
	"10gen.com/mci/cloud"
	"10gen.com/mci/hostutil"
	"10gen.com/mci/model"
	"10gen.com/mci/model/distro"
	"10gen.com/mci/model/host"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
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
	// ec2 information
	AMI            string      `mapstructure:"ami"`
	InstanceType   string      `mapstructure:"instancetype"`
	SecurityGroups string      `mapstructure:"securitygroups"`
	KeyName        string      `mapstructure:"keyname"`
	MountInfo      []MountInfo `mapstructure:"mountinfo" yaml:"mountinfo"`
}

func (self *EC2ProviderSettings) Validate() error {
	if self.AMI == "" {
		return fmt.Errorf("AMI must not be blank")
	}

	if self.InstanceType == "" {
		return fmt.Errorf("Instance size must not be blank")
	}

	if self.SecurityGroups == "" {
		return fmt.Errorf("Security group must not be blank")
	}

	if self.KeyName == "" {
		return fmt.Errorf("Key name must not be blank")
	}

	_, err := makeBlockDeviceMappings(self.MountInfo)
	if err != nil {
		return err
	}

	return nil
}

//Configure loads necessary credentials or other settings from the global config
//object.
func (cloudManager *EC2Manager) Configure(mciSettings *mci.MCISettings) error {
	if mciSettings.Providers.AWS.Id == "" || mciSettings.Providers.AWS.Secret == "" {
		return fmt.Errorf("AWS ID/Secret must not be blank")
	}

	cloudManager.awsCredentials = &aws.Auth{
		AccessKey: mciSettings.Providers.AWS.Id,
		SecretKey: mciSettings.Providers.AWS.Secret,
	}
	return nil
}

func (cloudManager *EC2Manager) GetSSHOptions(host *host.Host, distro *distro.Distro,
	keyPath string) ([]string, error) {
	return getEC2KeyOptions(keyPath)
}

func (cloudManager *EC2Manager) IsSSHReachable(host *host.Host, distro *distro.Distro, keyPath string) (bool, error) {
	sshOpts, err := cloudManager.GetSSHOptions(host, distro, keyPath)
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

func (cloudManager *EC2Manager) SpawnInstance(distro *distro.Distro, owner string, userHost bool) (*host.Host, error) {
	if distro.Provider != OnDemandProviderName {
		return nil, fmt.Errorf("Can't use EC2 spawn instance on distro '%v'")
	}
	ec2Handle := getUSEast(*cloudManager.awsCredentials)

	//Decode and validate the ProviderSettings into the ec2-specific ones.
	ec2Settings := &EC2ProviderSettings{}
	if err := mapstructure.Decode(distro.ProviderConfig, ec2Settings); err != nil {
		return nil, fmt.Errorf("Error decoding params for distro %v: %v", distro.Name, err)
	}

	if err := ec2Settings.Validate(); err != nil {
		return nil, fmt.Errorf("Invalid EC2 settings in distro %v: %v", distro.Name, err)
	}

	blockDevices, err := makeBlockDeviceMappings(ec2Settings.MountInfo)
	if err != nil {
		return nil, err
	}

	instanceName := generateName(distro.Name)

	// proactively write all possible information pertaining
	// to the host we want to create. this way, if we are unable
	// to start it or record its instance id, we have a way of knowing
	// something went wrong - and what
	intentHost := &host.Host{
		Id:               instanceName,
		User:             distro.User,
		Distro:           distro.Name,
		Tag:              instanceName,
		CreationTime:     time.Now(),
		Status:           mci.HostUninitialized,
		TerminationTime:  model.ZeroTime,
		TaskDispatchTime: model.ZeroTime,
		Provider:         mci.HostTypeEC2,
		InstanceType:     ec2Settings.InstanceType,
		StartedBy:        owner,
		UserHost:         userHost,
	}

	// record this 'intent host'
	if err := intentHost.Insert(); err != nil {
		return nil, mci.Logger.Errorf(slogger.ERROR, "Could not insert intent "+
			"host “%v”: %v", intentHost.Id, err)
	}

	mci.Logger.Logf(slogger.DEBUG, "Successfully inserted intent host “%v” "+
		"for distro “%v” to signal cloud instance spawn intent", instanceName,
		distro.Name)

	options := ec2.RunInstances{
		MinCount:       1,
		MaxCount:       1,
		ImageId:        ec2Settings.AMI,
		KeyName:        ec2Settings.KeyName,
		InstanceType:   ec2Settings.InstanceType,
		SecurityGroups: ec2.SecurityGroupNames(ec2Settings.SecurityGroups),
		BlockDevices:   blockDevices,
	}

	// start the instance - starting an instance does not mean you can connect
	// to it immediately you have to use GetInstanceStatus below to ensure that
	// it's actually running
	newHost, resp, err := startEC2Instance(ec2Handle, &options, intentHost)

	if err != nil {
		return nil, mci.Logger.Errorf(slogger.ERROR, "Could not start new "+
			"instance for distro “%v”. Accompanying host record is “%v”: %v",
			distro.Name, intentHost.Id, err)
	}

	instance := resp.Instances[0]

	// create some tags based on user, hostname, owner, time, etc.
	tags := makeTags(intentHost)

	// attach the tags to this instance
	err = attachTags(ec2Handle, tags, instance.InstanceId)

	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Unable to attach tags for %v: %v",
			instance.InstanceId, err)
	} else {
		mci.Logger.Logf(slogger.DEBUG, "Attached tag name “%v” for “%v”",
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
		mci.Logger.Logf(slogger.INFO, "Stopped %v", stateChange.InstanceId)
	}

	err = host.ClearRunningTask()

	if err != nil {
		return err
	}

	return nil
}

func (cloudManager *EC2Manager) TerminateInstance(host *host.Host) error {
	// terminate the instance
	if host.Status == mci.HostTerminated {
		errMsg := fmt.Errorf("Can not terminate %v - already marked as "+
			"terminated!", host.Id)
		mci.Logger.Errorf(slogger.ERROR, errMsg.Error())
		return errMsg
	}

	ec2Handle := getUSEast(*cloudManager.awsCredentials)
	resp, err := ec2Handle.TerminateInstances([]string{host.Id})

	if err != nil {
		return err
	}

	for _, stateChange := range resp.StateChanges {
		mci.Logger.Logf(slogger.INFO, "Terminated %v", stateChange.InstanceId)
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
			mci.Logger.Errorf(slogger.ERROR, "Could not remove intent host "+
				"“%v”: %v", intentHost.Id, rmErr)
		}
		return nil, nil, mci.Logger.Errorf(slogger.ERROR,
			"EC2 RunInstances API call returned error: %v", err)
	}

	mci.Logger.Logf(slogger.DEBUG, "Spawned %v instance", len(resp.Instances))

	// the instance should have been successfully spawned
	instance := resp.Instances[0]
	mci.Logger.Logf(slogger.DEBUG, "Started %v", instance.InstanceId)
	mci.Logger.Logf(slogger.DEBUG, "Key name: %v", string(options.KeyName))

	// find old intent host
	host, err := host.FindOne(host.ById(intentHost.Id))
	if host == nil {
		return nil, nil, mci.Logger.Errorf(slogger.ERROR, "Can't locate "+
			"record inserted for intended host “%v”", intentHost.Id)
	}
	if err != nil {
		return nil, nil, mci.Logger.Errorf(slogger.ERROR, "Can't locate "+
			"record inserted for intended host “%v” due to error: %v",
			intentHost.Id, err)
	}

	// we found the old document now we can insert the new one
	host.Id = instance.InstanceId
	err = host.Insert()
	if err != nil {
		return nil, nil, mci.Logger.Errorf(slogger.ERROR, "Could not insert "+
			"updated host information for “%v” with “%v”: %v", intentHost.Id,
			host.Id, err)
	}

	// remove the intent host document
	err = intentHost.Remove()
	if err != nil {
		return nil, nil, mci.Logger.Errorf(slogger.ERROR, "Could not remove "+
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
				mci.Logger.Errorf(slogger.ERROR, "There was an error querying for the "+
					"instance's information and retries are exhausted. The insance may "+
					"be up.")
				return nil, resp, err
			}

			mci.Logger.Errorf(slogger.DEBUG, "There was an error querying for the "+
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
