package buildbaron

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

func init() {
	plugin.Publish(&BuildBaronPlugin{})
}

const (
	PluginName  = "buildbaron"
	JIRAFailure = "Error searching jira for ticket"
	JQLBFQuery  = "(project in (%v)) and ( %v ) order by updatedDate desc"

	msPerNS     = 1000 * 1000
	maxNoteSize = 16 * 1024 // 16KB
)

type bbProject evergreen.BuildBaronProject

type bbPluginOptions struct {
	Host     string
	Username string
	Password string
	Projects map[string]bbProject
}

type BuildBaronPlugin struct {
	opts        *bbPluginOptions
	jiraHandler thirdparty.JiraHandler
}

// A regex that matches either / or \ for splitting directory paths
// on either windows or linux paths.
var eitherSlash *regexp.Regexp = regexp.MustCompile(`[/\\]`)

func (bbp *BuildBaronPlugin) Name() string {
	return PluginName
}

// GetUIHandler adds a path for looking up build failures in JIRA.
func (bbp *BuildBaronPlugin) GetUIHandler() http.Handler {
	if bbp.opts == nil {
		panic("build baron plugin missing configuration")
	}
	r := mux.NewRouter()
	r.Path("/jira_bf_search/{task_id}/{execution}").HandlerFunc(bbp.buildFailuresSearch)
	r.Path("/created_tickets/{task_id}").HandlerFunc(bbp.getCreatedTickets)
	r.Path("/note/{task_id}").Methods("GET").HandlerFunc(bbp.getNote)
	r.Path("/note/{task_id}").Methods("PUT").HandlerFunc(bbp.saveNote)
	r.Path("/file_ticket").Methods("POST").HandlerFunc(bbp.fileTicket)
	return r
}

func (bbp *BuildBaronPlugin) Configure(conf map[string]interface{}) error {
	// pull out options needed from config file (JIRA authentication info, and list of projects)
	bbpOptions := &bbPluginOptions{}

	err := mapstructure.Decode(conf, bbpOptions)
	if err != nil {
		return err
	}
	if bbpOptions.Host == "" || bbpOptions.Username == "" || bbpOptions.Password == "" {
		return fmt.Errorf("Host, username, and password in config must not be blank")
	}
	if len(bbpOptions.Projects) == 0 {
		return fmt.Errorf("Must specify at least 1 Evergreen project")
	}
	for projName, proj := range bbpOptions.Projects {
		if proj.TicketCreateProject == "" {
			return fmt.Errorf("ticket_create_project cannot be blank")
		}
		if len(proj.TicketSearchProjects) == 0 {
			return fmt.Errorf("ticket_search_projects cannot be empty")
		}
		if proj.AlternativeEndpointURL != "" {
			if _, err := url.Parse(proj.AlternativeEndpointURL); err != nil {
				return errors.Wrapf(err, `Failed to parse alt_endpoint_url for project "%s"`, projName)
			}
			if proj.AlternativeEndpointUsername == "" && proj.AlternativeEndpointPassword != "" {
				return errors.Errorf(`Failed validating configuration for project "%s": `+
					"alt_endpoint_password must be blank if alt_endpoint_username is blank", projName)
			}
			if proj.AlternativeEndpointTimeoutSecs <= 0 {
				return errors.Errorf(`Failed validating configuration for project "%s": `+
					"alt_endpoint_timeout_secs must be positive", projName)
			}
		} else if proj.AlternativeEndpointUsername != "" || proj.AlternativeEndpointPassword != "" {
			return errors.Errorf(`Failed validating configuration for project "%s": `+
				"alt_endpoint_username and alt_endpoint_password must be blank alt_endpoint_url is blank", projName)
		} else if proj.AlternativeEndpointTimeoutSecs != 0 {
			return errors.Errorf(`Failed validating configuration for project "%s": `+
				"alt_endpoint_timeout_secs must be zero when alt_endpoint_url is blank", projName)
		}
	}
	bbp.opts = bbpOptions
	bbp.jiraHandler = thirdparty.NewJiraHandler(
		bbp.opts.Host,
		bbp.opts.Username,
		bbp.opts.Password,
	)
	return nil
}

