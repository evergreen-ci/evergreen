package model

import (
	"strings"

	"github.com/evergreen-ci/evergreen/model/stats"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// APITestStats is the model to be returned by the API when querying test execution statistics.
type APITestStats struct {
	TestFile     *string `json:"test_file"`
	TaskName     *string `json:"task_name,omitempty"`
	BuildVariant *string `json:"variant,omitempty"`
	Distro       *string `json:"distro,omitempty"`
	Date         *string `json:"date"`

	NumPass         int     `json:"num_pass"`
	NumFail         int     `json:"num_fail"`
	AvgDurationPass float64 `json:"avg_duration_pass"`
}

// BuildFromService converts a service level struct to an API level struct.
func (ts *APITestStats) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *stats.TestStats:
		ts.TestFile = utility.ToStringPtr(v.TestFile)
		ts.TaskName = utility.ToStringPtr(v.TaskName)
		ts.BuildVariant = utility.ToStringPtr(v.BuildVariant)
		ts.Distro = utility.ToStringPtr(v.Distro)
		ts.Date = utility.ToStringPtr(v.Date.UTC().Format("2006-01-02"))

		ts.NumPass = v.NumPass
		ts.NumFail = v.NumFail
		ts.AvgDurationPass = v.AvgDurationPass
	default:
		return errors.Errorf("programmatic error: expected test stats but got type %T", h)
	}
	return nil
}

// ToService is not implemented for APITestStats.
func (ts *APITestStats) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not implemented for APITestStats")
}

// StartAtKey returns the start_at key parameter that can be used to paginate and start at this element.
func (ts *APITestStats) StartAtKey() string {
	return StartAtKey{
		date:         utility.FromStringPtr(ts.Date),
		buildVariant: utility.FromStringPtr(ts.BuildVariant),
		taskName:     utility.FromStringPtr(ts.TaskName),
		testFile:     utility.FromStringPtr(ts.TestFile),
		distro:       utility.FromStringPtr(ts.Distro),
	}.String()
}

// APITaskStats is the model to be returned by the API when querying task execution statistics
type APITaskStats struct {
	TaskName     *string `json:"task_name"`
	BuildVariant *string `json:"variant,omitempty"`
	Distro       *string `json:"distro,omitempty"`
	Date         *string `json:"date"`

	NumSuccess         int     `json:"num_success"`
	NumFailed          int     `json:"num_failed"`
	NumTotal           int     `json:"num_total"`
	NumTimeout         int     `json:"num_timeout"`
	NumTestFailed      int     `json:"num_test_failed"`
	NumSystemFailed    int     `json:"num_system_failed"`
	NumSetupFailed     int     `json:"num_setup_failed"`
	AvgDurationSuccess float64 `json:"avg_duration_success"`
}

// Converts a service level struct to an API level struct
func (ts *APITaskStats) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *stats.TaskStats:
		ts.TaskName = utility.ToStringPtr(v.TaskName)
		ts.BuildVariant = utility.ToStringPtr(v.BuildVariant)
		ts.Distro = utility.ToStringPtr(v.Distro)
		ts.Date = utility.ToStringPtr(v.Date.UTC().Format("2006-01-02"))

		ts.NumSuccess = v.NumSuccess
		ts.NumFailed = v.NumFailed
		ts.NumTotal = v.NumTotal
		ts.NumTimeout = v.NumTimeout
		ts.NumTestFailed = v.NumTestFailed
		ts.NumSystemFailed = v.NumSystemFailed
		ts.NumSetupFailed = v.NumSetupFailed
		ts.AvgDurationSuccess = v.AvgDurationSuccess
	default:
		return errors.Errorf("programmatic error: expected task stats but got type %T", h)
	}
	return nil
}

// ToService is not implemented for APITaskStats.
func (ts *APITaskStats) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not implemented for APITaskStats")
}

// StartAtKey returns the start_at key parameter that can be used to paginate and start at this element.
func (ts *APITaskStats) StartAtKey() string {
	return StartAtKey{
		date:         utility.FromStringPtr(ts.Date),
		buildVariant: utility.FromStringPtr(ts.BuildVariant),
		taskName:     utility.FromStringPtr(ts.TaskName),
		distro:       utility.FromStringPtr(ts.Distro),
	}.String()
}

// StartAtKey is a struct used to build the start_at key parameter for pagination.
type StartAtKey struct {
	date         string
	buildVariant string
	taskName     string
	testFile     string
	distro       string
}

func (s StartAtKey) String() string {
	elements := []string{s.date, s.buildVariant, s.taskName, s.testFile, s.distro}
	return strings.Join(elements, "|")
}
