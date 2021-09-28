package model

import (
	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

////////////////////////////////////////////////////////////////////////////////
//
// APIPlannerSettings is the model to be returned by the API whenever distro.PlannerSettings are fetched

type APIPlannerSettings struct {
	Version                   *string     `json:"version"`
	TargetTime                APIDuration `json:"target_time"`
	GroupVersions             *bool       `json:"group_versions"`
	PatchFactor               int64       `json:"patch_factor"`
	PatchTimeInQueueFactor    int64       `json:"patch_time_in_queue_factor"`
	MainlineTimeInQueueFactor int64       `json:"mainline_time_in_queue_factor"`
	ExpectedRuntimeFactor     int64       `json:"expected_runtime_factor"`
	GenerateTaskFactor        int64       `json:"generate_task_factor"`
}

// BuildFromService converts from service level distro.PlannerSetting to an APIPlannerSettings
func (s *APIPlannerSettings) BuildFromService(h interface{}) error {
	var settings distro.PlannerSettings
	switch v := h.(type) {
	case distro.PlannerSettings:
		settings = v
	case *distro.PlannerSettings:
		settings = *v
	default:
		return errors.Errorf("%T is not a supported expansion type", h)
	}

	if settings.Version == "" {
		s.Version = utility.ToStringPtr(evergreen.PlannerVersionLegacy)
	} else {
		s.Version = utility.ToStringPtr(settings.Version)
	}
	s.TargetTime = NewAPIDuration(settings.TargetTime)
	s.GroupVersions = settings.GroupVersions
	s.PatchFactor = settings.PatchFactor
	s.ExpectedRuntimeFactor = settings.ExpectedRuntimeFactor
	s.PatchTimeInQueueFactor = settings.PatchTimeInQueueFactor
	s.MainlineTimeInQueueFactor = settings.MainlineTimeInQueueFactor
	s.GenerateTaskFactor = settings.GenerateTaskFactor
	return nil
}

// ToService returns a service layer distro.PlannerSettings using the data from APIPlannerSettings
func (s *APIPlannerSettings) ToService() (interface{}, error) {
	settings := distro.PlannerSettings{}
	settings.Version = utility.FromStringPtr(s.Version)
	if settings.Version == "" {
		settings.Version = evergreen.PlannerVersionLegacy
	}
	settings.TargetTime = s.TargetTime.ToDuration()
	settings.GroupVersions = s.GroupVersions
	settings.PatchFactor = s.PatchFactor
	settings.PatchTimeInQueueFactor = s.PatchTimeInQueueFactor
	settings.MainlineTimeInQueueFactor = s.MainlineTimeInQueueFactor
	settings.ExpectedRuntimeFactor = s.ExpectedRuntimeFactor
	settings.GenerateTaskFactor = s.GenerateTaskFactor

	return interface{}(settings), nil
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
}

// BuildFromService converts from service level distro.HostAllocatorSettings to an APIHostAllocatorSettings
func (s *APIHostAllocatorSettings) BuildFromService(h interface{}) error {
	var settings distro.HostAllocatorSettings
	switch v := h.(type) {
	case distro.HostAllocatorSettings:
		settings = v
	case *distro.HostAllocatorSettings:
		settings = *v
	default:
		return errors.Errorf("%T is not a supported expansion type", h)
	}

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

	return nil
}

// ToService returns a service layer distro.HostAllocatorSettings using the data from APIHostAllocatorSettings
func (s *APIHostAllocatorSettings) ToService() (interface{}, error) {
	settings := distro.HostAllocatorSettings{}
	settings.Version = utility.FromStringPtr(s.Version)
	if settings.Version == "" {
		settings.Version = evergreen.HostAllocatorUtilization
	}
	settings.MinimumHosts = s.MinimumHosts
	settings.MaximumHosts = s.MaximumHosts
	settings.AcceptableHostIdleTime = s.AcceptableHostIdleTime.ToDuration()
	settings.RoundingRule = utility.FromStringPtr(s.RoundingRule)
	settings.FeedbackRule = utility.FromStringPtr(s.FeedbackRule)
	settings.HostsOverallocatedRule = utility.FromStringPtr(s.HostsOverallocatedRule)

	return interface{}(settings), nil
}

////////////////////////////////////////////////////////////////////////////////
//
// APIFinderSettings is the model to be returned by the API whenever distro.FinderSettings are fetched

type APIFinderSettings struct {
	Version *string `json:"version"`
}

// BuildFromService converts from service level distro.FinderSettings to an APIFinderSettings
func (s *APIFinderSettings) BuildFromService(h interface{}) error {
	var settings distro.FinderSettings
	switch v := h.(type) {
	case distro.FinderSettings:
		settings = v
	case *distro.FinderSettings:
		settings = *v
	default:
		return errors.Errorf("%T is not a supported expansion type", h)
	}

	if settings.Version == "" {
		s.Version = utility.ToStringPtr(evergreen.FinderVersionLegacy)
	} else {
		s.Version = utility.ToStringPtr(settings.Version)
	}

	return nil
}

// ToService returns a service layer distro.FinderSettings using the data from APIFinderSettings
func (s *APIFinderSettings) ToService() (interface{}, error) {
	settings := distro.FinderSettings{}
	settings.Version = utility.FromStringPtr(s.Version)
	if settings.Version == "" {
		settings.Version = evergreen.FinderVersionLegacy
	}

	return interface{}(settings), nil
}

////////////////////////////////////////////////////////////////////////////////
//
// APIDispatcherSettings is the model to be returned by the API whenever distro.DispatcherSettings are fetched

type APIDispatcherSettings struct {
	Version *string `json:"version"`
}

// BuildFromService converts from service level distro.DispatcherSettings to an APIDispatcherSettings
func (s *APIDispatcherSettings) BuildFromService(h interface{}) error {
	var settings distro.DispatcherSettings
	switch v := h.(type) {
	case distro.DispatcherSettings:
		settings = v
	case *distro.DispatcherSettings:
		settings = *v
	default:
		return errors.Errorf("%T is not a supported expansion type", h)
	}

	if settings.Version == "" {
		s.Version = utility.ToStringPtr(evergreen.DispatcherVersionRevised)
	} else {
		s.Version = utility.ToStringPtr(settings.Version)
	}

	return nil
}

// ToService returns a service layer distro.DispatcherSettings using the data from APIDispatcherSettings
func (s *APIDispatcherSettings) ToService() (interface{}, error) {
	settings := distro.DispatcherSettings{}
	settings.Version = utility.FromStringPtr(s.Version)
	if settings.Version == "" {
		settings.Version = evergreen.DispatcherVersionRevised
	}

	return interface{}(settings), nil
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
func (e *APIEnvVar) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case distro.EnvVar:
		e.Key = utility.ToStringPtr(v.Key)
		e.Value = utility.ToStringPtr(v.Value)
	default:
		return errors.Errorf("%T is not a supported environment variable type", h)
	}
	return nil
}

