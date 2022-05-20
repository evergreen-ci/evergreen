package data

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

func UpdateSettings(dbUser *user.DBUser, settings user.UserSettings) error {
	if strings.HasPrefix(settings.SlackUsername, "#") {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "expected a Slack username, but got a channel",
		}
	}
	settings.SlackUsername = strings.TrimPrefix(settings.SlackUsername, "@")
	settings.Notifications.PatchFinishID = dbUser.Settings.Notifications.PatchFinishID
	settings.Notifications.SpawnHostOutcomeID = dbUser.Settings.Notifications.SpawnHostOutcomeID
	settings.Notifications.SpawnHostExpirationID = dbUser.Settings.Notifications.SpawnHostExpirationID
	settings.Notifications.CommitQueueID = dbUser.Settings.Notifications.CommitQueueID

	var patchSubscriber event.Subscriber
	switch settings.Notifications.PatchFinish {
	case user.PreferenceSlack:
		patchSubscriber = event.NewSlackSubscriber(fmt.Sprintf("@%s", settings.SlackUsername))
	case user.PreferenceEmail:
		patchSubscriber = event.NewEmailSubscriber(dbUser.Email())
	}
	patchSubscription, err := event.CreateOrUpdateGeneralSubscription(event.GeneralSubscriptionPatchOutcome,
		dbUser.Settings.Notifications.PatchFinishID, patchSubscriber, dbUser.Id)
	if err != nil {
		return err
	}
	if patchSubscription != nil {
		settings.Notifications.PatchFinishID = patchSubscription.ID
	} else {
		settings.Notifications.PatchFinishID = ""
	}

	var patchFailureSubscriber event.Subscriber
	switch settings.Notifications.PatchFirstFailure {
	case user.PreferenceSlack:
		patchFailureSubscriber = event.NewSlackSubscriber(fmt.Sprintf("@%s", settings.SlackUsername))
	case user.PreferenceEmail:
		patchFailureSubscriber = event.NewEmailSubscriber(dbUser.Email())
	}
	patchFailureSubscription, err := event.CreateOrUpdateGeneralSubscription(event.GeneralSubscriptionPatchFirstFailure,
		dbUser.Settings.Notifications.PatchFirstFailureID, patchFailureSubscriber, dbUser.Id)
	if err != nil {
		return err
	}
	if patchFailureSubscription != nil {
		settings.Notifications.PatchFirstFailureID = patchFailureSubscription.ID
	} else {
		settings.Notifications.PatchFirstFailureID = ""
	}

	var buildBreakSubscriber event.Subscriber
	switch settings.Notifications.BuildBreak {
	case user.PreferenceSlack:
		buildBreakSubscriber = event.NewSlackSubscriber(fmt.Sprintf("@%s", settings.SlackUsername))
	case user.PreferenceEmail:
		buildBreakSubscriber = event.NewEmailSubscriber(dbUser.Email())
	}
	buildBreakSubscription, err := event.CreateOrUpdateGeneralSubscription(event.GeneralSubscriptionBuildBreak,
		dbUser.Settings.Notifications.BuildBreakID, buildBreakSubscriber, dbUser.Id)
	if err != nil {
		return err
	}
	if buildBreakSubscription != nil {
		settings.Notifications.BuildBreakID = buildBreakSubscription.ID
	} else {
		settings.Notifications.BuildBreakID = ""
	}

	var spawnhostSubscriber event.Subscriber
	switch settings.Notifications.SpawnHostExpiration {
	case user.PreferenceSlack:
		spawnhostSubscriber = event.NewSlackSubscriber(fmt.Sprintf("@%s", settings.SlackUsername))
	case user.PreferenceEmail:
		spawnhostSubscriber = event.NewEmailSubscriber(dbUser.Email())
	}
	spawnhostSubscription, err := event.CreateOrUpdateGeneralSubscription(event.GeneralSubscriptionSpawnhostExpiration,
		dbUser.Settings.Notifications.SpawnHostExpirationID, spawnhostSubscriber, dbUser.Id)
	if err != nil {
		return err
	}
	if spawnhostSubscription != nil {
		settings.Notifications.SpawnHostExpirationID = spawnhostSubscription.ID
	} else {
		settings.Notifications.SpawnHostExpirationID = ""
	}

	var spawnHostOutcomeSubscriber event.Subscriber
	switch settings.Notifications.SpawnHostOutcome {
	case user.PreferenceSlack:
		spawnHostOutcomeSubscriber = event.NewSlackSubscriber(fmt.Sprintf("@%s", settings.SlackUsername))
	case user.PreferenceEmail:
		spawnHostOutcomeSubscriber = event.NewEmailSubscriber(dbUser.Email())
	}
	spawnHostOutcomeSubscription, err := event.CreateOrUpdateGeneralSubscription(event.GeneralSubscriptionSpawnHostOutcome,
		dbUser.Settings.Notifications.SpawnHostOutcomeID, spawnHostOutcomeSubscriber, dbUser.Id)
	if err != nil {
		return errors.Wrap(err, "creating spawn host outcome subscription")
	}
	if spawnHostOutcomeSubscription != nil {
		settings.Notifications.SpawnHostOutcomeID = spawnHostOutcomeSubscription.ID
	} else {
		settings.Notifications.SpawnHostOutcomeID = ""
	}

	var commitQueueSubscriber event.Subscriber
	switch settings.Notifications.CommitQueue {
	case user.PreferenceSlack:
		commitQueueSubscriber = event.NewSlackSubscriber(fmt.Sprintf("@%s", settings.SlackUsername))
	case user.PreferenceEmail:
		commitQueueSubscriber = event.NewEmailSubscriber(dbUser.Email())
	}
	commitQueueSubscription, err := event.CreateOrUpdateGeneralSubscription(event.GeneralSubscriptionCommitQueue,
		dbUser.Settings.Notifications.CommitQueueID, commitQueueSubscriber, dbUser.Id)
	if err != nil {
		return errors.Wrap(err, "creating commit queue subscription")
	}
	if commitQueueSubscription != nil {
		settings.Notifications.CommitQueueID = commitQueueSubscription.ID
	} else {
		settings.Notifications.CommitQueueID = ""
	}

	return dbUser.UpdateSettings(settings)
}

func SubmitFeedback(in restModel.APIFeedbackSubmission) error {
	f, _ := in.ToService()
	feedback, isValid := f.(model.FeedbackSubmission)
	if !isValid {
		return errors.Errorf("unknown feedback submission type %T", feedback)
	}

	return errors.Wrap(feedback.Insert(), "error saving feedback")
}

func GetServiceUsers() ([]restModel.APIDBUser, error) {
	users, err := user.FindServiceUsers()
	if err != nil {
		return nil, errors.Wrap(err, "finding service users")
	}
	apiUsers := []restModel.APIDBUser{}
	for _, u := range users {
		apiUsers = append(apiUsers, *restModel.APIDBUserBuildFromService(u))
	}
	return apiUsers, nil
}

func AddOrUpdateServiceUser(toUpdate restModel.APIDBUser) error {
	userID := utility.FromStringPtr(toUpdate.UserID)
	existingUser, err := user.FindOneById(userID)
	if err != nil {
		return errors.Wrapf(err, "finding user '%s'", userID)
	}
	if existingUser != nil && !existingUser.OnlyAPI {
		return errors.New("cannot update an existing non-service user")
	}
	toUpdate.OnlyApi = true
	dbUser := restModel.APIDBUserToService(toUpdate)
	return errors.Wrap(user.AddOrUpdateServiceUser(*dbUser), "updating service user")
}
