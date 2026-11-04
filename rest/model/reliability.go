package model

import (
	"github.com/evergreen-ci/evergreen/model/reliability"
	"github.com/evergreen-ci/utility"
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

// BuildFromService converts a service level struct to an API level struct
func (tr *APITaskReliability) BuildFromService(in reliability.TaskReliability) {
	tr.TaskName = utility.ToStringPtr(in.TaskName)
	tr.BuildVariant = utility.ToStringPtr(in.BuildVariant)
	tr.Distro = utility.ToStringPtr(in.Distro)
	tr.Date = utility.ToStringPtr(in.Date.UTC().Format("2006-01-02"))

	tr.NumSuccess = in.NumSuccess
	tr.NumFailed = in.NumFailed
	tr.NumTotal = in.NumTotal
	tr.NumTimeout = in.NumTimeout
	tr.NumTestFailed = in.NumTestFailed
	tr.NumSystemFailed = in.NumSystemFailed
	tr.NumSetupFailed = in.NumSetupFailed
	tr.AvgDurationSuccess = in.AvgDurationSuccess
	tr.SuccessRate = in.SuccessRate
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
