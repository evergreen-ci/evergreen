package distro

import (
	"fmt"
	"math/rand"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	mgobson "gopkg.in/mgo.v2/bson"
)

type Distro struct {
	Id                    string                  `bson:"_id" json:"_id,omitempty" mapstructure:"_id,omitempty"`
	Aliases               []string                `bson:"aliases,omitempty" json:"aliases,omitempty" mapstructure:"aliases,omitempty"`
	Arch                  string                  `bson:"arch" json:"arch,omitempty" mapstructure:"arch,omitempty"`
	WorkDir               string                  `bson:"work_dir" json:"work_dir,omitempty" mapstructure:"work_dir,omitempty"`
	Provider              string                  `bson:"provider" json:"provider,omitempty" mapstructure:"provider,omitempty"`
	ProviderSettings      *map[string]interface{} `bson:"settings" json:"settings,omitempty" mapstructure:"settings,omitempty"`
	ProviderSettingsList  []*birch.Document       `bson:"provider_settings,omitempty" json:"provider_settings,omitempty" mapstructure:"provider_settings,omitempty"`
	SetupAsSudo           bool                    `bson:"setup_as_sudo,omitempty" json:"setup_as_sudo,omitempty" mapstructure:"setup_as_sudo,omitempty"`
	Setup                 string                  `bson:"setup,omitempty" json:"setup,omitempty" mapstructure:"setup,omitempty"`
	Teardown              string                  `bson:"teardown,omitempty" json:"teardown,omitempty" mapstructure:"teardown,omitempty"`
	User                  string                  `bson:"user,omitempty" json:"user,omitempty" mapstructure:"user,omitempty"`
	BootstrapSettings     BootstrapSettings       `bson:"bootstrap_settings" json:"bootstrap_settings" mapstructure:"bootstrap_settings"`
	CloneMethod           string                  `bson:"clone_method" json:"clone_method,omitempty" mapstructure:"clone_method,omitempty"`
	SSHKey                string                  `bson:"ssh_key,omitempty" json:"ssh_key,omitempty" mapstructure:"ssh_key,omitempty"`
	SSHOptions            []string                `bson:"ssh_options,omitempty" json:"ssh_options,omitempty" mapstructure:"ssh_options,omitempty"`
	AuthorizedKeysFile    string                  `bson:"authorized_keys_file,omitempty" json:"authorized_keys_file,omitempty" mapstructure:"authorized_keys_file,omitempty"`
	SpawnAllowed          bool                    `bson:"spawn_allowed" json:"spawn_allowed,omitempty" mapstructure:"spawn_allowed,omitempty"`
	Expansions            []Expansion             `bson:"expansions,omitempty" json:"expansions,omitempty" mapstructure:"expansions,omitempty"`
	Disabled              bool                    `bson:"disabled,omitempty" json:"disabled,omitempty" mapstructure:"disabled,omitempty"`
	ContainerPool         string                  `bson:"container_pool,omitempty" json:"container_pool,omitempty" mapstructure:"container_pool,omitempty"`
	FinderSettings        FinderSettings          `bson:"finder_settings" json:"finder_settings" mapstructure:"finder_settings"`
	PlannerSettings       PlannerSettings         `bson:"planner_settings" json:"planner_settings" mapstructure:"planner_settings"`
	DispatcherSettings    DispatcherSettings      `bson:"dispatcher_settings" json:"dispatcher_settings" mapstructure:"dispatcher_settings"`
	HostAllocatorSettings HostAllocatorSettings   `bson:"host_allocator_settings" json:"host_allocator_settings" mapstructure:"host_allocator_settings"`
	DisableShallowClone   bool                    `bson:"disable_shallow_clone" json:"disable_shallow_clone" mapstructure:"disable_shallow_clone"`
	UseLegacyAgent        bool                    `bson:"use_legacy_agent" json:"use_legacy_agent" mapstructure:"use_legacy_agent"`
	Note                  string                  `bson:"note" json:"note" mapstructure:"note"`
	ValidProjects         []string                `bson:"valid_projects,omitempty" json:"valid_projects,omitempty" mapstructure:"valid_projects,omitempty"`
	IsVirtualWorkstation  bool                    `bson:"is_virtual_workstation" json:"is_virtual_workstation" mapstructure:"is_virtual_workstation"`
	HomeVolumeSettings    HomeVolumeSettings      `bson:"home_volume_settings" json:"home_volume_settings" mapstructure:"home_volume_settings"`
}

