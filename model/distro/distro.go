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
	Id                string                  `bson:"_id" json:"_id,omitempty" mapstructure:"_id,omitempty"`
	Aliases           []string                `bson:"aliases,omitempty" json:"aliases,omitempty" mapstructure:"aliases,omitempty"`
	Arch              string                  `bson:"arch" json:"arch,omitempty" mapstructure:"arch,omitempty"`
	WorkDir           string                  `bson:"work_dir" json:"work_dir,omitempty" mapstructure:"work_dir,omitempty"`
	PoolSize          int                     `bson:"pool_size,omitempty" json:"pool_size,omitempty" mapstructure:"pool_size,omitempty" yaml:"poolsize"`
	Provider          string                  `bson:"provider" json:"provider,omitempty" mapstructure:"provider,omitempty"`
	ProviderSettings  *map[string]interface{} `bson:"settings" json:"settings,omitempty" mapstructure:"settings,omitempty"`
	SetupAsSudo       bool                    `bson:"setup_as_sudo,omitempty" json:"setup_as_sudo,omitempty" mapstructure:"setup_as_sudo,omitempty"`
	Setup             string                  `bson:"setup,omitempty" json:"setup,omitempty" mapstructure:"setup,omitempty"`
	Teardown          string                  `bson:"teardown,omitempty" json:"teardown,omitempty" mapstructure:"teardown,omitempty"`
	User              string                  `bson:"user,omitempty" json:"user,omitempty" mapstructure:"user,omitempty"`
	BootstrapSettings BootstrapSettings       `bson:"bootstrap_settings" json:"bootstrap_settings" mapstructure:"bootstrap_settings"`
	CloneMethod       string                  `bson:"clone_method" json:"clone_method,omitempty" mapstructure:"clone_method,omitempty"`
	SSHKey            string                  `bson:"ssh_key,omitempty" json:"ssh_key,omitempty" mapstructure:"ssh_key,omitempty"`
	SSHOptions        []string                `bson:"ssh_options,omitempty" json:"ssh_options,omitempty" mapstructure:"ssh_options,omitempty"`
	SpawnAllowed      bool                    `bson:"spawn_allowed" json:"spawn_allowed,omitempty" mapstructure:"spawn_allowed,omitempty"`
	Expansions        []Expansion             `bson:"expansions,omitempty" json:"expansions,omitempty" mapstructure:"expansions,omitempty"`
	Disabled          bool                    `bson:"disabled,omitempty" json:"disabled,omitempty" mapstructure:"disabled,omitempty"`
	ContainerPool     string                  `bson:"container_pool,omitempty" json:"container_pool,omitempty" mapstructure:"container_pool,omitempty"`
	PlannerSettings   PlannerSettings         `bson:"planner_settings" json:"planner_settings,omitempty" mapstructure:"planner_settings,omitempty"`
	FinderSettings    FinderSettings          `bson:"finder_settings" json:"finder_settings,omitempty" mapstructure:"finder_settings,omitempty"`
}

// BootstrapSettings encapsulates all settings related to bootstrapping hosts.
type BootstrapSettings struct {
	Method                string `bson:"method" json:"method" mapstructure:"method"`
	Communication         string `bson:"communcation,omitempty" json:"communication,omitempty" mapstructure:"communcation,omitempty"`
	ClientDir             string `bson:"client_dir,omitempty" json:"client_dir,omitempty" mapstructure:"client_dir,omitempty"`
	CuratorDir            string `bson:"curator_dir,omitempty" json:"curator_dir,omitempty" mapstructure:"curator_dir,omitempty"`
	JasperCredentialsPath string `json:"jasper_credentials_path,omitempty" bson:"jasper_credentials_path,omitempty" mapstructure:"jasper_credentials_path,omitempty"`
	ShellPath             string `bson:"shell_path,omitempty" json:"shell_path,omitempty" mapstructure:"shell_path,omitempty"`
}

