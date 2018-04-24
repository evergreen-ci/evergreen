package model

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

// APIDistro is the model to be returned by the API whenever distros are fetched.
// EVG-1717 will implement the remainder of the distro model.
type APIDistro struct {
	Name             APIString `json:"name"`
	UserSpawnAllowed bool      `json:"user_spawn_allowed"`
	Provider         APIString `json:"provider"`
	ImageID          APIString `json:"image_id,omitempty"`
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
	default:
		return errors.Errorf("incorrect type when fetching converting distro type")
	}
	return nil
}

// ToService returns a service layer distro using the data from APIDistro.
func (apiDistro *APIDistro) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not impelemented for APIDistro")
}
