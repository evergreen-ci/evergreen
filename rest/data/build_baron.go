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
	"github.com/pkg/errors"
)

const jiraIssueType = "Build Failure"

type FailingTaskData struct {
	TaskId    string `bson:"task_id"`
	Execution int    `bson:"execution"`
}

// BbFileTicket creates a JIRA ticket for a task with the given test failures.
func BbFileTicket(context context.Context, taskId string, execution int) (int, error) {
	// Find information about the task
	t, err := task.FindOneIdAndExecution(taskId, execution)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	if t == nil {
		return http.StatusNotFound, errors.Wrapf(err, "task '%s' not found with execution '%d'", taskId, execution)
	}
	env := evergreen.GetEnvironment()
	settings := env.Settings()
	queue := env.RemoteQueue()
	bbProject, ok := model.GetBuildBaronSettings(t.Project, t.Version)
	if !ok {
		return http.StatusInternalServerError, errors.Errorf("could not find build baron plugin for task '%s' with execution '%d'", taskId, execution)
	}

	webHook, ok, err := model.IsWebhookConfigured(t.Project, t.Version)
	if err != nil {
		return http.StatusInternalServerError, errors.Wrapf(err, "retrieving webhook config for project '%s'", t.Project)
	}
	if ok && webHook.Endpoint != "" {
		var resp *http.Response
		resp, err = fileTicketCustomHook(context, taskId, execution, webHook)
		return resp.StatusCode, err
	}

	//if there is no custom web-hook, use the build baron
	n, err := makeNotification(settings, bbProject.TicketCreateProject, t)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	ts := utility.RoundPartOfMinute(1).Format(units.TSFormat)
	err = queue.Put(context, units.NewEventSendJob(n.ID, ts))
	if err != nil {
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
	n.SetTaskMetadata(t.Id, t.Execution)

	err = notification.InsertMany(*n)
	if err != nil {
		return nil, errors.Wrap(err, "batch inserting notifications")
	}
	return n, nil
}
