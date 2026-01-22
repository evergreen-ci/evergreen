package data

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

func UpdateSettings(ctx context.Context, dbUser *user.DBUser, settings user.UserSettings) error {
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

	slackTarget := fmt.Sprintf("@%s", settings.SlackUsername)

	if settings.SlackMemberId != "" {
		slackTarget = settings.SlackMemberId
	}

	var patchSubscriber event.Subscriber
	switch settings.Notifications.PatchFinish {
	case user.PreferenceSlack:
		patchSubscriber = event.NewSlackSubscriber(slackTarget)
	case user.PreferenceEmail:
		patchSubscriber = event.NewEmailSubscriber(dbUser.Email())
	}
	patchSubscription, err := event.CreateOrUpdateGeneralSubscription(ctx, event.GeneralSubscriptionPatchOutcome,
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
		patchFailureSubscriber = event.NewSlackSubscriber(slackTarget)
	case user.PreferenceEmail:
		patchFailureSubscriber = event.NewEmailSubscriber(dbUser.Email())
	}
	patchFailureSubscription, err := event.CreateOrUpdateGeneralSubscription(ctx, event.GeneralSubscriptionPatchFirstFailure,
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
		buildBreakSubscriber = event.NewSlackSubscriber(slackTarget)
	case user.PreferenceEmail:
		buildBreakSubscriber = event.NewEmailSubscriber(dbUser.Email())
	}
	buildBreakSubscription, err := event.CreateOrUpdateGeneralSubscription(ctx, event.GeneralSubscriptionBuildBreak,
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
		spawnhostSubscriber = event.NewSlackSubscriber(slackTarget)
	case user.PreferenceEmail:
		spawnhostSubscriber = event.NewEmailSubscriber(dbUser.Email())
	}
	spawnhostSubscription, err := event.CreateOrUpdateGeneralSubscription(ctx, event.GeneralSubscriptionSpawnhostExpiration,
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
		spawnHostOutcomeSubscriber = event.NewSlackSubscriber(slackTarget)
	case user.PreferenceEmail:
		spawnHostOutcomeSubscriber = event.NewEmailSubscriber(dbUser.Email())
	}
	spawnHostOutcomeSubscription, err := event.CreateOrUpdateGeneralSubscription(ctx, event.GeneralSubscriptionSpawnHostOutcome,
		dbUser.Settings.Notifications.SpawnHostOutcomeID, spawnHostOutcomeSubscriber, dbUser.Id)
	if err != nil {
		return errors.Wrap(err, "creating spawn host outcome subscription")
	}
	if spawnHostOutcomeSubscription != nil {
		settings.Notifications.SpawnHostOutcomeID = spawnHostOutcomeSubscription.ID
	} else {
		settings.Notifications.SpawnHostOutcomeID = ""
	}

	return dbUser.UpdateSettings(ctx, settings)
}

func SubmitFeedback(ctx context.Context, in restModel.APIFeedbackSubmission) error {
	f, _ := in.ToService()
	return errors.Wrap(f.Insert(ctx), "error saving feedback")
}

func GetServiceUsers(ctx context.Context) ([]restModel.APIDBUser, error) {
	users, err := user.FindServiceUsers(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "finding service users")
	}
	apiUsers := []restModel.APIDBUser{}
	for _, u := range users {
		apiUser := restModel.APIDBUser{}
		apiUser.BuildFromService(u)
		apiUsers = append(apiUsers, apiUser)
	}
	return apiUsers, nil
}

func AddOrUpdateServiceUser(ctx context.Context, toUpdate restModel.APIDBUser) error {
	userID := utility.FromStringPtr(toUpdate.UserID)
	existingUser, err := user.FindOneById(ctx, userID)
	if err != nil {
		return errors.Wrapf(err, "finding user '%s'", userID)
	}
	if existingUser != nil && !existingUser.OnlyAPI {
		return errors.New("cannot update an existing non-service user")
	}
	toUpdate.OnlyApi = true

	dbUser, err := toUpdate.ToService()
	if err != nil {
		return errors.Wrapf(err, "converting service user '%s' from API model to service", userID)
	}
	if dbUser == nil {
		return errors.Wrapf(err, "cannot perform add or update with nil user")
	}
	return errors.Wrap(user.AddOrUpdateServiceUser(ctx, *dbUser), "updating service user")
}