// BootstrapSettings encapsulates all settings related to bootstrapping hosts.
type BootstrapSettings struct {
	// Required
	Method        string `bson:"method" json:"method" mapstructure:"method"`
	Communication string `bson:"communication,omitempty" json:"communication,omitempty" mapstructure:"communication,omitempty"`

	// Required for new provisioning
	ClientDir             string `bson:"client_dir,omitempty" json:"client_dir,omitempty" mapstructure:"client_dir,omitempty"`
	JasperBinaryDir       string `bson:"jasper_binary_dir,omitempty" json:"jasper_binary_dir,omitempty" mapstructure:"jasper_binary_dir,omitempty"`
	JasperCredentialsPath string `json:"jasper_credentials_path,omitempty" bson:"jasper_credentials_path,omitempty" mapstructure:"jasper_credentials_path,omitempty"`

	// Windows-specific
	ServiceUser string `bson:"service_user,omitempty" json:"service_user,omitempty" mapstructure:"service_user,omitempty"`
	ShellPath   string `bson:"shell_path,omitempty" json:"shell_path,omitempty" mapstructure:"shell_path,omitempty"`
	RootDir     string `bson:"root_dir,omitempty" json:"root_dir,omitempty" mapstructure:"root_dir,omitempty"`

	Env []EnvVar `bson:"env,omitempty" json:"env,omitempty" mapstructure:"env,omitempty"`

	// Linux-specific
	ResourceLimits ResourceLimits `bson:"resource_limits,omitempty" json:"resource_limits,omitempty" mapstructure:"resource_limits,omitempty"`
}

type EnvVar struct {
	Key   string `bson:"key" json:"key"`
	Value string `bson:"value" json:"value"`
}

// ResourceLimits represents resource limits in Linux.
type ResourceLimits struct {
	NumFiles        int `bson:"num_files,omitempty" json:"num_files,omitempty" mapstructure:"num_files,omitempty"`
	NumProcesses    int `bson:"num_processes,omitempty" json:"num_processes,omitempty" mapstructure:"num_processes,omitempty"`
	LockedMemoryKB  int `bson:"locked_memory,omitempty" json:"locked_memory,omitempty" mapstructure:"locked_memory,omitempty"`
	VirtualMemoryKB int `bson:"virtual_memory,omitempty" json:"virtual_memory,omitempty" mapstructure:"virtual_memory,omitempty"`
}

type HomeVolumeSettings struct {
	DeviceName    string `bson:"device_name" json:"device_name" mapstructure:"device_name"`
	FormatCommand string `bson:"format_command" json:"format_command" mapstructure:"format_command"`
}

func (d *Distro) SetBSON(raw mgobson.Raw) error {
	return bson.Unmarshal(raw.Data, d)
}

