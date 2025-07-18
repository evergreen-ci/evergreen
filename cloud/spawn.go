package cloud

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
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
	host.SleepScheduleOptions
	IsVirtualWorkstation bool
	IsCluster            bool
	HomeVolumeSize       int
	HomeVolumeID         string
	Expiration           *time.Time
}

// Validate returns an instance of BadOptionsErr if the SpawnOptions object contains invalid
// data, SpawnLimitErr if the user is already at the spawned host limit, or some other untyped
// instance of Error if something fails during validation.
func (so *SpawnOptions) validate(ctx context.Context, settings *evergreen.Settings) error {
	d, err := distro.FindOneId(ctx, so.DistroId)
	if err != nil {
		return errors.Errorf("finding distro '%s'", so.DistroId)
	}
	if d == nil {
		return errors.Errorf("distro '%s' not found", so.DistroId)
	}

	if !d.SpawnAllowed {
		return errors.Errorf("spawn hosts not allowed for distro '%s'", so.DistroId)
	}

	// if the user already has too many active spawned hosts, deny the request
	activeSpawnedHosts, err := host.Find(ctx, host.ByUserWithRunningStatus(so.UserName))
	if err != nil {
		return errors.Wrap(err, "finding user's current hosts")
	}

	if err = checkSpawnHostLimitExceeded(len(activeSpawnedHosts), settings); err != nil {
		return err
	}

	// validate public key
	if err = evergreen.ValidateSSHKey(so.PublicKey); err != nil {
		return errors.Wrap(err, "invalid SSH key options")
	}

	sections := strings.Split(so.PublicKey, " ")
	if len(sections) < 2 {
		return errors.Errorf("missing space in public key")
	}

	// check for valid base64
	if _, err = base64.StdEncoding.DecodeString(sections[1]); err != nil {
		return errors.New("public key contains invalid base64 string")
	}

	if !so.SleepScheduleOptions.IsZero() {
		if !so.NoExpiration {
			return errors.New("cannot set a sleep schedule on an expirable host")
		}
		if err := so.SleepScheduleOptions.Validate(); err != nil {
			return errors.Wrap(err, "invalid sleep schedule options")
		}
	}

	return nil
}

func checkSpawnHostLimitExceeded(numCurrentHosts int, settings *evergreen.Settings) error {
	if numCurrentHosts >= settings.Spawnhost.SpawnHostsPerUser {
		return errors.Errorf("user is already running the max allowed number of spawn hosts (%d of %d)", numCurrentHosts, settings.Spawnhost.SpawnHostsPerUser)
	}
	return nil
}

