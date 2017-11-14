package model

import (
	"path"
	"strings"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
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

var (
	TestLogPath = "/test_log/"
)

// StatusDiffBuilds takes two builds and returns a diff of their results
// for easy comparison and analysis.
func StatusDiffBuilds(original, patch *build.Build) BuildStatusDiff {
	// return an empty diff if one of builds is nonexistant
	// this is likely to occur after adding a new buildvariant or task
	if original == nil || patch == nil {
		return BuildStatusDiff{}
	}

	diff := BuildStatusDiff{
		Name: original.DisplayName,
		Diff: StatusDiff{original.Status, patch.Status},
	}

	// build maps of task statuses, for matching
	originalTasks := make(map[string]build.TaskCache)
	for _, task := range original.Tasks {
		originalTasks[task.DisplayName] = task
	}

	// iterate through all patch tasks and create diffs
	// NOTE: this implicitly skips all tasks not present in the patch
	for _, task := range patch.Tasks {
		baseTask := originalTasks[task.DisplayName]
		diff.Tasks = append(diff.Tasks,
			TaskStatusDiff{
				Name:         task.DisplayName,
				Diff:         StatusDetailsDiff{baseTask.StatusDetails, task.StatusDetails},
				Original:     baseTask.Id,
				Patch:        task.Id,
				BuildVariant: diff.Name,
			})
	}
	return diff
}

// getTestUrl returns the correct relative URL to a test log, given a
// TestResult structure
func getTestUrl(tr *task.TestResult) string {
	// Return url if it exists. If there is no test, return empty string.
	if tr.URL != "" || tr.LogId == "" { // If LogId is empty, URL must also be empty
		return tr.URL
	}
	return TestLogPath + tr.LogId
}

// StatusDiffTasks takes two tasks and returns a diff of their results
// for easy comparison and analysis.
func StatusDiffTasks(original *task.Task, patch *task.Task) TaskStatusDiff {
	// return an empty diff if one of tasks is nonexistant
	// this is likely to occur after adding a new buildvariant or task
	if original == nil || patch == nil {
		return TaskStatusDiff{}
	}

	diff := TaskStatusDiff{
		Name:     original.DisplayName,
		Original: original.Id,
		Patch:    patch.Id,
		Diff:     StatusDetailsDiff{original.Details, patch.Details},
	}

	if original.LocalTestResults == nil || patch.LocalTestResults == nil {
		return diff
	}

	// build maps of test statuses, for matching
	originalTests := make(map[string]task.TestResult)
	for _, test := range original.LocalTestResults {
		originalTests[test.TestFile] = test
	}

	// iterate through all patch tests and create diffs
	for _, test := range patch.LocalTestResults {
		baseTest := originalTests[test.TestFile]

		// get the base name for windows/non-windows paths
		testFile := path.Base(strings.Replace(test.TestFile, "\\", "/", -1))
		diff.Tests = append(diff.Tests,
			TestStatusDiff{
				Name:     testFile,
				Diff:     StatusDiff{baseTest.Status, test.Status},
				Original: getTestUrl(&baseTest),
				Patch:    getTestUrl(&test),
			})
	}
	return diff
}