// Validate checks if all of the bootstrap settings are valid for legacy or
// non-legacy bootstrapping.
func (s *BootstrapSettings) Validate() error {
	catcher := grip.NewBasicCatcher()
	if !util.StringSliceContains(validBootstrapMethods, s.Method) {
		catcher.Errorf("'%s' is not a valid bootstrap method", s.Method)
	}

	if !util.StringSliceContains(validCommunicationMethods, s.Communication) {
		catcher.Errorf("'%s' is not a valid communication method", s.Communication)
	}

	switch s.Method {
	case BootstrapMethodLegacySSH:
		if s.Communication != CommunicationMethodLegacySSH {
			catcher.New("bootstrapping hosts using legacy SSH is incompatible with non-legacy host communication")
		}
	default:
		if s.Communication == CommunicationMethodLegacySSH {
			catcher.New("communicating with hosts using legacy SSH is incompatible with non-legacy host bootstrapping")
		}
	}

	if s.Method == BootstrapMethodLegacySSH || s.Communication == CommunicationMethodLegacySSH {
		return catcher.Resolve()
	}

	if s.ClientDir == "" {
		catcher.New("client directory cannot be empty for non-legacy bootstrapping")
	}

	if s.CuratorDir == "" {
		catcher.New("curator directory cannot be empty for non-legacy bootstrapping")
	}

	if s.JasperCredentialsPath == "" {
		catcher.New("Jasper credentials path cannot be empty for non-legacy bootstrapping")
	}

	if s.ShellPath == "" {
		catcher.New("shell path cannot be empty for non-legacy bootstrapping")
	}

	return catcher.Resolve()
}

type PlannerSettings struct {
	Version                string        `bson:"version" json:"version" mapstructure:"version"`
	MinimumHosts           int           `bson:"minimum_hosts" json:"minimum_hosts,omitempty" mapstructure:"minimum_hosts,omitempty"`
	MaximumHosts           int           `bson:"maximum_hosts" json:"maximum_hosts,omitempty" mapstructure:"maximum_hosts,omitempty"`
	TargetTime             time.Duration `bson:"target_time" json:"target_time" mapstructure:"target_time,omitempty"`
	AcceptableHostIdleTime time.Duration `bson:"acceptable_host_idle_time" json:"acceptable_host_idle_time" mapstructure:"acceptable_host_idle_time,omitempty"`
	GroupVersions          *bool         `bson:"group_versions" json:"group_versions" mapstructure:"group_versions,omitempty"`
	TaskOrdering           string        `bson:"task_ordering" json:"task_ordering" mapstructure:"task_ordering,omitempty"`
	PatchZipperFactor      int64         `bson:"patch_zipper_factor" json:"patch_zipper_factor" mapstructure:"patch_zipper_factor"`
	TimeInQueueFactor      int64         `bson:"time_in_queue_factor" json:"time_in_queue_factor" mapstructure:"time_in_queue_factor"`
	ExpectedRuntimeFactor  int64         `bson:"expected_runtime_factor" json:"expected_runtime_factor" mapstructure:"expected_runtime_factor"`

	maxDurationPerHost time.Duration
}

type FinderSettings struct {
	Version string `bson:"version" json:"version" mapstructure:"version"`
}

type DistroGroup []Distro

type Expansion struct {
	Key   string `bson:"key,omitempty" json:"key,omitempty"`
	Value string `bson:"value,omitempty" json:"value,omitempty"`
}

const (
	DockerImageBuildTypeImport = "import"
	DockerImageBuildTypePull   = "pull"

	// Recognized architectures, should be in the form ${GOOS}_${GOARCH}.
	ArchDarwinAmd64  = "darwin_amd64"
	ArchLinux386     = "linux_386"
	ArchLinuxPpc64le = "linux_ppc64le"
	ArchLinuxS390x   = "linux_s390x"
	ArchLinuxArm64   = "linux_arm64"
	ArchLinuxAmd64   = "linux_amd64"
	ArchWindows386   = "windows_386"
	ArchWindowsAmd64 = "windows_amd64"

	// Bootstrapping mechanisms
	BootstrapMethodLegacySSH          = "legacy-ssh"
	BootstrapMethodSSH                = "ssh"
	BootstrapMethodPreconfiguredImage = "preconfigured-image"
	BootstrapMethodUserData           = "user-data"

	// Means of communicating with hosts
	CommunicationMethodLegacySSH = "legacy-ssh"
	CommunicationMethodSSH       = "ssh"
	CommunicationMethodRPC       = "rpc"

	CloneMethodLegacySSH = "legacy-ssh"
	CloneMethodOAuth     = "oauth"
)

