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
