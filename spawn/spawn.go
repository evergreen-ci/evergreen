package spawn

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/cloud/providers"
	"github.com/evergreen-ci/evergreen/cloud/providers/ec2"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"gopkg.in/yaml.v2"
)

const (
	MaxPerUser        = 3
	DefaultExpiration = time.Duration(24 * time.Hour)
)

var SpawnLimitErr = errors.New("User is already running the max allowed # of spawn hosts")

// BadOptionsErr represents an in valid set of spawn options.
type BadOptionsErr struct {
	message string
}

func (bsoe BadOptionsErr) Error() string {
	return "Invalid spawn options:" + bsoe.message
}

// Spawn handles Spawning hosts for users.
type Spawn struct {
	settings *evergreen.Settings
}

// Options holds the required parameters for spawning a host.
type Options struct {
	Distro    string
	UserName  string
	PublicKey string
	UserData  string
	TaskId    string
}

// New returns an initialized Spawn controller.
func New(settings *evergreen.Settings) Spawn {
	return Spawn{settings}
}

// Validate returns an instance of BadOptionsErr if the SpawnOptions object contains invalid
// data, SpawnLimitErr if the user is already at the spawned host limit, or some other untyped
// instance of Error if something fails during validation.
func (sm Spawn) Validate(so Options) error {
	d, err := distro.FindOne(distro.ById(so.Distro))
	if err != nil {
		return BadOptionsErr{fmt.Sprintf("Invalid dist %v", so.Distro)}
	}

	if !d.SpawnAllowed {
		return BadOptionsErr{fmt.Sprintf("Spawning not allowed for dist %v", so.Distro)}
	}

	// if the user already has too many active spawned hosts, deny the request
	activeSpawnedHosts, err := host.Find(host.ByUserWithRunningStatus(so.UserName))
	if err != nil {
		return fmt.Errorf("Error occurred finding user's current hosts: %v", err)
	}

	if len(activeSpawnedHosts) >= MaxPerUser {
		return SpawnLimitErr
	}

	// validate public key
	rsa := "ssh-rsa"
	dss := "ssh-dss"
	isRSA := strings.HasPrefix(so.PublicKey, rsa)
	isDSS := strings.HasPrefix(so.PublicKey, dss)
	if !isRSA && !isDSS {
		return BadOptionsErr{"key does not start with ssh-rsa or ssh-dss"}
	}

	sections := strings.Split(so.PublicKey, " ")
	if len(sections) < 2 {
		keyType := rsa
		if sections[0] == dss {
			keyType = dss
		}
		return BadOptionsErr{fmt.Sprintf("missing space after '%v'", keyType)}
	}

	// check for valid base64
	if _, err = base64.StdEncoding.DecodeString(sections[1]); err != nil {
		return BadOptionsErr{"key contains invalid base64 string"}
	}

	if d.UserData.File != "" {
		if strings.TrimSpace(so.UserData) == "" {
			return BadOptionsErr{}
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
			return BadOptionsErr{fmt.Sprintf("invalid %v: %v", d.UserData.Validate, err)}
		}
	}
	return nil
}

// CreateHost spawns a host with the given options.
func (sm Spawn) CreateHost(so Options, owner *user.DBUser) error {

	// load in the appropriate distro
	d, err := distro.FindOne(distro.ById(so.Distro))
	if err != nil {
		return err
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

	// replace expansions in the script
	exp := command.NewExpansions(sm.settings.Expansions)
	d.Setup, err = exp.ExpandString(d.Setup)
	if err != nil {
		return fmt.Errorf("expansions error: %v", err)
	}

	// fake out replacing spot instances with on-demand equivalents
	if d.Provider == ec2.SpotProviderName {
		d.Provider = ec2.OnDemandProviderName
	}

	// get the appropriate cloud manager
	cloudManager, err := providers.GetCloudManager(d.Provider, sm.settings)
	if err != nil {
		return err
	}

	// spawn the host
	provisionOptions := &host.ProvisionOptions{
		LoadCLI: true,
		TaskId:  so.TaskId,
		OwnerId: owner.Id,
	}
	expiration := DefaultExpiration
	hostOptions := cloud.HostOptions{
		ProvisionOptions:   provisionOptions,
		UserName:           so.UserName,
		ExpirationDuration: &expiration,
		UserData:           so.UserData,
		UserHost:           true,
	}

	_, err = cloudManager.SpawnInstance(d, hostOptions)
	if err != nil {
		return err
	}

	return nil
}