// ToService returns a service layer distro.EnvVar using the data from an APIEnvVar
func (e *APIEnvVar) ToService() (interface{}, error) {
	d := distro.EnvVar{}
	d.Key = utility.FromStringPtr(e.Key)
	d.Value = utility.FromStringPtr(e.Value)

	return interface{}(d), nil
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
func (s *APIPreconditionScript) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case distro.PreconditionScript:
		s.Path = utility.ToStringPtr(v.Path)
		s.Script = utility.ToStringPtr(v.Script)
		return nil
	default:
		return errors.Errorf("%T is not a supported precondition script type", h)
	}
}

// ToService returns a service-level distro.PreconditionScript using the data
// from the APIPreconditionScript.
func (s *APIPreconditionScript) ToService() (interface{}, error) {
	return distro.PreconditionScript{
		Path:   utility.FromStringPtr(s.Path),
		Script: utility.FromStringPtr(s.Script),
	}, nil
}

// BuildFromService converts from service level distro.BootstrapSettings to an
// APIBootstrapSettings.
func (s *APIBootstrapSettings) BuildFromService(h interface{}) error {
	var settings distro.BootstrapSettings
	switch v := h.(type) {
	case distro.BootstrapSettings:
		settings = v
	case *distro.BootstrapSettings:
		settings = *v
	default:
		return errors.Errorf("%T is not a supported expansion type", h)
	}

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
		if err := apiEnvVar.BuildFromService(envVar); err != nil {
			return errors.Wrap(err, "error building environment variable")
		}
		s.Env = append(s.Env, apiEnvVar)
	}
	for _, script := range settings.PreconditionScripts {
		var apiScript APIPreconditionScript
		if err := apiScript.BuildFromService(script); err != nil {
			return errors.Wrap(err, "precondition script")
		}
		s.PreconditionScripts = append(s.PreconditionScripts, apiScript)
	}
	s.ResourceLimits.NumFiles = settings.ResourceLimits.NumFiles
	s.ResourceLimits.NumProcesses = settings.ResourceLimits.NumProcesses
	s.ResourceLimits.NumTasks = settings.ResourceLimits.NumTasks
	s.ResourceLimits.LockedMemoryKB = settings.ResourceLimits.LockedMemoryKB
	s.ResourceLimits.VirtualMemoryKB = settings.ResourceLimits.VirtualMemoryKB

	return nil
}

