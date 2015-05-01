package model

import (
	"10gen.com/mci/model/build"
	"path"
	"strings"
)

// StatusDiff stores a pairing of status strings
// for easy visualization/aggregation later
type StatusDiff struct {
	Original string `json:"original"`
	Patch    string `json:"patch"`
}

// Stores a diff of two build statuses
type BuildStatusDiff struct {
	Name  string           `json:"name"`
	Diff  StatusDiff       `json:"diff"`
	Tasks []TaskStatusDiff `json:"tasks"`
}

// Stores a diff of two task statuses
type TaskStatusDiff struct {
	Name         string           `json:"name"`
	Diff         StatusDiff       `json:"diff"`
	Tests        []TestStatusDiff `json:"tests"`
	Original     string           `json:"original"`
	Patch        string           `json:"patch"`
	BuildVariant string           `json:"build_variant"`
}

// Stores a diff of two test results
type TestStatusDiff struct {
	Name     string     `json:"name"`
	Diff     StatusDiff `json:"diff"`
	Original string     `json:"original"`
	Patch    string     `json:"patch"`
}

// Takes two builds and returns a diff of their results
// for easy comparison and analysis
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
				Diff:         StatusDiff{baseTask.Status, task.Status},
				Original:     baseTask.Id,
				Patch:        task.Id,
				BuildVariant: diff.Name,
			})
	}
	return diff
}

// Takes two tasks and returns a diff of their results
// for easy comparison and analysis
func StatusDiffTasks(original *Task, patch *Task) TaskStatusDiff {
	// return an empty diff if one of tasks is nonexistant
	// this is likely to occur after adding a new buildvariant or task
	if original == nil || patch == nil {
		return TaskStatusDiff{}
	}

	diff := TaskStatusDiff{
		Name:     original.DisplayName,
		Original: original.Id,
		Patch:    patch.Id,
		Diff:     StatusDiff{original.Status, patch.Status},
	}

	if original.TestResults == nil || patch.TestResults == nil {
		return diff
	}

	// build maps of test statuses, for matching
	originalTests := make(map[string]TestResult)
	for _, test := range original.TestResults {
		originalTests[test.TestFile] = test
	}

	// iterate through all patch tests and create diffs
	for _, test := range patch.TestResults {
		baseTest := originalTests[test.TestFile]

		// get the base name for windows/non-windows paths
		testFile := path.Base(strings.Replace(test.TestFile, "\\", "/", -1))
		diff.Tests = append(diff.Tests,
			TestStatusDiff{
				Name:     testFile,
				Diff:     StatusDiff{baseTest.Status, test.Status},
				Original: baseTest.URL,
				Patch:    test.URL,
			})
	}
	return diff
}
