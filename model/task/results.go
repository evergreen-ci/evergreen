package task

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/message"
)

// ResultStatus returns the status for this task that should be displayed in the
// UI. It uses a combination of the TaskEndDetails and the Task's status to
// determine the state of the task.
func (t *Task) ResultStatus() string {
	status := t.Status
	if t.Status == evergreen.TaskUndispatched {
		if !t.Activated {
			status = evergreen.TaskInactive
		}
	} else if t.Status == evergreen.TaskFailed {
		if t.Details.Type == evergreen.CommandTypeSystem {
			status = evergreen.TaskSystemFailed
			if t.Details.TimedOut {
				if t.Details.Description == evergreen.TaskDescriptionHeartbeat {
					status = evergreen.TaskSystemUnresponse
				} else {
					status = evergreen.TaskSystemTimedOut
				}
			}
		} else if t.Details.Type == evergreen.CommandTypeSetup {
			status = evergreen.TaskSetupFailed
		} else if t.Details.TimedOut {
			status = evergreen.TaskTestTimedOut
		}
	}
	return status
}

// ResultCounts stores a collection of counters related to.
//
// This type implements the grip/message.Composer interface and may be
// passed directly to grip loggers.
type ResultCounts struct {
	Total              int `json:"total"`
	Inactive           int `json:"inactive"`
	Unstarted          int `json:"unstarted"`
	Started            int `json:"started"`
	Succeeded          int `json:"succeeded"`
	Failed             int `json:"failed"`
	SetupFailed        int `json:"setup-failed"`
	SystemFailed       int `json:"system-failed"`
	SystemUnresponsive int `json:"system-unresponsive"`
	SystemTimedOut     int `json:"system-timed-out"`
	TestTimedOut       int `json:"test-timed-out"`

	loggable      bool
	cachedMessage string
	message.Base  `json:"metadata"`
}

// GetResultCounts takes a list of tasks and collects their
// outcomes using the same status as the.
func GetResultCounts(tasks []Task) *ResultCounts {
	out := ResultCounts{}

	for _, t := range tasks {
		out.Total++
		switch t.ResultStatus() {
		case evergreen.TaskInactive:
			out.Inactive++
		case evergreen.TaskUndispatched:
			out.Unstarted++
		case evergreen.TaskStarted:
			out.Started++
		case evergreen.TaskSucceeded:
			out.Succeeded++
		case evergreen.TaskFailed:
			out.Failed++
		case evergreen.TaskSetupFailed:
			out.SetupFailed++
		case evergreen.TaskSystemFailed:
			out.SystemFailed++
		case evergreen.TaskSystemUnresponse:
			out.SystemUnresponsive++
		case evergreen.TaskSystemTimedOut:
			out.SystemTimedOut++
		case evergreen.TaskTestTimedOut:
			out.TestTimedOut++
		}
	}

	if out.Total > 0 {
		out.loggable = true
	}

	out.Time = time.Now()
	return &out
}

func (c *ResultCounts) Raw() any       { _ = c.Collect(true); return c }
func (c *ResultCounts) Loggable() bool { return c.loggable }
func (c *ResultCounts) String() string {
	if !c.Loggable() {
		return ""
	}

	if c.cachedMessage == "" {
		out, _ := json.Marshal(c)
		c.cachedMessage = string(out)
	}

	return c.cachedMessage
}

// FilterTasksOnStatus tasks in a slice of tasks and removes tasks whose result
// status do not match the passed-in statuses
func FilterTasksOnStatus(tasks []Task, statuses ...string) []Task {
	out := make([]Task, 0)
	for _, task := range tasks {
		status := task.ResultStatus()
		if utility.StringSliceContains(statuses, status) {
			out = append(out, task)
		}
	}

	return out
}

func GetResultCountList(statuses []StatusItem) map[string][]Stat {
	statMap := make(map[string][]Stat)
	totals := make(map[string]int)
	for _, status := range statuses {
		statMap[status.Status] = status.Stats
		for _, stat := range status.Stats {
			totals[stat.Name] += stat.Count
		}
	}

	// Reshape totals
	totalsList := make([]Stat, 0, len(totals))
	for name, count := range totals {
		totalsList = append(totalsList, Stat{Name: name, Count: count})
	}
	sort.Slice(totalsList, func(i, j int) bool {
		return totalsList[i].Count > totalsList[j].Count
	})
	statMap["totals"] = totalsList

	return statMap
}