// ToService returns a service layer distro.BootstrapSettings using the data
// from APIBootstrapSettings.
func (s *APIBootstrapSettings) ToService() (interface{}, error) {
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
		i, err := apiEnvVar.ToService()
		if err != nil {
			return nil, errors.Wrap(err, "error building environment variable")
		}
		envVar, ok := i.(distro.EnvVar)
		if !ok {
			return nil, errors.Errorf("programmatic error: unexpected type %T for environment variable", i)
		}
		settings.Env = append(settings.Env, envVar)
	}
	for _, apiScript := range s.PreconditionScripts {
		i, err := apiScript.ToService()
		if err != nil {
			return nil, errors.Wrap(err, "building precondition script")
		}
		script, ok := i.(distro.PreconditionScript)
		if !ok {
			return nil, errors.Errorf("programmatic error: unexpected type %T for precondition script", i)
		}
		settings.PreconditionScripts = append(settings.PreconditionScripts, script)
	}
	settings.ResourceLimits.NumFiles = s.ResourceLimits.NumFiles
	settings.ResourceLimits.NumProcesses = s.ResourceLimits.NumProcesses
	settings.ResourceLimits.NumTasks = s.ResourceLimits.NumTasks
	settings.ResourceLimits.LockedMemoryKB = s.ResourceLimits.LockedMemoryKB
	settings.ResourceLimits.VirtualMemoryKB = s.ResourceLimits.VirtualMemoryKB

	return settings, nil
}

type APIHomeVolumeSettings struct {
	FormatCommand *string `json:"format_command"`
}

func (s *APIHomeVolumeSettings) BuildFromService(h interface{}) error {
	settings, ok := h.(distro.HomeVolumeSettings)
	if !ok {
		return errors.Errorf("Unexpected type '%T' for HomeVolumeSettings", h)
	}

	s.FormatCommand = utility.ToStringPtr(settings.FormatCommand)

	return nil
}

func (s *APIHomeVolumeSettings) ToService() (interface{}, error) {
	return distro.HomeVolumeSettings{
		FormatCommand: utility.FromStringPtr(s.FormatCommand),
	}, nil
}

type APIIcecreamSettings struct {
	SchedulerHost *string `json:"scheduler_host"`
	ConfigPath    *string `json:"config_path"`
}

func (s *APIIcecreamSettings) BuildFromService(h interface{}) error {
	settings, ok := h.(distro.IcecreamSettings)
	if !ok {
		return errors.Errorf("Unexpected type '%T' for IcecreamSettings", h)
	}

	s.SchedulerHost = utility.ToStringPtr(settings.SchedulerHost)
	s.ConfigPath = utility.ToStringPtr(settings.ConfigPath)

	return nil
}

func (s *APIIcecreamSettings) ToService() (interface{}, error) {
	return distro.IcecreamSettings{
		SchedulerHost: utility.FromStringPtr(s.SchedulerHost),
		ConfigPath:    utility.FromStringPtr(s.ConfigPath),
	}, nil
}

