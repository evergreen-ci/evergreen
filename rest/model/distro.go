package model

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

// APIDistro is the model to be returned by the API whenever distros are fetched.
type APIDistro struct {
	Name             APIString              `json:"name"`
	UserSpawnAllowed bool                   `json:"user_spawn_allowed"`
	Provider         APIString              `json:"provider"`
	ProviderSettings map[string]interface{} `json:"settings"`
	ImageID          APIString              `json:"image_id"`
	Arch             APIString              `json:"arch"`
	WorkDir          APIString              `json:"work_dir"`
	PoolSize         int                    `json:"pool_size"`
	SetupAsSudo      bool                   `json:"setup_as_sudo"`
	Setup            APIString              `json:"setup"`
	Teardown         APIString              `json:"teardown"`
	User             APIString              `json:"user"`
	SSHKey           APIString              `json:"ssh_key"`
	SSHOptions       []string               `json:"ssh_options"`
	Expansions       []APIExpansion         `json:"expansions"`
	Disabled         bool                   `json:"disabled"`
	ContainerPool    APIString              `json:"container_pool"`
}

// BuildFromService converts from service level structs to an APIDistro.
func (apiDistro *APIDistro) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case distro.Distro:
		apiDistro.Name = ToAPIString(v.Id)
		apiDistro.UserSpawnAllowed = v.SpawnAllowed
		apiDistro.Provider = ToAPIString(v.Provider)
		if v.ProviderSettings != nil && (v.Provider == evergreen.ProviderNameEc2Auto || v.Provider == evergreen.ProviderNameEc2OnDemand || v.Provider == evergreen.ProviderNameEc2Spot) {
			ec2Settings := &cloud.EC2ProviderSettings{}
			err := mapstructure.Decode(v.ProviderSettings, ec2Settings)
			if err != nil {
				return err
			}
			apiDistro.ImageID = ToAPIString(ec2Settings.AMI)
		}
		if v.ProviderSettings != nil {
			apiDistro.ProviderSettings = *v.ProviderSettings
		}
		apiDistro.Arch = ToAPIString(v.Arch)
		apiDistro.WorkDir = ToAPIString(v.WorkDir)
		apiDistro.PoolSize = v.PoolSize
		apiDistro.SetupAsSudo = v.SetupAsSudo
		apiDistro.Setup = ToAPIString(v.Setup)
		apiDistro.Teardown = ToAPIString(v.Teardown)
		apiDistro.User = ToAPIString(v.User)
		apiDistro.SSHKey = ToAPIString(v.SSHKey)
		apiDistro.Disabled = v.Disabled
		apiDistro.ContainerPool = ToAPIString(v.ContainerPool)
		apiDistro.SSHOptions = v.SSHOptions
		apiDistro.Expansions = []APIExpansion{}
		for _, e := range v.Expansions {
			apiExpansion := &APIExpansion{}
			apiExpansion.BuildFromService(e)
			apiDistro.Expansions = append(apiDistro.Expansions, *apiExpansion)
		}

	default:
		return errors.Errorf("%T is not an supported expansion type", h)
	}
	return nil
}

// ToService returns a service layer distro using the data from APIDistro.
func (apiDistro *APIDistro) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not impelemented for APIDistro")
}

// APIExpansion is the return model having parsed and transformed a distro.Expansion
type APIExpansion struct {
	Key   APIString `json:"key"`
	Value APIString `json:"value"`
}

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

// ToService returns a service layer distro using the data from AAPIExpansion.
func (e *APIExpansion) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not impelemented for APIExpansion")
}
