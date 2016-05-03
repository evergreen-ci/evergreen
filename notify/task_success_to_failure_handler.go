package notify

import (
	"fmt"

	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/web"
)

// Handler for notifications generated specifically when a task fails and the
// previous finished task succeeded. Implements NotificationHandler from
// notification_handler.go.
type TaskSuccessToFailureHandler struct {
	TaskNotificationHandler
	Name string
}

func (self *TaskSuccessToFailureHandler) GetNotifications(ae *web.App, configName string,
	key *NotificationKey) ([]Email, error) {
	var emails []Email
	tasks, err := getRecentlyFinishedTasks(key)
	if err != nil {
		return nil, err
	}

	preface := mciFailurePreface
	if key.NotificationRequester == evergreen.PatchVersionRequester {
		preface = patchFailurePreface
	}

	for _, currentTask := range tasks {
		// Copy by value to make pointer safe
		curr := currentTask

		// get previous task for this project/build variant
		previousTask, err := currentTask.PreviousCompletedTask(key.Project, []string{})
		if previousTask == nil {
			evergreen.Logger.Logf(slogger.DEBUG,
				"No previous completed task found for ”%v” on %v %v notification",
				currentTask.Id, key.Project, key.NotificationName)
			continue
		} else if err != nil {
			return nil, err
		}
		evergreen.Logger.Logf(slogger.DEBUG,
			"Previous completed task found for ”%v” on %v %v notification is %v",
			currentTask.Id, key.Project, key.NotificationName, previousTask.Id)

		if previousTask.Status == evergreen.TaskSucceeded &&
			currentTask.Status == evergreen.TaskFailed {

			// this is now a potential candidate but we must
			// ensure that no other more recent build has
			// triggered a notification for this event
			history, err := model.FindNotificationRecord(previousTask.Id, key.NotificationName,
				getType(key.NotificationName), key.Project, evergreen.RepotrackerVersionRequester)

			// if there's an error log it and move on
			if err != nil {
				evergreen.Logger.Errorf(slogger.ERROR, "Error finding notification record: %v", err)
				continue
			}

			// get the task's project to add to the notification subject line
			branchName := UnknownProjectBranch
			if projectRef, err := getProjectRef(currentTask.Project); err != nil {
				evergreen.Logger.Logf(slogger.WARN, "Unable to find project ref "+
					"for task ”%v”: %v", currentTask.Id, err)
			} else if projectRef != nil {
				branchName = projectRef.Branch
			}

			// if no notification for this handler has been registered, register it
			if history == nil {
				evergreen.Logger.Logf(slogger.DEBUG, "Adding ”%v” on %v %v notification",
					currentTask.Id, key.NotificationName, key.Project)
				notification := TriggeredTaskNotification{
					Current:    &curr,
					Previous:   previousTask,
					Key:        *key,
					Preface:    fmt.Sprintf(preface, branchName),
					Transition: transitionSubject,
				}

				email, err := self.TemplateNotification(ae, configName, &notification)
				if err != nil {
					evergreen.Logger.Errorf(slogger.ERROR, "Error executing template for `%v`: %v",
						currentTask.Id, err)
					continue
				}

				emails = append(emails, email)

				err = model.InsertNotificationRecord(previousTask.Id, currentTask.Id,
					key.NotificationName, getType(key.NotificationName), key.Project,
					evergreen.RepotrackerVersionRequester)
				if err != nil {
					evergreen.Logger.Errorf(slogger.ERROR, "Error inserting notification record: %v", err)
					continue
				}
			} else {
				evergreen.Logger.Logf(slogger.DEBUG, "Skipping intermediate %v handler trigger on ”%v”",
					key.NotificationName, currentTask.Id)
			}
		}
	}

	return emails, nil
}

func (self *TaskSuccessToFailureHandler) TemplateNotification(ae *web.App,
	configName string, notification *TriggeredTaskNotification) (Email, error) {
	changeInfo, err := self.GetChangeInfo(notification)
	if err != nil {
		return nil, err
	}
	return self.templateNotification(ae, configName, notification, changeInfo)
}

func (self *TaskSuccessToFailureHandler) GetChangeInfo(
	notification *TriggeredTaskNotification) ([]ChangeInfo, error) {
	current := notification.Current
	previous := current
	if notification.Previous != nil {
		previous = notification.Previous
	}

	intermediateTasks, err := current.FindIntermediateTasks(previous)
	if err != nil {
		return nil, err
	}
	allTasks := make([]task.Task, len(intermediateTasks)+1)

	// include the current/previous task
	allTasks[len(allTasks)-1] = *current

	// copy any intermediate task(s)
	if len(intermediateTasks) != 0 {
		copy(allTasks[0:len(allTasks)-1], intermediateTasks)
	}
	return self.constructChangeInfo(allTasks, &notification.Key)
}
