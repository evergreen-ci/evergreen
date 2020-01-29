package model

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/mitchellh/mapstructure"
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
		s.Version = ToStringPtr(evergreen.PlannerVersionLegacy)
	} else {
		s.Version = ToStringPtr(settings.Version)
	}
	s.TargetTime = NewAPIDuration(settings.TargetTime)
	s.GroupVersions = settings.GroupVersions
	s.PatchFactor = settings.PatchFactor
	s.ExpectedRuntimeFactor = settings.ExpectedRuntimeFactor
	s.PatchTimeInQueueFactor = settings.PatchTimeInQueueFactor
	s.MainlineTimeInQueueFactor = settings.MainlineTimeInQueueFactor

	return nil
}

// ToService returns a service layer distro.PlannerSettings using the data from APIPlannerSettings
func (s *APIPlannerSettings) ToService() (interface{}, error) {
	settings := distro.PlannerSettings{}
	settings.Version = FromStringPtr(s.Version)
	if settings.Version == "" {
		settings.Version = evergreen.PlannerVersionLegacy
	}
	settings.TargetTime = s.TargetTime.ToDuration()
	settings.GroupVersions = s.GroupVersions
	settings.PatchFactor = s.PatchFactor
	settings.PatchTimeInQueueFactor = s.PatchTimeInQueueFactor
	settings.MainlineTimeInQueueFactor = s.MainlineTimeInQueueFactor
	settings.ExpectedRuntimeFactor = s.ExpectedRuntimeFactor

	return interface{}(settings), nil
}

////////////////////////////////////////////////////////////////////////////////
//
// APIHostAllocatorSettings is the model to be returned by the API whenever distro.HostAllocatorSettings are fetched

type APIHostAllocatorSettings struct {
	Version                *string     `json:"version"`
	MinimumHosts           int         `json:"minimum_hosts"`
	MaximumHosts           int         `json:"maximum_hosts"`
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
		s.Version = ToStringPtr(evergreen.HostAllocatorUtilization)
	} else {
		s.Version = ToStringPtr(settings.Version)
	}
	s.MinimumHosts = settings.MinimumHosts
	s.MaximumHosts = settings.MaximumHosts
	s.AcceptableHostIdleTime = NewAPIDuration(settings.AcceptableHostIdleTime)

	return nil
}

// ToService returns a service layer distro.HostAllocatorSettings using the data from APIHostAllocatorSettings
func (s *APIHostAllocatorSettings) ToService() (interface{}, error) {
	settings := distro.HostAllocatorSettings{}
	settings.Version = FromStringPtr(s.Version)
	if settings.Version == "" {
		settings.Version = evergreen.HostAllocatorUtilization
	}
	settings.MinimumHosts = s.MinimumHosts
	settings.MaximumHosts = s.MaximumHosts
	settings.AcceptableHostIdleTime = s.AcceptableHostIdleTime.ToDuration()

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
		s.Version = ToStringPtr(evergreen.FinderVersionLegacy)
	} else {
		s.Version = ToStringPtr(settings.Version)
	}

	return nil
}

// ToService returns a service layer distro.FinderSettings using the data from APIFinderSettings
func (s *APIFinderSettings) ToService() (interface{}, error) {
	settings := distro.FinderSettings{}
	settings.Version = FromStringPtr(s.Version)
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
		s.Version = ToStringPtr(evergreen.DispatcherVersionRevised)
	} else {
		s.Version = ToStringPtr(settings.Version)
	}

	return nil
}

// ToService returns a service layer distro.DispatcherSettings using the data from APIDispatcherSettings
func (s *APIDispatcherSettings) ToService() (interface{}, error) {
	settings := distro.DispatcherSettings{}
	settings.Version = FromStringPtr(s.Version)
	if settings.Version == "" {
		settings.Version = evergreen.DispatcherVersionRevised
	}

	return interface{}(settings), nil
}

////////////////////////////////////////////////////////////////////////////////
//
// APIBootstrapSettings is the model to be returned by the API whenever distro.BootstrapSettings are fetched

