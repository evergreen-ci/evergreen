package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
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
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	msPerNS            = 1000 * 1000
	maxNoteSize        = 16 * 1024 // 16KB
	jiraSource         = "JIRA"
	bfSuggestionSource = "BF Suggestion Server"
	jiraIssueType      = "Build Failure"
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
	return t, nil
}

func (uis *UIServer) bbGetTaskAndBFSuggestionClient(taskId string, execution string) (*task.Task, *bfSuggestionClient, error) {
	t, err := bbGetTask(taskId, execution)
	if err != nil {
		return nil, nil, err
	}

	bbProj, ok := uis.buildBaronProjects[t.Project]
	if !ok {
		return nil, nil, errors.Errorf("Build Baron project for %s not found", t.Project)
	}

	bfsc := getBFSuggestionClient(bbProj)
	if bfsc == nil {
		return nil, nil, errors.Errorf("No BF Suggestion Server configured for the project %s", t.Project)
	}

	return t, bfsc, err
}

// saveNote reads a request containing a note's content along with the last seen
// edit time and updates the note in the database.
func bbSaveNote(w http.ResponseWriter, r *http.Request) {
	taskId := gimlet.GetVars(r)["task_id"]
	n := &model.Note{}
	if err := util.ReadJSONInto(r.Body, n); err != nil {
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

// Retrieve the user feedback for a task id that has been stored by
// the BF Suggestion Server.
func (uis *UIServer) bbGetFeedback(rw http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	taskId := vars["task_id"]
	exec := vars["execution"]

	// grab the user info
	u := MustHaveUser(r)

	t, bfsc, err := uis.bbGetTaskAndBFSuggestionClient(taskId, exec)
	if err != nil {
		gimlet.WriteJSONInternalError(rw, err.Error())
		return
	}

	feedbackItems, err := bfsc.getFeedback(r.Context(), t, u.Username())
	if err != nil {
		gimlet.WriteJSONInternalError(rw, err.Error())
		return
	}
	gimlet.WriteJSON(rw, feedbackItems)
}

func (uis *UIServer) bbSendFeedback(rw http.ResponseWriter, r *http.Request) {
	// grab the user info
	u := MustHaveUser(r)

	var input struct {
		FeedbackType string                 `json:"type"`
		FeedbackData map[string]interface{} `json:"data"`
		TaskId       string                 `json:"task_id"`
		Execution    int                    `json:"execution"`
	}

	if err := util.ReadJSONInto(r.Body, &input); err != nil {
		gimlet.WriteJSONInternalError(rw, err.Error())
		return
	}

	t, bfsc, err := uis.bbGetTaskAndBFSuggestionClient(input.TaskId, strconv.Itoa(input.Execution))
	if err != nil {
		gimlet.WriteJSONInternalError(rw, err.Error())
		return
	}

	if err = bfsc.sendFeedback(r.Context(), t, u.Username(), input.FeedbackType, input.FeedbackData); err != nil {
		gimlet.WriteJSONInternalError(rw, err.Error())
		return
	}
}

func (uis *UIServer) bbRemoveFeedback(rw http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	taskId := vars["task_id"]
	exec := vars["execution"]
	feedbackType := vars["feedback_type"]

	// grab the user info
	u := MustHaveUser(r)

	t, bfsc, err := uis.bbGetTaskAndBFSuggestionClient(taskId, exec)
	if err != nil {
		gimlet.WriteJSONInternalError(rw, err.Error())
		return
	}

	err = bfsc.removeFeedback(r.Context(), t, u.Username(), feedbackType)
	if err != nil {
		gimlet.WriteJSONInternalError(rw, err.Error())
		return
	}
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
	bfsc := getBFSuggestionClient(bbProj)
	var altEndpoint suggester
	if bfsc != nil {
		altEndpoint = &altEndpointSuggest{bfsc, bbProj.BFSuggestionTimeoutSecs}
	} else {
		altEndpoint = nil
	}
	multiSource := &multiSourceSuggest{jira, altEndpoint}

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
	err = uis.queue.Put(r.Context(), units.NewEventNotificationJob(n.ID))
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
	n, err := notification.New("", util.RandomString(), &sub, payload)
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

////////////////////////////////////////////////////
// altEndpointSuggest type (implements suggester) //
////////////////////////////////////////////////////

type altEndpointSuggest struct {
	bfsc        *bfSuggestionClient
	timeoutSecs int
}

func (aes *altEndpointSuggest) Suggest(ctx context.Context, t *task.Task) ([]thirdparty.JiraTicket, error) {
	data, err := aes.bfsc.getSuggestions(ctx, t)
	if err != nil {
		return nil, err
	}

	return aes.parseResponseData(data)
}

func (aes *altEndpointSuggest) parseResponseData(data bfSuggestionResponse) ([]thirdparty.JiraTicket, error) {
	if data.Status != "ok" {
		return nil, errors.Errorf("Build Baron suggestions weren't ready: status=%s", data.Status)
	}

	var tickets []thirdparty.JiraTicket
	for _, suggestion := range data.Suggestions {
		for _, issue := range suggestion.Issues {
			ticket := thirdparty.JiraTicket{
				Key: issue.Key,
				Fields: &thirdparty.TicketFields{
					Summary: issue.Summary,
					Created: issue.CreatedDate,
					Updated: issue.UpdatedDate,
					Status:  &thirdparty.JiraStatus{Name: issue.Status},
				},
			}

			if issue.Resolution != "" {
				ticket.Fields.Resolution = &thirdparty.JiraResolution{Name: issue.Resolution}
			}

			tickets = append(tickets, ticket)
		}
	}

	if len(tickets) == 0 {
		// We treat not having suggestions as an error so that it causes fallback to occur in a
		// unified way.
		return nil, errors.New("no suggestions found")
	}

	return tickets, nil
}

func (aes *altEndpointSuggest) GetTimeout() time.Duration {
	return time.Duration(aes.timeoutSecs) * time.Second
}

/////////////////////////////
// multiSourceSuggest type //
/////////////////////////////

type multiSourceSuggest struct {
	jiraSuggester suggester
	altSuggester  suggester
}

func (mss *multiSourceSuggest) Suggest(t *task.Task) ([]thirdparty.JiraTicket, string, error) {
	var tickets []thirdparty.JiraTicket
	var source string
	var err error

	if mss.altSuggester != nil {
		tickets, source, err = mss.raceSuggest(t)
	} else {
		source = jiraSource
		tickets, err = mss.jiraSuggester.Suggest(context.TODO(), t)
	}
	return tickets, source, err
}

// raceSuggest returns the JIRA ticket results from the altEndpoint suggester if it returns
// within its configured interval, and returns the JIRA ticket results from the fallback suggester
// otherwise.
func (mss *multiSourceSuggest) raceSuggest(t *task.Task) ([]thirdparty.JiraTicket, string, error) {
	type result struct {
		Tickets []thirdparty.JiraTicket
		Error   error
	}

	// thirdparty/jira.go and thirdparty/http.go do not expose an API that accepts a context.Context.
	fallbackCtx := context.TODO()
	fallbackChan := make(chan result, 1)
	go func() {
		suggestions, err := mss.jiraSuggester.Suggest(fallbackCtx, t)
		fallbackChan <- result{suggestions, err}
		close(fallbackChan)
	}()

	altEndpointTimeout := mss.altSuggester.GetTimeout()
	altEndpointCtx, altEndpointCancel := context.WithTimeout(context.Background(), altEndpointTimeout)
	defer altEndpointCancel()
	suggestions, err := mss.altSuggester.Suggest(altEndpointCtx, t)

	// If the alternative endpoint didn't respond quickly enough or didn't have results available,
	// then we wait for the fallback results. Ideally we'd otherwise be able to cancel the request
	// for fetching the fallback results, but we instead just return back to the caller without
	// waiting for the associated goroutine to complete.
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message":   "failed to get results from alternative endpoint",
			"task_id":   t.Id,
			"execution": t.Execution,
		}))

		fallbackChanRes := <-fallbackChan
		return fallbackChanRes.Tickets, jiraSource, fallbackChanRes.Error
	}

	return suggestions, bfSuggestionSource, nil
}
