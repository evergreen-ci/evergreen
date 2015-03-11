package model

import (
	"10gen.com/mci"
	"10gen.com/mci/db/bsonutil"
	"10gen.com/mci/model/host"
	"10gen.com/mci/util"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Distro struct {
	Name           string                 `bson:"_id"`
	Arch           string                 `bson:"arch"`
	SpawnAllowed   bool                   `bson:"spawn_allowed" yaml:"spawn_allowed"`
	SpawnUserData  SpawnUserData          `bson:"spawn_userdata" yaml:"spawn_userdata"`
	Provider       string                 `bson:"provider"`
	ProviderConfig map[string]interface{} `yaml:"settings" json:"-"`
	SSHOptions     []string               `yaml:"ssh_opts" json:"-"`

	// whether or not the distro is a just a template for creating other distros
	Template bool `yaml:"template"`

	UserData     string            `bson:"user_data"`
	Key          string            `bson:"key" yaml:"key"`
	Setup        string            `bson:"setup"`
	SetupMciOnly string            `bson:"setup_mci_only" yaml:"setup_mci_only"`
	SetupAsSudo  bool              `bson:"setup_as_sudo" yaml:"setup_as_sudo"`
	User         string            `bson:"user"`
	Hosts        []string          `bson:"hosts"`
	MaxHosts     int               `bson:"max_hosts"`
	Expansions   map[string]string `bson:"expansions"`
}

type ValidateFormat string

const (
	UserDataFormatFormURLEncoded = "x-www-form-urlencoded"
	UserDataFormatJSON           = "json"
	UserDataFormatYAML           = "yaml"
)

type SpawnUserData struct {
	File     string         `bson:"file"`
	Validate ValidateFormat `bson:"validate"`
}

var (
	// bson fields for the distro struct
	DistroIdKey           = bsonutil.MustHaveTag(Distro{}, "Name")
	DistroArchKey         = bsonutil.MustHaveTag(Distro{}, "Arch")
	DistroSpawnAllowedKey = bsonutil.MustHaveTag(Distro{}, "SpawnAllowed")
	DistroSetupKey        = bsonutil.MustHaveTag(Distro{}, "Setup")
	DistroSetupMCIOnlyKey = bsonutil.MustHaveTag(Distro{}, "SetupMciOnly")
	DistroSetupAsSudoKey  = bsonutil.MustHaveTag(Distro{}, "SetupAsSudo")
	DistroKeyKey          = bsonutil.MustHaveTag(Distro{}, "Key")
	DistroUserKey         = bsonutil.MustHaveTag(Distro{}, "User")
	DistroHostsKey        = bsonutil.MustHaveTag(Distro{}, "Hosts")
	DistroMaxHostsKey     = bsonutil.MustHaveTag(Distro{}, "MaxHosts")
	DistroExpansionsKey   = bsonutil.MustHaveTag(Distro{}, "Expansions")

	// bson fields for the spawn userdata struct
	SpawnUserDataFileKey     = bsonutil.MustHaveTag(SpawnUserData{}, "File")
	SpawnUserDataValidateKey = bsonutil.MustHaveTag(SpawnUserData{}, "Validate")
)

const (
	DistrosCollection = "distros"
)

// Make sure the static hosts stored in the database are correct.
func RefreshStaticHosts(configName string) error {

	distros, err := LoadDistros(configName)
	if err != nil {
		return err
	}

	activeStaticHosts := make([]string, 0)
	for _, distro := range distros {
		for _, hostId := range distro.Hosts {
			hostInfo, err := util.ParseSSHInfo(hostId)
			if err != nil {
				return err
			}
			user := hostInfo.User
			if user == "" {
				user = distro.User
			}
			staticHost := host.Host{
				Id:           hostId,
				User:         user,
				Host:         hostId,
				Distro:       distro.Name,
				CreationTime: time.Now(),
				Provider:     mci.HostTypeStatic,
				StartedBy:    mci.MCIUser,
				InstanceType: "",
				Status:       mci.HostRunning,
				Provisioned:  true,
			}

			// upsert the host
			_, err = staticHost.Upsert()
			if err != nil {
				return err
			}
			activeStaticHosts = append(activeStaticHosts, hostId)
		}
	}
	return host.DecommissionInactiveStaticHosts(activeStaticHosts)
}

// Given the name of the directory we are using for configs, return the
// location of the directory when the distro specs live.
func locateDistrosDirectory(configDir string) (string, error) {
	fullConfigDirPath, err := mci.FindMCIConfig(configDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(fullConfigDirPath, "distros"), nil
}

// Load a single distro from the specified config directory.  Locates the
// directory on the filesystem where the distro specs live, based on the name
// of the config directory, and finds the specified distro within it.
func LoadOneDistro(configDir, distroName string) (*Distro, error) {

	// find the directory where the distro specs live
	distrosDir, err := locateDistrosDirectory(configDir)
	if err != nil {
		return nil, fmt.Errorf("error locating distros directory: %v", err)
	}

	// load the distro
	return LoadOneDistroFromDirectory(distrosDir, distroName)
}

// Load in an individual distro from the specified directory. Walks over all
// of the .yml files in the directory until it finds the distro in one of them.
// Returns an error if there is an error reading any of the files, or if the
// distro does not exist.
func LoadOneDistroFromDirectory(directory, distroName string) (*Distro, error) {

	// to be returned
	var distro *Distro

	// for short-circuiting the Walk() func if the distro has been found
	found := false

	// walk the files in the directory, unmarshalling distros until the one
	// we want is found
	err := filepath.Walk(
		directory,
		func(path string, info os.FileInfo, err error) error {
			// if we've already found the distro in a previous file, don't
			// bother reading in this one
			// TODO: this means we won't catch errors if a distro is defined
			// twice
			if found {
				return nil
			}

			// only use .yml files
			if strings.HasSuffix(path, ".yml") {

				// get the distros
				fileDistros, err := LoadDistrosFromFile(path)
				if err != nil {
					return err
				}

				// found it
				if desiredDistro, ok := fileDistros[distroName]; ok {
					distro = &desiredDistro
					found = true
				}

			}

			return nil
		},
	)

	if err != nil {
		return nil, err
	}

	if distro == nil {
		return nil, fmt.Errorf("distro %v could not be found in directory %v",
			distroName, directory)
	}

	return distro, nil
}

// Load all distros from the specified config directory.  Locates the directory
// on the filesystem where the distros live, based on the name of the config
// directory, and loads all distros from it.
func LoadDistros(configDir string) (map[string]Distro, error) {

	// find the directory where the distro specs live
	distrosDir, err := locateDistrosDirectory(configDir)
	if err != nil {
		return nil, fmt.Errorf("error locating distros directory: %v", err)
	}

	// load all the distros from that directory
	return LoadDistrosFromDirectory(distrosDir)
}

// Walk all of the .yml files in the specified directory, and load in all
// of the distros specified in them.
func LoadDistrosFromDirectory(directory string) (map[string]Distro, error) {
	distros := map[string]Distro{}

	// walk all of the files in the directory, unmarshalling the distros from
	// the .yml files and saving them
	err := filepath.Walk(
		directory,
		func(path string, info os.FileInfo, err error) error {
			// only use .yml files
			if strings.HasSuffix(path, ".yml") {

				// get the distros
				fileDistros, err := LoadDistrosFromFile(path)
				if err != nil {
					return err
				}

				// add them to the master list
				for k, v := range fileDistros {
					// ignore distro templates
					if !v.Template {
						distros[k] = v
					}
				}
			}
			return nil
		},
	)

	if err != nil {
		return nil, err
	}

	return distros, nil
}

// Load in all of the distros from the specified file. Returns a map of
// name -> distro as specified in the file, as well as an error if any
// occurs
func LoadDistrosFromFile(file string) (map[string]Distro, error) {

	// read in the file into a map of distros
	distros := map[string]Distro{}
	err := util.UnmarshalYAMLFile(file, distros)
	if err != nil {
		return nil, fmt.Errorf("error loading distros from file %v: %v", file,
			err)
	}

	return distros, nil

}
