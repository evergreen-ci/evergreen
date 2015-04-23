package notify

import (
	"10gen.com/mci"
	"10gen.com/mci/apimodels"
	"10gen.com/mci/model"
	"10gen.com/mci/web"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"time"
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
	Details      apimodels.TaskEndDetails
	FailedTests  []model.TestResult
	Subject      string
}

// convenience wrapper about everything we want to know about a task
// notification before it goes off for templating.
type TriggeredTaskNotification struct {
	Current    *model.Task
	Previous   *model.Task
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
			mci.Logger.Logf(slogger.DEBUG, "Adding ”%v” on %v %v notification",
				curr.Id, key.Project, key.NotificationName)

			// get the task's project to add to the notification subject line
			branchName := UnknownProjectBranch
			if projectRef, err := getProjectRef(curr.Project); err != nil {
				mci.Logger.Logf(slogger.WARN, "Unable to find project ref "+
					"for task ”%v”: %v", curr.Id, err)
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

func (self *TaskNotificationHandler) templateNotification(ae *web.App, configName string,
	notification *TriggeredTaskNotification, changeInfo []ChangeInfo) (email Email, err error) {
	// *This could potential break some buildlogger links when MCI changes version as in-progress
	// tasks will still be using the previous version number.*
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error getting MCI version: %v", err)
		return
	}

	current := notification.Current
	taskNotification := TaskNotificationForTemplate{}
	taskNotification.Notification = notification

	build := current.RemoteArgs.Options["build"]

	displayName := getDisplayName(build)

	// add the task end status details
	taskNotification.Details = current.StatusDetails

	// add change information to notification
	notification.Info = changeInfo

	// get the failed tests (if any)
	taskNotification.FailedTests = getFailedTests(current, notification.Key.NotificationName)
	testFailureMessage := ""
	switch len(taskNotification.FailedTests) {
	case 0:
		if current.StatusDetails.TimedOut {
			testFailureMessage = TimeOutMessage
			if current.StatusDetails.TimeoutStage == "heartbeat" {
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

func (self *TaskNotificationHandler) constructChangeInfo(allTasks []model.Task,
	key *NotificationKey) ([]ChangeInfo, error) {
	changeInfoSlice := make([]ChangeInfo, 0)

	for _, task := range allTasks {
		// add blamelist information for each task
		version, err := model.FindVersion(task.Version)
		if err != nil {
			return changeInfoSlice, err
		}
		if version == nil {
			return changeInfoSlice, fmt.Errorf("No version found for task %v "+
				"with version id %v", task.Id, task.Version)
		}
		changeInfo := constructChangeInfo(version, key)
		changeInfo.Pushtime = task.PushTime.Format(time.RFC850)
		changeInfoSlice = append(changeInfoSlice, *changeInfo)
	}

	return changeInfoSlice, nil
}
