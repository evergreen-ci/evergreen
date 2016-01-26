package digitalocean

import (
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	digo "github.com/dynport/gocloud/digitalocean"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db/bsonutil"
	"github.com/evergreen-ci/evergreen/hostutil"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mitchellh/mapstructure"
	"math"
	"math/rand"
	"strconv"
	"time"
)

const (
	DigitalOceanStatusOff     = "off"
	DigitalOceanStatusNew     = "new"
	DigitalOceanStatusActive  = "active"
	DigitalOceanStatusArchive = "archive"

	ProviderName = "digitalocean"
)

type DigitalOceanManager struct {
	account *digo.Account
}

type Settings struct {
	ImageId  int `mapstructure:"image_id" json:"image_id" bson:"image_id"`
	SizeId   int `mapstructure:"size_id" json:"size_id" bson:"size_id"`
	RegionId int `mapstructure:"region_id" json:"region_id" bson:"region_id"`
	SSHKeyId int `mapstructure:"ssh_key_id" json:"ssh_key_id" bson:"ssh_key_id"`
}

var (
	// bson fields for the Settings struct
	ImageIdKey  = bsonutil.MustHaveTag(Settings{}, "ImageId")
	SizeIdKey   = bsonutil.MustHaveTag(Settings{}, "SizeId")
	RegionIdKey = bsonutil.MustHaveTag(Settings{}, "RegionId")
	SSHKeyIdKey = bsonutil.MustHaveTag(Settings{}, "SSHKeyId")
)

//Validate checks that the settings from the config file are sane.
func (self *Settings) Validate() error {
	if self.ImageId == 0 {
		return fmt.Errorf("ImageId must not be blank")
	}

	if self.SizeId == 0 {
		return fmt.Errorf("Size ID must not be blank")
	}

	if self.RegionId == 0 {
		return fmt.Errorf("Region must not be blank")
	}

	if self.SSHKeyId == 0 {
		return fmt.Errorf("SSH Key ID must not be blank")
	}

	return nil
}

func (_ *DigitalOceanManager) GetSettings() cloud.ProviderSettings {
	return &Settings{}
}

//SpawnInstance creates a new droplet for the given distro.
func (digoMgr *DigitalOceanManager) SpawnInstance(d *distro.Distro, owner string, userHost bool) (*host.Host, error) {
	if d.Provider != ProviderName {
		return nil, fmt.Errorf("Can't spawn instance of %v for distro %v: provider is %v", ProviderName, d.Id, d.Provider)
	}

	digoSettings := &Settings{}
	if err := mapstructure.Decode(d.ProviderSettings, digoSettings); err != nil {
		return nil, fmt.Errorf("Error decoding params for distro %v: %v", d.Id, err)
	}

	if err := digoSettings.Validate(); err != nil {
		return nil, fmt.Errorf("Invalid DigitalOcean settings in distro %v: %v", d.Id, err)
	}

	instanceName := "droplet-" +
		fmt.Sprintf("%v", rand.New(rand.NewSource(time.Now().UnixNano())).Int())
	intentHost := &host.Host{
		Id:               instanceName,
		User:             d.User,
		Distro:           *d,
		Tag:              instanceName,
		CreationTime:     time.Now(),
		Status:           evergreen.HostUninitialized,
		TerminationTime:  model.ZeroTime,
		TaskDispatchTime: model.ZeroTime,
		Provider:         ProviderName,
		StartedBy:        owner,
	}

	dropletReq := &digo.Droplet{
		SizeId:   digoSettings.SizeId,
		ImageId:  digoSettings.ImageId,
		RegionId: digoSettings.RegionId,
		Name:     instanceName,
		SshKey:   digoSettings.SSHKeyId,
	}

	if err := intentHost.Insert(); err != nil {
		return nil, evergreen.Logger.Errorf(slogger.ERROR, "Could not insert intent "+
			"host '%v': %v", intentHost.Id, err)
	}

	evergreen.Logger.Logf(slogger.DEBUG, "Successfully inserted intent host '%v' "+
		"for distro '%v' to signal cloud instance spawn intent", instanceName,
		d.Id)

	newDroplet, err := digoMgr.account.CreateDroplet(dropletReq)
	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, "DigitalOcean create droplet API call failed"+
			" for intent host '%v': %v", intentHost.Id, err)

		// remove the intent host document
		rmErr := intentHost.Remove()
		if rmErr != nil {
			return nil, evergreen.Logger.Errorf(slogger.ERROR, "Could not remove intent host "+
				"'%v': %v", intentHost.Id, rmErr)
		}
		return nil, err
	}

	// find old intent host
	host, err := host.FindOne(host.ById(intentHost.Id))
	if host == nil {
		return nil, evergreen.Logger.Errorf(slogger.ERROR, "Can't locate "+
			"record inserted for intended host “%v”", intentHost.Id)
	}
	if err != nil {
		return nil, evergreen.Logger.Errorf(slogger.ERROR, "Failed to look up intent host %v: %v",
			intentHost.Id, err)
	}

	host.Id = fmt.Sprintf("%v", newDroplet.Id)
	host.Host = newDroplet.IpAddress
	err = host.Insert()
	if err != nil {
		return nil, evergreen.Logger.Errorf(slogger.ERROR, "Failed to insert new host %v"+
			"for intent host %v: %v", host.Id, intentHost.Id, err)
	}

	// remove the intent host document
	err = intentHost.Remove()
	if err != nil {
		return nil, evergreen.Logger.Errorf(slogger.ERROR, "Could not remove "+
			"insert host “%v” (replaced by “%v”): %v", intentHost.Id, host.Id,
			err)
	}
	return host, nil

}