func (bbp *BuildBaronPlugin) GetPanelConfig() (*plugin.PanelConfig, error) {
	return &plugin.PanelConfig{
		Panels: []plugin.UIPanel{
			{
				Page:      plugin.TaskPage,
				Position:  plugin.PageRight,
				PanelHTML: template.HTML(`<div ng-include="'/plugin/buildbaron/static/partials/task_build_baron.html'"></div>`),
				Includes: []template.HTML{
					template.HTML(`<link href="/plugin/buildbaron/static/css/task_build_baron.css" rel="stylesheet"/>`),
					template.HTML(`<script type="text/javascript" src="/plugin/buildbaron/static/js/task_build_baron.js"></script>`),
				},
				DataFunc: func(context plugin.UIContext) (interface{}, error) {
					_, enabled := bbp.opts.Projects[context.ProjectRef.Identifier]
					return struct {
						Enabled bool `json:"enabled"`
					}{enabled}, nil
				},
			},
		},
	}, nil
}

type searchReturnInfo struct {
	Issues []thirdparty.JiraTicket `json:"issues"`
	Search string                  `json:"search"`
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
	fallbackChan := make(chan result)
	go func() {
		suggestions, err := fallback.Suggest(fallbackCtx, t)
		fallbackChan <- result{suggestions, err}
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
		grip.Warningf(
			"Failed to get results from alternative endpoint for task_id=%s, execution=%d: %s",
			t.Id, t.Execution, err)

		fallbackChanRes := <-fallbackChan
		return fallbackChanRes.Tickets, fallbackChanRes.Error
	}

	return suggestions, nil
}

