package cloud

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
)

//CloudHost is a provider-agnostic host object that delegates methods
//like status checks, ssh options, DNS name checks, termination, etc. to the
//underlying provider's implementation.
type CloudHost struct {
	Host     *host.Host
	KeyPath  string
	CloudMgr Manager
}

// GetCloudHost returns an instance of CloudHost wrapping the given model.Host,
// giving access to the provider-specific methods to manipulate on the host.
func GetCloudHost(ctx context.Context, host *host.Host, env evergreen.Environment) (*CloudHost, error) {
	mgrOpts := ManagerOpts{
		Provider: host.Provider,
		Region:   GetRegion(host.Distro),
	}
	mgr, err := GetManager(ctx, env, mgrOpts)
	if err != nil {
		return nil, err
	}

	keyPath := ""
	if host.Distro.SSHKey != "" {
		keyPath = env.Settings().Keys[host.Distro.SSHKey]
	}
	return &CloudHost{host, keyPath, mgr}, nil
}

func (cloudHost *CloudHost) IsUp(ctx context.Context) (bool, error) {
	return cloudHost.CloudMgr.IsUp(ctx, cloudHost.Host)
}

func (cloudHost *CloudHost) ModifyHost(ctx context.Context, opts host.HostModifyOptions) error {
	return cloudHost.CloudMgr.ModifyHost(ctx, cloudHost.Host, opts)
}

func (cloudHost *CloudHost) TerminateInstance(ctx context.Context, user, reason string) error {
	return cloudHost.CloudMgr.TerminateInstance(ctx, cloudHost.Host, user, reason)
}

func (cloudHost *CloudHost) StopInstance(ctx context.Context, user string) error {
	return cloudHost.CloudMgr.StopInstance(ctx, cloudHost.Host, user)
}

func (cloudHost *CloudHost) StartInstance(ctx context.Context, user string) error {
	return cloudHost.CloudMgr.StartInstance(ctx, cloudHost.Host, user)
}

func (cloudHost *CloudHost) GetInstanceStatus(ctx context.Context) (CloudStatus, error) {
	return cloudHost.CloudMgr.GetInstanceStatus(ctx, cloudHost.Host)
}

func (cloudHost *CloudHost) GetDNSName(ctx context.Context) (string, error) {
	return cloudHost.CloudMgr.GetDNSName(ctx, cloudHost.Host)
}
