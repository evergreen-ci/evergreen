package model

import (
	"github.com/evergreen-ci/evergreen/model/reliability"
	"github.com/pkg/errors"
)

// APITaskReliability is the model to be returned by the API when querying task execution statistics
type APITaskReliability struct {
	TaskName     APIString `json:"task_name"`
	BuildVariant APIString `json:"variant,omitempty"`
	Distro       APIString `json:"distro,omitempty"`
	Date         APIString `json:"date"`

	NumSuccess         int     `json:"num_success"`
	NumFailed          int     `json:"num_failed"`
	NumTotal           int     `json:"num_total"`
	NumTimeout         int     `json:"num_timeout"`
	NumTestFailed      int     `json:"num_test_failed"`
	NumSystemFailed    int     `json:"num_system_failed"`
	NumSetupFailed     int     `json:"num_setup_failed"`
	AvgDurationSuccess float64 `json:"avg_duration_success"`
	SuccessRate        float64 `json:"success_rate"`
}

// Converts a service level struct to an API level struct
func (apiTaskReliability *APITaskReliability) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *reliability.TaskReliability:
		apiTaskReliability.TaskName = ToAPIString(v.TaskName)
		apiTaskReliability.BuildVariant = ToAPIString(v.BuildVariant)
		apiTaskReliability.Distro = ToAPIString(v.Distro)
		apiTaskReliability.Date = ToAPIString(v.Date.UTC().Format("2006-01-02"))

		apiTaskReliability.NumSuccess = v.NumSuccess
		apiTaskReliability.NumFailed = v.NumFailed
		apiTaskReliability.NumTotal = v.NumTotal
		apiTaskReliability.NumTimeout = v.NumTimeout
		apiTaskReliability.NumTestFailed = v.NumTestFailed
		apiTaskReliability.NumSystemFailed = v.NumSystemFailed
		apiTaskReliability.NumSetupFailed = v.NumSetupFailed
		apiTaskReliability.AvgDurationSuccess = v.AvgDurationSuccess
		apiTaskReliability.SuccessRate = v.SuccessRate
	default:
		return errors.Errorf("incorrect type when converting task stats (%T)", v)
	}
	return nil
}

// ToService is not implemented for APITaskStats.
func (apiTaskReliability *APITaskReliability) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not implemented for APITaskStats")
}

// StartAtKey returns the start_at key parameter that can be used to paginate and start at this element.
func (apiTaskReliability *APITaskReliability) StartAtKey() string {
	return StartAtKey{
		date:         FromAPIString(apiTaskReliability.Date),
		buildVariant: FromAPIString(apiTaskReliability.BuildVariant),
		taskName:     FromAPIString(apiTaskReliability.TaskName),
		distro:       FromAPIString(apiTaskReliability.Distro),
	}.String()
}
