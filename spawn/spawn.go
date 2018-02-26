package spawn

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/subprocess"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// each regex matches one of the 5 categories listed here:
// https://technet.microsoft.com/en-us/library/cc786468(v=ws.10).aspx
var passwordRegexps = []*regexp.Regexp{
	regexp.MustCompile(`[\p{Ll}]`), // lowercase letter
	regexp.MustCompile(`[\p{Lu}]`), // uppercase letter
	regexp.MustCompile(`[0-9]`),
	regexp.MustCompile(`[~!@#$%^&*_\-+=|\\\(\){}\[\]:;"'<>,.?/` + "`]"),
	regexp.MustCompile(`[\p{Lo}]`), // letters without upper/lower variants (ex: Japanese)
}

const (
	MaxPerUser                 = 3
	DefaultExpiration          = 24 * time.Hour
	MaxExpirationDurationHours = 24 * time.Hour * 7 // 7 days
)

// Options holds the required parameters for spawning a host.
type Options struct {
	Distro    string
	UserName  string
	PublicKey string
	TaskId    string
	Owner     *user.DBUser
}

// Validate returns an instance of BadOptionsErr if the SpawnOptions object contains invalid
// data, SpawnLimitErr if the user is already at the spawned host limit, or some other untyped
// instance of Error if something fails during validation.
func (so *Options) validate() error {
	if so.Owner == nil {
		return errors.New("spawn options include nil user")
	}

	d, err := distro.FindOne(distro.ById(so.Distro))
	if err != nil {
		return errors.Errorf("Invalid spawn options: distro %v", so.Distro)
	}

	if !d.SpawnAllowed {
		return errors.Errorf("Invalid spawn options: spawning not allowed for distro  %v", so.Distro)
	}

	// if the user already has too many active spawned hosts, deny the request
	activeSpawnedHosts, err := host.Find(host.ByUserWithRunningStatus(so.UserName))
	if err != nil {
		return errors.Wrap(err, "Error occurred finding user's current hosts")
	}

	if len(activeSpawnedHosts) >= MaxPerUser {
		return errors.Errorf("User is already running the max allowed number of spawn hosts (%d of %d)",
			len(activeSpawnedHosts), MaxPerUser)
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

// CreateHost spawns a host with the given options.
func CreateHost(so Options) (*host.Host, error) {
	if err := so.validate(); err != nil {
		return nil, errors.WithStack(err)
	}

	// load in the appropriate distro
	d, err := distro.FindOne(distro.ById(so.Distro))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// modify the setup script to add the user's public key
	d.Setup += fmt.Sprintf("\necho \"\n%v\" >> ~%v/.ssh/authorized_keys\n", so.PublicKey, d.User)

	// fake out replacing spot instances with on-demand equivalents
	if d.Provider == evergreen.ProviderNameEc2Spot {
		d.Provider = evergreen.ProviderNameEc2OnDemand
	}

	// spawn the host
	provisionOptions := &host.ProvisionOptions{
		LoadCLI: true,
		TaskId:  so.TaskId,
		OwnerId: so.Owner.Id,
	}
	expiration := DefaultExpiration
	hostOptions := cloud.HostOptions{
		ProvisionOptions:   provisionOptions,
		UserName:           so.UserName,
		ExpirationDuration: &expiration,
		UserHost:           true,
	}

	intentHost := cloud.NewIntent(*d, d.GenerateName(), d.Provider, hostOptions)
	if intentHost == nil { // theoretically this should not happen
		return nil, errors.New("unable to intent host: NewIntent did not return a host")
	}
	return intentHost, errors.WithStack(err)
}

func SetHostRDPPassword(ctx context.Context, host *host.Host, password string) error {
	pwdUpdateCmd, err := constructPwdUpdateCommand(ctx, evergreen.GetEnvironment().Settings(), host, password)
	if err != nil {
		return errors.Wrap(err, "Error constructing host RDP password")
	}

	stdout := &util.CappedWriter{&bytes.Buffer{}, 1024 * 1024}
	stderr := &util.CappedWriter{&bytes.Buffer{}, 1024 * 1024}

	opts := subprocess.OutputOptions{Error: stderr, Output: stdout}
	if err = pwdUpdateCmd.SetOutput(opts); err != nil {
		return errors.WithStack(err)
	}

	// update RDP and sshd password
	if err = pwdUpdateCmd.Run(ctx); err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"stdout":    stdout.Buffer.String(),
			"stderr":    stderr.Buffer.String(),
			"operation": "set host rdp password",
			"host":      host.Id,
		}))
		return errors.Wrap(err, "Error updating host RDP password")
	}

	grip.Debug(message.Fields{
		"stdout":    stdout.Buffer.String(),
		"stderr":    stderr.Buffer.String(),
		"operation": "set host rdp password",
		"host":      host.Id,
	})

	return nil
}

