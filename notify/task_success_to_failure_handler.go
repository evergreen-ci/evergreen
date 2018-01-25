package notify

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/web"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

// Handler for notifications generated specifically when a task fails and the
// previous finished task succeeded. Implements NotificationHandler from
// notification_handler.go.
type TaskSuccessToFailureHandler struct {
	TaskNotificationHandler
	Name string
}

func (self *TaskSuccessToFailureHandler) GetNotifications(ae *web.App, key *NotificationKey) ([]Email, error) {
	var emails []Email
	tasks, err := getRecentlyFinishedTasks(key)
	if err != nil {
		return nil, err
	}

	preface := mciFailurePreface
	if evergreen.IsPatchRequester(key.NotificationRequester) {
		preface = patchFailurePreface
	}

	for _, currentTask := range tasks {
		// Copy by value to make pointer safe
		curr := currentTask

		// get previous task for this project/build variant
		previousTask, err := currentTask.PreviousCompletedTask(key.Project, []string{})
		if previousTask == nil {
			grip.Debug(message.Fields{
				"runner":       RunnerName,
				"notification": key.NotificationName,
				"project":      key.Project,
				"current":      currentTask.Id,
				"message":      "no previous task completed",
			})
			continue
		} else if err != nil {
			return nil, err
		}

		grip.Debug(message.Fields{
			"runner":       RunnerName,
			"notification": key.NotificationName,
			"project":      key.Project,
			"previous":     previousTask.Id,
			"current":      currentTask.Id,
			"message":      "previous completed task found",
		})

		if previousTask.Status == evergreen.TaskSucceeded && currentTask.Status == evergreen.TaskFailed {

			// this is now a potential candidate but we must
			// ensure that no other more recent build has
			// triggered a notification for this event
			history, err := model.FindNotificationRecord(previousTask.Id, key.NotificationName,
				getType(key.NotificationName), key.Project, evergreen.RepotrackerVersionRequester)

			// if there's an error log it and move on
			if err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"runner":       RunnerName,
					"previous":     previousTask.Id,
					"project":      key.Project,
					"notification": key.NotificationName,
					"message":      "error finding notification record",
				}))
				continue
			}

			// get the task's project to add to the notification subject line
			branchName := UnknownProjectBranch

			if projectRef, err := getProjectRef(currentTask.Project); err != nil {
				grip.Warning(message.WrapError(err, (message.Fields{
					"runner":       RunnerName,
					"notification": key.NotificationName,
					"project":      key.Project,
					"current":      currentTask.Id,
					"message":      "previous completed task found",
				})))
			} else if projectRef != nil {
				branchName = projectRef.Branch
			}

			// if no notification for this handler has been registered, register it
			if history == nil {
				grip.Debug(message.Fields{
					"runner":       RunnerName,
					"notification": key.NotificationName,
					"project":      key.Project,
					"current":      currentTask.Id,
					"message":      "adding task to notification",
				})

				notification := TriggeredTaskNotification{
					Current:    &curr,
					Previous:   previousTask,
					Key:        *key,
					Preface:    fmt.Sprintf(preface, branchName),
					Transition: transitionSubject,
				}

				email, err := self.TemplateNotification(ae, &notification)
				if err != nil {
					grip.Error(message.WrapError(err, message.Fields{
						"runner":  RunnerName,
						"message": "error executing template",
						"current": currentTask.Id,
					}))

					continue
				}

				emails = append(emails, email)

				err = model.InsertNotificationRecord(previousTask.Id, currentTask.Id,
					key.NotificationName, getType(key.NotificationName), key.Project,
					evergreen.RepotrackerVersionRequester)
				if err != nil {
					grip.Error(message.WrapError(err, message.Fields{
						"runner":       RunnerName,
						"message":      "error inserting notification record",
						"notification": key.NotificationName,
						"project":      key.Project,
						"current":      currentTask.Id,
					}))

					continue
				}
			} else {
				grip.Debug(message.Fields{
					"runner":       RunnerName,
					"project":      key.Project,
					"notification": key.NotificationName,
					"current":      currentTask.Id,
					"message":      "skipping intermediate trigger",
				})
			}
		}
	}

	return emails, nil
}

func (self *TaskSuccessToFailureHandler) TemplateNotification(ae *web.App, notification *TriggeredTaskNotification) (Email, error) {
	changeInfo, err := self.GetChangeInfo(notification)
	if err != nil {
		return nil, err
	}
	return self.templateNotification(ae, notification, changeInfo)
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