////////////////////////////////////////////////////////////////////////////////
//
// APIDistro is the model to be returned by the API whenever distros are fetched

type APIDistro struct {
	Name                  *string                  `json:"name"`
	Aliases               []string                 `json:"aliases"`
	UserSpawnAllowed      bool                     `json:"user_spawn_allowed"`
	Provider              *string                  `json:"provider"`
	ProviderSettingsList  []*birch.Document        `json:"provider_settings"`
	Arch                  *string                  `json:"arch"`
	WorkDir               *string                  `json:"work_dir"`
	SetupAsSudo           bool                     `json:"setup_as_sudo"`
	Setup                 *string                  `json:"setup"`
	User                  *string                  `json:"user"`
	BootstrapSettings     APIBootstrapSettings     `json:"bootstrap_settings"`
	CloneMethod           *string                  `json:"clone_method"`
	SSHKey                *string                  `json:"ssh_key"`
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
	IcecreamSettings      APIIcecreamSettings      `json:"icecream_settings"`
	IsVirtualWorkstation  bool                     `json:"is_virtual_workstation"`
	IsCluster             bool                     `json:"is_cluster"`
	Note                  *string                  `json:"note"`
	ValidProjects         []*string                `json:"valid_projects"`
}

// BuildFromService converts from service level distro.Distro to an APIDistro
func (apiDistro *APIDistro) BuildFromService(h interface{}) error {
	var d distro.Distro
	switch v := h.(type) {
	case distro.Distro:
		d = v
	case *distro.Distro:
		d = *v
	default:
		return errors.Errorf("%T is not a supported expansion type", h)
	}

	apiDistro.Name = utility.ToStringPtr(d.Id)
	apiDistro.Aliases = d.Aliases
	apiDistro.UserSpawnAllowed = d.SpawnAllowed
	apiDistro.Provider = utility.ToStringPtr(d.Provider)
	apiDistro.ProviderSettingsList = d.ProviderSettingsList
	apiDistro.Arch = utility.ToStringPtr(d.Arch)
	apiDistro.WorkDir = utility.ToStringPtr(d.WorkDir)
	apiDistro.SetupAsSudo = d.SetupAsSudo
	apiDistro.Setup = utility.ToStringPtr(d.Setup)
	apiDistro.User = utility.ToStringPtr(d.User)
	bootstrapSettings := APIBootstrapSettings{}
	if err := bootstrapSettings.BuildFromService(d.BootstrapSettings); err != nil {
		return errors.Wrap(err, "error converting from distro.BootstrapSettings to model.APIBootstrapSettings")
	}
	apiDistro.BootstrapSettings = bootstrapSettings
	if d.CloneMethod == "" {
		d.CloneMethod = distro.CloneMethodLegacySSH
	}
	apiDistro.CloneMethod = utility.ToStringPtr(d.CloneMethod)
	apiDistro.SSHKey = utility.ToStringPtr(d.SSHKey)
	apiDistro.SSHOptions = d.SSHOptions
	apiDistro.AuthorizedKeysFile = utility.ToStringPtr(d.AuthorizedKeysFile)
	apiDistro.Disabled = d.Disabled
	apiDistro.ContainerPool = utility.ToStringPtr(d.ContainerPool)
	if d.Expansions != nil {
		apiDistro.Expansions = []APIExpansion{}
		for _, e := range d.Expansions {
			expansion := APIExpansion{}
			if err := expansion.BuildFromService(e); err != nil {
				return errors.Wrap(err, "Error converting from distro.Expansion to model.APIExpansion")
			}
			apiDistro.Expansions = append(apiDistro.Expansions, expansion)
		}
	}
	// FinderSetting
	findSettings := APIFinderSettings{}
	if err := findSettings.BuildFromService(d.FinderSettings); err != nil {
		return errors.Wrap(err, "Error converting from distro.FinderSettings to model.APIFinderSettings")
	}
	apiDistro.FinderSettings = findSettings
	// PlannerSettings
	planSettings := APIPlannerSettings{}
	if err := planSettings.BuildFromService(d.PlannerSettings); err != nil {
		return errors.Wrap(err, "Error converting from distro.PlannerSettings to model.APIPlannerSettings")
	}
	apiDistro.PlannerSettings = planSettings
	// HostAllocatorSettings
	allocatorSettings := APIHostAllocatorSettings{}
	if err := allocatorSettings.BuildFromService(d.HostAllocatorSettings); err != nil {
		return errors.Wrap(err, "Error converting from distro.HostAllocatorSettings to model.APIHostAllocatorSettings")
	}
	apiDistro.HostAllocatorSettings = allocatorSettings
	// DispatcherSettings
	dispatchSettings := APIDispatcherSettings{}
	if err := dispatchSettings.BuildFromService(d.DispatcherSettings); err != nil {
		return errors.Wrap(err, "Error converting from distro.HostAllocatorSettings to model.APIHostAllocatorSettings")
	}
	apiDistro.DispatcherSettings = dispatchSettings
	apiDistro.DisableShallowClone = d.DisableShallowClone
	apiDistro.Note = utility.ToStringPtr(d.Note)
	apiDistro.ValidProjects = utility.ToStringPtrSlice(d.ValidProjects)
	homeVolumeSettings := APIHomeVolumeSettings{}
	if err := homeVolumeSettings.BuildFromService(d.HomeVolumeSettings); err != nil {
		return errors.Wrap(err, "Error converting from distro.HomeVolumeSettings to model.APIHomeVolumeSettings")
	}
	apiDistro.HomeVolumeSettings = homeVolumeSettings
	icecreamSettings := APIIcecreamSettings{}
	if err := icecreamSettings.BuildFromService(d.IcecreamSettings); err != nil {
		return errors.Wrap(err, "Error converting from distro.IcecreamSettings to model.APIIcecreamSettings")
	}
	apiDistro.IcecreamSettings = icecreamSettings
	apiDistro.IsVirtualWorkstation = d.IsVirtualWorkstation
	apiDistro.IsCluster = d.IsCluster

	return nil
}

