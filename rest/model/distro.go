package model

import (
	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/utility"
)

////////////////////////////////////////////////////////////////////////////////
//
// APIPlannerSettings is the model to be returned by the API whenever distro.PlannerSettings are fetched

type APIPlannerSettings struct {
	Version                   *string     `json:"version"`
	TargetTime                APIDuration `json:"target_time"`
	GroupVersions             bool        `json:"group_versions"`
	PatchFactor               int64       `json:"patch_factor"`
	PatchTimeInQueueFactor    int64       `json:"patch_time_in_queue_factor"`
	MainlineTimeInQueueFactor int64       `json:"mainline_time_in_queue_factor"`
	ExpectedRuntimeFactor     int64       `json:"expected_runtime_factor"`
	GenerateTaskFactor        int64       `json:"generate_task_factor"`
	NumDependentsFactor       float64     `json:"num_dependents_factor"`
	CommitQueueFactor         int64       `json:"commit_queue_factor"`
}

// BuildFromService converts from service level distro.PlannerSetting to an APIPlannerSettings
func (s *APIPlannerSettings) BuildFromService(settings distro.PlannerSettings) {
	if settings.Version == "" {
		s.Version = utility.ToStringPtr(evergreen.PlannerVersionLegacy)
	} else {
		s.Version = utility.ToStringPtr(settings.Version)
	}
	s.TargetTime = NewAPIDuration(settings.TargetTime)
	s.GroupVersions = utility.FromBoolPtr(settings.GroupVersions)
	s.PatchFactor = settings.PatchFactor
	s.ExpectedRuntimeFactor = settings.ExpectedRuntimeFactor
	s.PatchTimeInQueueFactor = settings.PatchTimeInQueueFactor
	s.MainlineTimeInQueueFactor = settings.MainlineTimeInQueueFactor
	s.GenerateTaskFactor = settings.GenerateTaskFactor
	s.NumDependentsFactor = settings.NumDependentsFactor
	s.CommitQueueFactor = settings.CommitQueueFactor
}

// ToService returns a service layer distro.PlannerSettings using the data from APIPlannerSettings
func (s *APIPlannerSettings) ToService() distro.PlannerSettings {
	settings := distro.PlannerSettings{}
	settings.Version = utility.FromStringPtr(s.Version)
	if settings.Version == "" {
		settings.Version = evergreen.PlannerVersionLegacy
	}
	settings.TargetTime = s.TargetTime.ToDuration()
	settings.GroupVersions = utility.ToBoolPtr(s.GroupVersions)
	settings.PatchFactor = s.PatchFactor
	settings.PatchTimeInQueueFactor = s.PatchTimeInQueueFactor
	settings.MainlineTimeInQueueFactor = s.MainlineTimeInQueueFactor
	settings.ExpectedRuntimeFactor = s.ExpectedRuntimeFactor
	settings.GenerateTaskFactor = s.GenerateTaskFactor
	settings.NumDependentsFactor = s.NumDependentsFactor
	settings.CommitQueueFactor = s.CommitQueueFactor

	return settings
}

////////////////////////////////////////////////////////////////////////////////
//
// APIHostAllocatorSettings is the model to be returned by the API whenever distro.HostAllocatorSettings are fetched

type APIHostAllocatorSettings struct {
	Version                *string     `json:"version"`
	MinimumHosts           int         `json:"minimum_hosts"`
	MaximumHosts           int         `json:"maximum_hosts"`
	RoundingRule           *string     `json:"rounding_rule"`
	FeedbackRule           *string     `json:"feedback_rule"`
	HostsOverallocatedRule *string     `json:"hosts_overallocated_rule"`
	AcceptableHostIdleTime APIDuration `json:"acceptable_host_idle_time"`
	FutureHostFraction     float64     `json:"future_host_fraction"`
}

