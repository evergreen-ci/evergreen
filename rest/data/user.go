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
	"github.com/pkg/errors"
)

// FindUserById uses the service layer's user type to query the backing database for
// the user with the given Id.
func FindUserById(userId string) (gimlet.User, error) {
	t, err := user.FindOne(user.ById(userId))
	if err != nil {
		return nil, err
	}
	return t, nil
}

func FindUserByGithubName(name string) (gimlet.User, error) {
	return user.FindByGithubName(name)
}

func AddPublicKey(user *user.DBUser, keyName, keyValue string) error {
	return user.AddPublicKey(keyName, keyValue)
}

func DeletePublicKey(user *user.DBUser, keyName string) error {
	return user.DeletePublicKey(keyName)
}

func GetPublicKey(user *user.DBUser, keyName string) (string, error) {
	return user.GetPublicKey(keyName)
}

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
	patchSubscription, err := event.CreateOrUpdateImplicitSubscription(event.ImplicitSubscriptionPatchOutcome,
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
	patchFailureSubscription, err := event.CreateOrUpdateImplicitSubscription(event.ImplicitSubscriptionPatchFirstFailure,
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
	buildBreakSubscription, err := event.CreateOrUpdateImplicitSubscription(event.ImplicitSubscriptionBuildBreak,
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
	spawnhostSubscription, err := event.CreateOrUpdateImplicitSubscription(event.ImplicitSubscriptionSpawnhostExpiration,
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
	spawnHostOutcomeSubscription, err := event.CreateOrUpdateImplicitSubscription(event.ImplicitSubscriptionSpawnHostOutcome,
		dbUser.Settings.Notifications.SpawnHostOutcomeID, spawnHostOutcomeSubscriber, dbUser.Id)
	if err != nil {
		return errors.Wrap(err, "failed to create spawn host outcome subscription")
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
	commitQueueSubscription, err := event.CreateOrUpdateImplicitSubscription(event.ImplicitSubscriptionCommitQueue,
		dbUser.Settings.Notifications.CommitQueueID, commitQueueSubscriber, dbUser.Id)
	if err != nil {
		return errors.Wrap(err, "failed to create commit queue subscription")
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
		return errors.Errorf("unknown type of feedback submission: %T", feedback)
	}

	return errors.Wrap(feedback.Insert(), "error saving feedback")
}

func GetServiceUsers() ([]restModel.APIDBUser, error) {
	users, err := user.FindServiceUsers()
	if err != nil {
		return nil, errors.Wrap(err, "unable to get service users")
	}
	apiUsers := []restModel.APIDBUser{}
	for _, u := range users {
		apiUsers = append(apiUsers, *restModel.APIDBUserBuildFromService(u))
	}
	return apiUsers, nil
}

func AddOrUpdateServiceUser(toUpdate restModel.APIDBUser) error {
	existingUser, err := user.FindOneById(*toUpdate.UserID)
	if err != nil {
		return errors.Wrap(err, "unable to query for user")
	}
	if existingUser != nil && !existingUser.OnlyAPI {
		return errors.New("cannot update an existing non-service user")
	}
	toUpdate.OnlyApi = true
	dbUser := restModel.APIDBUserToService(toUpdate)
	return errors.Wrap(user.AddOrUpdateServiceUser(*dbUser), "unable to update service user")
}

func DeleteServiceUser(id string) error {
	return user.DeleteServiceUser(id)
}
