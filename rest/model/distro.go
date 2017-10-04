package model

import (
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/pkg/errors"
)

// APIDistro is the model to be returned by the API whenever distros are fetched.
// EVG-1717 will implement the remainder of the distro model.
type APIDistro struct {
	Name APIString `json:"name"`
}

// BuildFromService converts from service level structs to an APIDistro.
func (apiDistro *APIDistro) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case distro.Distro:
		apiDistro.Id = APIString(v.Id)
	default:
		return errors.Errorf("incorrect type when fetching converting distro type")
	}
	return nil
}

// ToService returns a service layer distro using the data from APIDistro.
func (apiDistro *APIDistro) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not impelemented for APIDistro")
}