//getDropletInfo is a helper function to retrieve metadata about a droplet by
//querying DigitalOcean's API directly.
func (digoMgr *DigitalOceanManager) getDropletInfo(dropletId int) (*digo.Droplet, error) {
	droplet := digo.Droplet{Id: dropletId}
	droplet.Account = digoMgr.account
	err := droplet.Reload()
	if err != nil {
		return nil, err
	}
	return &droplet, nil
}

//GetInstanceStatus returns a universal status code representing the state
//of a droplet.
func (digoMgr *DigitalOceanManager) GetInstanceStatus(host *host.Host) (cloud.CloudStatus, error) {
	hostIdAsInt, err := strconv.Atoi(host.Id)
	if err != nil {
		return cloud.StatusUnknown, evergreen.Logger.Errorf(slogger.ERROR,
			"Can't get status of '%v': DigitalOcean host id's must be integers", host.Id)
	}
	droplet, err := digoMgr.getDropletInfo(hostIdAsInt)
	if err != nil {
		return cloud.StatusUnknown, fmt.Errorf("Failed to get droplet info: %v", err)
	}

	switch droplet.Status {
	case DigitalOceanStatusNew:
		return cloud.StatusInitializing, nil
	case DigitalOceanStatusActive:
		return cloud.StatusRunning, nil
	case DigitalOceanStatusArchive:
		return cloud.StatusStopped, nil
	case DigitalOceanStatusOff:
		return cloud.StatusTerminated, nil
	default:
		return cloud.StatusUnknown, nil
	}
}

//GetDNSName gets the DNS hostname of a droplet by reading it directly from
//the DigitalOcean API
func (digoMgr *DigitalOceanManager) GetDNSName(host *host.Host) (string, error) {
	hostIdAsInt, err := strconv.Atoi(host.Id)
	if err != nil {
		return "", evergreen.Logger.Errorf(slogger.ERROR,
			"Can't get DNS name of '%v': DigitalOcean host id's must be integers", host.Id)
	}
	droplet, err := digoMgr.getDropletInfo(hostIdAsInt)
	if err != nil {
		return "", err
	}
	return droplet.IpAddress, nil

}

//CanSpawn returns if a given cloud provider supports spawning a new host
//dynamically. Always returns true for DigitalOcean.
func (digoMgr *DigitalOceanManager) CanSpawn() (bool, error) {
	return true, nil
}

//TerminateInstance destroys a droplet.
func (digoMgr *DigitalOceanManager) TerminateInstance(host *host.Host) error {
	hostIdAsInt, err := strconv.Atoi(host.Id)
	if err != nil {
		return evergreen.Logger.Errorf(slogger.ERROR, "Can't terminate '%v': DigitalOcean host id's must be integers", host.Id)
	}
	response, err := digoMgr.account.DestroyDroplet(hostIdAsInt)
	if err != nil {
		return evergreen.Logger.Errorf(slogger.ERROR, "Failed to destroy droplet '%v': %v", host.Id, err)
	}

	if response.Status != "OK" {
		return evergreen.Logger.Errorf(slogger.ERROR, "Failed to destroy droplet '%v': error message %v", err, response.ErrorMessage)
	}

	return host.Terminate()
}

//Configure populates a DigitalOceanManager by reading relevant settings from the
//config object.
func (digoMgr *DigitalOceanManager) Configure(settings *evergreen.Settings) error {
	digoMgr.account = digo.NewAccount(settings.Providers.DigitalOcean.ClientId,
		settings.Providers.DigitalOcean.Key)
	return nil
}

//IsSSHReachable checks if a droplet appears to be reachable via SSH by
//attempting to contact the host directly.
func (digoMgr *DigitalOceanManager) IsSSHReachable(host *host.Host, keyPath string) (bool, error) {
	sshOpts, err := digoMgr.GetSSHOptions(host, keyPath)
	if err != nil {
		return false, err
	}
	return hostutil.CheckSSHResponse(host, sshOpts)
}

//IsUp checks the droplet's state by querying the DigitalOcean API and
//returns true if the host should be available to connect with SSH.
func (digoMgr *DigitalOceanManager) IsUp(host *host.Host) (bool, error) {
	cloudStatus, err := digoMgr.GetInstanceStatus(host)
	if err != nil {
		return false, err
	}
	if cloudStatus == cloud.StatusRunning {
		return true, nil
	}
	return false, nil
}

func (digoMgr *DigitalOceanManager) OnUp(host *host.Host) error {
	//Currently a no-op as DigitalOcean doesn't support tags.
	return nil
}

//GetSSHOptions returns an array of default SSH options for connecting to a
//droplet.
func (digoMgr *DigitalOceanManager) GetSSHOptions(host *host.Host, keyPath string) ([]string, error) {
	if keyPath == "" {
		return []string{}, fmt.Errorf("No key specified for DigitalOcean host")
	}
	opts := []string{"-i", keyPath}
	for _, opt := range host.Distro.SSHOptions {
		opts = append(opts, "-o", opt)
	}
	return opts, nil
}

// TimeTilNextPayment returns the amount of time until the next payment is due
// for the host
func (digoMgr *DigitalOceanManager) TimeTilNextPayment(host *host.Host) time.Duration {

	now := time.Now()

	// the time since the host was created
	timeSinceCreation := now.Sub(host.CreationTime)

	// the hours since the host was created, rounded up
	hoursRoundedUp := time.Duration(math.Ceil(timeSinceCreation.Hours()))

	// the next round number of hours the host will have been up - the time
	// that the next payment will be due
	nextPaymentTime := host.CreationTime.Add(hoursRoundedUp)

	return nextPaymentTime.Sub(now)
}