type APIBootstrapSettings struct {
	Method                *string           `json:"method"`
	Communication         *string           `json:"communication"`
	ClientDir             *string           `json:"client_dir"`
	JasperBinaryDir       *string           `json:"jasper_binary_dir"`
	JasperCredentialsPath *string           `json:"jasper_credentials_path"`
	ServiceUser           *string           `json:"service_user"`
	ShellPath             *string           `json:"shell_path"`
	RootDir               *string           `json:"root_dir"`
	Env                   []APIEnvVar       `json:"env"`
	ResourceLimits        APIResourceLimits `json:"resource_limits"`
}

type APIEnvVar struct {
	Key   *string `json:"key"`
	Value *string `json:"value"`
}

// BuildFromService converts a service level distro.EnvVar to an APIEnvVar
func (e *APIEnvVar) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case distro.EnvVar:
		e.Key = ToStringPtr(v.Key)
		e.Value = ToStringPtr(v.Value)
	default:
		return errors.Errorf("%T is not a supported environment variable type", h)
	}
	return nil
}

// ToService returns a service layer distro.EnvVar using the data from an APIEnvVar
func (e *APIEnvVar) ToService() (interface{}, error) {
	d := distro.EnvVar{}
	d.Key = FromStringPtr(e.Key)
	d.Value = FromStringPtr(e.Value)

	return interface{}(d), nil
}

type APIResourceLimits struct {
	NumFiles        int `json:"num_files"`
	NumProcesses    int `json:"num_processes"`
	LockedMemoryKB  int `json:"locked_memory"`
	VirtualMemoryKB int `json:"virtual_memory"`
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

	s.Method = ToStringPtr(settings.Method)
	if FromStringPtr(s.Method) == "" {
		s.Method = ToStringPtr(distro.BootstrapMethodLegacySSH)
	}
	s.Communication = ToStringPtr(settings.Communication)
	if FromStringPtr(s.Communication) == "" {
		s.Communication = ToStringPtr(distro.CommunicationMethodLegacySSH)
	}
	s.ClientDir = ToStringPtr(settings.ClientDir)
	s.JasperBinaryDir = ToStringPtr(settings.JasperBinaryDir)
	s.JasperCredentialsPath = ToStringPtr(settings.JasperCredentialsPath)
	s.ServiceUser = ToStringPtr(settings.ServiceUser)
	s.ShellPath = ToStringPtr(settings.ShellPath)
	s.RootDir = ToStringPtr(settings.RootDir)
	for _, envVar := range settings.Env {
		apiEnvVar := APIEnvVar{}
		if err := apiEnvVar.BuildFromService(envVar); err != nil {
			return errors.Wrap(err, "error building environment variable")
		}
		s.Env = append(s.Env, apiEnvVar)
	}

	s.ResourceLimits.NumFiles = settings.ResourceLimits.NumFiles
	s.ResourceLimits.NumProcesses = settings.ResourceLimits.NumProcesses
	s.ResourceLimits.LockedMemoryKB = settings.ResourceLimits.LockedMemoryKB
	s.ResourceLimits.VirtualMemoryKB = settings.ResourceLimits.VirtualMemoryKB

	return nil
}

