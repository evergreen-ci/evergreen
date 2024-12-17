package distro

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"strings"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Distro struct {
	Id                    string                `bson:"_id" json:"_id,omitempty" mapstructure:"_id,omitempty"`
	AdminOnly             bool                  `bson:"admin_only,omitempty" json:"admin_only,omitempty" mapstructure:"admin_only,omitempty"`
	Aliases               []string              `bson:"aliases,omitempty" json:"aliases,omitempty" mapstructure:"aliases,omitempty"`
	Arch                  string                `bson:"arch" json:"arch,omitempty" mapstructure:"arch,omitempty"`
	WorkDir               string                `bson:"work_dir" json:"work_dir,omitempty" mapstructure:"work_dir,omitempty"`
	Provider              string                `bson:"provider" json:"provider,omitempty" mapstructure:"provider,omitempty"`
	ProviderSettingsList  []*birch.Document     `bson:"provider_settings,omitempty" json:"provider_settings,omitempty" mapstructure:"provider_settings,omitempty"`
	SetupAsSudo           bool                  `bson:"setup_as_sudo,omitempty" json:"setup_as_sudo,omitempty" mapstructure:"setup_as_sudo,omitempty"`
	Setup                 string                `bson:"setup,omitempty" json:"setup,omitempty" mapstructure:"setup,omitempty"`
	User                  string                `bson:"user,omitempty" json:"user,omitempty" mapstructure:"user,omitempty"`
	BootstrapSettings     BootstrapSettings     `bson:"bootstrap_settings" json:"bootstrap_settings" mapstructure:"bootstrap_settings"`
	SSHOptions            []string              `bson:"ssh_options,omitempty" json:"ssh_options,omitempty" mapstructure:"ssh_options,omitempty"`
	AuthorizedKeysFile    string                `bson:"authorized_keys_file,omitempty" json:"authorized_keys_file,omitempty" mapstructure:"authorized_keys_file,omitempty"`
	SpawnAllowed          bool                  `bson:"spawn_allowed" json:"spawn_allowed,omitempty" mapstructure:"spawn_allowed,omitempty"`
	Expansions            []Expansion           `bson:"expansions,omitempty" json:"expansions,omitempty" mapstructure:"expansions,omitempty"`
	Disabled              bool                  `bson:"disabled,omitempty" json:"disabled,omitempty" mapstructure:"disabled,omitempty"`
	ContainerPool         string                `bson:"container_pool,omitempty" json:"container_pool,omitempty" mapstructure:"container_pool,omitempty"`
	FinderSettings        FinderSettings        `bson:"finder_settings" json:"finder_settings" mapstructure:"finder_settings"`
	PlannerSettings       PlannerSettings       `bson:"planner_settings" json:"planner_settings" mapstructure:"planner_settings"`
	DispatcherSettings    DispatcherSettings    `bson:"dispatcher_settings" json:"dispatcher_settings" mapstructure:"dispatcher_settings"`
	HostAllocatorSettings HostAllocatorSettings `bson:"host_allocator_settings" json:"host_allocator_settings" mapstructure:"host_allocator_settings"`
	DisableShallowClone   bool                  `bson:"disable_shallow_clone" json:"disable_shallow_clone" mapstructure:"disable_shallow_clone"`
	Note                  string                `bson:"note" json:"note" mapstructure:"note"`
	WarningNote           string                `bson:"warning_note,omitempty" json:"warning_note,omitempty" mapstructure:"warning_note,omitempty"`
	ValidProjects         []string              `bson:"valid_projects,omitempty" json:"valid_projects,omitempty" mapstructure:"valid_projects,omitempty"`
	IsVirtualWorkstation  bool                  `bson:"is_virtual_workstation" json:"is_virtual_workstation" mapstructure:"is_virtual_workstation"`
	IsCluster             bool                  `bson:"is_cluster" json:"is_cluster" mapstructure:"is_cluster"`
	HomeVolumeSettings    HomeVolumeSettings    `bson:"home_volume_settings" json:"home_volume_settings" mapstructure:"home_volume_settings"`
	IceCreamSettings      IceCreamSettings      `bson:"icecream_settings,omitempty" json:"icecream_settings,omitempty" mapstructure:"icecream_settings,omitempty"`
	Mountpoints           []string              `bson:"mountpoints,omitempty" json:"mountpoints,omitempty" mapstructure:"mountpoints,omitempty"`
	SingleTaskDistro      bool                  `bson:"single_task_distro,omitempty" json:"single_task_distro,omitempty" mapstructure:"single_task_distro,omitempty"`
	// ImageID is not equivalent to AMI. It is the identifier of the base image for the distro.
	ImageID string `bson:"image_id,omitempty" json:"image_id,omitempty" mapstructure:"image_id,omitempty"`

	// ExecUser is the user to run shell.exec and subprocess.exec processes as. If unset, processes are run as the regular distro User.
	ExecUser string `bson:"exec_user,omitempty" json:"exec_user,omitempty" mapstructure:"exec_user,omitempty"`
}

