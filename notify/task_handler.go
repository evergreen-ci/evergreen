package notify

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/web"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	TimeOutMessage      = "timeout"
	UnresponsiveMessage = "unresponsive"
)

// "Base class" for all task_*_handler.go structs. Contains code that's common
// to all the task_*_handlers. Note that this struct does NOT implement
// NotificationHandler
type TaskNotificationHandler struct {
	Type string
}

type TaskNotificationForTemplate struct {
	Notification *TriggeredTaskNotification
	LogsUrl      string
	Details      apimodels.TaskEndDetail
	FailedTests  []task.TestResult
	Subject      string
}

// convenience wrapper about everything we want to know about a task
// notification before it goes off for templating.
type TriggeredTaskNotification struct {
	Current    *task.Task
	Previous   *task.Task
	Info       []ChangeInfo
	Key        NotificationKey
	Preface    string
	Transition string
}

func (self *TaskNotificationHandler) getRecentlyFinishedTasksWithStatus(key *NotificationKey,
	status string, preface string, transition string) ([]TriggeredTaskNotification, error) {
	taskNotifications := []TriggeredTaskNotification{}
	tasks, err := getRecentlyFinishedTasks(key)
	if err != nil {
		return nil, err
	}

	for _, currentTask := range tasks {
		// Copy by value to make pointer safe
		curr := currentTask
		if status == "" || curr.Status == status {
			grip.Debugf("Adding '%s' on %s %s notification",
				curr.Id, key.Project, key.NotificationName)

			// get the task's project to add to the notification subject line
			branchName := UnknownProjectBranch
			if projectRef, err := getProjectRef(curr.Project); err != nil {
				grip.Warningf("Unable to find project ref for task '%s': %v",
					curr.Id, err)
			} else if projectRef != nil {
				branchName = projectRef.Branch
			}

			notification := TriggeredTaskNotification{
				Current:    &curr,
				Previous:   nil,
				Key:        *key,
				Preface:    fmt.Sprintf(preface, branchName),
				Transition: transition,
			}
			taskNotifications = append(taskNotifications, notification)
		}
	}
	return taskNotifications, nil
}

func (self *TaskNotificationHandler) templateNotification(ae *web.App,
	notification *TriggeredTaskNotification, changeInfo []ChangeInfo) (email Email, err error) {
	// *This could potential break some buildlogger links when MCI changes version as in-progress
	// tasks will still be using the previous version number.*
	if err != nil {
		grip.Errorln("Error getting MCI version:", err)
		return
	}

	current := notification.Current
	taskNotification := TaskNotificationForTemplate{}
	taskNotification.Notification = notification

	displayName := getDisplayName(current.BuildId)

	// add the task end status details
	taskNotification.Details = current.Details

	// add change information to notification
	notification.Info = changeInfo

	// get the failed tests (if any)
	taskNotification.FailedTests = getFailedTests(current, notification.Key.NotificationName)
	var testFailureMessage string
	switch len(taskNotification.FailedTests) {
	case 0:
		if current.Details.TimedOut {
			testFailureMessage = TimeOutMessage
			if current.Details.Description == task.AgentHeartbeat {
				testFailureMessage = UnresponsiveMessage
			}
		} else {
			testFailureMessage = "possible MCI failure"
		}
	case 1:
		testFailureMessage = taskNotification.FailedTests[0].TestFile
	default:
		testFailureMessage = fmt.Sprintf("%v tests failed",
			len(taskNotification.FailedTests))
	}

	// construct the task notification subject line
	subject := fmt.Sprintf("%v %v in %v (%v on %v)", notification.Preface,
		testFailureMessage, current.DisplayName, notification.Transition, displayName)
	taskNotification.Subject = subject

	taskNotification.LogsUrl = fmt.Sprintf("task/%v", current.Id)

	// template task notification body
	body, err := TemplateEmailBody(ae, "task_notification.html", taskNotification)
	if err != nil {
		return
	}

	email = &TaskEmail{EmailBase{body, subject, notification.Info}, *notification}
	return
}

func (self *TaskNotificationHandler) constructChangeInfo(allTasks []task.Task,
	key *NotificationKey) ([]ChangeInfo, error) {
	changeInfoSlice := make([]ChangeInfo, 0)

	for _, task := range allTasks {
		// add blamelist information for each task
		v, err := version.FindOne(version.ById(task.Version))
		if err != nil {
			return changeInfoSlice, err
		}
		if v == nil {
			return changeInfoSlice, errors.Errorf("No version found for task %v with version id %v",
				task.Id, task.Version)
		}
		changeInfo := constructChangeInfo(v, key)
		changeInfo.Pushtime = task.PushTime.Format(time.RFC850)
		changeInfoSlice = append(changeInfoSlice, *changeInfo)
	}

	return changeInfoSlice, nil
}
