package model

import (
	"fmt"
	"path"
	"strings"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/pkg/errors"
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
func StatusDiffBuilds(original, patch *build.Build) (BuildStatusDiff, error) {
	// return an empty diff if one of builds is nonexistant
	// this is likely to occur after adding a new buildvariant or task
	if original == nil || patch == nil {
		return BuildStatusDiff{}, nil
	}

	diff := BuildStatusDiff{
		Name: original.DisplayName,
		Diff: StatusDiff{original.Status, patch.Status},
	}

	query := db.Query(task.ByBuildIds([]string{original.Id, patch.Id})).WithFields(task.BuildIdKey, task.StatusKey, task.DetailsKey, task.DisplayNameKey)
	tasks, err := task.FindAll(query)
	if err != nil {
		return BuildStatusDiff{}, errors.Wrap(err, "finding tasks")
	}

	// build maps of tasks, for matching
	originalTaskMap := make(map[string]task.Task)
	patchTaskMap := make(map[string]task.Task)
	for _, t := range tasks {
		if t.BuildId == patch.Id {
			patchTaskMap[t.Id] = t
		} else {
			originalTaskMap[t.DisplayName] = t
		}
	}

	// iterate through all patch tasks and create diffs
	// NOTE: this implicitly skips all tasks not present in the patch
	for _, task := range patch.Tasks {
		patchTask, ok := patchTaskMap[task.Id]
		if !ok {
			return BuildStatusDiff{}, errors.Errorf("patch task '%s' doesn't exist", task.Id)
		}
		baseTask := originalTaskMap[patchTask.DisplayName]
		newDiff := TaskStatusDiff{
			Name:         patchTask.DisplayName,
			Diff:         StatusDetailsDiff{Original: baseTask.Details, Patch: patchTask.Details},
			Original:     baseTask.Id,
			Patch:        patchTask.Id,
			BuildVariant: diff.Name,
		}
		// handle if the status details do not contain a status, such as in display tasks
		if newDiff.Diff.Original.Status == "" {
			newDiff.Diff.Original.Status = baseTask.Status
		}
		if newDiff.Diff.Patch.Status == "" {
			newDiff.Diff.Patch.Status = patchTask.Status
		}
		diff.Tasks = append(diff.Tasks, newDiff)
	}

	return diff, nil
}

// getTestUrl returns the correct relative URL to a test log, given a
// TestResult structure
func getTestUrl(tr *testresult.TestResult) string {
	if tr.LogURL != "" {
		return tr.LogURL
	}
	if tr.TaskID != "" && tr.GetLogTestName() != "" {
		return fmt.Sprintf("%s%s/%d/%s?group_id=%s", TestLogPath, tr.TaskID, tr.Execution, tr.GetLogTestName(), tr.GroupID)
	}

	return ""
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
	originalTests := make(map[string]testresult.TestResult)
	for _, test := range original.LocalTestResults {
		originalTests[test.TestName] = test
	}

	// iterate through all patch tests and create diffs
	for _, test := range patch.LocalTestResults {
		baseTest := originalTests[test.TestName]

		// get the base name for windows/non-windows paths
		testName := path.Base(strings.Replace(test.TestName, "\\", "/", -1))
		diff.Tests = append(diff.Tests,
			TestStatusDiff{
				Name:     testName,
				Diff:     StatusDiff{baseTest.Status, test.Status},
				Original: getTestUrl(&baseTest),
				Patch:    getTestUrl(&test),
			})
	}

	return diff
}

// StatusDiffTests takes two sets of tests and returns a diff of their results
// for easy comparison and analysis.
func StatusDiffTests(original, patch []testresult.TestResult) []TestStatusDiff {
	diff := []TestStatusDiff{}
	if len(original) == 0 || len(patch) == 0 {
		return diff
	}

	originalMap := make(map[string]testresult.TestResult)
	for _, test := range original {
		originalMap[test.GetDisplayTestName()] = test
	}

	for _, test := range patch {
		baseTest := originalMap[test.GetDisplayTestName()]

		// Get the base name for windows/non-windows paths.
		testName := path.Base(strings.Replace(test.GetDisplayTestName(), "\\", "/", -1))
		diff = append(diff, TestStatusDiff{
			Name:     testName,
			Diff:     StatusDiff{baseTest.Status, test.Status},
			Original: getTestUrl(&baseTest),
			Patch:    getTestUrl(&test),
		})
	}

	return diff
}
