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
func (apiTestStats *APITestStats) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *stats.TestStats:
		apiTestStats.TestFile = utility.ToStringPtr(v.TestFile)
		apiTestStats.TaskName = utility.ToStringPtr(v.TaskName)
		apiTestStats.BuildVariant = utility.ToStringPtr(v.BuildVariant)
		apiTestStats.Distro = utility.ToStringPtr(v.Distro)
		apiTestStats.Date = utility.ToStringPtr(v.Date.UTC().Format("2006-01-02"))

		apiTestStats.NumPass = v.NumPass
		apiTestStats.NumFail = v.NumFail
		apiTestStats.AvgDurationPass = v.AvgDurationPass
	default:
		return errors.Errorf("incorrect type when converting test stats (%T)", v)
	}
	return nil
}

// ToService is not implemented for APITestStats.
func (apiTestStats *APITestStats) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not implemented for APITestStats")
}

// StartAtKey returns the start_at key parameter that can be used to paginate and start at this element.
func (apiTestStats *APITestStats) StartAtKey() string {
	return StartAtKey{
		date:         utility.FromStringPtr(apiTestStats.Date),
		buildVariant: utility.FromStringPtr(apiTestStats.BuildVariant),
		taskName:     utility.FromStringPtr(apiTestStats.TaskName),
		testFile:     utility.FromStringPtr(apiTestStats.TestFile),
		distro:       utility.FromStringPtr(apiTestStats.Distro),
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
func (apiTaskStats *APITaskStats) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *stats.TaskStats:
		apiTaskStats.TaskName = utility.ToStringPtr(v.TaskName)
		apiTaskStats.BuildVariant = utility.ToStringPtr(v.BuildVariant)
		apiTaskStats.Distro = utility.ToStringPtr(v.Distro)
		apiTaskStats.Date = utility.ToStringPtr(v.Date.UTC().Format("2006-01-02"))

		apiTaskStats.NumSuccess = v.NumSuccess
		apiTaskStats.NumFailed = v.NumFailed
		apiTaskStats.NumTotal = v.NumTotal
		apiTaskStats.NumTimeout = v.NumTimeout
		apiTaskStats.NumTestFailed = v.NumTestFailed
		apiTaskStats.NumSystemFailed = v.NumSystemFailed
		apiTaskStats.NumSetupFailed = v.NumSetupFailed
		apiTaskStats.AvgDurationSuccess = v.AvgDurationSuccess
	default:
		return errors.Errorf("incorrect type when converting task stats (%T)", v)
	}
	return nil
}

// ToService is not implemented for APITaskStats.
func (apiTaskStats *APITaskStats) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not implemented for APITaskStats")
}

// StartAtKey returns the start_at key parameter that can be used to paginate and start at this element.
func (apiTaskStats *APITaskStats) StartAtKey() string {
	return StartAtKey{
		date:         utility.FromStringPtr(apiTaskStats.Date),
		buildVariant: utility.FromStringPtr(apiTaskStats.BuildVariant),
		taskName:     utility.FromStringPtr(apiTaskStats.TaskName),
		distro:       utility.FromStringPtr(apiTaskStats.Distro),
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