// DistroData is the same as a distro, with the only difference being that all
// the provider settings are stored as maps instead of Birch BSON documents.
type DistroData struct {
	Distro              Distro                   `bson:",inline"`
	ProviderSettingsMap []map[string]interface{} `bson:"provider_settings_list" json:"provider_settings_list"`
}

// DistroData creates distro data from this distro. The provider settings are
// converted into maps instead of Birch BSON documents.
func (d *Distro) DistroData() DistroData {
	res := DistroData{ProviderSettingsMap: []map[string]interface{}{}}
	res.Distro = *d
	for _, each := range d.ProviderSettingsList {
		res.ProviderSettingsMap = append(res.ProviderSettingsMap, each.ExportMap())
	}
	res.Distro.ProviderSettingsList = nil
	return res
}

func (d *Distro) GetDefaultAMI() string {
	if len(d.ProviderSettingsList) == 0 {
		return ""
	}
	settingsDoc, err := d.GetProviderSettingByRegion(evergreen.DefaultEC2Region)
	// An error indicates that settings aren't set up to have a relevant AMI.
	if err != nil {
		return ""
	}
	ami, _ := settingsDoc.Lookup("ami").StringValueOK()
	return ami
}

// BootstrapSettings encapsulates all settings related to bootstrapping hosts.
type BootstrapSettings struct {
	// Required
	Method        string `bson:"method" json:"method" mapstructure:"method"`
	Communication string `bson:"communication,omitempty" json:"communication,omitempty" mapstructure:"communication,omitempty"`

	// Optional
	Env                 []EnvVar             `bson:"env,omitempty" json:"env,omitempty" mapstructure:"env,omitempty"`
	ResourceLimits      ResourceLimits       `bson:"resource_limits,omitempty" json:"resource_limits,omitempty" mapstructure:"resource_limits,omitempty"`
	PreconditionScripts []PreconditionScript `bson:"precondition_scripts,omitempty" json:"precondition_scripts,omitempty" mapstructure:"precondition_scripts,omitempty"`

	// Required for new provisioning
	ClientDir             string `bson:"client_dir,omitempty" json:"client_dir,omitempty" mapstructure:"client_dir,omitempty"`
	JasperBinaryDir       string `bson:"jasper_binary_dir,omitempty" json:"jasper_binary_dir,omitempty" mapstructure:"jasper_binary_dir,omitempty"`
	JasperCredentialsPath string `json:"jasper_credentials_path,omitempty" bson:"jasper_credentials_path,omitempty" mapstructure:"jasper_credentials_path,omitempty"`

	// Windows-specific
	ServiceUser string `bson:"service_user,omitempty" json:"service_user,omitempty" mapstructure:"service_user,omitempty"`
	ShellPath   string `bson:"shell_path,omitempty" json:"shell_path,omitempty" mapstructure:"shell_path,omitempty"`
	RootDir     string `bson:"root_dir,omitempty" json:"root_dir,omitempty" mapstructure:"root_dir,omitempty"`
}

// PreconditionScript represents a script that must run and succeed before the
// Jasper service can start on a provisioning host.
type PreconditionScript struct {
	Path   string `bson:"path,omitempty" json:"path,omitempty" mapstructure:"path,omitempty"`
	Script string `bson:"script,omitempty" json:"script,omitempty" mapstructure:"script,omitempty"`
}

