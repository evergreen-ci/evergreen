package cloud

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/pkg/errors"
)

// CloudHost is a provider-agnostic host object that delegates methods
// like status checks, ssh options, DNS name checks, termination, etc. to the
// underlying provider's implementation.
type CloudHost struct {
	Host     *host.Host
	CloudMgr Manager
}

// GetCloudHost returns an instance of CloudHost wrapping the given model.Host,
// giving access to the provider-specific methods to manipulate on the host.
func GetCloudHost(ctx context.Context, host *host.Host, env evergreen.Environment) (*CloudHost, error) {
	mgrOpts, err := GetManagerOptions(host.Distro)
	if err != nil {
		return nil, errors.Wrapf(err, "getting cloud manager options for host '%s'", host.Id)
	}
	mgr, err := GetManager(ctx, env, mgrOpts)
	if err != nil {
		return nil, err
	}
	return &CloudHost{Host: host, CloudMgr: mgr}, nil
}

func (cloudHost *CloudHost) ModifyHost(ctx context.Context, opts host.HostModifyOptions) error {
	return cloudHost.CloudMgr.ModifyHost(ctx, cloudHost.Host, opts)
}

func (cloudHost *CloudHost) TerminateInstance(ctx context.Context, user, reason string) error {
	return cloudHost.CloudMgr.TerminateInstance(ctx, cloudHost.Host, user, reason)
}

func (cloudHost *CloudHost) StopInstance(ctx context.Context, shouldKeepOff bool, user string) error {
	return cloudHost.CloudMgr.StopInstance(ctx, cloudHost.Host, shouldKeepOff, user)
}

func (cloudHost *CloudHost) StartInstance(ctx context.Context, user string) error {
	return cloudHost.CloudMgr.StartInstance(ctx, cloudHost.Host, user)
}

func (cloudHost *CloudHost) RebootInstance(ctx context.Context, user string) error {
	return cloudHost.CloudMgr.RebootInstance(ctx, cloudHost.Host, user)
}

func (cloudHost *CloudHost) GetInstanceState(ctx context.Context) (CloudInstanceState, error) {
	return cloudHost.CloudMgr.GetInstanceState(ctx, cloudHost.Host)
}

func (cloudHost *CloudHost) GetDNSName(ctx context.Context) (string, error) {
	return cloudHost.CloudMgr.GetDNSName(ctx, cloudHost.Host)
}

func (cloudHost *CloudHost) AssociateIP(ctx context.Context, h *host.Host) error {
	return cloudHost.CloudMgr.AssociateIP(ctx, cloudHost.Host)
}

func (cloudHost *CloudHost) CleanupIP(ctx context.Context) error {
	return cloudHost.CloudMgr.CleanupIP(ctx, cloudHost.Host)
}
