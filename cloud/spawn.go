package cloud

import (
	"context"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
)

const (
	DefaultMaxSpawnHostsPerUser         = 3
	DefaultSpawnHostExpiration          = 24 * time.Hour
	SpawnHostNoExpirationDuration       = 7 * 24 * time.Hour
	MaxSpawnHostExpirationDurationHours = 24 * time.Hour * 14
)

// Options holds the required parameters for spawning a host.
type SpawnOptions struct {
	DistroId             string
	Userdata             string
	UserName             string
	PublicKey            string
	TaskId               string
	Owner                *user.DBUser
	InstanceTags         []host.Tag
	InstanceType         string
	Region               string
	NoExpiration         bool
	IsVirtualWorkstation bool
	HomeVolumeSize       int
}

// Validate returns an instance of BadOptionsErr if the SpawnOptions object contains invalid
// data, SpawnLimitErr if the user is already at the spawned host limit, or some other untyped
// instance of Error if something fails during validation.
func (so *SpawnOptions) validate() error {
	if so.Owner == nil {
		return errors.New("spawn options include nil user")
	}

	d, err := distro.FindOne(distro.ById(so.DistroId))
	if err != nil {
		return errors.Errorf("error finding distro '%s'", so.DistroId)
	}

	if !d.SpawnAllowed {
		return errors.Errorf("Invalid spawn options: spawning not allowed for distro  %v", so.DistroId)
	}

	// if the user already has too many active spawned hosts, deny the request
	activeSpawnedHosts, err := host.Find(host.ByUserWithRunningStatus(so.UserName))
	if err != nil {
		return errors.Wrap(err, "Error occurred finding user's current hosts")
	}

	if err = checkSpawnHostLimitExceeded(len(activeSpawnedHosts)); err != nil {
		return err
	}

	if !evergreen.UseSpawnHostRegions && so.Region != "" && so.Region != evergreen.DefaultEC2Region {
		return errors.Wrap(err, "no configurable regions supported")
	}

	// validate public key
	rsa := "ssh-rsa"
	dss := "ssh-dss"
	isRSA := strings.HasPrefix(so.PublicKey, rsa)
	isDSS := strings.HasPrefix(so.PublicKey, dss)
	if !isRSA && !isDSS {
		return errors.New("Invalid spawn options: key does not start with ssh-rsa or ssh-dss")
	}

	sections := strings.Split(so.PublicKey, " ")
	if len(sections) < 2 {
		keyType := rsa
		if sections[0] == dss {
			keyType = dss
		}
		return errors.Errorf("Invalid spawn options: missing space after '%s'", keyType)
	}

	// check for valid base64
	if _, err = base64.StdEncoding.DecodeString(sections[1]); err != nil {
		return errors.New("Invalid spawn options: key contains invalid base64 string")
	}

	return nil
}

func checkSpawnHostLimitExceeded(numCurrentHosts int) error {
	settings, err := evergreen.GetConfig()
	if err != nil {
		return errors.Wrapf(err, "Error occurred getting evergreen settings")
	}

	if numCurrentHosts >= settings.SpawnHostsPerUser {
		return errors.Errorf("User is already running the max allowed number of spawn hosts (%d of %d)",
			numCurrentHosts, settings.SpawnHostsPerUser)
	}
	return nil
}