// BuildFromService converts from service level distro.HostAllocatorSettings to an APIHostAllocatorSettings
func (s *APIHostAllocatorSettings) BuildFromService(settings distro.HostAllocatorSettings) {
	if settings.Version == "" {
		s.Version = utility.ToStringPtr(evergreen.HostAllocatorUtilization)
	} else {
		s.Version = utility.ToStringPtr(settings.Version)
	}
	s.MinimumHosts = settings.MinimumHosts
	s.MaximumHosts = settings.MaximumHosts
	s.AcceptableHostIdleTime = NewAPIDuration(settings.AcceptableHostIdleTime)
	s.RoundingRule = utility.ToStringPtr(settings.RoundingRule)
	s.FeedbackRule = utility.ToStringPtr(settings.FeedbackRule)
	s.HostsOverallocatedRule = utility.ToStringPtr(settings.HostsOverallocatedRule)
	s.FutureHostFraction = settings.FutureHostFraction
}

// ToService returns a service layer distro.HostAllocatorSettings using the data from APIHostAllocatorSettings
func (s *APIHostAllocatorSettings) ToService() distro.HostAllocatorSettings {
	settings := distro.HostAllocatorSettings{
		Version: utility.FromStringPtr(s.Version),
	}
	if settings.Version == "" {
		settings.Version = evergreen.HostAllocatorUtilization
	}
	settings.MinimumHosts = s.MinimumHosts
	settings.MaximumHosts = s.MaximumHosts
	settings.AcceptableHostIdleTime = s.AcceptableHostIdleTime.ToDuration()
	settings.RoundingRule = utility.FromStringPtr(s.RoundingRule)
	settings.FeedbackRule = utility.FromStringPtr(s.FeedbackRule)
	settings.HostsOverallocatedRule = utility.FromStringPtr(s.HostsOverallocatedRule)
	settings.FutureHostFraction = s.FutureHostFraction

	return settings
}

////////////////////////////////////////////////////////////////////////////////
//
// APIFinderSettings is the model to be returned by the API whenever distro.FinderSettings are fetched

type APIFinderSettings struct {
	Version *string `json:"version"`
}

// BuildFromService converts from service level distro.FinderSettings to an APIFinderSettings
func (s *APIFinderSettings) BuildFromService(settings distro.FinderSettings) {
	if settings.Version == "" {
		s.Version = utility.ToStringPtr(evergreen.FinderVersionLegacy)
	} else {
		s.Version = utility.ToStringPtr(settings.Version)
	}
}

// ToService returns a service layer distro.FinderSettings using the data from APIFinderSettings
func (s *APIFinderSettings) ToService() distro.FinderSettings {
	settings := distro.FinderSettings{
		Version: utility.FromStringPtr(s.Version),
	}
	if settings.Version == "" {
		settings.Version = evergreen.FinderVersionLegacy
	}

	return settings
}

////////////////////////////////////////////////////////////////////////////////
//
// APIDispatcherSettings is the model to be returned by the API whenever distro.DispatcherSettings are fetched

type APIDispatcherSettings struct {
	Version *string `json:"version"`
}

// BuildFromService converts from service level distro.DispatcherSettings to an APIDispatcherSettings
func (s *APIDispatcherSettings) BuildFromService() {
	s.Version = utility.ToStringPtr(evergreen.DispatcherVersionRevisedWithDependencies)
}

// ToService returns a service layer distro.DispatcherSettings using the data from APIDispatcherSettings
func (s *APIDispatcherSettings) ToService() distro.DispatcherSettings {
	settings := distro.DispatcherSettings{
		Version: utility.FromStringPtr(s.Version),
	}
	if settings.Version == "" {
		settings.Version = evergreen.DispatcherVersionRevisedWithDependencies
	}

	return settings
}

////////////////////////////////////////////////////////////////////////////////
//
// APIBootstrapSettings is the model to be returned by the API whenever distro.BootstrapSettings are fetched

type APIBootstrapSettings struct {
	Method                *string                 `json:"method"`
	Communication         *string                 `json:"communication"`
	ClientDir             *string                 `json:"client_dir"`
	JasperBinaryDir       *string                 `json:"jasper_binary_dir"`
	JasperCredentialsPath *string                 `json:"jasper_credentials_path"`
	ServiceUser           *string                 `json:"service_user"`
	ShellPath             *string                 `json:"shell_path"`
	RootDir               *string                 `json:"root_dir"`
	Env                   []APIEnvVar             `json:"env"`
	ResourceLimits        APIResourceLimits       `json:"resource_limits"`
	PreconditionScripts   []APIPreconditionScript `json:"precondition_scripts"`
}