type EnvVar struct {
	Key   string `bson:"key" json:"key" mapstructure:"key,omitempty"`
	Value string `bson:"value" json:"value" mapstructure:"value,omitempty"`
}

// ResourceLimits represents resource limits in Linux.
type ResourceLimits struct {
	NumFiles        int `bson:"num_files,omitempty" json:"num_files,omitempty" mapstructure:"num_files,omitempty"`
	NumProcesses    int `bson:"num_processes,omitempty" json:"num_processes,omitempty" mapstructure:"num_processes,omitempty"`
	NumTasks        int `bson:"num_tasks,omitempty" json:"num_tasks,omitempty" mapstructure:"num_tasks,omitempty"`
	LockedMemoryKB  int `bson:"locked_memory,omitempty" json:"locked_memory,omitempty" mapstructure:"locked_memory,omitempty"`
	VirtualMemoryKB int `bson:"virtual_memory,omitempty" json:"virtual_memory,omitempty" mapstructure:"virtual_memory,omitempty"`
}

type HomeVolumeSettings struct {
	FormatCommand string `bson:"format_command" json:"format_command" mapstructure:"format_command"`
}

type IceCreamSettings struct {
	SchedulerHost string `bson:"scheduler_host,omitempty" json:"scheduler_host,omitempty" mapstructure:"scheduler_host,omitempty"`
	ConfigPath    string `bson:"config_path,omitempty" json:"config_path,omitempty" mapstructure:"config_path,omitempty"`
}

// WriteConfigScript returns the shell script to update the icecream config
// file.
func (s IceCreamSettings) GetUpdateConfigScript() string {
	if !s.Populated() {
		return ""
	}

	return fmt.Sprintf(`#!/usr/bin/env bash
set -o errexit

mkdir -p "%s"
touch "%s"
chmod 644 "%s"
if [[ $(grep 'ICECC_SCHEDULER_HOST=".*"' "%s") ]]; then
	sed -i 's/ICECC_SCHEDULER_HOST=".*"/ICECC_SCHEDULER_HOST="%s"/g' "%s"
else
	echo 'ICECC_SCHEDULER_HOST="%s"' >> "%s"
fi
`,
		filepath.Dir(s.ConfigPath),
		s.ConfigPath,
		s.ConfigPath,
		s.ConfigPath,
		s.SchedulerHost, s.ConfigPath,
		s.SchedulerHost, s.ConfigPath,
	)
}

func (s IceCreamSettings) Populated() bool {
	return s.SchedulerHost != "" && s.ConfigPath != ""
}

func (d *Distro) SetBSON(raw mgobson.Raw) error {
	return bson.Unmarshal(raw.Data, d)
}

// ValidateBootstrapSettings checks if all of the bootstrap settings are valid
// for legacy or non-legacy bootstrapping.
func (d *Distro) ValidateBootstrapSettings() error {
	catcher := grip.NewBasicCatcher()
	if !utility.StringSliceContains(validBootstrapMethods, d.BootstrapSettings.Method) {
		catcher.Errorf("'%s' is not a valid bootstrap method", d.BootstrapSettings.Method)
	}

	if d.BootstrapSettings.Method == BootstrapMethodNone {
		return catcher.Resolve()
	}

	if !utility.StringSliceContains(validCommunicationMethods, d.BootstrapSettings.Communication) {
		catcher.Errorf("'%s' is not a valid communication method", d.BootstrapSettings.Communication)
	}

	switch d.BootstrapSettings.Method {
	case BootstrapMethodLegacySSH:
		catcher.NewWhen(d.BootstrapSettings.Communication != CommunicationMethodLegacySSH, "bootstrapping hosts using legacy SSH is incompatible with non-legacy host communication")
	default:
		catcher.NewWhen(d.BootstrapSettings.Communication == CommunicationMethodLegacySSH, "communicating with hosts using legacy SSH is incompatible with non-legacy host bootstrapping")
	}

	catcher.NewWhen(d.IsWindows() && d.BootstrapSettings.RootDir == "", "root directory cannot be empty for Windows")

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

	catcher.NewWhen(d.IsLinux() && d.BootstrapSettings.ResourceLimits.NumFiles < -1, "max number of files should be a positive number or -1")
	catcher.NewWhen(d.IsLinux() && d.BootstrapSettings.ResourceLimits.NumProcesses < -1, "max number of processes should be a positive number or -1")
	catcher.NewWhen(d.IsLinux() && d.BootstrapSettings.ResourceLimits.NumTasks < -1, "max number of tasks should be a positive number or -1")
	catcher.NewWhen(d.IsLinux() && d.BootstrapSettings.ResourceLimits.LockedMemoryKB < -1, "max locked memory should be a positive number or -1")
	catcher.NewWhen(d.IsLinux() && d.BootstrapSettings.ResourceLimits.VirtualMemoryKB < -1, "max virtual memory should be a positive number or -1")

	return catcher.Resolve()
}

