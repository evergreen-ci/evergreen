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
	Version                APIString   `json:"version"`
	MinimumHosts           int         `json:"minimum_hosts"`
	MaximumHosts           int         `json:"maximum_hosts"`
	TargetTime             APIDuration `json:"target_time"`
	AcceptableHostIdleTime APIDuration `json:"acceptable_host_idle_time"`
	GroupVersions          *bool       `json:"group_versions"`
	PatchZipperFactor      int         `json:"patch_zipper_factor"`
	TaskOrdering           APIString   `json:"task_ordering"`
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
		return errors.Errorf("%T is not an supported expansion type", h)
	}

	if settings.Version == "" {
		s.Version = ToAPIString(evergreen.PlannerVersionLegacy)
	} else {
		s.Version = ToAPIString(settings.Version)
	}
	s.MinimumHosts = settings.MinimumHosts
	s.MaximumHosts = settings.MaximumHosts
	s.TargetTime = NewAPIDuration(settings.TargetTime)
	s.AcceptableHostIdleTime = NewAPIDuration(settings.AcceptableHostIdleTime)
	s.GroupVersions = settings.GroupVersions
	s.PatchZipperFactor = settings.PatchZipperFactor
	s.TaskOrdering = ToAPIString(settings.TaskOrdering)

	return nil
}

