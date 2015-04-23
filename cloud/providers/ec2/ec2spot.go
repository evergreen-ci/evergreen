package ec2

import (
	"10gen.com/mci"
	"10gen.com/mci/cloud"
	"10gen.com/mci/hostutil"
	"10gen.com/mci/model"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/ec2"
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
	// ec2 information
	AMI            string      `mapstructure:"ami"`
	InstanceType   string      `mapstructure:"instancetype"`
	SecurityGroups string      `mapstructure:"securitygroups"`
	KeyName        string      `mapstructure:"keyname"`
	MountInfo      []MountInfo `mapstructure:"mountinfo" yaml:"mountinfo"`
	BidPrice       float64     `mapstructure:"bid_price"`
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
func (cloudManager *EC2SpotManager) Configure(mciSettings *mci.MCISettings) error {
	if mciSettings.Providers.AWS.Id == "" || mciSettings.Providers.AWS.Secret == "" {
		return fmt.Errorf("AWS ID/Secret must not be blank")
	}
	cloudManager.awsCredentials = &aws.Auth{
		AccessKey: mciSettings.Providers.AWS.Id,
		SecretKey: mciSettings.Providers.AWS.Secret,
	}
	return nil
}

func (cloudManager *EC2SpotManager) GetSSHOptions(host *model.Host, distro *model.Distro,
	keyPath string) ([]string, error) {
	return getEC2KeyOptions(keyPath)
}

func (cloudManager *EC2SpotManager) IsUp(host *model.Host) (bool, error) {
	instanceStatus, err := cloudManager.GetInstanceStatus(host)
	if err != nil {
		return false, mci.Logger.Errorf(slogger.ERROR,
			"Failed to check if host %v is up: %v", host.Id, err)
	}

	if instanceStatus == cloud.StatusRunning {
		return true, nil
	} else {
		return false, nil
	}
}

func (cloudManager *EC2SpotManager) OnUp(host *model.Host) error {
	tags := makeTags(host)
	tags["spot"] = "true" // mark this as a spot instance
	spotReq, err := cloudManager.describeSpotRequest(host.Id)
	if err != nil {
		return err
	}
	if spotReq.InstanceId == "" {
		return mci.Logger.Errorf(slogger.ERROR, "Could not retrieve instanceID for filled SpotRequest '%v'",
			host.Id)
	}
	return attachTags(getUSEast(*cloudManager.awsCredentials), tags, spotReq.InstanceId)
}

func (cloudManager *EC2SpotManager) IsSSHReachable(host *model.Host, distro *model.Distro,
	keyPath string) (bool, error) {
	sshOpts, err := cloudManager.GetSSHOptions(host, distro, keyPath)
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
func (cloudManager *EC2SpotManager) GetInstanceStatus(host *model.Host) (cloud.CloudStatus, error) {
	spotDetails, err := cloudManager.describeSpotRequest(host.Id)
	if err != nil {
		return cloud.StatusUnknown, mci.Logger.Errorf(slogger.ERROR,
			"failed to get spot request info for %v: %v", host.Id, err)
	}

	//Spot request has been fulfilled, so get status of the instance itself
	if spotDetails.InstanceId != "" {
		ec2Handle := getUSEast(*cloudManager.awsCredentials)
		instanceInfo, err := getInstanceInfo(ec2Handle, spotDetails.InstanceId)
		if err != nil {
			mci.Logger.Logf(slogger.ERROR, "Got an error checking spot details %v", err)
			return cloud.StatusUnknown, err
		}
		return ec2StatusToMCIStatus(instanceInfo.State.Name), nil
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
		mci.Logger.Logf(slogger.ERROR, "Unexpected status code in spot req: %v", spotDetails.State)
		return cloud.StatusUnknown, nil
	}
}

func (cloudManager *EC2SpotManager) CanSpawn() (bool, error) {
	return true, nil
}

func (cloudManager *EC2SpotManager) GetDNSName(host *model.Host) (string, error) {
	spotDetails, err := cloudManager.describeSpotRequest(host.Id)
	if err != nil {
		return "", mci.Logger.Errorf(slogger.ERROR, "failed to get spot request info for %v: %v", host.Id, err)
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

func (cloudManager *EC2SpotManager) SpawnInstance(distro *model.Distro,
	owner string,
	userHost bool) (*model.Host, error) {
	if distro.Provider != SpotProviderName {
		return nil, fmt.Errorf("Can't use EC2SPOT spawn instance on distro '%v'")
	}
	ec2Handle := getUSEast(*cloudManager.awsCredentials)

	//Decode and validate the ProviderSettings into the ec2-specific ones.
	ec2Settings := &EC2SpotSettings{}
	if err := mapstructure.Decode(distro.ProviderConfig, ec2Settings); err != nil {
		return nil, fmt.Errorf("Error decoding params for distro %v: %v", distro.Name, err)
	}

	if err := ec2Settings.Validate(); err != nil {
		return nil, fmt.Errorf("Invalid EC2 spot settings in distro %v: %v", distro.Name, err)
	}

	blockDevices, err := makeBlockDeviceMappings(ec2Settings.MountInfo)
	if err != nil {
		return nil, err
	}

	instanceName := generateName(distro.Name)
	intentHost := &model.Host{
		Id:               instanceName,
		User:             distro.User,
		Distro:           distro.Name,
		Tag:              instanceName,
		CreationTime:     time.Now(),
		Status:           mci.HostUninitialized,
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
		return nil, mci.Logger.Errorf(slogger.ERROR, "Could not insert intent "+
			"host “%v”: %v", intentHost.Id, err)
	}

	mci.Logger.Logf(slogger.DEBUG, "Successfully inserted intent host “%v” "+
		"for distro “%v” to signal cloud instance spawn intent", instanceName,
		distro.Name)

	spotRequest := &ec2.RequestSpotInstances{
		SpotPrice:      fmt.Sprintf("%v", ec2Settings.BidPrice),
		InstanceCount:  1,
		ImageId:        ec2Settings.AMI,
		KeyName:        ec2Settings.KeyName,
		InstanceType:   ec2Settings.InstanceType,
		SecurityGroups: ec2.SecurityGroupNames(ec2Settings.SecurityGroups),
		BlockDevices:   blockDevices,
	}

	spotResp, err := ec2Handle.RequestSpotInstances(spotRequest)
	if err != nil {
		//Remove the intent host if the API call failed
		if err := intentHost.Remove(); err != nil {
			mci.Logger.Logf(slogger.ERROR, "Failed to remove intent host %v: %v", intentHost.Id, err)
		}
		return nil, mci.Logger.Errorf(slogger.ERROR, "Failed starting spot instance "+
			" for distro '%v' on intent host %v: %v", distro.Name, intentHost.Id, err)
	}

	spotReqRes := spotResp.SpotRequestResults[0]
	if spotReqRes.State != SpotStatusOpen && spotReqRes.State != SpotStatusActive {
		return nil, mci.Logger.Errorf(slogger.ERROR, "Spot request %v was found in "+
			" state %v on intent host %v", spotReqRes.SpotRequestId, spotReqRes.State, intentHost.Id)
	}

	intentHost.Id = spotReqRes.SpotRequestId
	err = intentHost.Insert()
	if err != nil {
		return nil, mci.Logger.Errorf(slogger.ERROR, "Could not insert updated host info with id %v"+
			" for intent host %v: %v", intentHost.Id, instanceName, err)
	}

	//find the old intent host and remove it, since we now have the real
	//host doc successfully stored.
	oldIntenthost, err := model.FindHost(instanceName)
	if err != nil {
		return nil, mci.Logger.Errorf(slogger.ERROR, "Can't locate "+
			"record inserted for intended host '%v' due to error: %v",
			instanceName, err)
	}
	if oldIntenthost == nil {
		return nil, mci.Logger.Errorf(slogger.ERROR, "Can't locate "+
			"record inserted for intended host '%v'", instanceName)
	}

	err = oldIntenthost.Remove()
	if err != nil {
		mci.Logger.Logf(slogger.ERROR, "Could not remove intent host "+
			"“%v”: %v", oldIntenthost.Id, err)
		return nil, err
	}

	// create some tags based on user, hostname, owner, time, etc.
	tags := makeTags(intentHost)

	// attach the tags to this instance
	err = attachTags(ec2Handle, tags, intentHost.Id)

	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Unable to attach tags for %v: %v",
			intentHost.Id, err)
	} else {
		mci.Logger.Logf(slogger.DEBUG, "Attached tag name “%v” for “%v”",
			instanceName, intentHost.Id)
	}
	return intentHost, nil
}

func (cloudManager *EC2SpotManager) TerminateInstance(host *model.Host) error {
	// terminate the instance
	if host.Status == mci.HostTerminated {
		errMsg := fmt.Errorf("Can not terminate %v - already marked as "+
			"terminated!", host.Id)
		mci.Logger.Errorf(slogger.ERROR, errMsg.Error())
		return errMsg
	}

	spotDetails, err := cloudManager.describeSpotRequest(host.Id)
	if err != nil {
		ec2err, ok := err.(*ec2.Error)
		if ok && ec2err.Code == EC2ErrorSpotRequestNotFound {
			// EC2 says the spot request is not found - assume this means amazon
			// terminated our spot instance
			mci.Logger.Logf(slogger.WARN, "EC2 could not find spot instance '%v', "+
				"marking as terminated", host.Id)
			return host.Terminate()
		}
		return mci.Logger.Errorf(slogger.ERROR, "Couldn't terminate, "+
			"failed to get spot request info for %v: %v", host.Id, err)
	}

	mci.Logger.Logf(slogger.INFO, "Cancelling spot request %v", host.Id)
	//First cancel the spot request
	ec2Handle := getUSEast(*cloudManager.awsCredentials)
	_, err = ec2Handle.CancelSpotRequests([]string{host.Id})
	if err != nil {
		return mci.Logger.Errorf(slogger.ERROR, "Failed to cancel spot request for host %v: %v",
			host.Id, err)
	}

	//Canceling the spot request doesn't terminate the instance that fulfilled it,
	// if it was fulfilled. We need to terminate the instance explicitly
	if spotDetails.InstanceId != "" {
		mci.Logger.Logf(slogger.INFO, "Spot request %v cancelled, now terminating instance %v",
			spotDetails.InstanceId, host.Id)
		resp, err := ec2Handle.TerminateInstances([]string{spotDetails.InstanceId})
		if err != nil {
			return mci.Logger.Errorf(slogger.INFO, "Failed to terminate host %v: %v", host.Id)
		}

		for _, stateChange := range resp.StateChanges {
			mci.Logger.Logf(slogger.INFO, "Terminated %v", stateChange.InstanceId)
		}
	} else {
		mci.Logger.Logf(slogger.INFO, "Spot request %v cancelled (no instances have fulfilled it)", host.Id)
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
		return nil, mci.Logger.Errorf(slogger.ERROR, "Received a nil response from EC2 looking up spot request %v",
			spotReqId)
	}
	if len(resp.SpotRequestResults) != 1 {
		return nil, mci.Logger.Errorf(slogger.ERROR, "Expected one spot request info, but got %v",
			len(resp.SpotRequestResults))
	}
	return &resp.SpotRequestResults[0], nil
}
