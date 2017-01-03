package notify

import (
	"github.com/tychoish/grip/slogger"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/web"
)

// Handler for build completion notifications, i.e. send notifications whenever
// a build finishes. Implements NotificationHandler from
// notification_handler.go.
type BuildCompletionHandler struct {
	BuildNotificationHandler
	Name string
}

func (self *BuildCompletionHandler) GetNotifications(ae *web.App, configName string,
	key *NotificationKey) ([]Email, error) {
	var emails []Email
	preface := mciCompletionPreface
	if key.NotificationRequester == evergreen.PatchVersionRequester {
		preface = patchCompletionPreface
	}
	triggeredNotifications, err :=
		self.getRecentlyFinishedBuildsWithStatus(key, "", preface, completionSubject)
	if err != nil {
		return nil, err
	}

	for _, triggered := range triggeredNotifications {
		email, err := self.TemplateNotification(ae, configName, &triggered)
		if err != nil {
			evergreen.Logger.Logf(slogger.WARN, "Error templating notification for build `%v`: %v",
				triggered.Current.Id, err)
			continue
		}

		emails = append(emails, email)
	}

	return emails, nil
}

func (self *BuildCompletionHandler) TemplateNotification(ae *web.App, _ string,
	notification *TriggeredBuildNotification) (Email, error) {
	changeInfo, err := self.GetChangeInfo(notification)
	if err != nil {
		return nil, err
	}
	return self.templateNotification(ae, notification, changeInfo)
}

func (self *BuildCompletionHandler) GetChangeInfo(
	notification *TriggeredBuildNotification) ([]ChangeInfo, error) {
	return self.constructChangeInfo([]build.Build{*notification.Current}, &notification.Key)
}