// ToService returns a service layer distro.BootstrapSettings using the data
// from APIBootstrapSettings.
func (s *APIBootstrapSettings) ToService() (interface{}, error) {
	settings := distro.BootstrapSettings{}
	settings.Method = FromStringPtr(s.Method)
	if settings.Method == "" {
		settings.Method = distro.BootstrapMethodLegacySSH
	}
	settings.Communication = FromStringPtr(s.Communication)
	if settings.Communication == "" {
		settings.Communication = distro.CommunicationMethodLegacySSH
	}
	settings.ClientDir = FromStringPtr(s.ClientDir)
	settings.JasperBinaryDir = FromStringPtr(s.JasperBinaryDir)
	settings.JasperCredentialsPath = FromStringPtr(s.JasperCredentialsPath)
	settings.ServiceUser = FromStringPtr(s.ServiceUser)
	settings.ShellPath = FromStringPtr(s.ShellPath)
	settings.RootDir = FromStringPtr(s.RootDir)
	for _, apiEnvVar := range s.Env {
		i, err := apiEnvVar.ToService()
		if err != nil {
			return nil, errors.Wrap(err, "error building environment variable")
		}
		envVar, ok := i.(distro.EnvVar)
		if !ok {
			return nil, errors.Errorf("unexpected type %T for environment variable", i)
		}
		settings.Env = append(settings.Env, envVar)
	}

	settings.ResourceLimits.NumFiles = s.ResourceLimits.NumFiles
	settings.ResourceLimits.NumProcesses = s.ResourceLimits.NumProcesses
	settings.ResourceLimits.LockedMemoryKB = s.ResourceLimits.LockedMemoryKB
	settings.ResourceLimits.VirtualMemoryKB = s.ResourceLimits.VirtualMemoryKB

	return settings, nil
}

////////////////////////////////////////////////////////////////////////////////
//
// APIDistro is the model to be returned by the API whenever distros are fetched

type APIDistro struct {
	Name                  *string                  `json:"name"`
	Aliases               []string                 `json:"aliases"`
	UserSpawnAllowed      bool                     `json:"user_spawn_allowed"`
	Provider              *string                  `json:"provider"`
	ProviderSettings      map[string]interface{}   `json:"settings"`
	ImageID               *string                  `json:"image_id"`
	Arch                  *string                  `json:"arch"`
	WorkDir               *string                  `json:"work_dir"`
	SetupAsSudo           bool                     `json:"setup_as_sudo"`
	Setup                 *string                  `json:"setup"`
	Teardown              *string                  `json:"teardown"`
	User                  *string                  `json:"user"`
	BootstrapSettings     APIBootstrapSettings     `json:"bootstrap_settings"`
	CloneMethod           *string                  `json:"clone_method"`
	SSHKey                *string                  `json:"ssh_key"`
	SSHOptions            []string                 `json:"ssh_options"`
	Expansions            []APIExpansion           `json:"expansions"`
	Disabled              bool                     `json:"disabled"`
	ContainerPool         *string                  `json:"container_pool"`
	FinderSettings        APIFinderSettings        `json:"finder_settings"`
	PlannerSettings       APIPlannerSettings       `json:"planner_settings"`
	DispatcherSettings    APIDispatcherSettings    `json:"dispatcher_settings"`
	HostAllocatorSettings APIHostAllocatorSettings `json:"host_allocator_settings"`
	DisableShallowClone   bool                     `json:"disable_shallow_clone"`
	UseLegacyAgent        bool                     `json:"use_legacy_agent"`
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

	apiDistro.Name = ToStringPtr(d.Id)
	apiDistro.Aliases = d.Aliases
	apiDistro.UserSpawnAllowed = d.SpawnAllowed
	apiDistro.Provider = ToStringPtr(d.Provider)
	if d.ProviderSettings != nil && cloud.IsEc2Provider(d.Provider) {
		ec2Settings := &cloud.EC2ProviderSettings{}
		err := mapstructure.Decode(d.ProviderSettings, ec2Settings)
		if err != nil {
			return err
		}
		apiDistro.ImageID = ToStringPtr(ec2Settings.AMI)
	}
	if d.ProviderSettings != nil {
		apiDistro.ProviderSettings = *d.ProviderSettings
	}
	apiDistro.Arch = ToStringPtr(d.Arch)
	apiDistro.WorkDir = ToStringPtr(d.WorkDir)
	apiDistro.SetupAsSudo = d.SetupAsSudo
	apiDistro.Setup = ToStringPtr(d.Setup)
	apiDistro.Teardown = ToStringPtr(d.Teardown)
	apiDistro.User = ToStringPtr(d.User)
	bootstrapSettings := APIBootstrapSettings{}
	if err := bootstrapSettings.BuildFromService(d.BootstrapSettings); err != nil {
		return errors.Wrap(err, "error converting from distro.BootstrapSettings to model.APIBootstrapSettings")
	}
	apiDistro.BootstrapSettings = bootstrapSettings
	if d.CloneMethod == "" {
		d.CloneMethod = distro.CloneMethodLegacySSH
	}
	apiDistro.CloneMethod = ToStringPtr(d.CloneMethod)
	apiDistro.SSHKey = ToStringPtr(d.SSHKey)
	apiDistro.Disabled = d.Disabled
	apiDistro.ContainerPool = ToStringPtr(d.ContainerPool)
	apiDistro.SSHOptions = d.SSHOptions
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
		return errors.Wrap(err, "Error converting from distro.HostAllocatorSettings to model.API.HostAllocatorSettings")
	}
	apiDistro.HostAllocatorSettings = allocatorSettings
	// DispatcherSettings
	dispatchSettings := APIDispatcherSettings{}
	if err := dispatchSettings.BuildFromService(d.DispatcherSettings); err != nil {
		return errors.Wrap(err, "Error converting from distro.HostAllocatorSettings to model.API.HostAllocatorSettings")
	}
	apiDistro.DispatcherSettings = dispatchSettings
	apiDistro.DisableShallowClone = d.DisableShallowClone
	apiDistro.UseLegacyAgent = d.UseLegacyAgent
	apiDistro.Note = ToStringPtr(d.Note)
	apiDistro.ValidProjects = ToStringPtrSlice(d.ValidProjects)

	return nil
}

