package model

import (
	"github.com/evergreen-ci/evergreen/model/stats"
	"github.com/pkg/errors"
)

// APITestStats is the model to be returned by the API when querying test execution statistics.
type APITestStats struct {
	TestFile     string `json:"test_file"`
	TaskName     string `json:"task_name,omitempty"`
	BuildVariant string `json:"variant,omitempty"`
	Distro       string `json:"distro,omitempty"`
	Date         string `json:"date"`

	NumPass         int     `json:"num_pass"`
	NumFail         int     `json:"num_fail"`
	AvgDurationPass float64 `json:"avg_duration_pass"`
}

// BuildFromService converts a service level struct to an API level struct.
func (apiTestStats *APITestStats) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *stats.TestStats:
		apiTestStats.TestFile = v.TestFile
		apiTestStats.TaskName = v.TaskName
		apiTestStats.BuildVariant = v.BuildVariant
		apiTestStats.Distro = v.Distro
		apiTestStats.Date = v.Date.Format("2006-01-02")

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

// APITaskStats is the model to be returned by the API when querying task execution statistics
type APITaskStats struct {
	TaskName     string `json:"task_name,omitempty"`
	BuildVariant string `json:"variant,omitempty"`
	Distro       string `json:"distro,omitempty"`
	Date         string `json:"date"`

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
		apiTaskStats.TaskName = v.TaskName
		apiTaskStats.BuildVariant = v.BuildVariant
		apiTaskStats.Distro = v.Distro
		apiTaskStats.Date = v.Date.Format("2006-01-02")

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

func (apiTaskStats *APITaskStats) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not implemented for APITaskStats")
}
