package model

import (
	"github.com/evergreen-ci/evergreen/apimodels"
)

// StatusDiff stores a pairing of status strings
// for easy visualization/aggregation later.
type StatusDiff struct {
	Original string `json:"original"`
	Patch    string `json:"patch"`
}

// StatusDetailsDiff stores a pairing of status details
// for easy visualization/aggregation later.
type StatusDetailsDiff struct {
	Original apimodels.TaskEndDetail `json:"original"`
	Patch    apimodels.TaskEndDetail `json:"patch"`
}

// BuildStatusDiff stores a diff of two build statuses.
type BuildStatusDiff struct {
	Name  string           `json:"name"`
	Diff  StatusDiff       `json:"diff"`
	Tasks []TaskStatusDiff `json:"tasks"`
}

// TaskStatusDiff stores a diff of two task statuses.
type TaskStatusDiff struct {
	Name         string            `json:"name"`
	Diff         StatusDetailsDiff `json:"diff"`
	Tests        []TestStatusDiff  `json:"tests"`
	Original     string            `json:"original"`
	Patch        string            `json:"patch"`
	BuildVariant string            `json:"build_variant"`
}

// TestStatusDiff stores a diff of two test results.
type TestStatusDiff struct {
	Name     string     `json:"name"`
	Diff     StatusDiff `json:"diff"`
	Original string     `json:"original"`
	Patch    string     `json:"patch"`
}