// validArches includes all recognized architectures.
var validArches = []string{
	ArchDarwinAmd64,
	ArchLinux386,
	ArchLinuxPpc64le,
	ArchLinuxS390x,
	ArchLinuxArm64,
	ArchLinuxAmd64,
	ArchWindows386,
	ArchWindowsAmd64,
}

// validBootstrapMethods includes all recognized bootstrap methods.
var validBootstrapMethods = []string{
	BootstrapMethodLegacySSH,
	BootstrapMethodSSH,
	BootstrapMethodPreconfiguredImage,
	BootstrapMethodUserData,
}

// validCommunicationMethods includes all recognized host communication methods.
var validCommunicationMethods = []string{
	CommunicationMethodLegacySSH,
	CommunicationMethodSSH,
	CommunicationMethodRPC,
}

// validCloneMethods includes all recognized clone methods.
var validCloneMethods = []string{
	CloneMethodLegacySSH,
	CloneMethodOAuth,
}

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

func (d *Distro) ShouldGroupVersions() bool {
	if d.PlannerSettings.GroupVersions == nil {
		return false
	}

	return *d.PlannerSettings.GroupVersions
}

func (d *Distro) GetPatchZipperFactor() int64 {
	if d.PlannerSettings.PatchZipperFactor <= 0 {
		return 1
	}
	return d.PlannerSettings.PatchZipperFactor
}

func (d *Distro) GetTimeInQueueFactor() int64 {
	if d.PlannerSettings.TimeInQueueFactor <= 0 {
		return 1
	}
	return d.PlannerSettings.TimeInQueueFactor
}

func (d *Distro) GetExpectedRuntimeFactor() int64 {
	if d.PlannerSettings.ExpectedRuntimeFactor <= 0 {
		return 1
	}

	return d.PlannerSettings.ExpectedRuntimeFactor
}

