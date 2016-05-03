package static

import (
	"fmt"
	"time"

	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db/bsonutil"
	"github.com/evergreen-ci/evergreen/hostutil"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
)

const ProviderName = "static"

type StaticManager struct{}

type Settings struct {
	Hosts []Host `mapstructure:"hosts" json:"hosts" bson:"hosts"`
}

type Host struct {
	Name string `mapstructure:"name" json:"name" bson:"name"`
}

var (
	// bson fields for the Settings struct
	HostsKey = bsonutil.MustHaveTag(Settings{}, "Hosts")

	// bson fields for the Host struct
	NameKey = bsonutil.MustHaveTag(Host{}, "Name")
)

// Validate checks that the settings from the configuration are valid.
func (s *Settings) Validate() error {
	for _, h := range s.Hosts {
		if h.Name == "" {
			return fmt.Errorf("host 'name' field can not be blank")
		}
	}
	return nil
}

func (staticMgr *StaticManager) SpawnInstance(distro *distro.Distro, owner string,
	userHost bool) (*host.Host, error) {
	return nil, fmt.Errorf("cannot start new instances with static provider")
}

// get the status of an instance
func (staticMgr *StaticManager) GetInstanceStatus(host *host.Host) (cloud.CloudStatus, error) {
	return cloud.StatusRunning, nil
}

// get instance DNS
func (staticMgr *StaticManager) GetDNSName(host *host.Host) (string, error) {
	return host.Id, nil
}

func (staticMgr *StaticManager) CanSpawn() (bool, error) {
	return false, nil
}

// terminate an instance
func (staticMgr *StaticManager) TerminateInstance(host *host.Host) error {
	// a decommissioned static host will be removed from the database
	if host.Status == evergreen.HostDecommissioned {
		evergreen.Logger.Logf(slogger.DEBUG, "Removing decommissioned %v "+
			"static host (%v)", host.Distro, host.Host)
		if err := host.Remove(); err != nil {
			evergreen.Logger.Errorf(slogger.ERROR, "Error removing "+
				"decommissioned %v static host (%v): %v",
				host.Distro, host.Host, err)
		}
	}
	evergreen.Logger.Logf(slogger.DEBUG, "Not terminating static %v host: %v", host.Distro, host.Host)
	return nil
}

func (_ *StaticManager) GetSettings() cloud.ProviderSettings {
	return &Settings{}
}

func (staticMgr *StaticManager) Configure(settings *evergreen.Settings) error {
	//no-op. maybe will need to load something from settings in the future.
	return nil
}

func (staticMgr *StaticManager) IsSSHReachable(host *host.Host, keyPath string) (bool, error) {
	sshOpts, err := staticMgr.GetSSHOptions(host, keyPath)
	if err != nil {
		return false, err
	}
	return hostutil.CheckSSHResponse(host, sshOpts)
}

func (staticMgr *StaticManager) IsUp(host *host.Host) (bool, error) {
	return true, nil
}

func (staticMgr *StaticManager) OnUp(host *host.Host) error {
	return nil
}

func (staticMgr *StaticManager) GetSSHOptions(h *host.Host, keyPath string) (opts []string, err error) {
	if keyPath != "" {
		opts = append(opts, "-i", keyPath)
	}
	for _, opt := range h.Distro.SSHOptions {
		opts = append(opts, "-o", opt)
	}
	return opts, nil
}

// determine how long until a payment is due for the host. static hosts always
// return 0 for this number
func (staticMgr *StaticManager) TimeTilNextPayment(host *host.Host) time.Duration {
	return time.Duration(0)
}