// ValidateBootstrapSettings checks if all of the bootstrap settings are valid
// for legacy or non-legacy bootstrapping.
func (d *Distro) ValidateBootstrapSettings() error {
	catcher := grip.NewBasicCatcher()
	if !util.StringSliceContains(validBootstrapMethods, d.BootstrapSettings.Method) {
		catcher.Errorf("'%s' is not a valid bootstrap method", d.BootstrapSettings.Method)
	}

	if d.BootstrapSettings.Method == BootstrapMethodNone {
		return catcher.Resolve()
	}

	if !util.StringSliceContains(validCommunicationMethods, d.BootstrapSettings.Communication) {
		catcher.Errorf("'%s' is not a valid communication method", d.BootstrapSettings.Communication)
	}

	switch d.BootstrapSettings.Method {
	case BootstrapMethodLegacySSH:
		catcher.NewWhen(d.BootstrapSettings.Communication != CommunicationMethodLegacySSH, "bootstrapping hosts using legacy SSH is incompatible with non-legacy host communication")
	default:
		catcher.NewWhen(d.BootstrapSettings.Communication == CommunicationMethodLegacySSH, "communicating with hosts using legacy SSH is incompatible with non-legacy host bootstrapping")
	}

	if d.BootstrapSettings.Method == BootstrapMethodLegacySSH || d.BootstrapSettings.Communication == CommunicationMethodLegacySSH {
		return catcher.Resolve()
	}

	for _, envVar := range d.BootstrapSettings.Env {
		catcher.NewWhen(envVar.Key == "", "environment variable key cannot be empty")
	}

	catcher.NewWhen(d.BootstrapSettings.ClientDir == "", "client directory cannot be empty for non-legacy bootstrapping")
	catcher.NewWhen(d.BootstrapSettings.JasperBinaryDir == "", "Jasper binary directory cannot be empty for non-legacy bootstrapping")
	catcher.NewWhen(d.BootstrapSettings.JasperCredentialsPath == "", "Jasper credentials path cannot be empty for non-legacy bootstrapping")
	catcher.NewWhen(d.BootstrapSettings.ShellPath == "", "shell path cannot be empty for non-legacy Windows bootstrapping")

	catcher.NewWhen(d.IsWindows() && d.BootstrapSettings.ServiceUser == "", "service user cannot be empty for non-legacy Windows bootstrapping")
	catcher.NewWhen(d.IsWindows() && d.BootstrapSettings.RootDir == "", "root directory cannot be empty for non-legacy Windows bootstrapping")

	catcher.NewWhen(d.IsLinux() && d.BootstrapSettings.ResourceLimits.NumFiles < -1, "max number of files should be a positive number or -1")
	catcher.NewWhen(d.IsLinux() && d.BootstrapSettings.ResourceLimits.NumProcesses < -1, "max number of files should be a positive number or -1")
	catcher.NewWhen(d.IsLinux() && d.BootstrapSettings.ResourceLimits.LockedMemoryKB < -1, "max locked memory should be a positive number or -1")
	catcher.NewWhen(d.IsLinux() && d.BootstrapSettings.ResourceLimits.VirtualMemoryKB < -1, "max virtual memory should be a positive number or -1")

	return catcher.Resolve()
}

// ShellPath returns the native path to the shell binary.
func (d *Distro) ShellBinary() string {
	return filepath.Join(d.BootstrapSettings.RootDir, d.BootstrapSettings.ShellPath)
}

type HostAllocatorSettings struct {
	Version                string        `bson:"version" json:"version" mapstructure:"version"`
	MinimumHosts           int           `bson:"minimum_hosts" json:"minimum_hosts" mapstructure:"minimum_hosts"`
	MaximumHosts           int           `bson:"maximum_hosts" json:"maximum_hosts" mapstructure:"maximum_hosts"`
	AcceptableHostIdleTime time.Duration `bson:"acceptable_host_idle_time" json:"acceptable_host_idle_time" mapstructure:"acceptable_host_idle_time"`
}

type FinderSettings struct {
	Version string `bson:"version" json:"version" mapstructure:"version"`
}

