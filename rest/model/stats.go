package model

import (
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/pkg/errors"
)

// APIStats is the model to be returned by the API whenever recent tasks are fetched.
type APIStats struct {
	Total              int `json:"total"`
	Inactive           int `json:"inactive"`
	Unstarted          int `json:"unstarted"`
	Started            int `json:"started"`
	Succeeded          int `json:"succeeded"`
	Failed             int `json:"failed"`
	SystemFailed       int `json:"system-failed"`
	SystemUnresponsive int `json:"system-unresponsive"`
	SystemTimedOut     int `json:"system-timed-out"`
	TestTimedOut       int `json:"test-timed-out"`
}

// BuildFromService converts from service level structs to an APIStats.
func (apiStatus *APIStats) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *task.ResultCounts:
		apiStatus.Total = v.Total
		apiStatus.Inactive = v.Inactive
		apiStatus.Unstarted = v.Unstarted
		apiStatus.Started = v.Started
		apiStatus.Succeeded = v.Succeeded
		apiStatus.Failed = v.Failed
		apiStatus.SystemFailed = v.SystemFailed
		apiStatus.SystemUnresponsive = v.SystemUnresponsive
		apiStatus.SystemTimedOut = v.SystemTimedOut
		apiStatus.TestTimedOut = v.TestTimedOut
	default:
		return errors.Errorf("incorrect type when converting result counts (%T)", v)
	}
	return nil
}

// ToService returns a service layer distro using the data from APIStats.
func (apiStatus *APIStats) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not implemented for APIStats")
}
