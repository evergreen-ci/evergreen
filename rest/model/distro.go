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
	Name             APIString               `json:"name"`
	UserSpawnAllowed bool                    `json:"user_spawn_allowed"`
	Provider         APIString               `json:"provider"`
	ProviderSettings *map[string]interface{} `json:"settings,omitempty"`
	ImageID          APIString               `json:"image_id,omitempty"`
	Arch             APIString               `json:"arch,omitempty"`
	WorkDir          APIString               `json:"work_dir,omitempty"`
	PoolSize         int                     `json:"pool_size,omitempty"`
	SetupAsSudo      bool                    `json:"setup_as_sudo,omitempty"`
	Setup            APIString               `json:"setup,omitempty"`
	Teardown         APIString               `json:"teardown,omitempty"`
	User             APIString               `json:"user,omitempty"`
	SSHKey           APIString               `json:"ssh_key,omitempty"`
	SSHOptions       []string                `json:"ssh_options,omitempty"`
	Expansions       map[string]string       `json:"expansions,omitempty"`
	Disabled         bool                    `json:"disabled,omitempty"`
	ContainerPool    APIString               `json:"container_pool,omitempty"`
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

		apiDistro.ProviderSettings = v.ProviderSettings
		apiDistro.Arch = ToAPIString(v.Arch)
		apiDistro.WorkDir = ToAPIString(v.WorkDir)
		apiDistro.PoolSize = v.PoolSize
		apiDistro.SetupAsSudo = v.SetupAsSudo
		apiDistro.Setup = ToAPIString(v.Setup)
		apiDistro.Teardown = ToAPIString(v.Teardown)
		apiDistro.User = ToAPIString(v.User)
		apiDistro.Disabled = v.Disabled
		apiDistro.ContainerPool = ToAPIString(v.ContainerPool)
		apiDistro.SSHOptions = v.SSHOptions
		expansions := make(map[string]string)
		for _, e := range v.Expansions {
			expansions[e.Key] = e.Value
		}
		apiDistro.Expansions = expansions

	default:
		return errors.Errorf("incorrect type when fetching converting distro type")
	}
	return nil
}

// ToService returns a service layer distro using the data from APIDistro.
func (apiDistro *APIDistro) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not impelemented for APIDistro")
}

func (apiDistro *APIDistro) Filter(isSuper bool) {
	if !isSuper {
		apiDistro.Expansions = nil
		apiDistro.User = nil
		apiDistro.Setup = nil
		apiDistro.Teardown = nil
		apiDistro.SSHKey = nil
	}
}