type PlannerSettings struct {
	Version                   string        `bson:"version" json:"version" mapstructure:"version"`
	TargetTime                time.Duration `bson:"target_time" json:"target_time" mapstructure:"target_time,omitempty"`
	GroupVersions             *bool         `bson:"group_versions" json:"group_versions" mapstructure:"group_versions,omitempty"`
	PatchFactor               int64         `bson:"patch_zipper_factor" json:"patch_factor" mapstructure:"patch_factor"`
	PatchTimeInQueueFactor    int64         `bson:"patch_time_in_queue_factor" json:"patch_time_in_queue_factor" mapstructure:"patch_time_in_queue_factor"`
	MainlineTimeInQueueFactor int64         `bson:"mainline_time_in_queue_factor" json:"mainline_time_in_queue_factor" mapstructure:"mainline_time_in_queue_factor"`
	ExpectedRuntimeFactor     int64         `bson:"expected_runtime_factor" json:"expected_runtime_factor" mapstructure:"expected_runtime_factor"`

	maxDurationPerHost time.Duration
}

type DispatcherSettings struct {
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
	// BootstrapMethodNone is for internal use only.
	BootstrapMethodNone               = "none"
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
	BootstrapMethodNone,
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

func (d *Distro) GetPatchFactor() int64 {
	if d.PlannerSettings.PatchFactor <= 0 {
		return 1
	}
	return d.PlannerSettings.PatchFactor
}

func (d *Distro) GetPatchTimeInQueueFactor() int64 {
	if d.PlannerSettings.PatchTimeInQueueFactor <= 0 {
		return 1
	}
	return d.PlannerSettings.PatchTimeInQueueFactor
}

func (d *Distro) GetMainlineTimeInQueueFactor() int64 {
	if d.PlannerSettings.MainlineTimeInQueueFactor <= 0 {
		return 1
	}
	return d.PlannerSettings.MainlineTimeInQueueFactor
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

func (d *Distro) GetTargetTime() time.Duration {
	if d.PlannerSettings.TargetTime == 0 {
		return d.MaxDurationPerHost()
	}

	return d.PlannerSettings.TargetTime
}

// IsPowerShellSetup returns whether or not the setup script is a powershell
// script based on the header shebang line.
func (d *Distro) IsPowerShellSetup() bool {
	start := strings.Index(d.Setup, "#!")
	if start == -1 {
		return false
	}
	end := strings.IndexByte(d.Setup[start:], '\n')
	if end == -1 {
		return false
	}
	end += start
	return strings.Contains(d.Setup[start:end], "powershell")
}

func (d *Distro) IsWindows() bool {
	// XXX: if this is-windows check is updated, make sure to also update
	// public/static/js/spawned_hosts.js as well
	return strings.Contains(d.Arch, "windows")
}

func (d *Distro) IsLinux() bool {
	return strings.Contains(d.Arch, "linux")
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
	arch := d.Arch
	if d.UseLegacyAgent {
		arch += "_legacy"
	}
	return filepath.Join(arch, d.BinaryName())
}

// HomeDir gets the absolute path to the home directory for this distro's user
// for non-legacy provisioned hosts.
func (d *Distro) HomeDir() string {
	if d.LegacyBootstrap() {
		return "~"
	}
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
	key := ""

	switch d.Provider {
	case evergreen.ProviderNameEc2Auto, evergreen.ProviderNameEc2OnDemand, evergreen.ProviderNameEc2Spot, evergreen.ProviderNameEc2Fleet:
		key = "ami"
	case evergreen.ProviderNameDocker, evergreen.ProviderNameDockerMock:
		key = "image_url"
	case evergreen.ProviderNameGce:
		key = "image_name"
	case evergreen.ProviderNameVsphere:
		key = "template"
	case evergreen.ProviderNameMock, evergreen.ProviderNameStatic, evergreen.ProviderNameOpenstack:
		return "", nil
	default:
		return "", errors.New("unknown provider name")
	}

	if d.ProviderSettings != nil {
		i := (*d.ProviderSettings)[key]
		s, ok := i.(string)
		if !ok {
			return "", errors.New("cannot extract image ID from provider settings")
		}
		return s, nil
	}

	if len(d.ProviderSettingsList) == 1 {
		res, ok := d.ProviderSettingsList[0].Lookup(key).StringValueOK()
		if !ok {
			return "", errors.Errorf("provider setting key '%s' is empty", key)
		}
		return res, nil
	}
	return "", errors.New("provider settings not configured correctly")
}

func (d *Distro) GetPoolSize() int {
	switch d.Provider {
	case evergreen.ProviderNameStatic:
		if len(d.ProviderSettingsList) != 1 && d.ProviderSettings == nil {
			return 0
		}
		if len(d.ProviderSettingsList) > 0 {
			hosts, ok := d.ProviderSettingsList[0].Lookup("hosts").Interface().([]interface{})
			if !ok {
				return 0
			}
			return len(hosts)
		}

		hosts, ok := (*d.ProviderSettings)["hosts"].([]interface{})
		if !ok {
			return 0
		}

		return len(hosts)
	default:
		return d.HostAllocatorSettings.MaximumHosts
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

func (d *Distro) RemoveExtraneousProviderSettings(region string) error {
	doc, err := d.GetProviderSettingByRegion(region)
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message":       "provider list missing region",
			"distro":        d.Id,
			"region":        region,
			"settings_list": d.ProviderSettingsList,
		}))
		return errors.Wrapf(err, "error getting provider settings by region")
	}
	d.ProviderSettingsList = []*birch.Document{doc}
	d.ProviderSettings = nil
	return nil
}