// ToService returns a service layer distro.PlannerSettings using the data from APIPlannerSettings
func (s *APIPlannerSettings) ToService() (interface{}, error) {
	settings := distro.PlannerSettings{}
	settings.Version = FromAPIString(s.Version)
	if settings.Version == "" {
		settings.Version = evergreen.PlannerVersionLegacy
	}
	settings.MinimumHosts = s.MinimumHosts
	settings.MaximumHosts = s.MaximumHosts
	settings.TargetTime = s.TargetTime.ToDuration()
	settings.AcceptableHostIdleTime = s.AcceptableHostIdleTime.ToDuration()
	settings.GroupVersions = s.GroupVersions
	settings.PatchZipperFactor = s.PatchZipperFactor
	settings.TaskOrdering = FromAPIString(s.TaskOrdering)

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
		return errors.Errorf("%T is not an supported expansion type", h)
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
// APIDistro is the model to be returned by the API whenever distros are fetched

type APIDistro struct {
	Name                  APIString              `json:"name"`
	UserSpawnAllowed      bool                   `json:"user_spawn_allowed"`
	Provider              APIString              `json:"provider"`
	ProviderSettings      map[string]interface{} `json:"settings"`
	ImageID               APIString              `json:"image_id"`
	Arch                  APIString              `json:"arch"`
	WorkDir               APIString              `json:"work_dir"`
	PoolSize              int                    `json:"pool_size"`
	SetupAsSudo           bool                   `json:"setup_as_sudo"`
	Setup                 APIString              `json:"setup"`
	Teardown              APIString              `json:"teardown"`
	User                  APIString              `json:"user"`
	BootstrapMethod       APIString              `json:"bootstrap_method"`
	CommunicationMethod   APIString              `json:"communication_method"`
	CloneMethod           APIString              `json:"clone_method"`
	ShellPath             APIString              `json:"shell_path"`
	CuratorDir            APIString              `json:"curator_dir"`
	ClientDir             APIString              `json:"client_dir"`
	JasperCredentialsPath APIString              `json:"jasper_credentials_path"`
	SSHKey                APIString              `json:"ssh_key"`
	SSHOptions            []string               `json:"ssh_options"`
	Expansions            []APIExpansion         `json:"expansions"`
	Disabled              bool                   `json:"disabled"`
	ContainerPool         APIString              `json:"container_pool"`
	PlannerSettings       APIPlannerSettings     `json:"planner_settings"`
	FinderSettings        APIFinderSettings      `json:"finder_settings"`
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
		return errors.Errorf("%T is not an supported expansion type", h)
	}

	apiDistro.Name = ToAPIString(d.Id)
	apiDistro.UserSpawnAllowed = d.SpawnAllowed
	apiDistro.Provider = ToAPIString(d.Provider)
	if d.ProviderSettings != nil && (d.Provider == evergreen.ProviderNameEc2Auto || d.Provider == evergreen.ProviderNameEc2OnDemand || d.Provider == evergreen.ProviderNameEc2Spot) {
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
	apiDistro.PoolSize = d.PoolSize
	apiDistro.SetupAsSudo = d.SetupAsSudo
	apiDistro.Setup = ToAPIString(d.Setup)
	apiDistro.Teardown = ToAPIString(d.Teardown)
	apiDistro.User = ToAPIString(d.User)
	if d.BootstrapMethod == "" {
		d.BootstrapMethod = distro.BootstrapMethodLegacySSH
	}
	apiDistro.BootstrapMethod = ToAPIString(d.BootstrapMethod)
	if d.CommunicationMethod == "" {
		d.CommunicationMethod = distro.CommunicationMethodLegacySSH
	}
	apiDistro.CommunicationMethod = ToAPIString(d.CommunicationMethod)
	if d.CloneMethod == "" {
		d.CloneMethod = distro.CloneMethodLegacySSH
	}
	apiDistro.CloneMethod = ToAPIString(d.CloneMethod)
	apiDistro.ShellPath = ToAPIString(d.ShellPath)
	apiDistro.CuratorDir = ToAPIString(d.CuratorDir)
	apiDistro.ClientDir = ToAPIString(d.ClientDir)
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
	plannerSettings := APIPlannerSettings{}
	if err := plannerSettings.BuildFromService(d.PlannerSettings); err != nil {
		return errors.Wrap(err, "Error converting from distro.PlannerSettings to model.APIPlannerSettings")
	}
	apiDistro.PlannerSettings = plannerSettings
	finderSettings := APIFinderSettings{}
	if err := finderSettings.BuildFromService(d.FinderSettings); err != nil {
		return errors.Wrap(err, "Error converting from distro.FinderSettings to model.APIFinderSettings")
	}
	apiDistro.FinderSettings = finderSettings

	return nil
}

// ToService returns a service layer distro using the data from APIDistro
func (apiDistro *APIDistro) ToService() (interface{}, error) {
	d := distro.Distro{}
	d.Id = FromAPIString(apiDistro.Name)
	d.Arch = FromAPIString(apiDistro.Arch)
	d.WorkDir = FromAPIString(apiDistro.WorkDir)
	d.PoolSize = apiDistro.PoolSize
	d.Provider = FromAPIString(apiDistro.Provider)
	if apiDistro.ProviderSettings != nil {
		d.ProviderSettings = &apiDistro.ProviderSettings
	}
	d.SetupAsSudo = apiDistro.SetupAsSudo
	d.Setup = FromAPIString(apiDistro.Setup)
	d.Teardown = FromAPIString(apiDistro.Teardown)
	d.User = FromAPIString(apiDistro.User)
	d.BootstrapMethod = FromAPIString(apiDistro.BootstrapMethod)
	if d.BootstrapMethod == "" {
		d.BootstrapMethod = distro.BootstrapMethodLegacySSH
	}
	d.CommunicationMethod = FromAPIString(apiDistro.CommunicationMethod)
	if d.CommunicationMethod == "" {
		d.CommunicationMethod = distro.CommunicationMethodLegacySSH
	}
	d.CloneMethod = FromAPIString(apiDistro.CloneMethod)
	if d.CloneMethod == "" {
		d.CloneMethod = distro.CloneMethodLegacySSH
	}
	d.ShellPath = FromAPIString(apiDistro.ShellPath)
	d.CuratorDir = FromAPIString(apiDistro.CuratorDir)
	d.ClientDir = FromAPIString(apiDistro.ClientDir)
	d.SSHKey = FromAPIString(apiDistro.SSHKey)
	d.SSHOptions = apiDistro.SSHOptions
	d.SpawnAllowed = apiDistro.UserSpawnAllowed
	d.Expansions = []distro.Expansion{}
	for _, e := range apiDistro.Expansions {
		i, err := e.ToService()
		if err != nil {
			return nil, errors.Wrap(err, "Error converting from model.APIExpansion to distro.Expansion")
		}
		expansion, ok := i.(distro.Expansion)
		if !ok {
			return nil, errors.Errorf("Unexpected type %T for distro.Expansion", i)
		}
		d.Expansions = append(d.Expansions, expansion)
	}
	d.Disabled = apiDistro.Disabled
	d.ContainerPool = FromAPIString(apiDistro.ContainerPool)
	i, err := apiDistro.PlannerSettings.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "Error converting from model.APIPlannerSettings to distro.PlannerSetting")
	}
	plannerSettings, ok := i.(distro.PlannerSettings)
	if !ok {
		return nil, errors.Errorf("Unexpected type %T for distro.PlannerSettings", i)
	}
	d.PlannerSettings = plannerSettings
	i, err = apiDistro.FinderSettings.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "Error converting from model.APIFinderSettings to distro.FinderSetting")
	}
	finderSettings, ok := i.(distro.FinderSettings)
	if !ok {
		return nil, errors.Errorf("Unexpected type %T for distro.FinderSettings", i)
	}
	d.FinderSettings = finderSettings

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
		return errors.Errorf("%T is not an supported expansion type", h)
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
