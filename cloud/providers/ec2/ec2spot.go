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
	"github.com/goamz/goamz/aws"
	"github.com/goamz/goamz/ec2"
	"github.com/mitchellh/mapstructure"
	"time"
)

const (
	SpotStatusOpen      = "open"
	SpotStatusActive    = "active"
	SpotStatusClosed    = "closed"
	SpotStatusCancelled = "cancelled"
	SpotStatusFailed    = "failed"

	EC2ErrorSpotRequestNotFound = "InvalidSpotInstanceRequestID.NotFound"
)

// EC2SpotManager implements the CloudManager interface for Amazon EC2 Spot
type EC2SpotManager struct {
	awsCredentials *aws.Auth
}

type EC2SpotSettings struct {
	AMI           string       `mapstructure:"ami" json:"ami,omitempty" bson:"ami,omitempty"`
	InstanceType  string       `mapstructure:"instance_type" json:"instance_type,omitempty" bson:"instance_type,omitempty"`
	SecurityGroup string       `mapstructure:"security_group" json:"security_group,omitempty" bson:"security_group,omitempty"`
	KeyName       string       `mapstructure:"key_name" json:"key_name,omitempty" bson:"key_name,omitempty"`
	MountPoints   []MountPoint `mapstructure:"mount_points" json:"mount_points,omitempty" bson:"mount_points,omitempty"`
	BidPrice      float64      `mapstructure:"bid_price" json:"bid_price,omitempty" bson:"bid_price,omitempty"`
}