func (d *Distro) GetProviderSettingByRegion(region string) (*birch.Document, error) {
	// check legacy case
	if (region == "" || region == evergreen.DefaultEC2Region) && len(d.ProviderSettingsList) == 0 {
		bytes, err := bson.Marshal(d.ProviderSettings)
		if err != nil {
			return nil, errors.Wrap(err, "error marshalling provider setting into bson")
		}
		doc := &birch.Document{}
		if err := doc.UnmarshalBSON(bytes); err != nil {
			return nil, errors.Wrapf(err, "error unmarshalling settings bytes into document")
		}
		if region == evergreen.DefaultEC2Region {
			doc.Set(birch.EC.String("region", evergreen.DefaultEC2Region))
		}
		return doc, nil
	}
	// if no region given but there's a provider settings list, we assume the list is accurate
	if region == "" {
		if len(d.ProviderSettingsList) > 1 {
			return nil, errors.Errorf("multiple provider settings available but no region given")
		}
		return d.ProviderSettingsList[0], nil
	}
	for _, s := range d.ProviderSettingsList {
		if val, ok := s.Lookup("region").StringValueOK(); ok {
			if val == region {
				return s, nil
			}
		}
	}
	return nil, errors.Errorf("distro '%s' has no settings for region '%s'", d.Id, region)
}

func (d *Distro) GetRegionsList() []string {
	regions := []string{}
	if len(d.ProviderSettingsList) <= 1 {
		return regions
	}
	for _, doc := range d.ProviderSettingsList {
		region, ok := doc.Lookup("region").StringValueOK()
		if !ok {
			grip.Debug(message.Fields{
				"message":  "provider settings list missing region",
				"distro":   d.Id,
				"settings": doc,
			})
			continue
		}
		regions = append(regions, region)
	}
	return regions
}

func (d *Distro) SetUserdata(userdata, region string) error {
	if d.ProviderSettings != nil {
		(*d.ProviderSettings)["user_data"] = userdata
	}
	if len(d.ProviderSettingsList) == 0 && evergreen.UseSpawnHostRegions {
		return errors.Errorf("distro '%s' has no provider settings", d.Id)
	}
	if region == "" {
		region = evergreen.DefaultEC2Region
	}
	doc, err := d.GetProviderSettingByRegion(region)
	if err != nil {
		return errors.Wrap(err, "error getting provider setting from list")
	}

	d.ProviderSettingsList = []*birch.Document{doc.Set(birch.EC.String("user_data", userdata))}
	return nil
}