// ToService returns a service layer distro using the data from APIDistro
func (apiDistro *APIDistro) ToService() (interface{}, error) {
	d := distro.Distro{}
	d.Id = FromStringPtr(apiDistro.Name)
	d.Aliases = apiDistro.Aliases
	d.Arch = FromStringPtr(apiDistro.Arch)
	d.WorkDir = FromStringPtr(apiDistro.WorkDir)
	d.Provider = FromStringPtr(apiDistro.Provider)
	if apiDistro.ProviderSettings != nil {
		d.ProviderSettings = &apiDistro.ProviderSettings
	}
	d.SetupAsSudo = apiDistro.SetupAsSudo
	d.Setup = FromStringPtr(apiDistro.Setup)
	d.Teardown = FromStringPtr(apiDistro.Teardown)
	d.User = FromStringPtr(apiDistro.User)
	i, err := apiDistro.BootstrapSettings.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "error converting from model.APIBootstrapSettings to distro.BootstrapSettings")
	}
	bootstrapSettings, ok := i.(distro.BootstrapSettings)
	if !ok {
		return nil, errors.Errorf("unexpected type %T for distro.BootstrapSettings", i)
	}
	d.BootstrapSettings = bootstrapSettings
	d.CloneMethod = FromStringPtr(apiDistro.CloneMethod)
	if d.CloneMethod == "" {
		d.CloneMethod = distro.CloneMethodLegacySSH
	}
	d.SSHKey = FromStringPtr(apiDistro.SSHKey)
	d.SSHOptions = apiDistro.SSHOptions
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
	d.ContainerPool = FromStringPtr(apiDistro.ContainerPool)
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
	d.UseLegacyAgent = apiDistro.UseLegacyAgent
	d.Note = FromStringPtr(apiDistro.Note)
	d.ValidProjects = FromStringPtrSlice(apiDistro.ValidProjects)

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
		e.Key = ToStringPtr(val.Key)
		e.Value = ToStringPtr(val.Value)
	default:
		return errors.Errorf("%T is not a supported expansion type", h)
	}
	return nil
}

// ToService returns a service layer distro.Expansion using the data from an APIExpansion
func (e *APIExpansion) ToService() (interface{}, error) {
	d := distro.Expansion{}
	d.Key = FromStringPtr(e.Key)
	d.Value = FromStringPtr(e.Value)

	return interface{}(d), nil
}