// CreateSpawnHost spawns a host with the given options.
func CreateSpawnHost(ctx context.Context, so SpawnOptions, settings *evergreen.Settings) (*host.Host, error) {
	if err := so.validate(ctx, settings); err != nil {
		return nil, errors.WithStack(err)
	}

	// load in the appropriate distro
	d, err := distro.FindOneId(ctx, so.DistroId)
	if err != nil {
		return nil, errors.WithStack(errors.Wrap(err, "finding distro"))
	}
	if d == nil {
		return nil, errors.Errorf("distro '%s' not found", so.DistroId)
	}
	if so.Region == "" && evergreen.IsEc2Provider(d.Provider) {
		u := gimlet.GetUser(ctx)
		dbUser, ok := u.(*user.DBUser)
		if !ok {
			return nil, errors.Errorf("getting DBUser from User")
		}
		so.Region = dbUser.GetRegion()
	}
	if so.Userdata != "" {
		if !evergreen.IsEc2Provider(d.Provider) {
			return nil, errors.Errorf("cannot set user data for non-EC2 provider '%s'", d.Provider)
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
		volume, err = host.ValidateVolumeCanBeAttached(ctx, so.HomeVolumeID)
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

	d.ProviderSettingsList, err = modifySpawnHostProviderSettings(ctx, *d, settings, so.Region, so.HomeVolumeID)
	if err != nil {
		return nil, errors.Wrap(err, "getting new provider settings")
	}

	if so.InstanceType != "" {
		if err := CheckInstanceTypeValid(ctx, *d, so.InstanceType, settings.Providers.AWS.AllowedInstanceTypes); err != nil {
			return nil, errors.Wrap(err, "validating instance type")
		}
	}

	// modify the setup script to add the user's public key
	d.Setup += fmt.Sprintf("\necho \"\n%s\" >> %s\n", so.PublicKey, d.GetAuthorizedKeysFile())

	// Replace fleet with on-demand explicitly.
	if d.Provider == evergreen.ProviderNameEc2Fleet {
		d.Provider = evergreen.ProviderNameEc2OnDemand
	}

	// spawn the host
	currentTime := time.Now()
	expiration := evergreen.DefaultSpawnHostExpiration

	if so.Expiration != nil {
		expiration = so.Expiration.Sub(currentTime)
	}
	var sleepSchedule host.SleepScheduleInfo
	if so.NoExpiration {
		expiration = evergreen.SpawnHostNoExpirationDuration

		schedule, err := host.NewSleepScheduleInfo(so.SleepScheduleOptions)
		if err != nil {
			return nil, errors.Wrap(err, "creating sleep schedule")
		}
		sleepSchedule = *schedule
	}
	hostOptions := host.CreateOptions{
		Distro:               *d,
		ProvisionOptions:     so.ProvisionOptions,
		UserName:             so.UserName,
		ExpirationTime:       currentTime.Add(expiration),
		UserHost:             true,
		InstanceTags:         so.InstanceTags,
		InstanceType:         so.InstanceType,
		NoExpiration:         so.NoExpiration,
		SleepScheduleInfo:    sleepSchedule,
		IsVirtualWorkstation: so.IsVirtualWorkstation,
		IsCluster:            so.IsCluster,
		HomeVolumeSize:       so.HomeVolumeSize,
		HomeVolumeID:         so.HomeVolumeID,
		Region:               so.Region,
	}

	intentHost := host.NewIntent(hostOptions)
	if intentHost == nil { // theoretically this should not happen
		return nil, errors.New("could not create new intent host")
	}
	return intentHost, nil
}

// assumes distro already modified to have one region
func CheckInstanceTypeValid(ctx context.Context, d distro.Distro, requestedType string, allowedTypes []string) error {
	if !utility.StringSliceContains(allowedTypes, requestedType) {
		// if it's not in the settings list, check the distro
		originalInstanceType, ok := d.ProviderSettingsList[0].Lookup("instance_type").StringValueOK()
		if !ok || originalInstanceType != requestedType {
			return errors.New("this instance type has not been allowed by admins")
		}
	}
	env := evergreen.GetEnvironment()
	opts, err := GetManagerOptions(d)
	if err != nil {
		return errors.Wrap(err, "getting cloud manager options")
	}
	m, err := GetManager(ctx, env, opts)
	if err != nil {
		return errors.Wrap(err, "getting cloud manager")
	}
	if err := m.CheckInstanceType(ctx, requestedType); err != nil {
		return errors.Wrapf(err, "checking instance type '%s'", requestedType)
	}
	return nil
}

// SetHostRDPPassword is a shared utility function to change the password on a windows host
func SetHostRDPPassword(ctx context.Context, env evergreen.Environment, h *host.Host, pwd string) (int, error) {
	if !h.Distro.IsWindows() {
		return http.StatusBadRequest, errors.New("RDP password can only be set on Windows hosts")
	}
	if !host.ValidateRDPPassword(pwd) {
		return http.StatusBadRequest, errors.New("invalid password")
	}
	if h.Status != evergreen.HostRunning {
		return http.StatusBadRequest, errors.New("RDP passwords can only be set on running hosts")
	}

	if err := updateRDPPassword(ctx, env, h, pwd); err != nil {
		return http.StatusInternalServerError, err
	}
	return http.StatusOK, nil
}

func updateRDPPassword(ctx context.Context, env evergreen.Environment, host *host.Host, password string) error {
	const redactedPasswordStr = "<REDACTED>"
	pwdUpdateCmd, err := constructPwdUpdateCommand(ctx, env, host, password)
	if err != nil {
		return errors.Wrap(err, "constructing host RDP password")
	}

	stdout := util.NewMBCappedWriter()
	stderr := util.NewMBCappedWriter()

	pwdUpdateCmd.SetErrorWriter(stderr).SetOutputWriter(stdout)

	// update RDP and sshd password
	if err = pwdUpdateCmd.Run(ctx); err != nil {
		grip.Warning(message.Fields{
			"stdout":    stdout.String(),
			"stderr":    stderr.String(),
			"operation": "set host RDP password",
			"host_id":   host.Id,
			"cmd":       strings.ReplaceAll(pwdUpdateCmd.String(), password, redactedPasswordStr),
			"err":       err.Error(),
		})
		return errors.Wrap(err, "updating host RDP password")
	}

	grip.Debug(message.Fields{
		"stdout":    stdout.String(),
		"stderr":    stderr.String(),
		"operation": "set host RDP password",
		"host_id":   host.Id,
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
		Append(fmt.Sprintf("echo -e \"%s\" | passwd", password)), nil
}

func TerminateSpawnHost(ctx context.Context, env evergreen.Environment, host *host.Host, user, reason string) error {
	if host.Status == evergreen.HostTerminated {
		return errors.New("host is already terminated")
	}
	if host.Status == evergreen.HostUninitialized {
		return host.SetTerminated(ctx, user, "host is an intent host")
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
	if err := host.ValidateExpirationExtension(extendBy); err != nil {
		return time.Time{}, err
	}
	newExp := host.ExpirationTime.Add(extendBy)
	return newExp, nil
}

func modifySpawnHostProviderSettings(ctx context.Context, d distro.Distro, settings *evergreen.Settings, region, volumeID string) ([]*birch.Document, error) {
	ec2Settings := EC2ProviderSettings{}
	if err := ec2Settings.FromDistroSettings(d, region); err != nil {
		return nil, errors.Wrapf(err, "getting ec2 provider from distro")
	}

	if volumeID != "" {
		volume, err := host.FindVolumeByID(ctx, volumeID)
		if err != nil {
			return nil, errors.Wrapf(err, "getting volume '%s'", volumeID)
		}

		ec2Settings.SubnetId, err = getSubnetForZone(settings.Providers.AWS.Subnets, volume.AvailabilityZone)
		if err != nil {
			return nil, errors.Wrapf(err, "getting subnet for AZ '%s'", volume.AvailabilityZone)
		}
	}

	doc, err := ec2Settings.ToDocument()
	if err != nil {
		return nil, errors.Wrap(err, "converting EC2 provider settings back to BSON doc")
	}

	return []*birch.Document{doc}, nil
}
