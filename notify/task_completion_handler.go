package notify

import (
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/web"
)

// Handler for task completion notifications, i.e. send notifications whenever
// a task finishes. Implements NotificationHandler from notification_handler.go.
type TaskCompletionHandler struct {
	TaskNotificationHandler
	Name string
}

func (self *TaskCompletionHandler) GetNotifications(ae *web.App, configName string,
	key *NotificationKey) ([]Email, error) {
	var emails []Email
	preface := mciCompletionPreface
	if key.NotificationRequester == evergreen.PatchVersionRequester {
		preface = patchCompletionPreface
	}
	triggeredNotifications, err :=
		self.getRecentlyFinishedTasksWithStatus(key, "", preface, completionSubject)

	if err != nil {
		return nil, err
	}

	for _, triggered := range triggeredNotifications {
		email, err := self.TemplateNotification(ae, configName, &triggered)
		if err != nil {
			evergreen.Logger.Logf(slogger.WARN, "Error templating notification for task `%v`: %v",
				triggered.Current.Id, err)
			continue
		}

		emails = append(emails, email)
	}

	return emails, nil
}

func (self *TaskCompletionHandler) TemplateNotification(ae *web.App, configName string,
	notification *TriggeredTaskNotification) (Email, error) {
	changeInfo, err := self.GetChangeInfo(notification)
	if err != nil {
		return nil, err
	}
	return self.templateNotification(ae, configName, notification, changeInfo)
}

func (self *TaskCompletionHandler) GetChangeInfo(
	notification *TriggeredTaskNotification) ([]ChangeInfo, error) {
	return self.constructChangeInfo([]model.Task{*notification.Current}, &notification.Key)
}