func (d *Distro) MaxDurationPerHost() time.Duration {
	if d.PlannerSettings.maxDurationPerHost != 0 {
		return d.PlannerSettings.maxDurationPerHost
	}

	if d.ContainerPool != "" {
		return evergreen.MaxDurationPerDistroHostWithContainers
	}

	return evergreen.MaxDurationPerDistroHost
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

// HomeDir gets the absolute path to the home directory for this distro's user.
func (d *Distro) HomeDir() string {
	if d.User == "root" {
		return filepath.Join("/", d.User)
	}
	if d.Arch == ArchDarwinAmd64 {
		return filepath.Join("/Users", d.User)
	}
	return filepath.Join("/home", d.User)
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
	case evergreen.ProviderNameEc2Fleet:
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

func (d *Distro) GetPoolSize() int {
	switch d.Provider {
	case evergreen.ProviderNameStatic:
		if d.ProviderSettings == nil {
			return 0
		}

		hosts, ok := (*d.ProviderSettings)["hosts"].([]interface{})
		if !ok {
			return 0
		}

		return len(hosts)
	default:
		return d.PoolSize + d.PlannerSettings.MaximumHosts
	}

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

// ValidateArch checks that the architecture is one of the supported
// architectures.
func ValidateArch(arch string) error {
	osAndArch := strings.Split(arch, "_")
	if len(osAndArch) != 2 {
		return errors.Errorf("architecture '%s' is not in the form ${GOOS}_${GOARCH}", arch)
	}

	if !util.StringSliceContains(validArches, arch) {
		return errors.Errorf("'%s' is not a recognized architecture", arch)
	}
	return nil
}

// ValidateCloneMethod checks that the clone mechanism is one of the supported
// methods.
func ValidateCloneMethod(method string) error {
	if !util.StringSliceContains(validCloneMethods, method) {
		return errors.Errorf("'%s' is not a valid clone method", method)
	}
	return nil
}

// GetDistroIds returns a slice of distro IDs for the given group of distros
func (distros DistroGroup) GetDistroIds() []string {
	var ids []string
	for _, d := range distros {
		ids = append(ids, d.Id)
	}
	return ids
}

// GetResolvedPlannerSettings combines the distro's PlannerSettings fields with the
// SchedulerConfig defaults to resolve and validate a canonical set of PlannerSettings' field values.
func (d *Distro) GetResolvedPlannerSettings(s *evergreen.Settings) (PlannerSettings, error) {
	config := s.Scheduler
	ps := d.PlannerSettings
	resolved := PlannerSettings{
		Version:                ps.Version,
		MinimumHosts:           ps.MinimumHosts,
		MaximumHosts:           ps.MaximumHosts,
		TargetTime:             ps.TargetTime,
		AcceptableHostIdleTime: ps.AcceptableHostIdleTime,
		GroupVersions:          ps.GroupVersions,
		TaskOrdering:           ps.TaskOrdering,
		PatchZipperFactor:      ps.PatchZipperFactor,
		TimeInQueueFactor:      ps.TimeInQueueFactor,
		ExpectedRuntimeFactor:  ps.ExpectedRuntimeFactor,
		maxDurationPerHost:     evergreen.MaxDurationPerDistroHost,
	}

	catcher := grip.NewBasicCatcher()
	catcher.Add(config.ValidateAndDefault())

	if d.ContainerPool != "" {
		if s.ContainerPools.GetContainerPool(d.ContainerPool) == nil {
			catcher.Errorf("could not find pool '%s' for distro '%s'", d.ContainerPool, d.Id)
		}
		resolved.maxDurationPerHost = evergreen.MaxDurationPerDistroHostWithContainers
	}

	// Validate the resolved PlannerSettings.Version
	if resolved.Version == "" {
		resolved.Version = config.Planner
	}

	if !util.StringSliceContains(evergreen.ValidPlannerVersions, resolved.Version) {
		catcher.Errorf("'%s' is not a valid PlannerSettings.Version", resolved.Version)
	}
	// Validate the PlannerSettings.MinimumHosts and PlannerSettings.MaximumHosts
	if resolved.MinimumHosts < 0 {
		catcher.Errorf("%d is not a valid PlannerSettings.MinimumHosts", resolved.MinimumHosts)
	}
	if resolved.MaximumHosts < 0 {
		catcher.Errorf("%d is not a valid PlannerSettings.MaximumHosts", resolved.MaximumHosts)
	}
	// Resolve PlannerSettings.TargetTime and PlannerSettings.AcceptableHostIdleTime
	if resolved.TargetTime == 0 {
		resolved.TargetTime = time.Duration(config.TargetTimeSeconds) * time.Second
	}
	if resolved.AcceptableHostIdleTime == 0 {
		resolved.AcceptableHostIdleTime = time.Duration(config.AcceptableHostIdleTimeSeconds) * time.Second
	}
	// Resolve whether to PlannerSettings.GroupVersions, or otherwise
	if resolved.GroupVersions == nil {
		resolved.GroupVersions = &config.GroupVersions
	}
	if resolved.PatchZipperFactor == 0 {
		resolved.PatchZipperFactor = config.PatchZipperFactor
	}
	if resolved.TimeInQueueFactor == 0 {
		resolved.TimeInQueueFactor = config.TimeInQueueFactor
	}
	if resolved.ExpectedRuntimeFactor == 0 {
		resolved.ExpectedRuntimeFactor = config.ExpectedRuntimeFactor
	}

	// Resolve and validate the PlannerSettings.TaskOrdering
	if resolved.TaskOrdering == "" {
		resolved.TaskOrdering = config.TaskOrdering
	}

	if !util.StringSliceContains(evergreen.ValidTaskOrderings, resolved.TaskOrdering) {
		catcher.Errorf("'%s' is not a valid PlannerSettings.TaskOrdering", resolved.TaskOrdering)
	}

	// Any validation errors?
	if catcher.HasErrors() {
		return PlannerSettings{}, errors.Wrapf(catcher.Resolve(), "cannot resolve PlannerSettings for distro '%s'", d.Id)
	}

	d.PlannerSettings = resolved
	return resolved, nil
}
