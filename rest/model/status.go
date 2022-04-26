package model

import (
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
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

type APIStat struct {
	Name  *string `json:"name"`
	Count int     `json:"count"`
}

type APIStatList []APIStat

func (s *APIStatList) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case []task.Stat:
		for _, stat := range v {
			*s = append(*s, APIStat{Name: utility.ToStringPtr(stat.Name), Count: stat.Count})
		}
	default:
		return errors.Errorf("incorrect type when converting API stat list (%T)", v)
	}

	return nil
}

type APIRecentTaskStatsList map[string][]APIStat

func (s *APIRecentTaskStatsList) BuildFromService(h interface{}) error {
	catcher := grip.NewBasicCatcher()
	switch v := h.(type) {
	case map[string][]task.Stat:
		for status, stat := range v {
			list := APIStatList{}
			catcher.Add(list.BuildFromService(stat))
			(*s)[status] = list
		}
	default:
		return errors.Errorf("incorrect type when converting APIRecentTaskStatsList (%T)", v)
	}

	return catcher.Resolve()
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
	Distro   *string `json:"distro"`
	Status   *string `json:"status"`
	NumHosts int     `json:"num_hosts"`
	NumTasks int     `json:"running_tasks"`
	MaxHosts int     `json:"max_hosts"`
}

// BuildFromService takes the slice of stats returned by GetHostStatsByDistro and embeds
// them so that the return value is a slice of distros
func (s *APIHostStatsByDistro) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case []host.StatsByDistro:
		for _, entry := range v {
			d := apiHostStatsForDistro{
				Distro:   utility.ToStringPtr(entry.Distro),
				Status:   utility.ToStringPtr(entry.Status),
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