// ShellPath returns the native path to the shell binary.
func (d *Distro) ShellBinary() string {
	return d.AbsPathNotCygwinCompatible(d.BootstrapSettings.ShellPath)
}

type HostAllocatorSettings struct {
	Version                string `bson:"version" json:"version" mapstructure:"version"`
	MinimumHosts           int    `bson:"minimum_hosts" json:"minimum_hosts" mapstructure:"minimum_hosts"`
	MaximumHosts           int    `bson:"maximum_hosts" json:"maximum_hosts" mapstructure:"maximum_hosts"`
	RoundingRule           string `bson:"rounding_rule" json:"rounding_rule" mapstructure:"rounding_rule"`
	FeedbackRule           string `bson:"feedback_rule" json:"feedback_rule" mapstructure:"feedback_rule"`
	HostsOverallocatedRule string `bson:"hosts_overallocated_rule" json:"hosts_overallocated_rule" mapstructure:"hosts_overallocated_rule"`
	// AcceptableHostIdleTime is the amount of time we wait for an idle host to be marked as idle.
	AcceptableHostIdleTime time.Duration `bson:"acceptable_host_idle_time" json:"acceptable_host_idle_time" mapstructure:"acceptable_host_idle_time"`
	FutureHostFraction     float64       `bson:"future_host_fraction" json:"future_host_fraction" mapstructure:"future_host_fraction"`
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
	CommitQueueFactor         int64         `bson:"commit_queue_factor" json:"commit_queue_factor" mapstructure:"commit_queue_factor"`
	MainlineTimeInQueueFactor int64         `bson:"mainline_time_in_queue_factor" json:"mainline_time_in_queue_factor" mapstructure:"mainline_time_in_queue_factor"`
	ExpectedRuntimeFactor     int64         `bson:"expected_runtime_factor" json:"expected_runtime_factor" mapstructure:"expected_runtime_factor"`
	GenerateTaskFactor        int64         `bson:"generate_task_factor" json:"generate_task_factor" mapstructure:"generate_task_factor"`
	NumDependentsFactor       float64       `bson:"num_dependents_factor" json:"num_dependents_factor" mapstructure:"num_dependents_factor"`
	StepbackTaskFactor        int64         `bson:"stepback_task_factor" json:"stepback_task_factor" mapstructure:"stepback_task_factor"`

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

	// Bootstrapping mechanisms
	// BootstrapMethodNone is for internal use only.
	BootstrapMethodNone      = "none"
	BootstrapMethodLegacySSH = "legacy-ssh"
	BootstrapMethodSSH       = "ssh"
	BootstrapMethodUserData  = "user-data"

	// Means of communicating with hosts
	CommunicationMethodLegacySSH = "legacy-ssh"
	CommunicationMethodSSH       = "ssh"
	CommunicationMethodRPC       = "rpc"
)

// validBootstrapMethods includes all recognized bootstrap methods.
var validBootstrapMethods = []string{
	BootstrapMethodNone,
	BootstrapMethodLegacySSH,
	BootstrapMethodSSH,
	BootstrapMethodUserData,
}

// validCommunicationMethods includes all recognized host communication methods.
var validCommunicationMethods = []string{
	CommunicationMethodLegacySSH,
	CommunicationMethodSSH,
	CommunicationMethodRPC,
}

// Seed the random number generator for creating distro names
func init() {
	rand.Seed(time.Now().UnixNano())
}

func (s *PlannerSettings) ShouldGroupVersions() bool {
	return utility.FromBoolPtr(s.GroupVersions)
}

