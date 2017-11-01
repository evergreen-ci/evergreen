package spawn

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/cloud/providers/ec2"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

const (
	MaxPerUser        = 3
	DefaultExpiration = time.Duration(24 * time.Hour)
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
	if d.Provider == ec2.SpotProviderName {
		d.Provider = ec2.OnDemandProviderName
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