// GetResolvedHostAllocatorSettings combines the distro's HostAllocatorSettings fields with the
// SchedulerConfig defaults to resolve and validate a canonical set of HostAllocatorSettings' field values.
func (d *Distro) GetResolvedHostAllocatorSettings(s *evergreen.Settings) (HostAllocatorSettings, error) {
	config := s.Scheduler
	has := d.HostAllocatorSettings
	resolved := HostAllocatorSettings{
		Version:                has.Version,
		MinimumHosts:           has.MinimumHosts,
		MaximumHosts:           has.MaximumHosts,
		AcceptableHostIdleTime: has.AcceptableHostIdleTime,
	}

	catcher := grip.NewBasicCatcher()
	catcher.Add(config.ValidateAndDefault())

	if resolved.Version == "" {
		resolved.Version = config.HostAllocator
	}
	if !util.StringSliceContains(evergreen.ValidHostAllocators, resolved.Version) {
		catcher.Errorf("'%s' is not a valid HostAllocationSettings.Version", resolved.Version)
	}
	if resolved.AcceptableHostIdleTime == 0 {
		resolved.AcceptableHostIdleTime = time.Duration(config.AcceptableHostIdleTimeSeconds) * time.Second
	}
	if catcher.HasErrors() {
		return HostAllocatorSettings{}, errors.Wrapf(catcher.Resolve(), "cannot resolve HostAllocatorSettings for distro '%s'", d.Id)
	}

	d.HostAllocatorSettings = resolved
	return resolved, nil
}

// GetResolvedFinderSettings combines the distro's FinderSettings fields with the
// SchedulerConfig defaults to resolve and validate a canonical set of FinderSettings' field values.
func (d *Distro) GetResolvedFinderSettings(s *evergreen.Settings) (FinderSettings, error) {
	config := s.Scheduler
	fs := d.FinderSettings
	resolved := FinderSettings{
		Version: fs.Version,
	}

	catcher := grip.NewBasicCatcher()
	catcher.Add(config.ValidateAndDefault())
	if catcher.HasErrors() {
		return FinderSettings{}, errors.Wrapf(catcher.Resolve(), "cannot resolve FinderSettings for distro '%s'", d.Id)
	}
	if resolved.Version == "" {
		resolved.Version = config.TaskFinder
	}

	d.FinderSettings = resolved
	return resolved, nil
}

// GetResolvedPlannerSettings combines the distro's PlannerSettings fields with the
// SchedulerConfig defaults to resolve and validate a canonical set of PlannerSettings' field values.
func (d *Distro) GetResolvedPlannerSettings(s *evergreen.Settings) (PlannerSettings, error) {
	config := s.Scheduler
	ps := d.PlannerSettings
	resolved := PlannerSettings{
		Version:                   ps.Version,
		TargetTime:                ps.TargetTime,
		GroupVersions:             ps.GroupVersions,
		PatchFactor:               ps.PatchFactor,
		PatchTimeInQueueFactor:    ps.PatchTimeInQueueFactor,
		MainlineTimeInQueueFactor: ps.MainlineTimeInQueueFactor,
		ExpectedRuntimeFactor:     ps.ExpectedRuntimeFactor,
		maxDurationPerHost:        evergreen.MaxDurationPerDistroHost,
	}

	catcher := grip.NewBasicCatcher()
	catcher.Add(config.ValidateAndDefault())

	if d.ContainerPool != "" {
		if s.ContainerPools.GetContainerPool(d.ContainerPool) == nil {
			catcher.Errorf("could not find pool '%s' for distro '%s'", d.ContainerPool, d.Id)
		}
		resolved.maxDurationPerHost = evergreen.MaxDurationPerDistroHostWithContainers
	}

	if resolved.Version == "" {
		resolved.Version = config.Planner
	}
	if !util.StringSliceContains(evergreen.ValidTaskPlannerVersions, resolved.Version) {
		catcher.Errorf("'%s' is not a valid PlannerSettings.Version", resolved.Version)
	}
	if resolved.TargetTime == 0 {
		resolved.TargetTime = time.Duration(config.TargetTimeSeconds) * time.Second
	}
	if resolved.GroupVersions == nil {
		resolved.GroupVersions = &config.GroupVersions
	}
	if resolved.PatchFactor == 0 {
		resolved.PatchFactor = config.PatchFactor
	}
	if resolved.PatchTimeInQueueFactor == 0 {
		resolved.PatchTimeInQueueFactor = config.PatchTimeInQueueFactor
	}
	if resolved.MainlineTimeInQueueFactor == 0 {
		resolved.MainlineTimeInQueueFactor = config.MainlineTimeInQueueFactor
	}
	if resolved.ExpectedRuntimeFactor == 0 {
		resolved.ExpectedRuntimeFactor = config.ExpectedRuntimeFactor
	}
	if catcher.HasErrors() {
		return PlannerSettings{}, errors.Wrapf(catcher.Resolve(), "cannot resolve PlannerSettings for distro '%s'", d.Id)
	}

	d.PlannerSettings = resolved
	return resolved, nil
}