func (s *PlannerSettings) GetPatchFactor() int64 {
	if s.PatchFactor <= 0 {
		return 1
	}
	return s.PatchFactor
}

func (s *PlannerSettings) GetPatchTimeInQueueFactor() int64 {
	if s.PatchTimeInQueueFactor <= 0 {
		return 1
	}
	return s.PatchTimeInQueueFactor
}

func (s *PlannerSettings) GetCommitQueueFactor() int64 {
	if s.CommitQueueFactor <= 0 {
		return 1
	}
	return s.CommitQueueFactor
}

func (s *PlannerSettings) GetGenerateTaskFactor() int64 {
	if s.GenerateTaskFactor <= 0 {
		return 1
	}
	return s.GenerateTaskFactor
}

func (s *PlannerSettings) GetNumDependentsFactor() float64 {
	if s.NumDependentsFactor <= 0 {
		return 1
	}
	return s.NumDependentsFactor
}

func (s *PlannerSettings) GetMainlineTimeInQueueFactor() int64 {
	if s.MainlineTimeInQueueFactor <= 0 {
		return 1
	}
	return s.MainlineTimeInQueueFactor
}

func (s *PlannerSettings) GetStepbackTaskFactor() int64 {
	if s.StepbackTaskFactor <= 0 {
		return 1
	}
	return s.StepbackTaskFactor
}

func (s *PlannerSettings) GetExpectedRuntimeFactor() int64 {
	if s.ExpectedRuntimeFactor <= 0 {
		return 1
	}

	return s.ExpectedRuntimeFactor
}