type APIEnvVar struct {
	Key   *string `json:"key"`
	Value *string `json:"value"`
}

// BuildFromService converts a service level distro.EnvVar to an APIEnvVar
func (e *APIEnvVar) BuildFromService(v distro.EnvVar) {
	e.Key = utility.ToStringPtr(v.Key)
	e.Value = utility.ToStringPtr(v.Value)
}

// ToService returns a service layer distro.EnvVar using the data from an APIEnvVar
func (e *APIEnvVar) ToService() distro.EnvVar {
	return distro.EnvVar{
		Key:   utility.FromStringPtr(e.Key),
		Value: utility.FromStringPtr(e.Value),
	}
}

type APIResourceLimits struct {
	NumFiles        int `json:"num_files"`
	NumProcesses    int `json:"num_processes"`
	NumTasks        int `json:"num_tasks"`
	LockedMemoryKB  int `json:"locked_memory"`
	VirtualMemoryKB int `json:"virtual_memory"`
}

// APIPreconditionScript is the model used by the API to represent a
// distro.PreconditionScript.
type APIPreconditionScript struct {
	Path   *string `json:"path"`
	Script *string `json:"script"`
}

// BuildFromService converts a service-level distro.PreconditionScript to an
// APIPreconditionScript.
func (s *APIPreconditionScript) BuildFromService(script distro.PreconditionScript) {
	s.Path = utility.ToStringPtr(script.Path)
	s.Script = utility.ToStringPtr(script.Script)
}

// ToService returns a service-level distro.PreconditionScript using the data
// from the APIPreconditionScript.
func (s *APIPreconditionScript) ToService() distro.PreconditionScript {
	return distro.PreconditionScript{
		Path:   utility.FromStringPtr(s.Path),
		Script: utility.FromStringPtr(s.Script),
	}
}

// BuildFromService converts from service level distro.BootstrapSettings to an
// APIBootstrapSettings.
func (s *APIBootstrapSettings) BuildFromService(settings distro.BootstrapSettings) {
	s.Method = utility.ToStringPtr(settings.Method)
	if utility.FromStringPtr(s.Method) == "" {
		s.Method = utility.ToStringPtr(distro.BootstrapMethodLegacySSH)
	}
	s.Communication = utility.ToStringPtr(settings.Communication)
	if utility.FromStringPtr(s.Communication) == "" {
		s.Communication = utility.ToStringPtr(distro.CommunicationMethodLegacySSH)
	}
	s.ClientDir = utility.ToStringPtr(settings.ClientDir)
	s.JasperBinaryDir = utility.ToStringPtr(settings.JasperBinaryDir)
	s.JasperCredentialsPath = utility.ToStringPtr(settings.JasperCredentialsPath)
	s.ServiceUser = utility.ToStringPtr(settings.ServiceUser)
	s.ShellPath = utility.ToStringPtr(settings.ShellPath)
	s.RootDir = utility.ToStringPtr(settings.RootDir)
	for _, envVar := range settings.Env {
		apiEnvVar := APIEnvVar{}
		apiEnvVar.BuildFromService(envVar)
		s.Env = append(s.Env, apiEnvVar)
	}
	for _, script := range settings.PreconditionScripts {
		var apiScript APIPreconditionScript
		apiScript.BuildFromService(script)
		s.PreconditionScripts = append(s.PreconditionScripts, apiScript)
	}
	s.ResourceLimits.NumFiles = settings.ResourceLimits.NumFiles
	s.ResourceLimits.NumProcesses = settings.ResourceLimits.NumProcesses
	s.ResourceLimits.NumTasks = settings.ResourceLimits.NumTasks
	s.ResourceLimits.LockedMemoryKB = settings.ResourceLimits.LockedMemoryKB
	s.ResourceLimits.VirtualMemoryKB = settings.ResourceLimits.VirtualMemoryKB
}

