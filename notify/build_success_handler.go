package notify

import (
	"10gen.com/mci"
	"10gen.com/mci/model"
	"10gen.com/mci/web"
	"github.com/10gen-labs/slogger/v1"
)

// Handler for build success notifications. Implements NotificationHandler from
// notification_handler.go.
type BuildSuccessHandler struct {
	BuildNotificationHandler
	Name string
}

func (self *BuildSuccessHandler) GetNotifications(ae *web.App, configName string,
	key *NotificationKey) ([]Email, error) {
	var emails []Email
	preface := mciSuccessPreface
	if key.NotificationRequester == mci.PatchVersionRequester {
		preface = patchSuccessPreface
	}
	triggeredNotifications, err :=
		self.getRecentlyFinishedBuildsWithStatus(key, mci.BuildSucceeded, preface, successSubject)

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

func (self *BuildSuccessHandler) TemplateNotification(ae *web.App, _ string,
	notification *TriggeredBuildNotification) (Email, error) {
	changeInfo, err := self.GetChangeInfo(notification)
	if err != nil {
		return nil, err
	}
	return self.templateNotification(ae, notification, changeInfo)
}

func (self *BuildSuccessHandler) GetChangeInfo(
	notification *TriggeredBuildNotification) ([]ChangeInfo, error) {
	return self.constructChangeInfo([]model.Build{*notification.Current}, &notification.Key)
}