// GenerateName generates a unique instance name for a host in a distro.
func (d *Distro) GenerateName() string {
	switch d.Provider {
	case evergreen.ProviderNameStatic:
		return "static"
	case evergreen.ProviderNameDocker:
		return fmt.Sprintf("container-%d", rand.New(rand.NewSource(time.Now().UnixNano())).Int())
	}

	return fmt.Sprintf("evg-%s-%s-%d", d.Id, time.Now().Format(evergreen.NameTimeFormat), rand.Int())
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

// IsWindows returns whether or not the distro's hosts run on Windows.
func (d *Distro) IsWindows() bool {
	// XXX: if this is-windows check is updated, make sure to also update
	// public/static/js/spawned_hosts.js as well
	return strings.Contains(d.Arch, "windows")
}

// IsLinux returns whether or not the distro's hosts run on Linux.
func (d *Distro) IsLinux() bool {
	return strings.Contains(d.Arch, "linux")
}

// IsMacOS returns whether or not the distro's hosts run on MacOS.
func (d *Distro) IsMacOS() bool {
	return strings.Contains(d.Arch, "darwin")
}

// Platform returns the distro's OS and architecture.
func (d *Distro) Platform() (string, string) {
	osAndArch := strings.Split(d.Arch, "_")
	return osAndArch[0], osAndArch[1]
}

func (d *Distro) IsEphemeral() bool {
	return utility.StringSliceContains(evergreen.ProviderSpawnable, d.Provider)
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
// This is compatible with Cygwin (see (*Distro).AbsPathCygwinCompatible for
// details).
func (d *Distro) HomeDir() string {
	if d.User == "root" {
		return filepath.Join("/", d.User)
	}
	if d.Arch == evergreen.ArchDarwinAmd64 || d.Arch == evergreen.ArchDarwinArm64 {
		return filepath.Join("/Users", d.User)
	}
	return filepath.Join("/home", d.User)
}

// GetAuthorizedKeysFile returns the path to the SSH authorized keys file for
// the distro. If not explicitly set for the distro, it returns the default
// location of the SSH authorized keys file in the home directory.
func (d *Distro) GetAuthorizedKeysFile() string {
	if d.AuthorizedKeysFile == "" {
		return filepath.Join(d.HomeDir(), ".ssh", "authorized_keys")
	}
	return d.AuthorizedKeysFile
}

// IsParent returns whether the distro is the parent distro for any container pool
func (d *Distro) IsParent(s *evergreen.Settings) bool {
	for _, p := range s.ContainerPools.Pools {
		if d.Id == p.Distro {
			return true
		}
	}
	return false
}

// GetImageID returns the distro provider's image.
func (d *Distro) GetImageID() (string, error) {
	key := ""

	switch d.Provider {
	case evergreen.ProviderNameEc2OnDemand, evergreen.ProviderNameEc2Fleet:
		key = "ami"
	case evergreen.ProviderNameDocker, evergreen.ProviderNameDockerMock:
		key = "image_url"
	case evergreen.ProviderNameMock, evergreen.ProviderNameStatic:
		return "", nil
	default:
		return "", errors.New("unknown provider name")
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
		if len(d.ProviderSettingsList) > 0 {
			hosts, ok := d.ProviderSettingsList[0].Lookup("hosts").Interface().([]interface{})
			if !ok {
				return 0
			}
			return len(hosts)
		}
		return 0
	default:
		return d.HostAllocatorSettings.MaximumHosts
	}
}

// ValidateContainerPoolDistros ensures that container pools have valid distros
func ValidateContainerPoolDistros(ctx context.Context, s *evergreen.Settings) error {
	catcher := grip.NewSimpleCatcher()

	for _, pool := range s.ContainerPools.Pools {
		d, err := FindOneId(ctx, pool.Distro)
		if err != nil {
			catcher.Add(fmt.Errorf("error finding distro for container pool '%s'", pool.Id))
		}
		if d == nil {
			catcher.Errorf("distro not found for container pool '%s'", pool.Id)
		}
		if d != nil && d.ContainerPool != "" {
			catcher.Add(fmt.Errorf("container pool '%s' has invalid distro '%s'", pool.Id, d.Id))
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

	if _, ok := evergreen.ValidArchDisplayNames[arch]; !ok {
		return errors.Errorf("'%s' is not a recognized architecture", arch)
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

func (d *Distro) GetProviderSettingByRegion(region string) (*birch.Document, error) {
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

func (d *Distro) GetRegionsList(allowedRegions []string) []string {
	regions := []string{}
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
		// admins don't allow this region
		if !utility.StringSliceContains(allowedRegions, region) {
			continue
		}
		regions = append(regions, region)
	}
	return regions
}

func (d *Distro) SetUserdata(userdata, region string) error {
	if len(d.ProviderSettingsList) == 0 {
		return errors.Errorf("distro '%s' has no provider settings", d.Id)
	}
	if region == "" {
		region = evergreen.DefaultEC2Region
	}
	doc, err := d.GetProviderSettingByRegion(region)
	if err != nil {
		return errors.Wrap(err, "getting provider setting from list")
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
		RoundingRule:           has.RoundingRule,
		FeedbackRule:           has.FeedbackRule,
		HostsOverallocatedRule: has.HostsOverallocatedRule,
		FutureHostFraction:     has.FutureHostFraction,
	}

	catcher := grip.NewBasicCatcher()
	catcher.Add(config.ValidateAndDefault())

	if resolved.Version == "" {
		resolved.Version = config.HostAllocator
	}

	if !utility.StringSliceContains(evergreen.ValidHostAllocators, resolved.Version) {
		catcher.Errorf("'%s' is not a valid host allocator version", resolved.Version)
	}

	if resolved.AcceptableHostIdleTime == 0 {
		resolved.AcceptableHostIdleTime = time.Duration(config.AcceptableHostIdleTimeSeconds) * time.Second
	}
	if resolved.RoundingRule == evergreen.HostAllocatorRoundDefault {
		resolved.RoundingRule = config.HostAllocatorRoundingRule
	}
	if resolved.FeedbackRule == evergreen.HostAllocatorUseDefaultFeedback {
		resolved.FeedbackRule = config.HostAllocatorFeedbackRule
	}
	if resolved.HostsOverallocatedRule == evergreen.HostsOverallocatedUseDefault {
		resolved.HostsOverallocatedRule = config.HostsOverallocatedRule
	}
	if resolved.FutureHostFraction == 0 {
		resolved.FutureHostFraction = config.FutureHostFraction
	}
	if catcher.HasErrors() {
		return HostAllocatorSettings{}, errors.Wrapf(catcher.Resolve(), "resolving host allocator settings for distro '%s'", d.Id)
	}

	d.HostAllocatorSettings = resolved
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
		CommitQueueFactor:         ps.CommitQueueFactor,
		MainlineTimeInQueueFactor: ps.MainlineTimeInQueueFactor,
		ExpectedRuntimeFactor:     ps.ExpectedRuntimeFactor,
		GenerateTaskFactor:        ps.GenerateTaskFactor,
		NumDependentsFactor:       ps.NumDependentsFactor,
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
	if !utility.StringSliceContains(evergreen.ValidTaskPlannerVersions, resolved.Version) {
		catcher.Errorf("'%s' is not a valid planner version", resolved.Version)
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
	if resolved.CommitQueueFactor == 0 {
		resolved.CommitQueueFactor = config.CommitQueueFactor
	}
	if resolved.MainlineTimeInQueueFactor == 0 {
		resolved.MainlineTimeInQueueFactor = config.MainlineTimeInQueueFactor
	}
	if resolved.ExpectedRuntimeFactor == 0 {
		resolved.ExpectedRuntimeFactor = config.ExpectedRuntimeFactor
	}
	if resolved.GenerateTaskFactor == 0 {
		resolved.GenerateTaskFactor = config.GenerateTaskFactor
	}
	if resolved.NumDependentsFactor == 0 {
		resolved.NumDependentsFactor = config.NumDependentsFactor
	}

	// StepbackTaskFactor isn't configurable by distro
	resolved.StepbackTaskFactor = config.StepbackTaskFactor

	if catcher.HasErrors() {
		return PlannerSettings{}, errors.Wrapf(catcher.Resolve(), "resolving planner settings for distro '%s'", d.Id)
	}

	d.PlannerSettings = resolved
	return resolved, nil
}

func (d *Distro) Add(ctx context.Context, creator *user.DBUser) error {
	err := d.Insert(ctx)
	if err != nil {
		return errors.Wrap(err, "Error inserting distro")
	}
	return d.AddPermissions(creator)
}

func (d *Distro) AddPermissions(creator *user.DBUser) error {
	rm := evergreen.GetEnvironment().RoleManager()
	if err := rm.AddResourceToScope(evergreen.AllDistrosScope, d.Id); err != nil {
		return errors.Wrapf(err, "adding distro '%s' to permissions scope containing all distros", d.Id)
	}
	newScope := gimlet.Scope{
		ID:          fmt.Sprintf("distro_%s", d.Id),
		Resources:   []string{d.Id},
		Name:        d.Id,
		Type:        evergreen.DistroResourceType,
		ParentScope: evergreen.AllDistrosScope,
	}
	if err := rm.AddScope(newScope); err != nil && !db.IsDuplicateKey(err) {
		return errors.Wrapf(err, "adding scope for distro '%s'", d.Id)
	}
	newRole := gimlet.Role{
		ID:     fmt.Sprintf("admin_distro_%s", d.Id),
		Owners: []string{creator.Id},
		Scope:  newScope.ID,
		Permissions: map[string]int{
			evergreen.PermissionDistroSettings: evergreen.DistroSettingsAdmin.Value,
			evergreen.PermissionHosts:          evergreen.HostsEdit.Value,
		},
	}
	if err := rm.UpdateRole(newRole); err != nil {
		return errors.Wrapf(err, "adding admin role for distro '%s'", d.Id)
	}
	if creator != nil {
		if err := creator.AddRole(newRole.ID); err != nil {
			return errors.Wrapf(err, "adding role '%s' to user '%s'", newRole.ID, creator.Id)
		}
	}
	return nil
}

// LegacyBootstrap returns whether hosts of this distro are bootstrapped using
// the legacy method.
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

// AbsPathCygwinCompatible creates an absolute path from the given path that is
// compatible with the host's provisioning settings.
//
// For example, in the context of an SSH session with Cygwin, if you invoke the
// "/usr/bin/echo" binary, Cygwin uses the binary located relative to Cygwin's
// filesystem root directory, so it will use the binary at
// "$ROOT_DIR/usr/bin/echo". Similarly, Cygwin binaries like "ls" will resolve
// filepaths as paths relative to the Cygwin root directory, so "ls /usr/bin",
// will correctly list the directory contents of "$ROOT_DIR/usr/bin".
//
// However, in almost all other cases, Windows binaries expect native Windows
// paths for everything. For example, if the evergreen binary accepts a filepath
// given in the command line flags, the Golang standard library uses native
// paths. Therefore, giving a path like "/home/Administrator/my_file" will fail,
// because the library has no awareness of the Cygwin filesystem context. The
// correct path would be to give an absolute native path,
// "$ROOT_DIR/home/Administrator/evergreen".
//
// Documentation for Cygwin paths:
// https://www.cygwin.com/cygwin-ug-net/using-effectively.html
func (d *Distro) AbsPathCygwinCompatible(path ...string) string {
	if d.LegacyBootstrap() {
		return filepath.Join(path...)
	}
	return d.AbsPathNotCygwinCompatible(path...)
}

// AbsPathNotCygwinCompatible creates a Cygwin-incompatible absolute path using
// RootDir to get around the fact that Cygwin binaries use POSIX paths relative
// to the Cygwin filesystem root directory, but most other paths require
// absolute filepaths using native Windows absolute paths. See
// (*Distro).AbsPathCygwinCompatible for more details.
func (d *Distro) AbsPathNotCygwinCompatible(path ...string) string {
	return filepath.Join(append([]string{d.BootstrapSettings.RootDir}, path...)...)
}

// S3ClientURL returns the URL in S3 where the Evergreen client version can be
// retrieved for this server's particular Evergreen build version.
func (d *Distro) S3ClientURL(env evergreen.Environment) string {
	return strings.Join([]string{
		env.ClientConfig().S3URLPrefix,
		d.ExecutableSubPath(),
	}, "/")
}

func AllDistros(ctx context.Context) ([]Distro, error) {
	return Find(ctx, bson.M{}, options.Find().SetSort(bson.M{IdKey: 1}))
}

// GetHostCreateDistro returns the distro based on the name and provider.
// If the provider is Docker, passing in the distro is required, and the
// distro must be a Docker distro. If the provider is EC2, the distro
// name is optional.
func GetHostCreateDistro(ctx context.Context, createHost apimodels.CreateHost) (*Distro, error) {
	var err error
	d := &Distro{}
	isDockerProvider := evergreen.IsDockerProvider(createHost.CloudProvider)
	if isDockerProvider {
		d, err = FindOneId(ctx, createHost.Distro)
		if err != nil {
			return nil, errors.Wrapf(err, "finding distro '%s'", createHost.Distro)
		}
		if d == nil {
			return nil, errors.Errorf("distro '%s' not found", createHost.Distro)
		}
		if !evergreen.IsDockerProvider(d.Provider) {
			return nil, errors.Errorf("distro '%s' provider must support Docker but actual provider is '%s'", d.Id, d.Provider)
		}
	} else {
		if createHost.Distro != "" {
			var dat AliasLookupTable
			dat, err := NewDistroAliasesLookupTable(ctx)
			if err != nil {
				return nil, errors.Wrap(err, "getting distro lookup table")
			}
			distroIDs := dat.Expand([]string{createHost.Distro})
			if len(distroIDs) == 0 {
				return nil, errors.Wrap(err, "distro lookup returned no matching distro IDs")
			}
			d, err = FindOneId(ctx, distroIDs[0])
			if err != nil {
				return nil, errors.Wrapf(err, "finding distro '%s'", createHost.Distro)
			}
			if d == nil {
				return nil, errors.Errorf("distro '%s' not found", createHost.Distro)
			}
		}
		d.Provider = evergreen.ProviderNameEc2OnDemand
	}
	// Do not provision task-spawned hosts.
	d.BootstrapSettings.Method = BootstrapMethodNone
	return d, nil
}

// GetDistrosForImage returns the distros for a given image.
func GetDistrosForImage(ctx context.Context, imageID string) ([]Distro, error) {
	if imageID == "" {
		return nil, errors.Errorf("imageID not provided")
	}
	return Find(ctx, bson.M{ImageIDKey: imageID})
}

// GetImageIDFromDistro returns the imageID corresponding to the given distro.
func GetImageIDFromDistro(ctx context.Context, distro string) (string, error) {
	d, err := FindOneId(ctx, distro)
	if err != nil {
		return "", errors.Wrapf(err, "finding distro '%s'", distro)
	}
	// Do not return an error, as the distro may have been deleted.
	if d == nil {
		return "", nil
	}
	return d.ImageID, nil
}