func (d *Distro) Add(creator *user.DBUser) error {
	err := d.Insert()
	if err != nil {
		return errors.Wrap(err, "Error inserting distro")
	}
	return d.AddPermissions(creator)
}

func (d *Distro) AddPermissions(creator *user.DBUser) error {
	rm := evergreen.GetEnvironment().RoleManager()
	if err := rm.AddResourceToScope(evergreen.AllDistrosScope, d.Id); err != nil {
		return errors.Wrapf(err, "error adding distro '%s' to list of all distros", d.Id)
	}
	newScope := gimlet.Scope{
		ID:          fmt.Sprintf("distro_%s", d.Id),
		Resources:   []string{d.Id},
		Name:        d.Id,
		Type:        evergreen.DistroResourceType,
		ParentScope: evergreen.AllDistrosScope,
	}
	if err := rm.AddScope(newScope); err != nil {
		return errors.Wrapf(err, "error adding scope for distro '%s'", d.Id)
	}
	newRole := gimlet.Role{
		ID:     fmt.Sprintf("admin_distro_%s", d.Id),
		Owners: []string{creator.Id},
		Scope:  newScope.ID,
		Permissions: map[string]int{
			evergreen.PermissionDistroSettings: evergreen.DistroSettingsRemove.Value,
			evergreen.PermissionHosts:          evergreen.HostsEdit.Value,
		},
	}
	if err := rm.UpdateRole(newRole); err != nil {
		return errors.Wrapf(err, "error adding admin role for distro '%s'", d.Id)
	}
	if creator != nil {
		if err := creator.AddRole(newRole.ID); err != nil {
			return errors.Wrapf(err, "error adding role '%s' to user '%s'", newRole.ID, creator.Id)
		}
	}
	return nil
}

// LegacyBootstrap returns whether hosts of this distro are bootstrapped using the legacy
// method.
func (d *Distro) LegacyBootstrap() bool {
	return d.BootstrapSettings.Method == "" || d.BootstrapSettings.Method == BootstrapMethodLegacySSH
}

// LegacyCommunication returns whether the app server is communicating with
// hosts of this distro using the legacy method.
func (d *Distro) LegacyCommunication() bool {
	return d.BootstrapSettings.Communication == "" || d.BootstrapSettings.Communication == CommunicationMethodLegacySSH
}

// JasperCommunication returns whether or not the app server is communicating with
// hosts of this distro's Jasper service.
func (d *Distro) JasperCommunication() bool {
	return d.BootstrapSettings.Communication == CommunicationMethodSSH || d.BootstrapSettings.Communication == CommunicationMethodRPC
}
