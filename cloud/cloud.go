package cloud

import (
	"10gen.com/mci"
	"10gen.com/mci/model"
	"10gen.com/mci/model/host"
	"time"
)

type CloudStatus int

const (
	//Catch-all for unrecognized status codes
	StatusUnknown = CloudStatus(iota)

	//StatusPending indicates that it is not yet clear if the instance
	//has been successfullly started or not (e.g., pending spot request)
	StatusPending

	//StatusInitializing means the instance request has been successfully
	//fulfilled, but it's not yet done booting up
	StatusInitializing

	//StatusFailed indicates that an attempt to start the instance has failed;
	//Could be due to billing, lack of capacity, etc.
	StatusFailed

	//StatusRunning means the machine is done booting, and active
	StatusRunning

	StatusStopped
	StatusTerminated
)

func (stat CloudStatus) String() string {
	switch stat {
	case StatusPending:
		return "pending"
	case StatusFailed:
		return "failed"
	case StatusInitializing:
		return "initializing"
	case StatusRunning:
		return "running"
	case StatusStopped:
		return "stopped"
	case StatusTerminated:
		return "terminated"
	default:
		return "unknown"
	}
}

//CloudManager is an interface which handles creating new hosts or modifying
//them via some third-party API.
type CloudManager interface {

	//Load credentials or other settings from the config file
	Configure(*mci.MCISettings) error

	// SpawnInstance attempts to create a new host by requesting one from the
	// provider's API.
	SpawnInstance(*model.Distro, string, bool) (*host.Host, error)

	// CanSpawn indicates if this provider is capable of creating new instances
	// with SpawnInstance(). If this provider doesn't support spawning new
	// hosts, this will return false (and calls to SpawnInstance will
	// return errors)
	CanSpawn() (bool, error)

	// get the status of an instance
	GetInstanceStatus(*host.Host) (CloudStatus, error)

	// TerminateInstances destroys the host in the underlying provider
	TerminateInstance(*host.Host) error

	//IsUp returns true if the underlying provider has not destroyed the
	//host (in other words, if the host "should" be reachable. This does not
	//necessarily mean that the host actually *is* reachable via SSH
	IsUp(*host.Host) (bool, error)

	//Called by the hostinit process when the host is actually up. Used
	//to set additional provider-specific metadata
	OnUp(*host.Host) error

	//IsSSHReachable returns true if the host can successfully
	//accept and run an ssh command.
	IsSSHReachable(host *host.Host, distro *model.Distro, keyPath string) (bool, error)

	// GetDNSName returns the DNS name of a host.
	GetDNSName(*host.Host) (string, error)

	// GetSSHOptions generates the command line args to be passed to ssh to
	// allow connection to the machine
	GetSSHOptions(host *host.Host, distro *model.Distro, keyName string) ([]string, error)

	// TimeTilNextPayment returns how long there is until the next payment
	// is due for a particular host
	TimeTilNextPayment(host *host.Host) time.Duration
}

//CloudHost is a provider-agnostic host object that delegates methods
//like status checks, ssh options, DNS name checks, termination, etc. to the
//underlying provider's implementation.
type CloudHost struct {
	Host     *host.Host
	Distro   *model.Distro
	KeyPath  string
	CloudMgr CloudManager
}

func (cloudHost *CloudHost) IsSSHReachable() (bool, error) {
	return cloudHost.CloudMgr.IsSSHReachable(cloudHost.Host, cloudHost.Distro,
		cloudHost.KeyPath)
}

func (cloudHost *CloudHost) IsUp() (bool, error) {
	return cloudHost.CloudMgr.IsUp(cloudHost.Host)
}

func (cloudHost *CloudHost) TerminateInstance() error {
	return cloudHost.CloudMgr.TerminateInstance(cloudHost.Host)
}

func (cloudHost *CloudHost) GetInstanceStatus() (CloudStatus, error) {
	return cloudHost.CloudMgr.GetInstanceStatus(cloudHost.Host)
}

func (cloudHost *CloudHost) GetDNSName() (string, error) {
	return cloudHost.CloudMgr.GetDNSName(cloudHost.Host)
}

func (cloudHost *CloudHost) GetSSHOptions() ([]string, error) {
	return cloudHost.CloudMgr.GetSSHOptions(cloudHost.Host, cloudHost.Distro, cloudHost.KeyPath)
}
