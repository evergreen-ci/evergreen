package notify

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/web"
	"github.com/mongodb/grip"
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
	if key.NotificationRequester == evergreen.PatchVersionRequester {
		preface = patchFailurePreface
	}

	triggeredNotifications, err := self.getRecentlyFinishedBuildsWithStatus(key,
		evergreen.BuildFailed, preface, failureSubject)

	if err != nil {
		return nil, err
	}

	for _, triggered := range triggeredNotifications {
		email, err := self.TemplateNotification(ae, configName, &triggered)
		if err != nil {
			grip.Warningf("Error templating notification for build '%s': %+v",
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
	return self.constructChangeInfo([]build.Build{*notification.Current}, &notification.Key)
}
