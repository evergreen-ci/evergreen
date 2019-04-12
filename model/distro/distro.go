package distro

import (
	"fmt"
	"math/rand"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type Distro struct {
	Id               string                  `bson:"_id" json:"_id,omitempty" mapstructure:"_id,omitempty"`
	Arch             string                  `bson:"arch" json:"arch,omitempty" mapstructure:"arch,omitempty"`
	WorkDir          string                  `bson:"work_dir" json:"work_dir,omitempty" mapstructure:"work_dir,omitempty"`
	PoolSize         int                     `bson:"pool_size,omitempty" json:"pool_size,omitempty" mapstructure:"pool_size,omitempty" yaml:"poolsize"`
	Provider         string                  `bson:"provider" json:"provider,omitempty" mapstructure:"provider,omitempty"`
	ProviderSettings *map[string]interface{} `bson:"settings" json:"settings,omitempty" mapstructure:"settings,omitempty"`
	SetupAsSudo      bool                    `bson:"setup_as_sudo,omitempty" json:"setup_as_sudo,omitempty" mapstructure:"setup_as_sudo,omitempty"`
	Setup            string                  `bson:"setup,omitempty" json:"setup,omitempty" mapstructure:"setup,omitempty"`
	Teardown         string                  `bson:"teardown,omitempty" json:"teardown,omitempty" mapstructure:"teardown,omitempty"`
	User             string                  `bson:"user,omitempty" json:"user,omitempty" mapstructure:"user,omitempty"`
	BootstrapMethod  string                  `bson:"bootstrap_method,omitempty" json:"bootstrap_method,omitempty" mapstructure:"bootstrap_method,omitempty"`
	SSHKey           string                  `bson:"ssh_key,omitempty" json:"ssh_key,omitempty" mapstructure:"ssh_key,omitempty"`
	SSHOptions       []string                `bson:"ssh_options,omitempty" json:"ssh_options,omitempty" mapstructure:"ssh_options,omitempty"`
	SpawnAllowed     bool                    `bson:"spawn_allowed" json:"spawn_allowed,omitempty" mapstructure:"spawn_allowed,omitempty"`
	Expansions       []Expansion             `bson:"expansions,omitempty" json:"expansions,omitempty" mapstructure:"expansions,omitempty"`
	Disabled         bool                    `bson:"disabled,omitempty" json:"disabled,omitempty" mapstructure:"disabled,omitempty"`
	ContainerPool    string                  `bson:"container_pool,omitempty" json:"container_pool,omitempty" mapstructure:"container_pool,omitempty"`
	PlannerSettings  PlannerSettings         `bson:"planner_settings" json:"planner_settings,omitempty" mapstructure:"planner_settings,omitempty"`
}

type PlannerSettings struct {
	Version                string        `bson:"version" json:"version" mapstructure:"version"`
	MinimumHosts           int           `bson:"minimum_hosts" json:"minimum_hosts,omitempty" mapstructure:"minimum_hosts,omitempty"`
	MaximumHosts           int           `bson:"maximum_hosts" json:"maximum_hosts,omitempty" mapstructure:"maximum_hosts,omitempty"`
	TargetTime             time.Duration `bson:"target_time" json:"target_time,omitempty" mapstructure:"target_time,omitempty"`
	AcceptableHostIdleTime time.Duration `bson:"acceptable_host_idle_time" json:"acceptable_host_idle_time,omitempty" mapstructure:"acceptable_host_idle_time,omitempty"`
	GroupVersions          bool          `bson:"group_versions" json:"group_versions,omitempty" mapstructure:"group_versions,omitempty"`
	PatchZipperFactor      int           `bson:"patch_zipper_factor" json:"patch_zipper_factor,omitempty" mapstructure:"patch_zipper_factor,omitempty"`
	MainlineFirst          bool          `bson:"mainline_first" json:"mainline_first,omitempty" mapstructure:"mainline_first,omitempty"`
	PatchFirst             bool          `bson:"patch_first" json:"patch_first,omitempty" mapstructure:"patch_first,omitempty"`
}

type DistroGroup []Distro

type Expansion struct {
	Key   string `bson:"key,omitempty" json:"key,omitempty"`
	Value string `bson:"value,omitempty" json:"value,omitempty"`
}

const (
	DockerImageBuildTypeImport = "import"
	DockerImageBuildTypePull   = "pull"

	// Bootstrapping mechanisms
	BootstrapMethodLegacySSH          = "legacy-ssh"
	BootstrapMethodSSH                = "ssh"
	BootstrapMethodPreconfiguredImage = "preconfigured-image"
	BootstrapMethodUserData           = "user-data"
)

// Seed the random number generator for creating distro names
func init() {
	rand.Seed(time.Now().UnixNano())
}

// GenerateName generates a unique instance name for a distro.
func (d *Distro) GenerateName() string {
	// gceMaxNameLength is the maximum length of an instance name permitted by GCE.
	const gceMaxNameLength = 63

	switch d.Provider {
	case evergreen.ProviderNameStatic:
		return "static"
	case evergreen.ProviderNameDocker:
		return fmt.Sprintf("container-%d", rand.New(rand.NewSource(time.Now().UnixNano())).Int())
	}

	name := fmt.Sprintf("evg-%s-%s-%d", d.Id, time.Now().Format(evergreen.NameTimeFormat), rand.Int())

	if d.Provider == evergreen.ProviderNameGce {
		// Ensure all characters in tags are on the whitelist
		r, _ := regexp.Compile("[^a-z0-9_-]+")
		name = string(r.ReplaceAll([]byte(strings.ToLower(name)), []byte("")))

		// Ensure the new name's is no longer than gceMaxNameLength
		if len(name) > gceMaxNameLength {
			name = name[:gceMaxNameLength]
		}
	}

	return name
}

func (d *Distro) IsWindows() bool {
	// XXX: if this is-windows check is updated, make sure to also update
	// public/static/js/spawned_hosts.js as well
	return strings.Contains(d.Arch, "windows")
}

func (d *Distro) Platform() (string, string) {
	osAndArch := strings.Split(d.Arch, "_")
	return osAndArch[0], osAndArch[1]
}

func (d *Distro) IsEphemeral() bool {
	return util.StringSliceContains(evergreen.ProviderSpawnable, d.Provider)
}

func (d *Distro) BinaryName() string {
	name := "evergreen"
	if d.IsWindows() {
		return name + ".exe"
	}
	return name
}

// ExecutableSubPath returns the directory containing the compiled agents.
func (d *Distro) ExecutableSubPath() string {
	return filepath.Join(d.Arch, d.BinaryName())
}

// IsParent returns whether the distro is the parent distro for any container pool
func (d *Distro) IsParent(s *evergreen.Settings) bool {
	if s == nil {
		var err error
		s, err = evergreen.GetConfig()
		if err != nil {
			grip.Critical("error retrieving settings object")
			return false
		}
	}
	for _, p := range s.ContainerPools.Pools {
		if d.Id == p.Distro {
			return true
		}
	}
	return false
}

func (d *Distro) GetImageID() (string, error) {
	var i interface{}
	switch d.Provider {
	case evergreen.ProviderNameEc2Auto:
		i = (*d.ProviderSettings)["ami"]
	case evergreen.ProviderNameEc2OnDemand:
		i = (*d.ProviderSettings)["ami"]
	case evergreen.ProviderNameEc2Spot:
		i = (*d.ProviderSettings)["ami"]
	case evergreen.ProviderNameDocker:
		i = (*d.ProviderSettings)["image_url"]
	case evergreen.ProviderNameDockerMock:
		i = (*d.ProviderSettings)["image_url"]
	case evergreen.ProviderNameGce:
		i = (*d.ProviderSettings)["image_name"]
	case evergreen.ProviderNameVsphere:
		i = (*d.ProviderSettings)["template"]
	case evergreen.ProviderNameMock:
		return "", nil
	case evergreen.ProviderNameStatic:
		return "", nil
	case evergreen.ProviderNameOpenstack:
		return "", nil
	default:
		return "", errors.New("unknown provider name")
	}

	s, ok := i.(string)
	if !ok {
		return "", errors.New("cannot extract image ID from provider settings")
	}
	return s, nil
}

// ValidateContainerPoolDistros ensures that container pools have valid distros
func ValidateContainerPoolDistros(s *evergreen.Settings) error {
	catcher := grip.NewSimpleCatcher()

	for _, pool := range s.ContainerPools.Pools {
		d, err := FindOne(ById(pool.Distro))
		if err != nil {
			catcher.Add(fmt.Errorf("error finding distro for container pool %s", pool.Id))
		}
		if d.ContainerPool != "" {
			catcher.Add(fmt.Errorf("container pool %s has invalid distro", pool.Id))
		}
	}
	return errors.WithStack(catcher.Resolve())
}

// ValidateBootstrapMethod ensure that the bootstrap mechanism is one of the
// supported methods.
func ValidateBootstrapMethod(method string) error {
	switch method {
	case BootstrapMethodLegacySSH, BootstrapMethodSSH, BootstrapMethodPreconfiguredImage, BootstrapMethodUserData:
		return nil
	default:
		return fmt.Errorf("'%s' is not a valid bootstrap method", method)
	}
}

// GetDistroIds returns a slice of distro IDs for the given group of distros
func (distros DistroGroup) GetDistroIds() []string {
	var ids []string
	for _, d := range distros {
		ids = append(ids, d.Id)
	}
	return ids
}
