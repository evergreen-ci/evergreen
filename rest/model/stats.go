package model

import (
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/pkg/errors"
)

// APITaskStats is the model to be returned by the API whenever recent tasks are fetched.
type APITaskStats struct {
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

// BuildFromService converts from service level structs to an APITaskStats.
func (apiStatus *APITaskStats) BuildFromService(h interface{}) error {
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

// ToService returns a service layer distro using the data from APITaskStats.
func (apiStatus *APITaskStats) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not implemented for APITaskStats")
}

// APIHostStatsByDistro is a slice of host stats for a distro
// the 3 structs below are nested within it
type APIHostStatsByDistro struct {
	Distros []apiHostStatsForDistro `json:"distros"`
}

type apiHostStatsForDistro struct {
	Distro string                   `json:"distro"`
	Stats  apiSingleDistroHostStats `json:"stats"`
}

type apiSingleDistroHostStats struct {
	Status string                   `json:"status"`
	Counts apiSingleStatusHostStats `json:"counts"`
}

type apiSingleStatusHostStats struct {
	NumHosts int `json:"num_hosts"`
	NumTasks int `json:"running_tasks"`
}

// BuildFromService takes the slice of stats returned by GetHostStatsByDistro and embeds
// them so that the return value is a slice of distros
func (s *APIHostStatsByDistro) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case []host.HostStatsByDistro:
		for _, entry := range v {
			d := apiHostStatsForDistro{
				Distro: entry.Distro,
				Stats: apiSingleDistroHostStats{
					Status: entry.Status,
					Counts: apiSingleStatusHostStats{
						NumHosts: entry.Count,
						NumTasks: entry.NumTasks,
					},
				},
			}

			s.Distros = append(s.Distros, d)
		}
	default:
		return errors.Errorf("incorrect type when converting host stats by distro (%T)", v)
	}
	return nil
}

// ToService is not implemented for APIHostStatsByDistro
func (s *APIHostStatsByDistro) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not implemented for APIHostStatsByDistro")
}
