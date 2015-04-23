package notify

import (
	"10gen.com/mci"
	"10gen.com/mci/model"
	"10gen.com/mci/web"
	"github.com/10gen-labs/slogger/v1"
)

// Handler for build failure notifications. Implements NotificationHandler from
// notification_handler.go.
type BuildFailureHandler struct {
	BuildNotificationHandler
	Name string
}

func (self *BuildFailureHandler) GetNotifications(ae *web.App, configName string,
	key *NotificationKey) ([]Email, error) {
	var emails []Email
	preface := mciFailurePreface
	if key.NotificationRequester == mci.PatchVersionRequester {
		preface = patchFailurePreface
	}
	triggeredNotifications, err :=
		self.getRecentlyFinishedBuildsWithStatus(key, mci.BuildFailed, preface, failureSubject)

	if err != nil {
		return nil, err
	}

	for _, triggered := range triggeredNotifications {
		email, err := self.TemplateNotification(ae, configName, &triggered)
		if err != nil {
			mci.Logger.Logf(slogger.WARN, "Error templating notification for build `%v`: %v",
				triggered.Current.Id, err)
			continue
		}

		emails = append(emails, email)
	}

	return emails, nil
}

func (self *BuildFailureHandler) TemplateNotification(ae *web.App, _ string,
	notification *TriggeredBuildNotification) (Email, error) {
	changeInfo, err := self.GetChangeInfo(notification)
	if err != nil {
		return nil, err
	}
	return self.templateNotification(ae, notification, changeInfo)
}

func (self *BuildFailureHandler) GetChangeInfo(
	notification *TriggeredBuildNotification) ([]ChangeInfo, error) {
	return self.constructChangeInfo([]model.Build{*notification.Current}, &notification.Key)
}
