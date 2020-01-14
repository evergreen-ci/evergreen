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
	Version               APIString   `json:"version"`
	TargetTime            APIDuration `json:"target_time"`
	GroupVersions         *bool       `json:"group_versions"`
	PatchFactor           int64       `json:"patch_factor"`
	TimeInQueueFactor     int64       `json:"time_in_queue_factor"`
	ExpectedRuntimeFactor int64       `json:"expected_runtime_factor"`
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
		s.Version = ToAPIString(evergreen.PlannerVersionLegacy)
	} else {
		s.Version = ToAPIString(settings.Version)
	}
	s.TargetTime = NewAPIDuration(settings.TargetTime)
	s.GroupVersions = settings.GroupVersions
	s.PatchFactor = settings.PatchFactor
	s.ExpectedRuntimeFactor = settings.ExpectedRuntimeFactor
	s.TimeInQueueFactor = settings.TimeInQueueFactor

	return nil
}

// ToService returns a service layer distro.PlannerSettings using the data from APIPlannerSettings
func (s *APIPlannerSettings) ToService() (interface{}, error) {
	settings := distro.PlannerSettings{}
	settings.Version = FromAPIString(s.Version)
	if settings.Version == "" {
		settings.Version = evergreen.PlannerVersionLegacy
	}
	settings.TargetTime = s.TargetTime.ToDuration()
	settings.GroupVersions = s.GroupVersions
	settings.PatchFactor = s.PatchFactor
	settings.TimeInQueueFactor = s.TimeInQueueFactor
	settings.ExpectedRuntimeFactor = s.ExpectedRuntimeFactor

	return interface{}(settings), nil
}

////////////////////////////////////////////////////////////////////////////////
//
// APIHostAllocatorSettings is the model to be returned by the API whenever distro.HostAllocatorSettings are fetched

type APIHostAllocatorSettings struct {
	Version                APIString   `json:"version"`
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
		s.Version = ToAPIString(evergreen.HostAllocatorUtilization)
	} else {
		s.Version = ToAPIString(settings.Version)
	}
	s.MinimumHosts = settings.MinimumHosts
	s.MaximumHosts = settings.MaximumHosts
	s.AcceptableHostIdleTime = NewAPIDuration(settings.AcceptableHostIdleTime)

	return nil
}

// ToService returns a service layer distro.HostAllocatorSettings using the data from APIHostAllocatorSettings
func (s *APIHostAllocatorSettings) ToService() (interface{}, error) {
	settings := distro.HostAllocatorSettings{}
	settings.Version = FromAPIString(s.Version)
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
	Version APIString `json:"version"`
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
		s.Version = ToAPIString(evergreen.FinderVersionLegacy)
	} else {
		s.Version = ToAPIString(settings.Version)
	}

	return nil
}

// ToService returns a service layer distro.FinderSettings using the data from APIFinderSettings
func (s *APIFinderSettings) ToService() (interface{}, error) {
	settings := distro.FinderSettings{}
	settings.Version = FromAPIString(s.Version)
	if settings.Version == "" {
		settings.Version = evergreen.FinderVersionLegacy
	}

	return interface{}(settings), nil
}

////////////////////////////////////////////////////////////////////////////////
//
// APIDispatcherSettings is the model to be returned by the API whenever distro.DispatcherSettings are fetched

type APIDispatcherSettings struct {
	Version APIString `json:"version"`
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
		s.Version = ToAPIString(evergreen.DispatcherVersionRevised)
	} else {
		s.Version = ToAPIString(settings.Version)
	}

	return nil
}

// ToService returns a service layer distro.DispatcherSettings using the data from APIDispatcherSettings
func (s *APIDispatcherSettings) ToService() (interface{}, error) {
	settings := distro.DispatcherSettings{}
	settings.Version = FromAPIString(s.Version)
	if settings.Version == "" {
		settings.Version = evergreen.DispatcherVersionRevised
	}

	return interface{}(settings), nil
}

////////////////////////////////////////////////////////////////////////////////
//
// APIBootstrapSettings is the model to be returned by the API whenever distro.BootstrapSettings are fetched

type APIBootstrapSettings struct {
	Method                APIString         `json:"method"`
	Communication         APIString         `json:"communication"`
	ClientDir             APIString         `json:"client_dir"`
	JasperBinaryDir       APIString         `json:"jasper_binary_dir"`
	JasperCredentialsPath APIString         `json:"jasper_credentials_path"`
	ServiceUser           APIString         `json:"service_user"`
	ShellPath             APIString         `json:"shell_path"`
	RootDir               APIString         `json:"root_dir"`
	Env                   []APIEnvVar       `json:"env"`
	ResourceLimits        APIResourceLimits `json:"resource_limits"`
}