// ToService returns a service layer distro.BootstrapSettings using the data
// from APIBootstrapSettings.
func (s *APIBootstrapSettings) ToService() distro.BootstrapSettings {
	settings := distro.BootstrapSettings{}
	settings.Method = utility.FromStringPtr(s.Method)
	if settings.Method == "" {
		settings.Method = distro.BootstrapMethodLegacySSH
	}
	settings.Communication = utility.FromStringPtr(s.Communication)
	if settings.Communication == "" {
		settings.Communication = distro.CommunicationMethodLegacySSH
	}
	settings.ClientDir = utility.FromStringPtr(s.ClientDir)
	settings.JasperBinaryDir = utility.FromStringPtr(s.JasperBinaryDir)
	settings.JasperCredentialsPath = utility.FromStringPtr(s.JasperCredentialsPath)
	settings.ServiceUser = utility.FromStringPtr(s.ServiceUser)
	settings.ShellPath = utility.FromStringPtr(s.ShellPath)
	settings.RootDir = utility.FromStringPtr(s.RootDir)
	for _, apiEnvVar := range s.Env {
		settings.Env = append(settings.Env, apiEnvVar.ToService())
	}
	for _, apiScript := range s.PreconditionScripts {
		settings.PreconditionScripts = append(settings.PreconditionScripts, apiScript.ToService())
	}
	settings.ResourceLimits.NumFiles = s.ResourceLimits.NumFiles
	settings.ResourceLimits.NumProcesses = s.ResourceLimits.NumProcesses
	settings.ResourceLimits.NumTasks = s.ResourceLimits.NumTasks
	settings.ResourceLimits.LockedMemoryKB = s.ResourceLimits.LockedMemoryKB
	settings.ResourceLimits.VirtualMemoryKB = s.ResourceLimits.VirtualMemoryKB

	return settings
}

type APIHomeVolumeSettings struct {
	FormatCommand *string `json:"format_command"`
}

func (s *APIHomeVolumeSettings) BuildFromService(settings distro.HomeVolumeSettings) {
	s.FormatCommand = utility.ToStringPtr(settings.FormatCommand)
}

func (s *APIHomeVolumeSettings) ToService() distro.HomeVolumeSettings {
	return distro.HomeVolumeSettings{
		FormatCommand: utility.FromStringPtr(s.FormatCommand),
	}
}

type APIIceCreamSettings struct {
	SchedulerHost *string `json:"scheduler_host"`
	ConfigPath    *string `json:"config_path"`
}

func (s *APIIceCreamSettings) BuildFromService(settings distro.IceCreamSettings) {
	s.SchedulerHost = utility.ToStringPtr(settings.SchedulerHost)
	s.ConfigPath = utility.ToStringPtr(settings.ConfigPath)
}

func (s *APIIceCreamSettings) ToService() distro.IceCreamSettings {
	return distro.IceCreamSettings{
		SchedulerHost: utility.FromStringPtr(s.SchedulerHost),
		ConfigPath:    utility.FromStringPtr(s.ConfigPath),
	}
}

////////////////////////////////////////////////////////////////////////////////
//
// APIDistro is the model to be returned by the API whenever distros are fetched

