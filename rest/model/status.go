package model

import (
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
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
	Name  APIString `json:"name"`
	Count int       `json:"count"`
}

type APIStatList []APIStat

func (s *APIStatList) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case []task.Stat:
		for _, stat := range v {
			*s = append(*s, APIStat{Name: ToAPIString(stat.Name), Count: stat.Count})
		}
	default:
		return errors.Errorf("incorrect type when converting API stat list (%T)", v)
	}

	return nil
}

type APIRecentTaskStatsList struct {
	Total              APIStatList `json:"total"`
	Inactive           APIStatList `json:"inactive"`
	Unstarted          APIStatList `json:"unstarted"`
	Started            APIStatList `json:"started"`
	Succeeded          APIStatList `json:"succeeded"`
	Failed             APIStatList `json:"failed"`
	SetupFailed        APIStatList `json:"setup-failed"`
	SystemFailed       APIStatList `json:"system-failed"`
	SystemUnresponsive APIStatList `json:"system-unresponsive"`
	SystemTimedOut     APIStatList `json:"system-timed-out"`
	TestTimedOut       APIStatList `json:"test-timed-out"`
}

func (s *APIRecentTaskStatsList) BuildFromService(h interface{}) error {
	catcher := grip.NewBasicCatcher()
	switch v := h.(type) {
	case task.ResultCountList:
		apiList := APIStatList{}
		catcher.Add(apiList.BuildFromService(v.Total))
		s.Total = apiList

		apiList = APIStatList{}
		catcher.Add(apiList.BuildFromService(v.Inactive))
		s.Inactive = apiList

		apiList = APIStatList{}
		catcher.Add(apiList.BuildFromService(v.Unstarted))
		s.Unstarted = apiList

		apiList = APIStatList{}
		catcher.Add(apiList.BuildFromService(v.Started))
		s.Started = apiList

		apiList = APIStatList{}
		catcher.Add(apiList.BuildFromService(v.Succeeded))
		s.Succeeded = apiList

		apiList = APIStatList{}
		catcher.Add(apiList.BuildFromService(v.Failed))
		s.Failed = apiList

		apiList = APIStatList{}
		catcher.Add(apiList.BuildFromService(v.SetupFailed))
		s.SetupFailed = apiList

		apiList = APIStatList{}
		catcher.Add(apiList.BuildFromService(v.SystemFailed))
		s.SystemFailed = apiList

		apiList = APIStatList{}
		catcher.Add(apiList.BuildFromService(v.SystemUnresponsive))
		s.SystemUnresponsive = apiList

		apiList = APIStatList{}
		catcher.Add(apiList.BuildFromService(v.SetupFailed))
		s.SetupFailed = apiList

		apiList = APIStatList{}
		catcher.Add(apiList.BuildFromService(v.Unstarted))
		s.SystemTimedOut = apiList

		apiList = APIStatList{}
		catcher.Add(apiList.BuildFromService(v.TestTimedOut))
		s.TestTimedOut = apiList
	default:
		return errors.Errorf("incorrect type when converting result count list (%T)", v)
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
