package model

import (
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/pkg/errors"
)

// APIRecentTaskStats is the model to be returned by the API whenever recent tasks are fetched.
type APIRecentTaskStats struct {
	Total              int `json:"total"`
	Inactive           int `json:"inactive"`
	Unstarted          int `json:"unstarted"`
	Started            int `json:"started"`
	Succeeded          int `json:"succeeded"`
	Failed             int `json:"failed"`
	SystemFailed       int `json:"system-failed"`
	SetupFailed        int `json:"setup-failed"`
	SystemUnresponsive int `json:"system-unresponsive"`
	SystemTimedOut     int `json:"system-timed-out"`
	TestTimedOut       int `json:"test-timed-out"`
}

// BuildFromService converts from service level structs to an APIRecentTaskStats.
func (apiStatus *APIRecentTaskStats) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *task.ResultCounts:
		apiStatus.Total = v.Total
		apiStatus.Inactive = v.Inactive
		apiStatus.Unstarted = v.Unstarted
		apiStatus.Started = v.Started
		apiStatus.Succeeded = v.Succeeded
		apiStatus.SetupFailed = v.SetupFailed
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

// ToService returns a service layer distro using the data from APIRecentTaskStats.
func (apiStatus *APIRecentTaskStats) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not implemented for APIRecentTaskStats")
}

type APIResult struct {
	Name  APIString `json:"name"`
	Count int       `json:"count"`
}

type APIRecentTaskStatsList struct {
	Total              []APIResult `json:"total"`
	Inactive           []APIResult `json:"inactive"`
	Unstarted          []APIResult `json:"unstarted"`
	Started            []APIResult `json:"started"`
	Succeeded          []APIResult `json:"succeeded"`
	Failed             []APIResult `json:"failed"`
	SetupFailed        []APIResult `json:"setup-failed"`
	SystemFailed       []APIResult `json:"system-failed"`
	SystemUnresponsive []APIResult `json:"system-unresponsive"`
	SystemTimedOut     []APIResult `json:"system-timed-out"`
	TestTimedOut       []APIResult `json:"test-timed-out"`
}

func (s *APIRecentTaskStatsList) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *task.ResultCountList:
		for _, result := range v.Total {
			s.Total = append(s.Total, APIResult{Name: ToAPIString(result.Name), Count: result.Count})
		}
		for _, result := range v.Inactive {
			s.Inactive = append(s.Inactive, APIResult{Name: ToAPIString(result.Name), Count: result.Count})
		}
		for _, result := range v.Unstarted {
			s.Unstarted = append(s.Unstarted, APIResult{Name: ToAPIString(result.Name), Count: result.Count})
		}
		for _, result := range v.Started {
			s.Started = append(s.Started, APIResult{Name: ToAPIString(result.Name), Count: result.Count})
		}
		for _, result := range v.Succeeded {
			s.Succeeded = append(s.Succeeded, APIResult{Name: ToAPIString(result.Name), Count: result.Count})
		}
		for _, result := range v.Failed {
			s.Failed = append(s.Failed, APIResult{Name: ToAPIString(result.Name), Count: result.Count})
		}
		for _, result := range v.SetupFailed {
			s.SetupFailed = append(s.SetupFailed, APIResult{Name: ToAPIString(result.Name), Count: result.Count})
		}
		for _, result := range v.SystemFailed {
			s.SystemFailed = append(s.SystemFailed, APIResult{Name: ToAPIString(result.Name), Count: result.Count})
		}
		for _, result := range v.SystemUnresponsive {
			s.SystemUnresponsive = append(s.SystemUnresponsive, APIResult{Name: ToAPIString(result.Name), Count: result.Count})
		}
		for _, result := range v.SystemTimedOut {
			s.SystemTimedOut = append(s.SystemTimedOut, APIResult{Name: ToAPIString(result.Name), Count: result.Count})
		}
		for _, result := range v.TestTimedOut {
			s.TestTimedOut = append(s.TestTimedOut, APIResult{Name: ToAPIString(result.Name), Count: result.Count})
		}
	default:
		return errors.Errorf("incorrect type when converting result count list (%T)", v)
	}
	return nil
}

func (s *APIRecentTaskStatsList) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not implemented for APIRecentTaskStatsList")
}

// APIHostStatsByDistro is a slice of host stats for a distro
// the 3 structs below are nested within it
type APIHostStatsByDistro struct {
	Distros []apiHostStatsForDistro `json:"distros"`
}

type apiHostStatsForDistro struct {
	Distro   APIString `json:"distro"`
	Status   APIString `json:"status"`
	NumHosts int       `json:"num_hosts"`
	NumTasks int       `json:"running_tasks"`
	MaxHosts int       `json:"max_hosts"`
}

// BuildFromService takes the slice of stats returned by GetHostStatsByDistro and embeds
// them so that the return value is a slice of distros
func (s *APIHostStatsByDistro) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case []host.StatsByDistro:
		for _, entry := range v {
			d := apiHostStatsForDistro{
				Distro:   ToAPIString(entry.Distro),
				Status:   ToAPIString(entry.Status),
				NumHosts: entry.Count,
				NumTasks: entry.NumTasks,
				MaxHosts: entry.MaxHosts,
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
