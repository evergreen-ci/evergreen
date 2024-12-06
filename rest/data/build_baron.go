package data

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/trigger"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type FailingTaskData struct {
	TaskId    string `bson:"task_id"`
	Execution int    `bson:"execution"`
}

// BbFileTicket creates a JIRA ticket for a task with the given test failures.
func BbFileTicket(ctx context.Context, taskId string, execution int) (int, error) {
	// Find information about the task
	t, err := task.FindOneIdAndExecution(taskId, execution)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	if t == nil {
		return http.StatusNotFound, errors.Wrapf(err, "task '%s' not found with execution '%d'", taskId, execution)
	}

	webHook, ok, err := model.IsWebhookConfigured(t.Project, t.Version)
	if err != nil {
		return http.StatusInternalServerError, errors.Wrapf(err, "retrieving webhook config for project '%s'", t.Project)
	}
	if ok && webHook.Endpoint != "" {
		var resp *http.Response
		resp, err = fileTicketCustomHook(ctx, taskId, execution, webHook)
		return resp.StatusCode, err
	}

	// If there is no custom webhook, use the build baron settings to file a
	// Jira ticket.
	env := evergreen.GetEnvironment()
	settings := env.Settings()
	queue := env.RemoteQueue()
	bbProject, ok := model.GetBuildBaronSettings(t.Project, t.Version)
	if !ok {
		return http.StatusInternalServerError, errors.Errorf("could not find build baron plugin for task '%s' with execution '%d'", taskId, execution)
	}

	n, err := makeJiraNotification(ctx, settings, t, jiraTicketOptions{
		project:   bbProject.TicketCreateProject,
		issueType: bbProject.TicketCreateIssueType,
	})
	if err != nil {
		return http.StatusInternalServerError, err
	}
	ts := utility.RoundPartOfMinute(1).Format(units.TSFormat)
	if err = amboy.EnqueueUniqueJob(ctx, queue, units.NewEventSendJob(n.ID, ts)); err != nil {
		return http.StatusInternalServerError, errors.Wrapf(err, "inserting notification job")
	}

	return http.StatusOK, nil
}

// fileTicketCustomHook uses a custom hook to create a ticket for a task with the given test failures.
func fileTicketCustomHook(context context.Context, taskId string, execution int, webHook evergreen.WebHook) (*http.Response, error) {
	failingTaskData := FailingTaskData{
		TaskId:    taskId,
		Execution: execution,
	}

	req, err := http.NewRequest(http.MethodPost, webHook.Endpoint, nil)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(context)

	jsonBytes, err := json.Marshal(failingTaskData)
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(jsonBytes))

	if len(webHook.Secret) > 0 {
		req.Header.Add(evergreen.APIKeyHeader, webHook.Secret)
	}

	client := utility.GetHTTPClient()
	defer utility.PutHTTPClient(client)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, errors.New("empty response from server")
	}
	return resp, nil
}

type jiraTicketOptions struct {
	project   string
	issueType string
}

func (o *jiraTicketOptions) validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(o.project == "", "must specify Jira project")
	if catcher.HasErrors() {
		return catcher.Resolve()
	}

	if o.issueType == "" {
		o.issueType = defaultJiraIssueType
	}

	return nil
}

const defaultJiraIssueType = "Build Failure"

func makeJiraNotification(ctx context.Context, settings *evergreen.Settings, t *task.Task, jiraOpts jiraTicketOptions) (*notification.Notification, error) {
	if err := jiraOpts.validate(); err != nil {
		return nil, errors.Wrap(err, "invalid Jira ticket options")
	}

	mappings := &evergreen.JIRANotificationsConfig{}
	if err := mappings.Get(ctx); err != nil {
		return nil, errors.Wrap(err, "getting Jira mappings")
	}
	payload, err := trigger.JIRATaskPayload(ctx, trigger.JiraIssueParameters{
		Project:  jiraOpts.project,
		UiURL:    settings.Ui.Url,
		Mappings: mappings,
		Task:     t,
	})
	if err != nil {
		return nil, err
	}
	sub := event.Subscriber{
		Type: event.JIRAIssueSubscriberType,
		Target: event.JIRAIssueSubscriber{
			Project:   jiraOpts.project,
			IssueType: jiraOpts.issueType,
		},
	}
	n, err := notification.New("", utility.RandomString(), &sub, payload)
	if err != nil {
		return nil, err
	}
	n.SetTaskMetadata(t.Id, t.Execution)

	err = notification.InsertMany(*n)
	if err != nil {
		return nil, errors.Wrap(err, "batch inserting notifications")
	}
	return n, nil
}
