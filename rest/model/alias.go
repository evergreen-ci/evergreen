package model

import (
	"github.com/evergreen-ci/evergreen/model"
	"github.com/pkg/errors"
)

// APIAlias is the model to be returned by the API whenever aliass are fetched.
type APIAlias struct {
	Alias   APIString `json:"alias"`
	Variant APIString `json:"variant"`
	Task    APIString `json:"task"`
}

// BuildFromService converts from service level structs to an APIAlias.
func (apiAlias *APIAlias) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case model.ProjectAlias:
		apiAlias.Alias = APIString(v.Alias)
		apiAlias.Variant = APIString(v.Variant)
		apiAlias.Task = APIString(v.Task)
	default:
		return errors.Errorf("incorrect type when fetching converting alias type")
	}
	return nil
}

// ToService returns a service layer alias using the data from APIAlias.
func (apiAlias *APIAlias) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not impelemented for APIAlias")
}
