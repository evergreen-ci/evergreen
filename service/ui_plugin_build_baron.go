package service

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/graphql"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
)

const (
	msPerNS     = 1000 * 1000
	maxNoteSize = 16 * 1024 // 16KB
	jiraSource  = "JIRA"
)

// saveNote reads a request containing a note's content along with the last seen
// edit time and updates the note in the database.
func bbSaveNote(w http.ResponseWriter, r *http.Request) {
	taskId := gimlet.GetVars(r)["task_id"]
	n := &model.Note{}
	if err := utility.ReadJSON(r.Body, n); err != nil {
		gimlet.WriteJSONError(w, err.Error())
		return
	}

	// prevent incredibly large notes
	if len(n.Content) > maxNoteSize {
		gimlet.WriteJSONError(w, "note is too large")
		return
	}

	// We need to make sure the user isn't blowing away a new edit,
	// so we load the existing note. If the user's last seen edit time is less
	// than the most recent edit, we error with a helpful message.
	old, err := model.NoteForTask(taskId)
	if err != nil {
		gimlet.WriteJSONInternalError(w, err.Error())
		return
	}
	// we compare times by millisecond rather than nanosecond so we can
	// work around the rounding that occurs when javascript forces these
	// large values into in float type.
	if old != nil && n.UnixNanoTime/msPerNS != old.UnixNanoTime/msPerNS {
		gimlet.WriteJSONError(w,
			"this note has already been edited. Please refresh and try again.")
		return
	}

	n.TaskId = taskId
	n.UnixNanoTime = time.Now().UnixNano()
	if err := n.Upsert(); err != nil {
		gimlet.WriteJSONInternalError(w, err.Error())
		return
	}
	gimlet.WriteJSON(w, n)
}

// getNote retrieves the latest note from the database.
func bbGetNote(w http.ResponseWriter, r *http.Request) {
	taskId := gimlet.GetVars(r)["task_id"]
	n, err := model.NoteForTask(taskId)
	if err != nil {
		gimlet.WriteJSONInternalError(w, err.Error())
		return
	}
	if n == nil {
		gimlet.WriteJSON(w, "")
		return
	}
	gimlet.WriteJSON(w, n)
}

func (uis *UIServer) bbGetCreatedTickets(w http.ResponseWriter, r *http.Request) {
	taskId := gimlet.GetVars(r)["task_id"]

	events, err := event.Find(event.AllLogCollection, event.TaskEventsForId(taskId))
	if err != nil {
		gimlet.WriteJSONInternalError(w, err.Error())
		return
	}

	var results []thirdparty.JiraTicket
	var searchTickets []string
	for _, evt := range events {
		data := evt.Data.(*event.TaskEventData)
		if evt.EventType == event.TaskJiraAlertCreated {
			searchTickets = append(searchTickets, data.JiraIssue)
		}
	}

	for _, ticket := range searchTickets {
		jiraIssue, err := uis.jiraHandler.GetJIRATicket(ticket)
		if err != nil {
			gimlet.WriteJSONInternalError(w, err.Error())
			return
		}
		if jiraIssue == nil {
			continue
		}
		results = append(results, *jiraIssue)
	}

	gimlet.WriteJSON(w, results)
}

func (uis *UIServer) bbJiraSearch(rw http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	taskId := vars["task_id"]
	exec := vars["execution"]

	searchReturnInfo, projectNotFound, err, _ := graphql.GetSearchReturnInfo(taskId, exec)
	if projectNotFound {
		gimlet.WriteJSON(rw, err.Error())
		return
	}
	if err != nil {
		gimlet.WriteJSONInternalError(rw, err.Error())
		return
	}

	gimlet.WriteJSON(rw, searchReturnInfo)

}

// bbFileTicket creates a JIRA ticket for a task with the given test failures.
func (uis *UIServer) bbFileTicket(w http.ResponseWriter, r *http.Request) {
	var input struct {
		TaskId  string   `json:"task"`
		TestIds []string `json:"tests"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		gimlet.WriteJSONInternalError(w, err.Error())
	}

	taskNotFound, err := graphql.BbFileTicket(r.Context(), input.TaskId)

	if taskNotFound {
		gimlet.WriteJSON(w, err.Error())
		return
	}
	if err != nil {
		gimlet.WriteJSONInternalError(w, err.Error())
		return
	}

	gimlet.WriteJSON(w, nil)
}
