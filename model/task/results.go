package task

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip/message"
)

// ResultStatus returns the status for this task that should be displayed in the
// UI. Itb uses a combination of the TaskEndDetails and the Task's status to
// determine the state of the task.
func (t *Task) ResultStatus() string {
	status := t.Status
	if t.Status == evergreen.TaskUndispatched {
		if !t.Activated {
			status = evergreen.TaskInactive
		} else {
			status = evergreen.TaskUnstarted
		}
	} else if t.Status == evergreen.TaskStarted {
		status = evergreen.TaskStarted
	} else if t.Status == evergreen.TaskSucceeded {
		status = evergreen.TaskSucceeded
	} else if t.Status == evergreen.TaskFailed {
		status = evergreen.TaskFailed
		if t.Details.Type == "system" {
			status = evergreen.TaskSystemFailed
			if t.Details.TimedOut {
				if t.Details.Description == "heartbeat" {
					status = evergreen.TaskSystemUnresponse
				} else {
					status = evergreen.TaskSystemTimedOut
				}
			}
		} else if t.Details.Type == "setup" {
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
	message.Base  `json:"metadata,omitempty"`
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
		case evergreen.TaskUnstarted:
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

func (c *ResultCounts) Raw() interface{} { _ = c.Collect(); return c } // nolint: golint
func (c *ResultCounts) Loggable() bool   { return c.loggable }         // nolint: golint
func (c *ResultCounts) String() string { // nolint: golint
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
		if util.StringSliceContains(statuses, status) {
			out = append(out, task)
		}
	}

	return out
}

type Stat struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}
type ResultCountList struct {
	Total              []Stat `json:"total"`
	Inactive           []Stat `json:"inactive"`
	Unstarted          []Stat `json:"unstarted"`
	Started            []Stat `json:"started"`
	Succeeded          []Stat `json:"success"`
	Failed             []Stat `json:"failed"`
	SetupFailed        []Stat `json:"setup-failed"`
	SystemFailed       []Stat `json:"system-failed"`
	SystemUnresponsive []Stat `json:"system-unresponsive"`
	SystemTimedOut     []Stat `json:"system-timed-out"`
	TestTimedOut       []Stat `json:"test-timed-out"`
}

func GetResultCountList(statuses []StatusItem) ResultCountList {
	list := ResultCountList{}
	totals := make(map[string]int)
	for _, status := range statuses {
		switch status.Pair.Status {
		case evergreen.TaskInactive:
			list.Inactive = append(list.Inactive, Stat{Name: status.Pair.Name, Count: status.Count})
		case evergreen.TaskUnstarted:
			list.Unstarted = append(list.Unstarted, Stat{Name: status.Pair.Name, Count: status.Count})
		case evergreen.TaskStarted:
			list.Started = append(list.Started, Stat{Name: status.Pair.Name, Count: status.Count})
		case evergreen.TaskSucceeded:
			list.Succeeded = append(list.Succeeded, Stat{Name: status.Pair.Name, Count: status.Count})
		case evergreen.TaskFailed:
			list.Failed = append(list.Failed, Stat{Name: status.Pair.Name, Count: status.Count})
		case evergreen.TaskSetupFailed:
			list.SetupFailed = append(list.SetupFailed, Stat{Name: status.Pair.Name, Count: status.Count})
		case evergreen.TaskSystemFailed:
			list.SystemFailed = append(list.SystemFailed, Stat{Name: status.Pair.Name, Count: status.Count})
		case evergreen.TaskSystemUnresponse:
			list.SystemUnresponsive = append(list.SystemUnresponsive, Stat{Name: status.Pair.Name, Count: status.Count})
		case evergreen.TaskSystemTimedOut:
			list.SystemTimedOut = append(list.SystemTimedOut, Stat{Name: status.Pair.Name, Count: status.Count})
		case evergreen.TaskTestTimedOut:
			list.TestTimedOut = append(list.TestTimedOut, Stat{Name: status.Pair.Name, Count: status.Count})
		}
		totals[status.Pair.Name] += status.Count
	}

	totalsList := make([]Stat, 0, len(totals))
	for name, count := range totals {
		totalsList = append(totalsList, Stat{Name: name, Count: count})
	}
	sort.Slice(totalsList, func(i, j int) bool {
		return totalsList[i].Count > totalsList[j].Count
	})
	list.Total = totalsList

	return list
}
