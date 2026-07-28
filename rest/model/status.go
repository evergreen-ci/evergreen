package model

import (
	"time"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/go-redis/redis_rate/v10"
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
func (ts *APIRecentTaskStats) BuildFromService(rc task.ResultCounts) {
	ts.Total = rc.Total
	ts.Inactive = rc.Inactive
	ts.Unstarted = rc.Unstarted
	ts.Started = rc.Started
	ts.Succeeded = rc.Succeeded
	ts.SetupFailed = rc.SetupFailed
	ts.Failed = rc.Failed
	ts.SystemFailed = rc.SystemFailed
	ts.SystemUnresponsive = rc.SystemUnresponsive
	ts.SystemTimedOut = rc.SystemTimedOut
	ts.TestTimedOut = rc.TestTimedOut
}

type APIStat struct {
	Name  *string `json:"name"`
	Count int     `json:"count"`
}

type APIStatList []APIStat

func (s *APIStatList) BuildFromService(tasks []task.Stat) {
	for _, stat := range tasks {
		*s = append(*s, APIStat{Name: utility.ToStringPtr(stat.Name), Count: stat.Count})
	}
}

type APIRecentTaskStatsList map[string][]APIStat

func (s *APIRecentTaskStatsList) BuildFromService(statsMap map[string][]task.Stat) {
	for status, stat := range statsMap {
		list := APIStatList{}
		list.BuildFromService(stat)
		(*s)[status] = list
	}
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
func (s *APIHostStatsByDistro) BuildFromService(stats []host.StatsByDistro) {
	for _, entry := range stats {
		d := apiHostStatsForDistro{
			Distro:   utility.ToStringPtr(entry.Distro),
			Status:   utility.ToStringPtr(entry.Status),
			NumHosts: entry.Count,
			NumTasks: entry.NumTasks,
			MaxHosts: entry.MaxHosts,
		}

		s.Distros = append(s.Distros, d)
	}
}

// APIRateLimitStatus is the model returned by the API when a caller checks their
// current REST rate limit status.
type APIRateLimitStatus struct {
	Limit      int   `json:"limit"`
	Burst      int   `json:"burst"`
	Allowed    int   `json:"allowed"`
	Remaining  int   `json:"remaining"`
	RetryAfter int64 `json:"retry_after_ns"`
	ResetAfter int64 `json:"reset_after_ns"`
}

// BuildFromService converts a redis_rate.Result into an APIRateLimitStatus. A nil
// result indicates that no limit is configured for the surface.
func (s *APIRateLimitStatus) BuildFromService(result *redis_rate.Result) {
	if result == nil {
		return
	}
	s.Limit = result.Limit.Rate
	s.Burst = result.Limit.Burst
	s.Allowed = result.Allowed
	s.Remaining = result.Remaining
	s.RetryAfter = int64(result.RetryAfter)
	s.ResetAfter = int64(result.ResetAfter)
}

// ToService converts an APIRateLimitStatus back into a redis_rate.Result.
func (s *APIRateLimitStatus) ToService() *redis_rate.Result {
	return &redis_rate.Result{
		Limit:      redis_rate.Limit{Rate: s.Limit, Burst: s.Burst},
		Allowed:    s.Allowed,
		Remaining:  s.Remaining,
		RetryAfter: time.Duration(s.RetryAfter),
		ResetAfter: time.Duration(s.ResetAfter),
	}
}