type APIDistro struct {
	Name                  *string                  `json:"name"`
	AdminOnly             bool                     `json:"admin_only"`
	Aliases               []string                 `json:"aliases"`
	UserSpawnAllowed      bool                     `json:"user_spawn_allowed"`
	Provider              *string                  `json:"provider"`
	ProviderSettingsList  []*birch.Document        `json:"provider_settings" swaggertype:"object"`
	Arch                  *string                  `json:"arch"`
	WorkDir               *string                  `json:"work_dir"`
	SetupAsSudo           bool                     `json:"setup_as_sudo"`
	Setup                 *string                  `json:"setup"`
	User                  *string                  `json:"user"`
	BootstrapSettings     APIBootstrapSettings     `json:"bootstrap_settings"`
	SSHOptions            []string                 `json:"ssh_options"`
	AuthorizedKeysFile    *string                  `json:"authorized_keys_file"`
	Expansions            []APIExpansion           `json:"expansions"`
	Disabled              bool                     `json:"disabled"`
	ContainerPool         *string                  `json:"container_pool"`
	FinderSettings        APIFinderSettings        `json:"finder_settings"`
	PlannerSettings       APIPlannerSettings       `json:"planner_settings"`
	DispatcherSettings    APIDispatcherSettings    `json:"dispatcher_settings"`
	HostAllocatorSettings APIHostAllocatorSettings `json:"host_allocator_settings"`
	DisableShallowClone   bool                     `json:"disable_shallow_clone"`
	HomeVolumeSettings    APIHomeVolumeSettings    `json:"home_volume_settings"`
	IcecreamSettings      APIIceCreamSettings      `json:"icecream_settings"`
	IsVirtualWorkstation  bool                     `json:"is_virtual_workstation"`
	IsCluster             bool                     `json:"is_cluster"`
	Note                  *string                  `json:"note"`
	WarningNote           *string                  `json:"warning_note"`
	ValidProjects         []*string                `json:"valid_projects"`
	Mountpoints           []string                 `json:"mountpoints"`
	SingleTaskDistro      bool                     `json:"single_task_distro"`
	ImageID               *string                  `json:"image_id"`
	ExecUser              *string                  `json:"exec_user"`
}

// BuildFromService converts from service level distro.Distro to an APIDistro
func (apiDistro *APIDistro) BuildFromService(d distro.Distro) {
	apiDistro.Name = utility.ToStringPtr(d.Id)
	apiDistro.AdminOnly = d.AdminOnly
	apiDistro.Aliases = d.Aliases
	apiDistro.UserSpawnAllowed = d.SpawnAllowed
	apiDistro.Provider = utility.ToStringPtr(d.Provider)
	apiDistro.ProviderSettingsList = d.ProviderSettingsList
	apiDistro.Arch = utility.ToStringPtr(d.Arch)
	apiDistro.WorkDir = utility.ToStringPtr(d.WorkDir)
	apiDistro.SetupAsSudo = d.SetupAsSudo
	apiDistro.Setup = utility.ToStringPtr(d.Setup)
	apiDistro.User = utility.ToStringPtr(d.User)
	apiDistro.SSHOptions = d.SSHOptions
	apiDistro.AuthorizedKeysFile = utility.ToStringPtr(d.AuthorizedKeysFile)
	apiDistro.Disabled = d.Disabled
	apiDistro.ContainerPool = utility.ToStringPtr(d.ContainerPool)
	apiDistro.DisableShallowClone = d.DisableShallowClone
	apiDistro.Note = utility.ToStringPtr(d.Note)
	apiDistro.WarningNote = utility.ToStringPtr(d.WarningNote)
	apiDistro.ValidProjects = utility.ToStringPtrSlice(d.ValidProjects)
	apiDistro.Mountpoints = d.Mountpoints
	apiDistro.SingleTaskDistro = d.SingleTaskDistro
	apiDistro.ImageID = utility.ToStringPtr(d.ImageID)
	apiDistro.ExecUser = utility.ToStringPtr(d.ExecUser)

	if d.Expansions != nil {
		apiDistro.Expansions = []APIExpansion{}
		for _, e := range d.Expansions {
			expansion := APIExpansion{}
			expansion.BuildFromService(e)
			apiDistro.Expansions = append(apiDistro.Expansions, expansion)
		}
	}
	findSettings := APIFinderSettings{}
	findSettings.BuildFromService(d.FinderSettings)
	apiDistro.FinderSettings = findSettings

	planSettings := APIPlannerSettings{}
	planSettings.BuildFromService(d.PlannerSettings)
	apiDistro.PlannerSettings = planSettings

	allocatorSettings := APIHostAllocatorSettings{}
	allocatorSettings.BuildFromService(d.HostAllocatorSettings)
	apiDistro.HostAllocatorSettings = allocatorSettings

	dispatchSettings := APIDispatcherSettings{}
	dispatchSettings.BuildFromService()
	apiDistro.DispatcherSettings = dispatchSettings

	homeVolumeSettings := APIHomeVolumeSettings{}
	homeVolumeSettings.BuildFromService(d.HomeVolumeSettings)
	apiDistro.HomeVolumeSettings = homeVolumeSettings

	icecreamSettings := APIIceCreamSettings{}
	icecreamSettings.BuildFromService(d.IceCreamSettings)
	apiDistro.IcecreamSettings = icecreamSettings
	apiDistro.IsVirtualWorkstation = d.IsVirtualWorkstation
	apiDistro.IsCluster = d.IsCluster

	bootstrapSettings := APIBootstrapSettings{}
	bootstrapSettings.BuildFromService(d.BootstrapSettings)
	apiDistro.BootstrapSettings = bootstrapSettings
}