func (self *EC2SpotSettings) Validate() error {
	if self.BidPrice <= 0 {
		return fmt.Errorf("Bid price must be greater than zero")
	}

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
func (cloudManager *EC2SpotManager) Configure(settings *evergreen.Settings) error {
	if settings.Providers.AWS.Id == "" || settings.Providers.AWS.Secret == "" {
		return fmt.Errorf("AWS ID/Secret must not be blank")
	}
	cloudManager.awsCredentials = &aws.Auth{
		AccessKey: settings.Providers.AWS.Id,
		SecretKey: settings.Providers.AWS.Secret,
	}
	return nil
}

func (_ *EC2SpotManager) GetSettings() cloud.ProviderSettings {
	return &EC2SpotSettings{}
}

// determine how long until a payment is due for the host
func (cloudManager *EC2SpotManager) TimeTilNextPayment(host *host.Host) time.Duration {
	return timeTilNextEC2Payment(host)
}

func (cloudManager *EC2SpotManager) GetSSHOptions(h *host.Host, keyPath string) ([]string, error) {
	return getEC2KeyOptions(h, keyPath)
}

func (cloudManager *EC2SpotManager) IsUp(host *host.Host) (bool, error) {
	instanceStatus, err := cloudManager.GetInstanceStatus(host)
	if err != nil {
		return false, evergreen.Logger.Errorf(slogger.ERROR,
			"Failed to check if host %v is up: %v", host.Id, err)
	}

	if instanceStatus == cloud.StatusRunning {
		return true, nil
	} else {
		return false, nil
	}
}

func (cloudManager *EC2SpotManager) OnUp(host *host.Host) error {
	tags := makeTags(host)
	tags["spot"] = "true" // mark this as a spot instance
	spotReq, err := cloudManager.describeSpotRequest(host.Id)
	if err != nil {
		return err
	}
	if spotReq.InstanceId == "" {
		return evergreen.Logger.Errorf(slogger.ERROR, "Could not retrieve instanceID for filled SpotRequest '%v'",
			host.Id)
	}
	return attachTags(getUSEast(*cloudManager.awsCredentials), tags, spotReq.InstanceId)
}

func (cloudManager *EC2SpotManager) IsSSHReachable(host *host.Host, keyPath string) (bool, error) {
	sshOpts, err := cloudManager.GetSSHOptions(host, keyPath)
	if err != nil {
		return false, err
	}
	return hostutil.CheckSSHResponse(host, sshOpts)
}

//GetInstanceStatus returns an mci-universal status code for the status of
//an ec2 spot-instance host. For unfulfilled spot requests, the behavior
//is as follows:
// Spot request open or active, but unfulfilled -> StatusPending
// Spot request closed or cancelled             -> StatusTerminated
// Spot request failed due to bidding/capacity  -> StatusFailed
//
// For a *fulfilled* spot request (the spot request has an instance ID)
// the status returned will be the status of the instance that fulfilled it,
// matching the behavior used in cloud/providers/ec2/ec2.go
func (cloudManager *EC2SpotManager) GetInstanceStatus(host *host.Host) (cloud.CloudStatus, error) {
	spotDetails, err := cloudManager.describeSpotRequest(host.Id)
	if err != nil {
		return cloud.StatusUnknown, evergreen.Logger.Errorf(slogger.ERROR,
			"failed to get spot request info for %v: %v", host.Id, err)
	}

	//Spot request has been fulfilled, so get status of the instance itself
	if spotDetails.InstanceId != "" {
		ec2Handle := getUSEast(*cloudManager.awsCredentials)
		instanceInfo, err := getInstanceInfo(ec2Handle, spotDetails.InstanceId)
		if err != nil {
			evergreen.Logger.Logf(slogger.ERROR, "Got an error checking spot details %v", err)
			return cloud.StatusUnknown, err
		}
		return ec2StatusToEvergreenStatus(instanceInfo.State.Name), nil
	}

	//Spot request is not fulfilled. Either it's failed/closed for some reason,
	//or still pending evaluation
	switch spotDetails.State {
	case SpotStatusOpen:
		return cloud.StatusPending, nil
	case SpotStatusActive:
		return cloud.StatusPending, nil
	case SpotStatusClosed:
		return cloud.StatusTerminated, nil
	case SpotStatusCancelled:
		return cloud.StatusTerminated, nil
	case SpotStatusFailed:
		return cloud.StatusFailed, nil
	default:
		evergreen.Logger.Logf(slogger.ERROR, "Unexpected status code in spot req: %v", spotDetails.State)
		return cloud.StatusUnknown, nil
	}
}

func (cloudManager *EC2SpotManager) CanSpawn() (bool, error) {
	return true, nil
}

func (cloudManager *EC2SpotManager) GetDNSName(host *host.Host) (string, error) {
	spotDetails, err := cloudManager.describeSpotRequest(host.Id)
	if err != nil {
		return "", evergreen.Logger.Errorf(slogger.ERROR, "failed to get spot request info for %v: %v", host.Id, err)
	}

	//Spot request has not been fulfilled yet, so there is still no DNS name
	if spotDetails.InstanceId == "" {
		return "", nil
	}

	//Spot request is fulfilled, find the instance info and get DNS info
	ec2Handle := getUSEast(*cloudManager.awsCredentials)
	instanceInfo, err := getInstanceInfo(ec2Handle, spotDetails.InstanceId)
	if err != nil {
		return "", err
	}
	return instanceInfo.DNSName, nil
}

func (cloudManager *EC2SpotManager) SpawnInstance(d *distro.Distro, owner string, userHost bool) (*host.Host, error) {
	if d.Provider != SpotProviderName {
		return nil, fmt.Errorf("Can't spawn instance of %v for distro %v: provider is %v", SpotProviderName, d.Id, d.Provider)
	}
	ec2Handle := getUSEast(*cloudManager.awsCredentials)

	//Decode and validate the ProviderSettings into the ec2-specific ones.
	ec2Settings := &EC2SpotSettings{}
	if err := mapstructure.Decode(d.ProviderSettings, ec2Settings); err != nil {
		return nil, fmt.Errorf("Error decoding params for distro %v: %v", d.Id, err)
	}

	if err := ec2Settings.Validate(); err != nil {
		return nil, fmt.Errorf("Invalid EC2 spot settings in distro %v: %v", d.Id, err)
	}

	blockDevices, err := makeBlockDeviceMappings(ec2Settings.MountPoints)
	if err != nil {
		return nil, err
	}

	instanceName := generateName(d.Id)
	intentHost := &host.Host{
		Id:               instanceName,
		User:             d.User,
		Distro:           *d,
		Tag:              instanceName,
		CreationTime:     time.Now(),
		Status:           evergreen.HostUninitialized,
		TerminationTime:  model.ZeroTime,
		TaskDispatchTime: model.ZeroTime,
		Provider:         SpotProviderName,
		InstanceType:     ec2Settings.InstanceType,
		RunningTask:      "",
		Provisioned:      false,
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

	spotRequest := &ec2.RequestSpotInstances{
		SpotPrice:      fmt.Sprintf("%v", ec2Settings.BidPrice),
		InstanceCount:  1,
		ImageId:        ec2Settings.AMI,
		KeyName:        ec2Settings.KeyName,
		InstanceType:   ec2Settings.InstanceType,
		SecurityGroups: ec2.SecurityGroupNames(ec2Settings.SecurityGroup),
		BlockDevices:   blockDevices,
	}

	spotResp, err := ec2Handle.RequestSpotInstances(spotRequest)
	if err != nil {
		//Remove the intent host if the API call failed
		if err := intentHost.Remove(); err != nil {
			evergreen.Logger.Logf(slogger.ERROR, "Failed to remove intent host %v: %v", intentHost.Id, err)
		}
		return nil, evergreen.Logger.Errorf(slogger.ERROR, "Failed starting spot instance "+
			" for distro '%v' on intent host %v: %v", d.Id, intentHost.Id, err)
	}

	spotReqRes := spotResp.SpotRequestResults[0]
	if spotReqRes.State != SpotStatusOpen && spotReqRes.State != SpotStatusActive {
		return nil, evergreen.Logger.Errorf(slogger.ERROR, "Spot request %v was found in "+
			" state %v on intent host %v", spotReqRes.SpotRequestId, spotReqRes.State, intentHost.Id)
	}

	intentHost.Id = spotReqRes.SpotRequestId
	err = intentHost.Insert()
	if err != nil {
		return nil, evergreen.Logger.Errorf(slogger.ERROR, "Could not insert updated host info with id %v"+
			" for intent host %v: %v", intentHost.Id, instanceName, err)
	}

	//find the old intent host and remove it, since we now have the real
	//host doc successfully stored.
	oldIntenthost, err := host.FindOne(host.ById(instanceName))
	if err != nil {
		return nil, evergreen.Logger.Errorf(slogger.ERROR, "Can't locate "+
			"record inserted for intended host '%v' due to error: %v",
			instanceName, err)
	}
	if oldIntenthost == nil {
		return nil, evergreen.Logger.Errorf(slogger.ERROR, "Can't locate "+
			"record inserted for intended host '%v'", instanceName)
	}

	err = oldIntenthost.Remove()
	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, "Could not remove intent host "+
			"“%v”: %v", oldIntenthost.Id, err)
		return nil, err
	}

	// create some tags based on user, hostname, owner, time, etc.
	tags := makeTags(intentHost)

	// attach the tags to this instance
	err = attachTags(ec2Handle, tags, intentHost.Id)

	if err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "Unable to attach tags for %v: %v",
			intentHost.Id, err)
	} else {
		evergreen.Logger.Logf(slogger.DEBUG, "Attached tag name “%v” for “%v”",
			instanceName, intentHost.Id)
	}
	return intentHost, nil
}

