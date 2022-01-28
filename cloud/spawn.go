package cloud

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
)

// Options holds the required parameters for spawning a host.
type SpawnOptions struct {
	DistroId              string
	Userdata              string
	UserName              string
	PublicKey             string
	ProvisionOptions      *host.ProvisionOptions
	UseProjectSetupScript bool
	InstanceTags          []host.Tag
	InstanceType          string
	Region                string
	NoExpiration          bool
	IsVirtualWorkstation  bool
	IsCluster             bool
	HomeVolumeSize        int
	HomeVolumeID          string
	Expiration            *time.Time
}

// Validate returns an instance of BadOptionsErr if the SpawnOptions object contains invalid
// data, SpawnLimitErr if the user is already at the spawned host limit, or some other untyped
// instance of Error if something fails during validation.
func (so *SpawnOptions) validate(settings *evergreen.Settings) error {
	d, err := distro.FindOne(distro.ById(so.DistroId))
	if err != nil {
		return errors.Errorf("error finding distro '%s'", so.DistroId)
	}

	if !d.SpawnAllowed {
		return errors.Errorf("Invalid spawn options: spawning not allowed for distro %s", so.DistroId)
	}

	// if the user already has too many active spawned hosts, deny the request
	activeSpawnedHosts, err := host.Find(host.ByUserWithRunningStatus(so.UserName))
	if err != nil {
		return errors.Wrap(err, "Error occurred finding user's current hosts")
	}

	if err = checkSpawnHostLimitExceeded(len(activeSpawnedHosts), settings); err != nil {
		return err
	}

	// validate public key
	rsa := "ssh-rsa"
	dss := "ssh-dss"
	isRSA := strings.HasPrefix(so.PublicKey, rsa)
	isDSS := strings.HasPrefix(so.PublicKey, dss)
	if !isRSA && !isDSS {
		return errors.Errorf("Invalid spawn options: "+
			"either an invalid Evergreen-managed key name has been provided,"+
			"or the key value does not start with %s or %s", rsa, dss)
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

func checkSpawnHostLimitExceeded(numCurrentHosts int, settings *evergreen.Settings) error {
	if numCurrentHosts >= settings.Spawnhost.SpawnHostsPerUser {
		return errors.Errorf("User is already running the max allowed number of spawn hosts (%d of %d)",
			numCurrentHosts, settings.Spawnhost.SpawnHostsPerUser)
	}
	return nil
}

// CreateSpawnHost spawns a host with the given options.
func CreateSpawnHost(ctx context.Context, so SpawnOptions, settings *evergreen.Settings) (*host.Host, error) {
	if err := so.validate(settings); err != nil {
		return nil, errors.WithStack(err)
	}

	// load in the appropriate distro
	d, err := distro.FindOne(distro.ById(so.DistroId))
	if err != nil {
		return nil, errors.WithStack(errors.Wrap(err, "error finding distro"))
	}
	if so.Region == "" && IsEc2Provider(d.Provider) {
		u := gimlet.GetUser(ctx)
		dbUser, ok := u.(*user.DBUser)
		if !ok {
			return nil, errors.Errorf("error getting DBUser from User")
		}
		so.Region = dbUser.GetRegion()
	}
	if so.Userdata != "" {
		if !IsEc2Provider(d.Provider) {
			return nil, errors.Errorf("cannot set userdata for provider '%s'", d.Provider)
		}
		if _, err = parseUserData(so.Userdata); err != nil {
			return nil, errors.Wrap(err, "user data is malformed")
		}
		err = d.SetUserdata(so.Userdata, so.Region)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}
	if so.HomeVolumeID != "" {
		var volume *host.Volume
		volume, err = host.ValidateVolumeCanBeAttached(so.HomeVolumeID)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if AztoRegion(volume.AvailabilityZone) != so.Region {
			return nil, errors.Errorf("cannot use volume in zone '%s' with host in region '%s'", volume.AvailabilityZone, so.Region)
		}
	}
	if so.UseProjectSetupScript {
		so.ProvisionOptions.SetupScript, err = model.GetSetupScriptForTask(ctx, so.ProvisionOptions.TaskId)
		if err != nil {
			// still spawn the host if the setup script is buggy
			grip.Error(message.WrapError(err, message.Fields{
				"message": "failed to get setup script for host",
				"task_id": so.ProvisionOptions.TaskId,
				"user_id": so.UserName,
			}))
		}
	}

	d.ProviderSettingsList, err = modifySpawnHostProviderSettings(d, settings, so.Region, so.HomeVolumeID)
	if err != nil {
		return nil, errors.Wrap(err, "can't get new provider settings")
	}

	if so.InstanceType != "" {
		if err := CheckInstanceTypeValid(ctx, d, so.InstanceType, settings.Providers.AWS.AllowedInstanceTypes); err != nil {
			return nil, errors.Wrap(err, "error validating instance type")
		}
	}

	// modify the setup script to add the user's public key
	d.Setup += fmt.Sprintf("\necho \"\n%s\" >> %s\n", so.PublicKey, d.GetAuthorizedKeysFile())

	// fake out replacing spot instances with on-demand equivalents
	if d.Provider == evergreen.ProviderNameEc2Spot || d.Provider == evergreen.ProviderNameEc2Fleet {
		d.Provider = evergreen.ProviderNameEc2OnDemand
	}

	// spawn the host
	expiration := evergreen.DefaultSpawnHostExpiration
	if so.Expiration != nil {
		expiration = time.Until(*so.Expiration)
	}
	if so.NoExpiration {
		expiration = evergreen.SpawnHostNoExpirationDuration
	}
	hostOptions := host.CreateOptions{
		ProvisionOptions:     so.ProvisionOptions,
		UserName:             so.UserName,
		ExpirationDuration:   &expiration,
		UserHost:             true,
		InstanceTags:         so.InstanceTags,
		InstanceType:         so.InstanceType,
		NoExpiration:         so.NoExpiration,
		IsVirtualWorkstation: so.IsVirtualWorkstation,
		IsCluster:            so.IsCluster,
		HomeVolumeSize:       so.HomeVolumeSize,
		HomeVolumeID:         so.HomeVolumeID,
		Region:               so.Region,
	}

	intentHost := host.NewIntent(d, d.GenerateName(), d.Provider, hostOptions)
	if intentHost == nil { // theoretically this should not happen
		return nil, errors.New("unable to intent host: NewIntent did not return a host")
	}
	return intentHost, nil
}

func CreateVolume(ctx context.Context, env evergreen.Environment, volume *host.Volume, provider string) (*host.Volume, error) {
	mgrOpts := ManagerOpts{
		Provider: provider,
		Region:   AztoRegion(volume.AvailabilityZone),
	}
	mgr, err := GetManager(ctx, env, mgrOpts)
	if err != nil {
		return nil, errors.Wrapf(err, "error getting manager")
	}

	if volume, err = mgr.CreateVolume(ctx, volume); err != nil {
		return nil, errors.Wrapf(err, "error creating volume")
	}
	return volume, nil
}

// assumes distro already modified to have one region
func CheckInstanceTypeValid(ctx context.Context, d distro.Distro, requestedType string, allowedTypes []string) error {
	if !utility.StringSliceContains(allowedTypes, requestedType) {
		// if it's not in the settings list, check the distro
		originalInstanceType, ok := d.ProviderSettingsList[0].Lookup("instance_type").StringValueOK()
		if !ok || originalInstanceType != requestedType {
			return errors.New("This instance type has not been allowed by admins")
		}
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
func constructPwdUpdateCommand(ctx context.Context, env evergreen.Environment, h *host.Host, password string) (*jasper.Command, error) {
	sshOpts, err := h.GetSSHOptions(env.Settings())
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return env.JasperManager().CreateCommand(ctx).
		Host(h.Host).User(h.User).
		ExtendRemoteArgs(sshOpts...).
		PrivKey(h.Distro.SSHKey).
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
	if remainingDuration > evergreen.MaxSpawnHostExpirationDurationHours {
		return time.Time{}, errors.Errorf("Can not extend host '%s' expiration by '%s'. Maximum host duration is limited to %s", host.Id, extendBy.String(), evergreen.MaxSpawnHostExpirationDurationHours.String())
	}

	return newExp, nil
}

func modifySpawnHostProviderSettings(d distro.Distro, settings *evergreen.Settings, region, volumeID string) ([]*birch.Document, error) {
	ec2Settings := EC2ProviderSettings{}
	if err := ec2Settings.FromDistroSettings(d, region); err != nil {
		return nil, errors.Wrapf(err, "error getting ec2 provider from distro")
	}

	if volumeID != "" {
		volume, err := host.FindVolumeByID(volumeID)
		if err != nil {
			return nil, errors.Wrapf(err, "can't get volume '%s'", volumeID)
		}

		ec2Settings.SubnetId, err = getSubnetForZone(settings.Providers.AWS.Subnets, volume.AvailabilityZone)
		if err != nil {
			return nil, errors.Wrapf(err, "no subnet found for AZ '%s'", volume.AvailabilityZone)
		}
	}

	doc, err := ec2Settings.ToDocument()
	if err != nil {
		return nil, errors.Wrap(err, "can't convert ec2Settings back to doc")
	}

	return []*birch.Document{doc}, nil
}