// CreateSpawnHost spawns a host with the given options.
func CreateSpawnHost(ctx context.Context, so SpawnOptions, settings *evergreen.Settings) (*host.Host, error) {
	if err := so.validate(); err != nil {
		return nil, errors.WithStack(err)
	}

	// load in the appropriate distro
	d, err := distro.FindOne(distro.ById(so.DistroId))
	if err != nil {
		return nil, errors.WithStack(errors.Wrap(err, "error finding distro"))
	}

	if so.Userdata != "" {
		if !IsEc2Provider(d.Provider) {
			return nil, errors.Errorf("cannot set userdata for provider '%s'", d.Provider)
		}
		err = d.SetUserdata(so.Userdata, so.Region)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	// remove extraneous provider settings from the host's saved distro document
	if err := d.RemoveExtraneousProviderSettings(so.Region); err != nil {
		return nil, errors.WithStack(err)
	}

	if so.InstanceType != "" {
		if err := CheckInstanceTypeValid(ctx, d, so.InstanceType, settings.Providers.AWS.AllowedInstanceTypes); err != nil {
			return nil, errors.Wrap(err, "error validating instance type")
		}
	}

	// modify the setup script to add the user's public key
	d.Setup += fmt.Sprintf("\necho \"\n%s\" >> %s\n", so.PublicKey, filepath.Join(d.HomeDir(), ".ssh", "authorized_keys"))

	// fake out replacing spot instances with on-demand equivalents
	if d.Provider == evergreen.ProviderNameEc2Spot || d.Provider == evergreen.ProviderNameEc2Fleet {
		d.Provider = evergreen.ProviderNameEc2OnDemand
	}

	// spawn the host
	provisionOptions := &host.ProvisionOptions{
		LoadCLI: true,
		TaskId:  so.TaskId,
		OwnerId: so.Owner.Id,
	}
	expiration := DefaultSpawnHostExpiration
	if so.NoExpiration {
		expiration = SpawnHostNoExpirationDuration
	}
	hostOptions := host.CreateOptions{
		ProvisionOptions:     provisionOptions,
		UserName:             so.UserName,
		ExpirationDuration:   &expiration,
		UserHost:             true,
		InstanceTags:         so.InstanceTags,
		InstanceType:         so.InstanceType,
		NoExpiration:         so.NoExpiration,
		IsVirtualWorkstation: so.IsVirtualWorkstation,
		HomeVolumeSize:       so.HomeVolumeSize,
		Region:               so.Region,
	}

	intentHost := host.NewIntent(d, d.GenerateName(), d.Provider, hostOptions)
	if intentHost == nil { // theoretically this should not happen
		return nil, errors.New("unable to intent host: NewIntent did not return a host")
	}
	return intentHost, nil
}

// assumes distro already modified to have one region
func CheckInstanceTypeValid(ctx context.Context, d distro.Distro, requestedType string, allowedTypes []string) error {
	if !util.StringSliceContains(allowedTypes, requestedType) {
		return errors.New("This instance type has not been allowed by admins")
	}
	env := evergreen.GetEnvironment()
	opts, err := GetManagerOptions(d)
	if err != nil {
		return errors.Wrap(err, "error getting manager options")
	}
	m, err := GetManager(ctx, env, opts)
	if err != nil {
		return errors.Wrap(err, "error getting manager")
	}
	if err := m.CheckInstanceType(ctx, requestedType); err != nil {
		return errors.Wrapf(err, "error checking instance type '%s'", requestedType)
	}
	return nil
}

func SetHostRDPPassword(ctx context.Context, env evergreen.Environment, host *host.Host, password string) error {
	pwdUpdateCmd, err := constructPwdUpdateCommand(ctx, env, host, password)
	if err != nil {
		return errors.Wrap(err, "Error constructing host RDP password")
	}

	stdout := util.NewMBCappedWriter()
	stderr := util.NewMBCappedWriter()

	pwdUpdateCmd.SetErrorWriter(stderr).SetOutputWriter(stdout)

	// update RDP and sshd password
	if err = pwdUpdateCmd.Run(ctx); err != nil {
		grip.Warning(message.Fields{
			"stdout":    stdout.String(),
			"stderr":    stderr.String(),
			"operation": "set host rdp password",
			"host_id":   host.Id,
			"cmd":       pwdUpdateCmd.String(),
			"err":       err.Error(),
		})
		return errors.Wrap(err, "Error updating host RDP password")
	}

	grip.Debug(message.Fields{
		"stdout":    stdout.String(),
		"stderr":    stderr.String(),
		"operation": "set host rdp password",
		"host_id":   host.Id,
		"cmd":       pwdUpdateCmd.String(),
	})

	return nil
}

// constructPwdUpdateCommand returns a RemoteCommand struct used to
// set the RDP password on a remote windows machine.
func constructPwdUpdateCommand(ctx context.Context, env evergreen.Environment, hostObj *host.Host, password string) (*jasper.Command, error) {
	hostInfo, err := hostObj.GetSSHInfo()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	sshOptions, err := hostObj.GetSSHOptions(env.Settings())
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return env.JasperManager().CreateCommand(ctx).Host(hostInfo.Hostname).User(hostObj.User).
		ExtendRemoteArgs("-p", hostInfo.Port).ExtendRemoteArgs(sshOptions...).PrivKey(hostObj.Distro.SSHKey).
		Append(fmt.Sprintf("echo -e \"%s\" | passwd", password)), nil
}

func TerminateSpawnHost(ctx context.Context, env evergreen.Environment, host *host.Host, user, reason string) error {
	if host.Status == evergreen.HostTerminated {
		return errors.New("Host is already terminated")
	}
	if host.Status == evergreen.HostUninitialized {
		return host.SetTerminated(user, "host never started")
	}
	cloudHost, err := GetCloudHost(ctx, host, env)
	if err != nil {
		return err
	}
	if err = cloudHost.TerminateInstance(ctx, user, reason); err != nil {
		return err
	}
	return nil
}

func ModifySpawnHost(ctx context.Context, env evergreen.Environment, host *host.Host, opts host.HostModifyOptions) error {
	cloudHost, err := GetCloudHost(ctx, host, env)
	if err != nil {
		return err
	}
	if err = cloudHost.ModifyHost(ctx, opts); err != nil {
		return err
	}
	return nil
}

func MakeExtendedSpawnHostExpiration(host *host.Host, extendBy time.Duration) (time.Time, error) {
	if err := host.PastMaxExpiration(extendBy); err != nil {
		return time.Time{}, err
	}

	newExp := host.ExpirationTime.Add(extendBy)
	remainingDuration := newExp.Sub(time.Now()) //nolint
	if remainingDuration > MaxSpawnHostExpirationDurationHours {
		return time.Time{}, errors.Errorf("Can not extend host '%s' expiration by '%s'. Maximum host duration is limited to %s", host.Id, extendBy.String(), MaxSpawnHostExpirationDurationHours.String())
	}

	return newExp, nil
}