// BuildFailuresSearchHandler handles the requests of searching jira in the build
//  failures project
func (bbp *BuildBaronPlugin) buildFailuresSearch(w http.ResponseWriter, r *http.Request) {
	taskId := mux.Vars(r)["task_id"]
	exec := mux.Vars(r)["execution"]
	oldId := fmt.Sprintf("%v_%v", taskId, exec)
	t, err := task.FindOneOld(task.ById(oldId))
	if err != nil {
		util.WriteJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	// if the archived task was not found, we must be looking for the most recent exec
	if t == nil {
		t, err = task.FindOne(task.ById(taskId))
		if err != nil {
			util.WriteJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	bbProj, ok := bbp.opts.Projects[t.Project]
	if !ok {
		util.WriteJSON(w, http.StatusInternalServerError,
			fmt.Sprintf("Corresponding JIRA project for %v not found", t.Project))
		return
	}

	fallback := jiraSuggest{bbProj, bbp.jiraHandler}
	altEndpoint := altEndpointSuggest{bbProj}

	var tickets []thirdparty.JiraTicket
	if bbProj.AlternativeEndpointURL != "" {
		tickets, err = raceSuggesters(&fallback, &altEndpoint, t)
	} else {
		tickets, err = fallback.Suggest(context.TODO(), t)
	}

	jql := taskToJQL(t, bbProj.TicketSearchProjects)
	if err != nil {
		message := fmt.Sprintf("%v: %v, %v", JIRAFailure, err, jql)
		grip.Error(message)
		util.WriteJSON(w, http.StatusInternalServerError, message)
		return
	}
	util.WriteJSON(w, http.StatusOK, searchReturnInfo{Issues: tickets, Search: jql})
}

type suggester interface {
	Suggest(ctx context.Context, t *task.Task) ([]thirdparty.JiraTicket, error)
	GetTimeout() time.Duration
}

type jiraSuggest struct {
	bbProj      bbProject
	jiraHandler thirdparty.JiraHandler
}

// Suggest returns JIRA ticket results based on the test and/or task name.
func (js *jiraSuggest) Suggest(ctx context.Context, t *task.Task) ([]thirdparty.JiraTicket, error) {
	jql := taskToJQL(t, js.bbProj.TicketSearchProjects)

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
	bbProj bbProject
}

type altEndpointResponse struct {
	Status      string `json:"status"`
	Suggestions []struct {
		TestName string `json:"test_name"`
		Issues   []struct {
			Key         string `json:"key"`
			Summary     string `json:"summary"`
			Status      string `json:"status"`
			Resolution  string `json:"resolution"`
			CreatedDate string `json:"created_date"`
			UpdatedDate string `json:"updated_date"`
		}
	} `json:"suggestions"`
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

func (bbp *BuildBaronPlugin) getCreatedTickets(w http.ResponseWriter, r *http.Request) {
	taskId := mux.Vars(r)["task_id"]

	events, err := event.Find(event.AllLogCollection, event.TaskEventsForId(taskId))
	if err != nil {
		util.WriteJSON(w, http.StatusInternalServerError, err.Error())
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
		jiraIssue, err := bbp.jiraHandler.GetJIRATicket(ticket)
		if err != nil {
			util.WriteJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		if jiraIssue == nil {
			continue
		}
		results = append(results, *jiraIssue)
	}

	util.WriteJSON(w, http.StatusOK, results)
}

// getNote retrieves the latest note from the database.
func (bbp *BuildBaronPlugin) getNote(w http.ResponseWriter, r *http.Request) {
	taskId := mux.Vars(r)["task_id"]
	n, err := model.NoteForTask(taskId)
	if err != nil {
		util.WriteJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n == nil {
		util.WriteJSON(w, http.StatusOK, "")
		return
	}
	util.WriteJSON(w, http.StatusOK, n)
}

// saveNote reads a request containing a note's content along with the last seen
// edit time and updates the note in the database.
func (bbp *BuildBaronPlugin) saveNote(w http.ResponseWriter, r *http.Request) {
	taskId := mux.Vars(r)["task_id"]
	n := &model.Note{}
	if err := util.ReadJSONInto(r.Body, n); err != nil {
		util.WriteJSON(w, http.StatusBadRequest, err.Error())
		return
	}

	// prevent incredibly large notes
	if len(n.Content) > maxNoteSize {
		util.WriteJSON(w, http.StatusBadRequest, "note is too large")
		return
	}

	// We need to make sure the user isn't blowing away a new edit,
	// so we load the existing note. If the user's last seen edit time is less
	// than the most recent edit, we error with a helpful message.
	old, err := model.NoteForTask(taskId)
	if err != nil {
		util.WriteJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	// we compare times by millisecond rather than nanosecond so we can
	// work around the rounding that occurs when javascript forces these
	// large values into in float type.
	if old != nil && n.UnixNanoTime/msPerNS != old.UnixNanoTime/msPerNS {
		util.WriteJSON(w, http.StatusBadRequest,
			"this note has already been edited. Please refresh and try again.")
		return
	}

	n.TaskId = taskId
	n.UnixNanoTime = time.Now().UnixNano()
	if err := n.Upsert(); err != nil {
		util.WriteJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	util.WriteJSON(w, http.StatusOK, n)
}

// Generates a jira JQL string from the task
// When we search in jira for a task we search in the specified JIRA project
// If there are any test results, then we only search by test file
// name of all of the failed tests.
// Otherwise we search by the task name.
func taskToJQL(t *task.Task, searchProjects []string) string {
	var jqlParts []string
	var jqlClause string
	for _, testResult := range t.LocalTestResults {
		if testResult.Status == evergreen.TestFailedStatus {
			fileParts := eitherSlash.Split(testResult.TestFile, -1)
			jqlParts = append(jqlParts, fmt.Sprintf("text~\"%v\"", fileParts[len(fileParts)-1]))
		}
	}
	if jqlParts != nil {
		jqlClause = strings.Join(jqlParts, " or ")
	} else {
		jqlClause = fmt.Sprintf("text~\"%v\"", t.DisplayName)
	}

	return fmt.Sprintf(JQLBFQuery, strings.Join(searchProjects, ", "), jqlClause)
}
