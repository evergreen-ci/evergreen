package model

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/pkg/errors"
)

// APIVersionCost is the model to be returned by the API whenever cost data is fetched by version id.
type APIVersionCost struct {
	VersionId     APIString   `json:"version_id"`
	SumTimeTaken  APIDuration `json:"sum_time_taken"`
	EstimatedCost float64     `json:"estimated_cost"`
}

// BuildFromService converts from a service level task by loading the data
// into the appropriate fields of the APIVersionCost.
func (apiVersionCost *APIVersionCost) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *task.VersionCost:
		apiVersionCost.VersionId = APIString(v.VersionId)
		apiVersionCost.SumTimeTaken = NewAPIDuration(v.SumTimeTaken)
		apiVersionCost.EstimatedCost = v.SumEstimatedCost
	default:
		return errors.Errorf("incorrect type when fetching converting version cost type")
	}
	return nil
}

// ToService returns a service layer version cost using the data from APIVersionCost.
func (apiVersionCost *APIVersionCost) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not implemented for APIVersionCost")
}

// APIDistroCost is the model to be returned by the API whenever cost data is fetched by distro id.
type APIDistroCost struct {
	DistroId      APIString   `json:"distro_id"`
	SumTimeTaken  APIDuration `json:"sum_time_taken"`
	Provider      APIString   `json:"provider"`
	InstanceType  APIString   `json:"instance_type,omitempty"`
	EstimatedCost float64     `json:"estimated_cost"`
	NumTasks      int         `json:"num_tasks"`
}

// BuildFromService converts from a service level task by loading the data
// into the appropriate fields of the APIDistroCost.
func (apiDistroCost *APIDistroCost) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *task.DistroCost:
		apiDistroCost.DistroId = APIString(v.DistroId)
		apiDistroCost.SumTimeTaken = NewAPIDuration(v.SumTimeTaken)
		apiDistroCost.Provider = APIString(v.Provider)
		apiDistroCost.EstimatedCost = v.SumEstimatedCost
		apiDistroCost.NumTasks = v.NumTasks

		// InstanceType field is only set if the provider is ec2 or ec2-spot.
		// It will default to an empty string for other providers.
		if v.Provider == evergreen.ProviderNameEc2OnDemand || v.Provider == evergreen.ProviderNameEc2Spot {
			instanceTypeStr, ok := v.ProviderSettings["instance_type"].(string)
			if !ok {
				return errors.Errorf("ec2 instance type in provider settings does not have a string value")
			}
			if instanceTypeStr == "" {
				return errors.Errorf("ec2 missing instance type in provider settings")
			}
			apiDistroCost.InstanceType = APIString(instanceTypeStr)
		}
	default:
		return errors.Errorf("incorrect type when fetching converting distro cost type")
	}
	return nil
}

// ToService returns a service layer distro cost using the data from APIDistroCost.
func (apiDistroCost *APIDistroCost) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not implemented for APIDistroCost")
}
