package spawn

import (
	"10gen.com/mci"
	"10gen.com/mci/cloud/providers"
	"10gen.com/mci/command"
	"10gen.com/mci/hostinit"
	"10gen.com/mci/model/distro"
	"10gen.com/mci/model/host"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"gopkg.in/yaml.v2"
	"net/url"
	"strings"
	"time"
)

const (
	MaxPerUser        = 3
	DefaultExpiration = time.Duration(24 * time.Hour)
)

var (
	SpawnLimitErr = errors.New("User is already running the max allowed # of spawn hosts")
)

type BadOptionsErr struct {
	message string
}

func (bsoe BadOptionsErr) Error() string {
	return "Invalid spawn options:" + bsoe.message
}

type Spawn struct {
	mciSettings *mci.MCISettings
}

type Options struct {
	Distro    string
	UserName  string
	PublicKey string
	UserData  string
}

func New(ms *mci.MCISettings) Spawn {
	return Spawn{ms}
}

// Validate() returns an instance of BadOptionsErr if the SpawnOptions object contains invalid
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
func (sm Spawn) CreateHost(so Options) (*host.Host, error) {

	// load in the appropriate distro
	d, err := distro.FindOne(distro.ById(so.Distro))
	if err != nil {
		return nil, err
	}

	// get the appropriate cloud manager
	cloudManager, err := providers.GetCloudManager(d.Provider, sm.mciSettings)
	if err != nil {
		return nil, err
	}

	// spawn the host
	h, err := cloudManager.SpawnInstance(d, so.UserName, true)
	if err != nil {
		return nil, err
	}

	// set the expiration time for the host
	expireTime := h.CreationTime.Add(DefaultExpiration)
	err = h.SetExpirationTime(expireTime)
	if err != nil {
		return h, mci.Logger.Errorf(slogger.ERROR,
			"error setting expiration on host %v: %v", h.Id, err)
	}

	// set the user data, if applicable
	if so.UserData != "" {
		err = h.SetUserData(so.UserData)
		if err != nil {
			return h, mci.Logger.Errorf(slogger.ERROR,
				"Failed setting userData on host %v: %v", h.Id, err)
		}
	}

	// create a hostinit to take care of setting up the host
	init := &hostinit.HostInit{
		MCISettings: sm.mciSettings,
	}

	// for making sure the host doesn't take too long to spawn
	startTime := time.Now()

	// spin until the host is ready for its setup script to be run
	for {

		// make sure we haven't been spinning for too long
		if time.Now().Sub(startTime) > 15*time.Minute {
			if err := h.SetDecommissioned(); err != nil {
				mci.Logger.Logf(slogger.ERROR, "error decommissioning host %v: %v", h.Id, err)
			}
			return nil, fmt.Errorf("host took too long to come up")
		}

		time.Sleep(5000 * time.Millisecond)

		mci.Logger.Logf(slogger.INFO, "Checking if host %v is up and ready", h.Id)

		// see if the host is ready for its setup script to be run
		ready, err := init.IsHostReady(h)
		if err != nil {
			if err := h.SetDecommissioned(); err != nil {
				mci.Logger.Logf(slogger.ERROR, "error decommissioning host %v: %v", h.Id, err)
			}
			return nil, fmt.Errorf("error checking on host %v; decommissioning to save resources: %v",
				h.Id, err)
		}

		// if the host is ready, move on to running the setup script
		if ready {
			break
		}

	}

	mci.Logger.Logf(slogger.INFO, "Host %v is ready for its setup script to be run", h.Id)

	// add any extra user-specified data into the setup script
	if h.Distro.UserData.File != "" {
		userDataCmd := fmt.Sprintf("echo \"%v\" > %v\n",
			strings.Replace(so.UserData, "\"", "\\\"", -1), h.Distro.UserData.File)
		// prepend the setup script to add the userdata file
		if strings.HasPrefix(h.Distro.Setup, "#!") {
			firstLF := strings.Index(h.Distro.Setup, "\n")
			h.Distro.Setup = h.Distro.Setup[0:firstLF+1] + userDataCmd + h.Distro.Setup[firstLF+1:]
		} else {
			h.Distro.Setup = userDataCmd + h.Distro.Setup
		}
	}

	// modify the setup script to add the user's public key
	h.Distro.Setup += fmt.Sprintf("echo \"\n%v\" >> ~%v/.ssh/authorized_keys\n",
		so.PublicKey, h.Distro.User)

	// replace expansions in the script
	exp := command.NewExpansions(init.MCISettings.Expansions)
	h.Distro.Setup, err = exp.ExpandString(h.Distro.Setup)
	if err != nil {
		return nil, fmt.Errorf("expansions error: %v", err)
	}

	// provision the host
	err = init.ProvisionHost(h)
	if err != nil {
		return nil, fmt.Errorf("error provisioning host %v: %v", h.Id, err)
	}

	return h, nil

}
