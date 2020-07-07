package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/trigger"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	msPerNS       = 1000 * 1000
	maxNoteSize   = 16 * 1024 // 16KB
	jiraSource    = "JIRA"
	jiraIssueType = "Build Failure"
)

func bbGetConfig(settings *evergreen.Settings) map[string]evergreen.BuildBaronProject {
	bbconf, ok := settings.Plugins["buildbaron"]
	if !ok {
		return nil
	}

	projectConfig, ok := bbconf["projects"]
	if !ok {
		grip.Error("no build baron projects configured")
		return nil
	}

	projects := map[string]evergreen.BuildBaronProject{}
	err := mapstructure.Decode(projectConfig, &projects)
	if err != nil {
		grip.Critical(errors.Wrap(err, "unable to parse bb project config"))
	}

	return projects
}

func bbGetTask(taskId string, execution string) (*task.Task, error) {
	oldId := fmt.Sprintf("%v_%v", taskId, execution)
	t, err := task.FindOneOld(task.ById(oldId))
	if err != nil {
		return t, errors.Wrap(err, "Failed to find task with old Id")
	}
	// if the archived task was not found, we must be looking for the most recent exec
	if t == nil {
		t, err = task.FindOne(task.ById(taskId))
		if err != nil {
			return nil, errors.Wrap(err, "Failed to find task")
		}
	}
	if t == nil {
		return nil, errors.Errorf("No task found for taskId: %s and execution: %s", taskId, execution)
	}
	if t.DisplayOnly {
		t.LocalTestResults, err = t.GetTestResultsForDisplayTask()
		if err != nil {
			return nil, errors.Wrapf(err, "Problem finding test results for display task '%s'", t.Id)
		}
	}
	return t, nil
}

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
	t, err := bbGetTask(taskId, exec)
	if err != nil {
		gimlet.WriteJSONInternalError(rw, err.Error())
		return
	}
	bbProj, ok := uis.buildBaronProjects[t.Project]
	if !ok {
		gimlet.WriteJSON(rw, fmt.Sprintf("Build Baron project for %s not found", t.Project))
		return
	}

	jira := &jiraSuggest{bbProj, uis.jiraHandler}
	multiSource := &multiSourceSuggest{jira}

	var tickets []thirdparty.JiraTicket
	var source string

	tickets, source, err = multiSource.Suggest(t)
	if err != nil {
		message := fmt.Sprintf("Error searching for tickets: %s", err)
		grip.Error(message)
		gimlet.WriteJSONInternalError(rw, message)
		return
	}
	jql := t.GetJQL(bbProj.TicketSearchProjects)
	var featuresURL string
	if bbProj.BFSuggestionFeaturesURL != "" {
		featuresURL = bbProj.BFSuggestionFeaturesURL
		featuresURL = strings.Replace(featuresURL, "{task_id}", taskId, -1)
		featuresURL = strings.Replace(featuresURL, "{execution}", exec, -1)
	} else {
		featuresURL = ""
	}
	gimlet.WriteJSON(rw, searchReturnInfo{Issues: tickets, Search: jql, Source: source, FeaturesURL: featuresURL})
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

	// Find information about the task
	t, err := task.FindOne(task.ById(input.TaskId))
	if err != nil {
		gimlet.WriteJSONInternalError(w, err.Error())
		return
	}
	if t == nil {
		gimlet.WriteJSONResponse(w, http.StatusNotFound, fmt.Sprintf("task not found for id %s", input.TaskId))
		return
	}

	n, err := uis.makeNotification(uis.buildBaronProjects[t.Project].TicketCreateProject, t)
	if err != nil {
		gimlet.WriteJSONInternalError(w, err.Error())
		return
	}
	ts := utility.RoundPartOfMinute(1).Format(units.TSFormat)
	err = uis.queue.Put(r.Context(), units.NewEventSendJob(n.ID, ts))
	if err != nil {
		gimlet.WriteJSONInternalError(w, fmt.Sprintf("error inserting notification job: %s", err.Error()))
		return
	}

	gimlet.WriteJSON(w, nil)
}

func (uis *UIServer) makeNotification(project string, t *task.Task) (*notification.Notification, error) {
	payload, err := trigger.JIRATaskPayload("", project, uis.Settings.Ui.Url, "", "", t)
	if err != nil {
		return nil, err
	}
	sub := event.Subscriber{
		Type: event.JIRAIssueSubscriberType,
		Target: event.JIRAIssueSubscriber{
			Project:   project,
			IssueType: jiraIssueType,
		},
	}
	n, err := notification.New("", utility.RandomString(), &sub, payload)
	if err != nil {
		return nil, err
	}
	if n == nil {
		return nil, errors.New("unexpected error creating notification")
	}
	n.SetTaskMetadata(t.Id, t.Execution)

	err = notification.InsertMany(*n)
	if err != nil {
		return nil, errors.Wrap(err, "error inserting notification")
	}
	return n, nil
}

type searchReturnInfo struct {
	Issues      []thirdparty.JiraTicket `json:"issues"`
	Search      string                  `json:"search"`
	Source      string                  `json:"source"`
	FeaturesURL string                  `json:"features_url"`
}

type suggester interface {
	Suggest(context.Context, *task.Task) ([]thirdparty.JiraTicket, error)
	GetTimeout() time.Duration
}

/////////////////////////////////////////////
// jiraSuggest type (implements suggester) //
/////////////////////////////////////////////

type jiraSuggest struct {
	bbProj      evergreen.BuildBaronProject
	jiraHandler thirdparty.JiraHandler
}

// Suggest returns JIRA ticket results based on the test and/or task name.
func (js *jiraSuggest) Suggest(ctx context.Context, t *task.Task) ([]thirdparty.JiraTicket, error) {
	jql := t.GetJQL(js.bbProj.TicketSearchProjects)

	results, err := js.jiraHandler.JQLSearch(jql, 0, -1)
	if err != nil {
		return nil, err
	}

	return results.Issues, nil
}

func (js *jiraSuggest) GetTimeout() time.Duration {
	// This function is never called because we are willing to wait forever for the fallback handler
	// to return JIRA ticket results.
	return 0
}

/////////////////////////////
// multiSourceSuggest type //
/////////////////////////////

type multiSourceSuggest struct {
	jiraSuggester suggester
}

func (mss *multiSourceSuggest) Suggest(t *task.Task) ([]thirdparty.JiraTicket, string, error) {
	tickets, err := mss.jiraSuggester.Suggest(context.TODO(), t)
	return tickets, jiraSource, err
}
