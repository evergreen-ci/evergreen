package graphql

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/trigger"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	jiraSource    = "JIRA"
	jiraIssueType = "Build Failure"
)

// bbFileTicket creates a JIRA ticket for a task with the given test failures.
func BbFileTicket(context context.Context, taskId string) (bool, error) {
	taskNotFound := false
	// Find information about the task
	t, err := task.FindOne(task.ById(taskId))
	if err != nil {
		return taskNotFound, err

	}
	if t == nil {
		taskNotFound = true
		return taskNotFound, errors.Wrap(err, fmt.Sprintf("task not found for id %s", taskId))
	}
	env := evergreen.GetEnvironment()
	settings := env.Settings()
	queue := env.RemoteQueue()
	buildBaronProjects := BbGetConfig(settings)
	n, err := makeNotification(settings, buildBaronProjects[t.Project].TicketCreateProject, t)
	if err != nil {
		return taskNotFound, err
	}
	ts := utility.RoundPartOfMinute(1).Format(units.TSFormat)
	err = queue.Put(context, units.NewEventSendJob(n.ID, ts))
	if err != nil {
		return taskNotFound, errors.Wrap(err, fmt.Sprintf("error inserting notification job: %s", err.Error()))

	}

	return taskNotFound, nil
}

func makeNotification(settings *evergreen.Settings, project string, t *task.Task) (*notification.Notification, error) {
	payload, err := trigger.JIRATaskPayload("", project, settings.Ui.Url, "", "", t)
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

func BbGetCreatedTicketsPointers(taskId string) ([]*thirdparty.JiraTicket, error) {

	events, err := event.Find(event.AllLogCollection, event.TaskEventsForId(taskId))
	if err != nil {
		return nil, err
	}

	var results []*thirdparty.JiraTicket
	var searchTickets []string
	for _, evt := range events {
		data := evt.Data.(*event.TaskEventData)
		if evt.EventType == event.TaskJiraAlertCreated {
			searchTickets = append(searchTickets, data.JiraIssue)
		}
	}
	settings := evergreen.GetEnvironment().Settings()
	jiraHandler := thirdparty.NewJiraHandler(*settings.Jira.Export())
	for _, ticket := range searchTickets {
		jiraIssue, err := jiraHandler.GetJIRATicket(ticket)
		if err != nil {
			return nil, err
		}
		if jiraIssue == nil {
			continue
		}
		results = append(results, jiraIssue)
	}

	return results, nil
}

type buildBaronConfig struct {
	ProjectFound     bool
	SearchConfigured bool
}

func GetSearchReturnInfo(taskId string, exec string) (*thirdparty.SearchReturnInfo, buildBaronConfig, error) {
	bbConfig := buildBaronConfig{}
	t, err := BbGetTask(taskId, exec)
	if err != nil {
		return nil, bbConfig, err
	}
	settings := evergreen.GetEnvironment().Settings()
	buildBaronProjects := BbGetConfig(settings)
	bbProj, ok := buildBaronProjects[t.Project]

	if !ok {
		bbConfig.ProjectFound = false
		return nil, bbConfig, errors.Errorf("Build Baron project for %s not found", t.Project)
	}
	bbConfig.ProjectFound = true

	// the build baron is configured if the jira search is configured
	if !(len(bbProj.TicketSearchProjects) > 0) {
		bbConfig.SearchConfigured = false
		return nil, bbConfig, errors.Errorf("Build Baron ticket search projects for %s not found", t.Project)
	}
	bbConfig.SearchConfigured = true
	jiraHandler := thirdparty.NewJiraHandler(*settings.Jira.Export())
	jira := &JiraSuggest{bbProj, jiraHandler}
	multiSource := &MultiSourceSuggest{jira}

	var tickets []thirdparty.JiraTicket
	var source string

	jql := t.GetJQL(bbProj.TicketSearchProjects)
	tickets, source, err = multiSource.Suggest(t)
	if err != nil {
		return nil, bbConfig, errors.Errorf("Error searching for tickets: %s", err.Error())
	}

	var featuresURL string
	if bbProj.BFSuggestionFeaturesURL != "" {
		featuresURL = bbProj.BFSuggestionFeaturesURL
		featuresURL = strings.Replace(featuresURL, "{task_id}", taskId, -1)
		featuresURL = strings.Replace(featuresURL, "{execution}", exec, -1)
	} else {
		featuresURL = ""
	}
	return &thirdparty.SearchReturnInfo{Issues: tickets, Search: jql, Source: source, FeaturesURL: featuresURL}, bbConfig, nil
}

func BbGetConfig(settings *evergreen.Settings) map[string]evergreen.BuildBaronProject {
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

func BbGetTask(taskId string, executionString string) (*task.Task, error) {
	execution, err := strconv.Atoi(executionString)
	if err != nil {
		return nil, errors.Wrap(err, "Invalid execution number")
	}
	t, err := task.FindOneIdOldOrNew(taskId, execution)
	if err != nil {
		return nil, errors.Wrap(err, "problem finding task")
	}
	if t == nil {
		return nil, errors.Errorf("No task found for taskId: %s and execution: %d", taskId, execution)
	}
	if t.DisplayOnly {
		t.LocalTestResults, err = t.GetTestResultsForDisplayTask()
		if err != nil {
			return nil, errors.Wrapf(err, "Problem finding test results for display task '%s'", t.Id)
		}
	}
	return t, nil
}
func (js *JiraSuggest) GetTimeout() time.Duration {
	// This function is never called because we are willing to wait forever for the fallback handler
	// to return JIRA ticket results.
	return 0
}

// Suggest returns JIRA ticket results based on the test and/or task name.
func (js *JiraSuggest) Suggest(ctx context.Context, t *task.Task) ([]thirdparty.JiraTicket, error) {
	jql := t.GetJQL(js.BbProj.TicketSearchProjects)

	results, err := js.JiraHandler.JQLSearch(jql, 0, 50)
	if err != nil {
		return nil, err
	}

	return results.Issues, nil
}

type Suggester interface {
	Suggest(context.Context, *task.Task) ([]thirdparty.JiraTicket, error)
	GetTimeout() time.Duration
}

type MultiSourceSuggest struct {
	JiraSuggester Suggester
}

type JiraSuggest struct {
	BbProj      evergreen.BuildBaronProject
	JiraHandler thirdparty.JiraHandler
}

func (mss *MultiSourceSuggest) Suggest(t *task.Task) ([]thirdparty.JiraTicket, string, error) {
	tickets, err := mss.JiraSuggester.Suggest(context.TODO(), t)
	return tickets, jiraSource, err
}
