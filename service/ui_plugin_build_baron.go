package service

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/gorilla/mux"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	msPerNS     = 1000 * 1000
	maxNoteSize = 16 * 1024 // 16KB
)

func bbGetConfig(settings *evergreen.Settings) map[string]evergreen.BuildBaronProject {
	bbproj := make(map[string]evergreen.BuildBaronProject)
	bbconf, ok := settings.Plugins["buildbaron"]
	if !ok {
		return bbproj
	}

	for k, v := range bbconf {
		proj, ok := v.(evergreen.BuildBaronProject)
		if !ok {
			continue
		}
		bbproj[k] = proj
	}

	return bbproj
}

// saveNote reads a request containing a note's content along with the last seen
// edit time and updates the note in the database.
func bbSaveNote(w http.ResponseWriter, r *http.Request) {
	taskId := mux.Vars(r)["task_id"]
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
	taskId := mux.Vars(r)["task_id"]
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
	taskId := mux.Vars(r)["task_id"]

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
	taskId := mux.Vars(r)["task_id"]
	exec := mux.Vars(r)["execution"]
	oldId := fmt.Sprintf("%v_%v", taskId, exec)
	t, err := task.FindOneOld(task.ById(oldId))
	if err != nil {
		gimlet.WriteJSONInternalError(rw, err.Error())
		return
	}
	// if the archived task was not found, we must be looking for the most recent exec
	if t == nil {
		t, err = task.FindOne(task.ById(taskId))
		if err != nil {
			gimlet.WriteJSONInternalError(rw, err.Error())
			return
		}
	}

	bbProj, ok := uis.buildBaronProjects[t.Project]
	if !ok {
		gimlet.WriteJSON(rw, fmt.Sprintf("Corresponding JIRA project for %v not found", t.Project))
		return
	}

	fallback := &jiraSuggest{bbProj, uis.jiraHandler}
	altEndpoint := &altEndpointSuggest{bbProj}

	var tickets []thirdparty.JiraTicket
	if bbProj.AlternativeEndpointURL != "" {
		tickets, err = raceSuggesters(fallback, altEndpoint, t)
	} else {
		tickets, err = fallback.Suggest(context.TODO(), t)
	}

	jql := t.GetJQL(bbProj.TicketSearchProjects)
	if err != nil {
		message := fmt.Sprintf("Error searching jira for ticket: %v, %s", err, jql)
		grip.Error(message)
		gimlet.WriteJSONInternalError(rw, message)
		return
	}
	gimlet.WriteJSON(rw, searchReturnInfo{Issues: tickets, Search: jql})
}

// raceSuggesters returns the JIRA ticket results from the altEndpoint suggester if it returns
// within its configured interval, and returns the JIRA ticket results from the fallback suggester
// otherwise.
func raceSuggesters(fallback, altEndpoint suggester, t *task.Task) ([]thirdparty.JiraTicket, error) {
	type result struct {
		Tickets []thirdparty.JiraTicket
		Error   error
	}

	// thirdparty/jira.go and thirdparty/http.go do not expose an API that accepts a context.Context.
	fallbackCtx := context.TODO()
	fallbackChan := make(chan result, 1)
	go func() {
		suggestions, err := fallback.Suggest(fallbackCtx, t)
		fallbackChan <- result{suggestions, err}
		close(fallbackChan)
	}()

	altEndpointTimeout := altEndpoint.GetTimeout()
	altEndpointCtx, altEndpointCancel := context.WithTimeout(context.Background(), altEndpointTimeout)
	defer altEndpointCancel()
	suggestions, err := altEndpoint.Suggest(altEndpointCtx, t)

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
		return fallbackChanRes.Tickets, fallbackChanRes.Error
	}

	return suggestions, nil
}

type searchReturnInfo struct {
	Issues []thirdparty.JiraTicket `json:"issues"`
	Search string                  `json:"search"`
}

type suggester interface {
	Suggest(context.Context, *task.Task) ([]thirdparty.JiraTicket, error)
	GetTimeout() time.Duration
}

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

type altEndpointSuggest struct {
	bbProj evergreen.BuildBaronProject
}

type altEndpointSuggestion struct {
	TestName string `json:"test_name"`
	Issues   []struct {
		Key         string `json:"key"`
		Summary     string `json:"summary"`
		Status      string `json:"status"`
		Resolution  string `json:"resolution"`
		CreatedDate string `json:"created_date"`
		UpdatedDate string `json:"updated_date"`
	}
}

type altEndpointResponse struct {
	Status      string                  `json:"status"`
	Suggestions []altEndpointSuggestion `json:"suggestions"`
}

// parseResponse converts the Build Baron tool's suggestion response into JIRA ticket results.
func (aes *altEndpointSuggest) parseResponse(r io.ReadCloser) ([]thirdparty.JiraTicket, error) {
	data := altEndpointResponse{}

	if err := util.ReadJSONInto(r, &data); err != nil {
		return nil, errors.Wrap(err, "Failed to parse Build Baron suggestions")
	}

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

func (aes *altEndpointSuggest) Suggest(ctx context.Context, t *task.Task) ([]thirdparty.JiraTicket, error) {
	client := util.GetHTTPClient()
	defer util.PutHTTPClient(client)

	url := aes.bbProj.AlternativeEndpointURL
	url = strings.Replace(url, "{task_id}", t.Id, -1)
	url = strings.Replace(url, "{execution}", strconv.Itoa(t.Execution), -1)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if aes.bbProj.AlternativeEndpointUsername != "" {
		req.SetBasicAuth(aes.bbProj.AlternativeEndpointUsername, aes.bbProj.AlternativeEndpointPassword)
	}

	req = req.WithContext(ctx)
	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, errors.Errorf("HTTP request returned unexpected status: %v", resp.Status)
		}
		return nil, errors.Errorf("HTTP request returned unexpected status=%v: %s", resp.Status, string(body))
	}

	return aes.parseResponse(resp.Body)
}

func (aes *altEndpointSuggest) GetTimeout() time.Duration {
	return time.Duration(aes.bbProj.AlternativeEndpointTimeoutSecs) * time.Second
}