type APIEnvVar struct {
	Key   APIString `json:"key"`
	Value APIString `json:"value"`
}

// BuildFromService converts a service level distro.EnvVar to an APIEnvVar
func (e *APIEnvVar) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case distro.EnvVar:
		e.Key = ToAPIString(v.Key)
		e.Value = ToAPIString(v.Value)
	default:
		return errors.Errorf("%T is not a supported environment variable type", h)
	}
	return nil
}

// ToService returns a service layer distro.EnvVar using the data from an APIEnvVar
func (e *APIEnvVar) ToService() (interface{}, error) {
	d := distro.EnvVar{}
	d.Key = FromAPIString(e.Key)
	d.Value = FromAPIString(e.Value)

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

	s.Method = ToAPIString(settings.Method)
	if FromAPIString(s.Method) == "" {
		s.Method = ToAPIString(distro.BootstrapMethodLegacySSH)
	}
	s.Communication = ToAPIString(settings.Communication)
	if FromAPIString(s.Communication) == "" {
		s.Communication = ToAPIString(distro.CommunicationMethodLegacySSH)
	}
	s.ClientDir = ToAPIString(settings.ClientDir)
	s.JasperBinaryDir = ToAPIString(settings.JasperBinaryDir)
	s.JasperCredentialsPath = ToAPIString(settings.JasperCredentialsPath)
	s.ServiceUser = ToAPIString(settings.ServiceUser)
	s.ShellPath = ToAPIString(settings.ShellPath)
	s.RootDir = ToAPIString(settings.RootDir)
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
	settings.Method = FromAPIString(s.Method)
	if settings.Method == "" {
		settings.Method = distro.BootstrapMethodLegacySSH
	}
	settings.Communication = FromAPIString(s.Communication)
	if settings.Communication == "" {
		settings.Communication = distro.CommunicationMethodLegacySSH
	}
	settings.ClientDir = FromAPIString(s.ClientDir)
	settings.JasperBinaryDir = FromAPIString(s.JasperBinaryDir)
	settings.JasperCredentialsPath = FromAPIString(s.JasperCredentialsPath)
	settings.ServiceUser = FromAPIString(s.ServiceUser)
	settings.ShellPath = FromAPIString(s.ShellPath)
	settings.RootDir = FromAPIString(s.RootDir)
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
	Name                  APIString                `json:"name"`
	Aliases               []string                 `json:"aliases"`
	UserSpawnAllowed      bool                     `json:"user_spawn_allowed"`
	Provider              APIString                `json:"provider"`
	ProviderSettings      map[string]interface{}   `json:"settings"`
	ImageID               APIString                `json:"image_id"`
	Arch                  APIString                `json:"arch"`
	WorkDir               APIString                `json:"work_dir"`
	SetupAsSudo           bool                     `json:"setup_as_sudo"`
	Setup                 APIString                `json:"setup"`
	Teardown              APIString                `json:"teardown"`
	User                  APIString                `json:"user"`
	BootstrapSettings     APIBootstrapSettings     `json:"bootstrap_settings"`
	CloneMethod           APIString                `json:"clone_method"`
	SSHKey                APIString                `json:"ssh_key"`
	SSHOptions            []string                 `json:"ssh_options"`
	Expansions            []APIExpansion           `json:"expansions"`
	Disabled              bool                     `json:"disabled"`
	ContainerPool         APIString                `json:"container_pool"`
	FinderSettings        APIFinderSettings        `json:"finder_settings"`
	PlannerSettings       APIPlannerSettings       `json:"planner_settings"`
	DispatcherSettings    APIDispatcherSettings    `json:"dispatcher_settings"`
	HostAllocatorSettings APIHostAllocatorSettings `json:"host_allocator_settings"`
	DisableShallowClone   bool                     `json:"disable_shallow_clone"`
	UseLegacyAgent        bool                     `json:"use_legacy_agent"`
	Note                  APIString                `json:"note"`
	MountCommand           APIString                `json:"mount_command"`
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

	apiDistro.Name = ToAPIString(d.Id)
	apiDistro.Aliases = d.Aliases
	apiDistro.UserSpawnAllowed = d.SpawnAllowed
	apiDistro.Provider = ToAPIString(d.Provider)
	if d.ProviderSettings != nil && cloud.IsEc2Provider(d.Provider) {
		ec2Settings := &cloud.EC2ProviderSettings{}
		err := mapstructure.Decode(d.ProviderSettings, ec2Settings)
		if err != nil {
			return err
		}
		apiDistro.ImageID = ToAPIString(ec2Settings.AMI)
	}
	if d.ProviderSettings != nil {
		apiDistro.ProviderSettings = *d.ProviderSettings
	}
	apiDistro.Arch = ToAPIString(d.Arch)
	apiDistro.WorkDir = ToAPIString(d.WorkDir)
	apiDistro.SetupAsSudo = d.SetupAsSudo
	apiDistro.Setup = ToAPIString(d.Setup)
	apiDistro.Teardown = ToAPIString(d.Teardown)
	apiDistro.User = ToAPIString(d.User)
	bootstrapSettings := APIBootstrapSettings{}
	if err := bootstrapSettings.BuildFromService(d.BootstrapSettings); err != nil {
		return errors.Wrap(err, "error converting from distro.BootstrapSettings to model.APIBootstrapSettings")
	}
	apiDistro.BootstrapSettings = bootstrapSettings
	if d.CloneMethod == "" {
		d.CloneMethod = distro.CloneMethodLegacySSH
	}
	apiDistro.CloneMethod = ToAPIString(d.CloneMethod)
	apiDistro.SSHKey = ToAPIString(d.SSHKey)
	apiDistro.Disabled = d.Disabled
	apiDistro.ContainerPool = ToAPIString(d.ContainerPool)
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
	apiDistro.Note = ToAPIString(d.Note)
	apiDistro.MountCommand = ToAPIString(d.MountCommand)

	return nil
}

