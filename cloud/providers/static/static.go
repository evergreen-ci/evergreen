package static

import (
	"10gen.com/mci"
	"10gen.com/mci/cloud"
	"10gen.com/mci/hostutil"
	"10gen.com/mci/model/distro"
	"10gen.com/mci/model/host"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"time"
)

const ProviderName = "static"

type StaticManager struct{}

func (staticMgr *StaticManager) SpawnInstance(distro *distro.Distro, owner string,
	userHost bool) (*host.Host, error) {
	return nil, fmt.Errorf("Cannot start new instances with static provider.")
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
	if host.Status == mci.HostDecommissioned {
		mci.Logger.Logf(slogger.DEBUG, "Removing decommissioned %v "+
			"static host (%v)", host.Distro, host.Host)
		if err := host.Remove(); err != nil {
			mci.Logger.Errorf(slogger.ERROR, "Error removing "+
				"decommissioned %v static host (%v): %v",
				host.Distro, host.Host, err)
		}
	}
	mci.Logger.Logf(slogger.DEBUG, "Not terminating static %v host: %v", host.Distro, host.Host)
	return nil
}

func (staticMgr *StaticManager) Configure(mciSettings *mci.MCISettings) error {
	//no-op. maybe will need to load something from mciSettings in the future.
	return nil
}

func (staticMgr *StaticManager) IsSSHReachable(host *host.Host, distro *distro.Distro,
	keyPath string) (bool, error) {
	sshOpts, err := staticMgr.GetSSHOptions(host, distro, keyPath)
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

func (staticMgr *StaticManager) GetSSHOptions(host *host.Host, distro *distro.Distro, keyPath string) ([]string, error) {

	//TODO - Note that currently, we're ignoring the keyPath here to be
	// consistent with how static hosts behaved before cloud manager interfaces. This will
	// probably need to change.
	opts := []string{
		"-o", "ConnectTimeout=10",
		"-o", "StrictHostKeyChecking=no",
	}
	if distro.SSHOptions != nil && len(distro.SSHOptions) > 0 {
		for _, opt := range distro.SSHOptions {
			opts = append(opts, "-o", opt)
		}
	}
	return opts, nil
}

// determine how long until a payment is due for the host. static hosts always
// return 0 for this number
func (staticMgr *StaticManager) TimeTilNextPayment(host *host.Host) time.Duration {
	return time.Duration(0)
}
