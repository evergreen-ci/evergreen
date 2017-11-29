package notify

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/web"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// "Base class" for all build_*_handler.go structs. Contains code that's common
// to all the build_*_handlers. Note that this struct does NOT implement
// NotificationHandler
type BuildNotificationHandler struct {
	Type string
}

type BuildNotificationForTemplate struct {
	Notification *TriggeredBuildNotification
	FailedTasks  []build.TaskCache
	Subject      string
}

// convenience wrapper about everything we want to know about a build
// notification before it goes off for templating.
type TriggeredBuildNotification struct {
	Current    *build.Build
	Previous   *build.Build
	Info       []ChangeInfo
	Key        NotificationKey
	Preface    string
	Transition string
}

func (self *BuildNotificationHandler) getRecentlyFinishedBuildsWithStatus(key *NotificationKey,
	status string, preface string, transition string) ([]TriggeredBuildNotification, error) {
	buildNotifications := []TriggeredBuildNotification{}
	builds, err := getRecentlyFinishedBuilds(key)
	if err != nil {
		return nil, err
	}

	for _, currentBuild := range builds {
		// Copy by value to make pointer safe
		curr := currentBuild
		if status == "" || curr.Status == status {
			grip.Debug(message.Fields{
				"message":      "adding notification",
				"notification": self.Type,
				"id":           curr.Id,
				"project":      curr.Project,
				"runner":       RunnerName,
				"name":         key.NotificationName,
			})

			// get the build's project to add to the notification subject line
			branchName := UnknownProjectBranch
			if projectRef, err := getProjectRef(curr.Project); err != nil {
				grip.Warning(message.WrapError(err, message.Fields{
					"notification": self.Type,
					"id":           curr.Id,
					"runner":       RunnerName,
					"project":      curr.Project,
					"message":      "unable to find project ref",
				}))
			} else if projectRef != nil {
				branchName = projectRef.Branch
			}

			notification := TriggeredBuildNotification{
				Current:    &curr,
				Previous:   nil,
				Key:        *key,
				Preface:    fmt.Sprintf(preface, branchName),
				Transition: transition,
			}
			buildNotifications = append(buildNotifications, notification)
		}
	}
	return buildNotifications, nil
}

func (self *BuildNotificationHandler) templateNotification(ae *web.App,
	notification *TriggeredBuildNotification, changeInfo []ChangeInfo) (email Email, err error) {

	current := notification.Current

	// add change information to notification
	notification.Info = changeInfo

	// get the failed tasks (if any)
	failedTasks := getFailedTasks(current, notification.Key.NotificationName)

	subject := fmt.Sprintf("%v Build #%v %v on %v", notification.Preface,
		current.BuildNumber, notification.Transition, current.DisplayName)

	buildNotification := BuildNotificationForTemplate{notification, failedTasks, subject}

	body, err := TemplateEmailBody(ae, "build_notification.html", buildNotification)
	if err != nil {
		return
	}

	// for notifications requiring comparisons
	// include what it was compared against here
	if notification.Previous != nil {
		previous := notification.Previous
		body += fmt.Sprintf(`(compared with this <a href="%v/build/%v">previous build</a>)`,
			ae.TemplateFuncs["Global"].(func(string) interface{})("UIRoot"), // FIXME
			previous.Id)
	}

	email = &BuildEmail{EmailBase{body, subject, notification.Info}, *notification}
	return
}

func (self *BuildNotificationHandler) constructChangeInfo(allBuilds []build.Build,
	key *NotificationKey) ([]ChangeInfo, error) {
	changeInfoSlice := make([]ChangeInfo, 0)

	for _, build := range allBuilds {
		// add blamelist information for each build
		v, err := version.FindOne(version.ById(build.Version))
		if err != nil {
			return changeInfoSlice, err
		}

		if v == nil {
			return changeInfoSlice, errors.Errorf(
				"No version found for build %s with version id %s",
				build.Id, build.Version)
		}
		changeInfo := constructChangeInfo(v, key)
		changeInfo.Pushtime = build.PushTime.Format(time.RFC850)
		changeInfoSlice = append(changeInfoSlice, *changeInfo)
	}
	return changeInfoSlice, nil
}