// ToService returns a service layer distro using the data from APIDistro
func (apiDistro *APIDistro) ToService() (interface{}, error) {
	d := distro.Distro{}
	d.Id = FromAPIString(apiDistro.Name)
	d.Aliases = apiDistro.Aliases
	d.Arch = FromAPIString(apiDistro.Arch)
	d.WorkDir = FromAPIString(apiDistro.WorkDir)
	d.Provider = FromAPIString(apiDistro.Provider)
	if apiDistro.ProviderSettings != nil {
		d.ProviderSettings = &apiDistro.ProviderSettings
	}
	d.SetupAsSudo = apiDistro.SetupAsSudo
	d.Setup = FromAPIString(apiDistro.Setup)
	d.Teardown = FromAPIString(apiDistro.Teardown)
	d.User = FromAPIString(apiDistro.User)
	i, err := apiDistro.BootstrapSettings.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "error converting from model.APIBootstrapSettings to distro.BootstrapSettings")
	}
	bootstrapSettings, ok := i.(distro.BootstrapSettings)
	if !ok {
		return nil, errors.Errorf("unexpected type %T for distro.BootstrapSettings", i)
	}
	d.BootstrapSettings = bootstrapSettings
	d.CloneMethod = FromAPIString(apiDistro.CloneMethod)
	if d.CloneMethod == "" {
		d.CloneMethod = distro.CloneMethodLegacySSH
	}
	d.SSHKey = FromAPIString(apiDistro.SSHKey)
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
	d.ContainerPool = FromAPIString(apiDistro.ContainerPool)
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
	d.Note = FromAPIString(apiDistro.Note)
	d.MountCommand = FromAPIString(apiDistro.MountCommand)

	return &d, nil
}

// APIExpansion is derived from a service layer distro.Expansion
type APIExpansion struct {
	Key   APIString `json:"key"`
	Value APIString `json:"value"`
}

// BuildFromService converts a service level distro.Expansion to an APIExpansion
func (e *APIExpansion) BuildFromService(h interface{}) error {
	switch val := h.(type) {
	case distro.Expansion:
		e.Key = ToAPIString(val.Key)
		e.Value = ToAPIString(val.Value)
	default:
		return errors.Errorf("%T is not a supported expansion type", h)
	}
	return nil
}

// ToService returns a service layer distro.Expansion using the data from an APIExpansion
func (e *APIExpansion) ToService() (interface{}, error) {
	d := distro.Expansion{}
	d.Key = FromAPIString(e.Key)
	d.Value = FromAPIString(e.Value)

	return interface{}(d), nil
}