// constructPwdUpdateCommand returns a RemoteCommand struct used to
// set the RDP password on a remote windows machine.
func constructPwdUpdateCommand(ctx context.Context, settings *evergreen.Settings, hostObj *host.Host, password string) (subprocess.Command, error) {
	cloudHost, err := cloud.GetCloudHost(ctx, hostObj, settings)
	if err != nil {
		return nil, err
	}

	hostInfo, err := util.ParseSSHInfo(hostObj.Host)
	if err != nil {
		return nil, err
	}

	sshOptions, err := cloudHost.GetSSHOptions()
	if err != nil {
		return nil, err
	}

	escapedPassword := strings.Replace(password, `\`, `\\`, -1)
	updatePwdCmd := fmt.Sprintf(`net user %s "%s" && sc config sshd obj= '.\%s' password= "%s"`,
		hostObj.User, escapedPassword, hostObj.User, escapedPassword)

	// construct the required termination command
	remoteCommand := subprocess.NewRemoteCommand(
		updatePwdCmd,
		hostInfo.Hostname,
		hostObj.User,
		nil,   // env
		false, // background
		append([]string{"-p", hostInfo.Port}, sshOptions...),
		true, // logging disabled
	)

	return remoteCommand, nil
}

func TerminateHost(ctx context.Context, host *host.Host, settings *evergreen.Settings, user string) error {
	if host.Status == evergreen.HostTerminated {
		return errors.New("Host is already terminated")
	}
	if host.Status == evergreen.HostUninitialized {
		return host.SetTerminated(user)
	}
	cloudHost, err := cloud.GetCloudHost(ctx, host, settings)
	if err != nil {
		return err
	}
	if err = cloudHost.TerminateInstance(ctx, user); err != nil {
		return err
	}
	return nil
}

func MakeExtendedHostExpiration(host *host.Host, extendBy time.Duration) (time.Time, error) {
	newExp := host.ExpirationTime.Add(extendBy)
	remainingDuration := newExp.Sub(time.Now()) //nolint
	if remainingDuration > MaxExpirationDurationHours {
		return time.Time{}, errors.Errorf("Can not extend host '%s' expiration by '%s'. Maximum host duration is limited to %s", host.Id, extendBy.String(), MaxExpirationDurationHours.String())
	}

	return newExp, nil
}

// XXX: if modifying any of the password validation logic, you changes must
// also be ported into public/static/js/directives/directives.spawn.js
func ValidateRDPPassword(password string) bool {
	// Golang regex doesn't support lookarounds, so we can't use
	// the regex as found in public/static/js/directives/directives.spawn.js
	if len([]rune(password)) < 6 || len([]rune(password)) > 255 {
		return false
	}

	// valid passwords need to match 3 of 5 categories listed on:
	// https://technet.microsoft.com/en-us/library/cc786468(v=ws.10).aspx
	matchedCategories := 0
	for _, regex := range passwordRegexps {
		if regex.MatchString(password) {
			matchedCategories++
		}
	}

	return matchedCategories >= 3
}