// ToService returns a service layer distro using the data from APIDistro
func (apiDistro *APIDistro) ToService() (interface{}, error) {
	d := distro.Distro{}
	d.Id = utility.FromStringPtr(apiDistro.Name)
	d.Aliases = apiDistro.Aliases
	d.Arch = utility.FromStringPtr(apiDistro.Arch)
	d.WorkDir = utility.FromStringPtr(apiDistro.WorkDir)
	d.Provider = utility.FromStringPtr(apiDistro.Provider)
	d.ProviderSettingsList = apiDistro.ProviderSettingsList
	d.SetupAsSudo = apiDistro.SetupAsSudo
	d.Setup = utility.FromStringPtr(apiDistro.Setup)
	d.User = utility.FromStringPtr(apiDistro.User)
	i, err := apiDistro.BootstrapSettings.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "error converting from model.APIBootstrapSettings to distro.BootstrapSettings")
	}
	bootstrapSettings, ok := i.(distro.BootstrapSettings)
	if !ok {
		return nil, errors.Errorf("unexpected type %T for distro.BootstrapSettings", i)
	}
	d.BootstrapSettings = bootstrapSettings
	d.CloneMethod = utility.FromStringPtr(apiDistro.CloneMethod)
	if d.CloneMethod == "" {
		d.CloneMethod = distro.CloneMethodLegacySSH
	}
	d.SSHKey = utility.FromStringPtr(apiDistro.SSHKey)
	d.SSHOptions = apiDistro.SSHOptions
	d.AuthorizedKeysFile = utility.FromStringPtr(apiDistro.AuthorizedKeysFile)
	d.SpawnAllowed = apiDistro.UserSpawnAllowed
	d.Expansions = []distro.Expansion{}
	for _, e := range apiDistro.Expansions {
		i, err = e.ToService()
		if err != nil {
			return nil, errors.Wrap(err, "Error converting from model.APIExpansion to distro.Expansion")
		}
		var expansion distro.Expansion
		expansion, ok = i.(distro.Expansion)
		if !ok {
			return nil, errors.Errorf("Unexpected type %T for distro.Expansion", i)
		}
		d.Expansions = append(d.Expansions, expansion)
	}
	d.Disabled = apiDistro.Disabled
	d.ContainerPool = utility.FromStringPtr(apiDistro.ContainerPool)
	// FinderSettings
	i, err = apiDistro.FinderSettings.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "Error converting from model.APIFinderSettings to distro.FinderSettings")
	}
	findSettings, ok := i.(distro.FinderSettings)
	if !ok {
		return nil, errors.Errorf("Unexpected type %T for distro.FinderSettings", i)
	}
	d.FinderSettings = findSettings
	// PlannerSettings
	i, err = apiDistro.PlannerSettings.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "Error converting from model.APIPlannerSettings to distro.PlannerSettings")
	}
	planSettings, ok := i.(distro.PlannerSettings)
	if !ok {
		return nil, errors.Errorf("Unexpected type %T for distro.PlannerSettings", i)
	}
	d.PlannerSettings = planSettings
	// HostAllocatorSettings
	i, err = apiDistro.HostAllocatorSettings.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "Error converting from model.APIHostAllocatorSettings to distro.HostAllocatorSettings")
	}
	allocatorSettings, ok := i.(distro.HostAllocatorSettings)
	if !ok {
		return nil, errors.Errorf("Unexpected type %T for distro.HostAllocatorSettings", i)
	}
	d.HostAllocatorSettings = allocatorSettings
	// DispatcherSettings
	i, err = apiDistro.DispatcherSettings.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "Error converting from model.APIDispatcherSettings to distro.DispatcherSettings")
	}
	dispatchSettings, ok := i.(distro.DispatcherSettings)
	if !ok {
		return nil, errors.Errorf("Unexpected type %T for distro.DispatcherSettings", i)
	}
	d.DispatcherSettings = dispatchSettings
	d.DisableShallowClone = apiDistro.DisableShallowClone
	d.Note = utility.FromStringPtr(apiDistro.Note)
	d.ValidProjects = utility.FromStringPtrSlice(apiDistro.ValidProjects)
	i, err = apiDistro.HomeVolumeSettings.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "Error converting from model.APIHomeVolumeSettings to distro.HomeVolumeSettings")
	}
	homeVolumeSettings, ok := i.(distro.HomeVolumeSettings)
	if !ok {
		return nil, errors.Errorf("Unexpected type %T for distro.HomeVolumeSettings", i)
	}
	d.HomeVolumeSettings = homeVolumeSettings

	i, err = apiDistro.IcecreamSettings.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "Error converting from model.APIIcecreamSettings to distro.IcecreamSettings")
	}
	icecreamSettings, ok := i.(distro.IcecreamSettings)
	if !ok {
		return nil, errors.Errorf("Unexpected type %T for distro.IcecreamSettings", i)
	}
	d.IcecreamSettings = icecreamSettings
	d.IsVirtualWorkstation = apiDistro.IsVirtualWorkstation
	d.IsCluster = apiDistro.IsCluster

	return &d, nil
}

