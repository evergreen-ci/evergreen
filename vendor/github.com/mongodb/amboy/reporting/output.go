package reporting

import "time"

// JobStatusReport contains data for the numbers of jobs that exist
// for a specified type.
type JobStatusReport struct {
	Filter string        `bson:"filter" json:"filter" yaml:"filter"`
	Stats  []JobCounters `bson:"data" json:"data" yaml:"data"`
}

// JobCounters holds data for counts of jobs by type.
type JobCounters struct {
	ID    string `bson:"_id" json:"type" yaml:"type"`
	Count int    `bson:"count" json:"count" yaml:"count"`
	Group string `bson:"group,omitempty" json:"group,omitempty" yaml:"group,omitempty"`
}

// JobRuntimeReport contains data for the runtime of jobs, by type,
// that have run a specified period.
type JobRuntimeReport struct {
	Filter string        `bson:"filter" json:"filter" yaml:"filter"`
	Period time.Duration `bson:"period" json:"period" yaml:"period"`
	Stats  []JobRuntimes `bson:"data" json:"data" yaml:"data"`
}

// JobRuntimes holds data for runtimes of jobs by type.
type JobRuntimes struct {
	ID       string        `bson:"_id" json:"type" yaml:"type"`
	Duration time.Duration `bson:"duration" json:"duration" yaml:"duration"`
	Group    string        `bson:"group,omitempty" json:"group,omitempty" yaml:"group,omitempty"`
}

// JobReportIDs contains the IDs of all jobs of a specific type.
type JobReportIDs struct {
	Type   string   `bson:"_id" json:"type" yaml:"type"`
	Filter string   `bson:"filter" json:"filter" yaml:"filter"`
	IDs    []string `bson:"jobs" json:"jobs" yaml:"jobs"`
	Group  string   `bson:"group,omitempty" json:"group,omitempty" yaml:"group,omitempty"`
}

// JobErrorsReport contains data for all job errors, by job type,
// for a specific period.
type JobErrorsReport struct {
	Period         time.Duration      `bson:"period" json:"period" yaml:"period"`
	FilteredByType bool               `bson:"filtered" json:"filtered" yaml:"filtered"`
	Data           []JobErrorsForType `bson:"data" json:"data" yaml:"data"`
}

// JobErrorsForType holds data about the errors for a specific type of
// jobs.
type JobErrorsForType struct {
	ID      string   `bson:"_id" json:"type" yaml:"type"`
	Count   int      `bson:"count" json:"count" yaml:"count"`
	Total   int      `bson:"total" json:"total" yaml:"total"`
	Average float64  `bson:"average" json:"average" yaml:"average"`
	Errors  []string `bson:"errors,omitempty" json:"errors,omitempty" yaml:"errors,omitempty"`
	Group   string   `bson:"group,omitempty" json:"group,omitempty" yaml:"group,omitempty"`
}
