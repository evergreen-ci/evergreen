package cloud

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
)

// HostOptions is a struct of options that are commonly passed around when creating a
// new cloud host.
type HostOptions struct {
	ProvisionOptions   *host.ProvisionOptions
	ExpirationDuration *time.Duration
	UserName           string
	UserData           string
	UserHost           bool
}

//CloudHost is a provider-agnostic host object that delegates methods
//like status checks, ssh options, DNS name checks, termination, etc. to the
//underlying provider's implementation.
type CloudHost struct {
	Host     *host.Host
	KeyPath  string
	CloudMgr CloudManager
}

// GetCloudHost returns an instance of CloudHost wrapping the given model.Host,
// giving access to the provider-specific methods to manipulate on the host.
func GetCloudHost(ctx context.Context, host *host.Host, settings *evergreen.Settings) (*CloudHost, error) {
	mgr, err := GetCloudManager(ctx, host.Provider, settings)
	if err != nil {
		return nil, err
	}

	keyPath := ""
	if host.Distro.SSHKey != "" {
		keyPath = settings.Keys[host.Distro.SSHKey]
	}
	return &CloudHost{host, keyPath, mgr}, nil
}

func (cloudHost *CloudHost) IsUp(ctx context.Context) (bool, error) {
	return cloudHost.CloudMgr.IsUp(ctx, cloudHost.Host)
}

func (cloudHost *CloudHost) TerminateInstance(ctx context.Context, user string) error {
	return cloudHost.CloudMgr.TerminateInstance(ctx, cloudHost.Host, user)
}

func (cloudHost *CloudHost) GetInstanceStatus(ctx context.Context) (CloudStatus, error) {
	return cloudHost.CloudMgr.GetInstanceStatus(ctx, cloudHost.Host)
}

func (cloudHost *CloudHost) GetDNSName(ctx context.Context) (string, error) {
	return cloudHost.CloudMgr.GetDNSName(ctx, cloudHost.Host)
}

func (cloudHost *CloudHost) GetSSHOptions() ([]string, error) {
	return cloudHost.CloudMgr.GetSSHOptions(cloudHost.Host, cloudHost.KeyPath)
}
