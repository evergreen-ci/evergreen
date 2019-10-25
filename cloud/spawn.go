package cloud

import (
	"bytes"
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
	MaxSpawnHostsPerUser                = 3
	DefaultSpawnHostExpiration          = 24 * time.Hour
	SpawnHostNoExpirationDuration       = 7 * 24 * time.Hour
	MaxSpawnHostExpirationDurationHours = 24 * time.Hour * 14
)

// Options holds the required parameters for spawning a host.
type SpawnOptions struct {
	DistroId         string
	ProviderSettings *map[string]interface{}
	UserName         string
	PublicKey        string
	TaskId           string
	Owner            *user.DBUser
	InstanceTags     []host.Tag
	InstanceType     string
	NoExpiration     bool
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
		return errors.Errorf("Invalid spawn options: distro %v", so.DistroId)
	}

	if !d.SpawnAllowed {
		return errors.Errorf("Invalid spawn options: spawning not allowed for distro  %v", so.DistroId)
	}

	// if the user already has too many active spawned hosts, deny the request
	activeSpawnedHosts, err := host.Find(host.ByUserWithRunningStatus(so.UserName))
	if err != nil {
		return errors.Wrap(err, "Error occurred finding user's current hosts")
	}

	if len(activeSpawnedHosts) >= MaxSpawnHostsPerUser {
		return errors.Errorf("User is already running the max allowed number of spawn hosts (%d of %d)",
			len(activeSpawnedHosts), MaxSpawnHostsPerUser)
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

// CreateSpawnHost spawns a host with the given options.
func CreateSpawnHost(so SpawnOptions) (*host.Host, error) {
	if err := so.validate(); err != nil {
		return nil, errors.WithStack(err)
	}

	// load in the appropriate distro
	d, err := distro.FindOne(distro.ById(so.DistroId))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if so.ProviderSettings != nil {
		d.ProviderSettings = so.ProviderSettings
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
		ProvisionOptions:   provisionOptions,
		UserName:           so.UserName,
		ExpirationDuration: &expiration,
		UserHost:           true,
		InstanceTags:       so.InstanceTags,
		InstanceType:       so.InstanceType,
		NoExpiration:       so.NoExpiration,
	}

	intentHost := host.NewIntent(d, d.GenerateName(), d.Provider, hostOptions)
	if intentHost == nil { // theoretically this should not happen
		return nil, errors.New("unable to intent host: NewIntent did not return a host")
	}
	return intentHost, nil
}

func SetHostRDPPassword(ctx context.Context, env evergreen.Environment, host *host.Host, password string) error {
	pwdUpdateCmd, err := constructPwdUpdateCommand(ctx, env, host, password)
	if err != nil {
		return errors.Wrap(err, "Error constructing host RDP password")
	}

	stdout := &util.CappedWriter{
		Buffer:   &bytes.Buffer{},
		MaxBytes: 1024 * 1024,
	}

	stderr := &util.CappedWriter{
		Buffer:   &bytes.Buffer{},
		MaxBytes: 1024 * 1024,
	}

	pwdUpdateCmd.SetErrorWriter(stderr).SetOutputWriter(stdout)

	// update RDP and sshd password
	if err = pwdUpdateCmd.Run(ctx); err != nil {
		grip.Warning(message.Fields{
			"stdout":    stdout.Buffer.String(),
			"stderr":    stderr.Buffer.String(),
			"operation": "set host rdp password",
			"host":      host.Id,
			"cmd":       pwdUpdateCmd.String(),
			"err":       err.Error(),
		})
		return errors.Wrap(err, "Error updating host RDP password")
	}

	grip.Debug(message.Fields{
		"stdout":    stdout.Buffer.String(),
		"stderr":    stderr.Buffer.String(),
		"operation": "set host rdp password",
		"host":      host.Id,
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
		ExtendRemoteArgs("-p", hostInfo.Port).ExtendRemoteArgs(sshOptions...).
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
	newExp := host.ExpirationTime.Add(extendBy)
	remainingDuration := newExp.Sub(time.Now()) //nolint
	if remainingDuration > MaxSpawnHostExpirationDurationHours {
		return time.Time{}, errors.Errorf("Can not extend host '%s' expiration by '%s'. Maximum host duration is limited to %s", host.Id, extendBy.String(), MaxSpawnHostExpirationDurationHours.String())
	}

	return newExp, nil
}