func (cloudManager *EC2SpotManager) TerminateInstance(host *host.Host) error {
	// terminate the instance
	if host.Status == evergreen.HostTerminated {
		errMsg := fmt.Errorf("Can not terminate %v - already marked as "+
			"terminated!", host.Id)
		evergreen.Logger.Errorf(slogger.ERROR, errMsg.Error())
		return errMsg
	}

	spotDetails, err := cloudManager.describeSpotRequest(host.Id)
	if err != nil {
		ec2err, ok := err.(*ec2.Error)
		if ok && ec2err.Code == EC2ErrorSpotRequestNotFound {
			// EC2 says the spot request is not found - assume this means amazon
			// terminated our spot instance
			evergreen.Logger.Logf(slogger.WARN, "EC2 could not find spot instance '%v', "+
				"marking as terminated", host.Id)
			return host.Terminate()
		}
		return evergreen.Logger.Errorf(slogger.ERROR, "Couldn't terminate, "+
			"failed to get spot request info for %v: %v", host.Id, err)
	}

	evergreen.Logger.Logf(slogger.INFO, "Cancelling spot request %v", host.Id)
	//First cancel the spot request
	ec2Handle := getUSEast(*cloudManager.awsCredentials)
	_, err = ec2Handle.CancelSpotRequests([]string{host.Id})
	if err != nil {
		return evergreen.Logger.Errorf(slogger.ERROR, "Failed to cancel spot request for host %v: %v",
			host.Id, err)
	}

	//Canceling the spot request doesn't terminate the instance that fulfilled it,
	// if it was fulfilled. We need to terminate the instance explicitly
	if spotDetails.InstanceId != "" {
		evergreen.Logger.Logf(slogger.INFO, "Spot request %v cancelled, now terminating instance %v",
			spotDetails.InstanceId, host.Id)
		resp, err := ec2Handle.TerminateInstances([]string{spotDetails.InstanceId})
		if err != nil {
			return evergreen.Logger.Errorf(slogger.INFO, "Failed to terminate host %v: %v", host.Id, err)
		}

		for _, stateChange := range resp.StateChanges {
			evergreen.Logger.Logf(slogger.INFO, "Terminated %v", stateChange.InstanceId)
		}
	} else {
		evergreen.Logger.Logf(slogger.INFO, "Spot request %v cancelled (no instances have fulfilled it)", host.Id)
	}

	// set the host status as terminated and update its termination time
	return host.Terminate()
}

// describeSpotRequest gets infomration about a spot request
// Note that if the SpotRequestResult object returned has a non-blank InstanceId
// field, this indicates that the spot request has been fulfilled.
func (cloudManager *EC2SpotManager) describeSpotRequest(spotReqId string) (*ec2.SpotRequestResult, error) {
	ec2Handle := getUSEast(*cloudManager.awsCredentials)
	resp, err := ec2Handle.DescribeSpotRequests([]string{spotReqId}, nil)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, evergreen.Logger.Errorf(slogger.ERROR, "Received a nil response from EC2 looking up spot request %v",
			spotReqId)
	}
	if len(resp.SpotRequestResults) != 1 {
		return nil, evergreen.Logger.Errorf(slogger.ERROR, "Expected one spot request info, but got %v",
			len(resp.SpotRequestResults))
	}
	return &resp.SpotRequestResults[0], nil
}