// APIExpansion is derived from a service layer distro.Expansion
type APIExpansion struct {
	Key   *string `json:"key"`
	Value *string `json:"value"`
}

// BuildFromService converts a service level distro.Expansion to an APIExpansion
func (e *APIExpansion) BuildFromService(h interface{}) error {
	switch val := h.(type) {
	case distro.Expansion:
		e.Key = utility.ToStringPtr(val.Key)
		e.Value = utility.ToStringPtr(val.Value)
	default:
		return errors.Errorf("%T is not a supported expansion type", h)
	}
	return nil
}

// ToService returns a service layer distro.Expansion using the data from an APIExpansion
func (e *APIExpansion) ToService() (interface{}, error) {
	d := distro.Expansion{}
	d.Key = utility.FromStringPtr(e.Key)
	d.Value = utility.FromStringPtr(e.Value)

	return interface{}(d), nil
}

// APIDistroScriptOptions provides a model to execute scripts on hosts in a
// distro.
type APIDistroScriptOptions struct {
	Script            string `json:"script"`
	IncludeTaskHosts  bool   `json:"include_task_hosts"`
	IncludeSpawnHosts bool   `json:"include_spawn_hosts"`
	Sudo              bool   `json:"sudo"`
	SudoUser          string `json:"sudo_user"`
}
