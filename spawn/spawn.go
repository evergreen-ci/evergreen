package spawn

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
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
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
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
	UserData  string
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

	if d.UserData.File != "" {
		if strings.TrimSpace(so.UserData) == "" {
			return errors.New("user data not specified")
		}

		var err error
		switch d.UserData.Validate {
		case distro.UserDataFormatFormURLEncoded:
			_, err = url.ParseQuery(so.UserData)
		case distro.UserDataFormatJSON:
			var out map[string]interface{}
			err = json.Unmarshal([]byte(so.UserData), &out)
		case distro.UserDataFormatYAML:
			var out map[string]interface{}
			err = yaml.Unmarshal([]byte(so.UserData), &out)
		}

		if err != nil {
			return errors.Wrapf(err, "invalid spawn options: %s", d.UserData.Validate)
		}
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
	// add any extra user-specified data into the setup script
	if d.UserData.File != "" {
		userDataCmd := fmt.Sprintf("echo \"%v\" > %v\n",
			strings.Replace(so.UserData, "\"", "\\\"", -1), d.UserData.File)
		// prepend the setup script to add the userdata file
		if strings.HasPrefix(d.Setup, "#!") {
			firstLF := strings.Index(d.Setup, "\n")
			d.Setup = d.Setup[0:firstLF+1] + userDataCmd + d.Setup[firstLF+1:]
		} else {
			d.Setup = userDataCmd + d.Setup
		}
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
		UserData:           so.UserData,
		UserHost:           true,
	}

	intentHost := cloud.NewIntent(*d, d.GenerateName(), d.Provider, hostOptions)
	if intentHost == nil { // theoretically this should not happen
		return nil, errors.New("unable to intent host: NewIntent did not return a host")
	}
	return intentHost, errors.WithStack(err)
}

func SetHostRDPPassword(ctx context.Context, host *host.Host, password string) error {
	pwdUpdateCmd, err := constructPwdUpdateCommand(evergreen.GetEnvironment().Settings(), host, password)
	if err != nil {
		return errors.Wrap(err, "Error constructing host RDP password")
	}

	// update RDP and sshd password
	if err = pwdUpdateCmd.Run(ctx); err != nil {
		return errors.Wrap(err, "Error updating host RDP password")
	}
	return nil
}

// constructPwdUpdateCommand returns a RemoteCommand struct used to
// set the RDP password on a remote windows machine.
func constructPwdUpdateCommand(settings *evergreen.Settings, hostObj *host.Host, password string) (subprocess.Command, error) {
	cloudHost, err := cloud.GetCloudHost(hostObj, settings)
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

	stderr := send.MakeWriterSender(grip.GetSender(), level.Error)
	defer stderr.Close()
	stdout := send.MakeWriterSender(grip.GetSender(), level.Info)
	defer stdout.Close()

	updatePwdCmd := fmt.Sprintf("net user %v %v && sc config "+
		"sshd obj= '.\\%v' password= \"%v\"", hostObj.User, password,
		hostObj.User, password)

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

	opts := subprocess.OutputOptions{Error: stderr, Output: stdout}
	if err = remoteCommand.SetOutput(opts); err != nil {
		return nil, err
	}

	return remoteCommand, nil
}

func TerminateHost(host *host.Host, settings *evergreen.Settings) error {
	if host.Status == evergreen.HostTerminated {
		return errors.New("Host is already terminated")
	}
	if host.Status == evergreen.HostUninitialized {
		return host.SetTerminated()
	}
	cloudHost, err := cloud.GetCloudHost(host, settings)
	if err != nil {
		return err
	}
	if err = cloudHost.TerminateInstance(); err != nil {
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
