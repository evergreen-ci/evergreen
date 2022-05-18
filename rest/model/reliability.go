package model

import (
	"github.com/evergreen-ci/evergreen/model/reliability"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// APITaskReliability is the model to be returned by the API when querying task execution statistics
type APITaskReliability struct {
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
	SuccessRate        float64 `json:"success_rate"`
}

// Converts a service level struct to an API level struct
func (tr *APITaskReliability) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *reliability.TaskReliability:
		tr.TaskName = utility.ToStringPtr(v.TaskName)
		tr.BuildVariant = utility.ToStringPtr(v.BuildVariant)
		tr.Distro = utility.ToStringPtr(v.Distro)
		tr.Date = utility.ToStringPtr(v.Date.UTC().Format("2006-01-02"))

		tr.NumSuccess = v.NumSuccess
		tr.NumFailed = v.NumFailed
		tr.NumTotal = v.NumTotal
		tr.NumTimeout = v.NumTimeout
		tr.NumTestFailed = v.NumTestFailed
		tr.NumSystemFailed = v.NumSystemFailed
		tr.NumSetupFailed = v.NumSetupFailed
		tr.AvgDurationSuccess = v.AvgDurationSuccess
		tr.SuccessRate = v.SuccessRate
	default:
		return errors.Errorf("programmatic error: expected task reliability stats but got type %T", h)
	}
	return nil
}

// ToService is not implemented for APITaskStats.
func (tr *APITaskReliability) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not implemented for APITaskReliability")
}

// StartAtKey returns the start_at key parameter that can be used to paginate and start at this element.
func (tr *APITaskReliability) StartAtKey() string {
	return StartAtKey{
		date:         utility.FromStringPtr(tr.Date),
		buildVariant: utility.FromStringPtr(tr.BuildVariant),
		taskName:     utility.FromStringPtr(tr.TaskName),
		distro:       utility.FromStringPtr(tr.Distro),
	}.String()
}