// ToService returns a service layer distro using the data from APIDistro
func (apiDistro *APIDistro) ToService() *distro.Distro {
	d := distro.Distro{}
	d.Id = utility.FromStringPtr(apiDistro.Name)
	d.AdminOnly = apiDistro.AdminOnly
	d.Aliases = apiDistro.Aliases
	d.Arch = utility.FromStringPtr(apiDistro.Arch)
	d.WorkDir = utility.FromStringPtr(apiDistro.WorkDir)
	d.Provider = utility.FromStringPtr(apiDistro.Provider)
	d.ProviderSettingsList = apiDistro.ProviderSettingsList
	d.SetupAsSudo = apiDistro.SetupAsSudo
	d.Setup = utility.FromStringPtr(apiDistro.Setup)
	d.User = utility.FromStringPtr(apiDistro.User)
	d.BootstrapSettings = apiDistro.BootstrapSettings.ToService()
	d.SSHOptions = apiDistro.SSHOptions
	d.AuthorizedKeysFile = utility.FromStringPtr(apiDistro.AuthorizedKeysFile)
	d.SpawnAllowed = apiDistro.UserSpawnAllowed
	d.Mountpoints = apiDistro.Mountpoints
	d.SingleTaskDistro = apiDistro.SingleTaskDistro
	d.Expansions = []distro.Expansion{}
	for _, e := range apiDistro.Expansions {
		d.Expansions = append(d.Expansions, e.ToService())
	}
	d.Disabled = apiDistro.Disabled
	d.ContainerPool = utility.FromStringPtr(apiDistro.ContainerPool)
	d.ImageID = utility.FromStringPtr(apiDistro.ImageID)
	d.ExecUser = utility.FromStringPtr(apiDistro.ExecUser)

	d.FinderSettings = apiDistro.FinderSettings.ToService()
	d.PlannerSettings = apiDistro.PlannerSettings.ToService()
	d.HostAllocatorSettings = apiDistro.HostAllocatorSettings.ToService()
	d.DispatcherSettings = apiDistro.DispatcherSettings.ToService()
	d.HomeVolumeSettings = apiDistro.HomeVolumeSettings.ToService()
	d.IceCreamSettings = apiDistro.IcecreamSettings.ToService()

	d.DisableShallowClone = apiDistro.DisableShallowClone
	d.Note = utility.FromStringPtr(apiDistro.Note)
	d.WarningNote = utility.FromStringPtr(apiDistro.WarningNote)
	d.ValidProjects = utility.FromStringPtrSlice(apiDistro.ValidProjects)

	d.IsVirtualWorkstation = apiDistro.IsVirtualWorkstation
	d.IsCluster = apiDistro.IsCluster

	return &d
}

// APIExpansion is derived from a service layer distro.Expansion
type APIExpansion struct {
	Key   *string `json:"key"`
	Value *string `json:"value"`
}

// BuildFromService converts a service level distro.Expansion to an APIExpansion
func (e *APIExpansion) BuildFromService(val distro.Expansion) {
	e.Key = utility.ToStringPtr(val.Key)
	e.Value = utility.ToStringPtr(val.Value)
}

// ToService returns a service layer distro.Expansion using the data from an APIExpansion
func (e *APIExpansion) ToService() distro.Expansion {
	d := distro.Expansion{
		Key:   utility.FromStringPtr(e.Key),
		Value: utility.FromStringPtr(e.Value),
	}

	return d
}
