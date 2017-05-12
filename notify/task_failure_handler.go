package notify

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/web"
	"github.com/mongodb/grip"
)

// Handler for task failure notifications. Implements NotificationHandler from
// notification_handler.go.
type TaskFailureHandler struct {
	TaskNotificationHandler
	Name string
}

func (self *TaskFailureHandler) GetNotifications(ae *web.App, key *NotificationKey) ([]Email, error) {
	var emails []Email
	preface := mciFailurePreface
	if key.NotificationRequester == evergreen.PatchVersionRequester {
		preface = patchFailurePreface
	}
	triggeredNotifications, err := self.getRecentlyFinishedTasksWithStatus(key,
		evergreen.TaskFailed, preface, failureSubject)
	if err != nil {
		return nil, err
	}

	for _, triggered := range triggeredNotifications {
		email, err := self.TemplateNotification(ae, &triggered)
		if err != nil {
			grip.Warningf("tempalte error with task failure notification for '%s': %s",
				triggered.Current.Id, err.Error())
			continue
		}

		emails = append(emails, email)
	}

	return emails, nil
}

func (self *TaskFailureHandler) TemplateNotification(ae *web.App, notification *TriggeredTaskNotification) (Email, error) {
	changeInfo, err := self.GetChangeInfo(notification)
	if err != nil {
		return nil, err
	}
	return self.templateNotification(ae, notification, changeInfo)
}

func (self *TaskFailureHandler) GetChangeInfo(
	notification *TriggeredTaskNotification) ([]ChangeInfo, error) {
	return self.constructChangeInfo([]task.Task{*notification.Current}, &notification.Key)
}
