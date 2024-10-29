package model

import (
	"context"
	"reflect"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/parsley"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v52/github"
	"github.com/pkg/errors"
)

type APIDBUser struct {
	BetaFeatures APIBetaFeatures `json:"beta_features"`
	DisplayName  *string         `json:"display_name"`
	EmailAddress *string         `json:"email_address"`
	// will be set to true if the user represents a service user
	OnlyApi         bool               `json:"only_api"`
	Roles           []string           `json:"roles"`
	ParsleyFilters  []APIParsleyFilter `json:"parsley_filters"`
	ParsleySettings APIParsleySettings `json:"parsley_settings"`
	Settings        APIUserSettings    `json:"settings"`
	UserID          *string            `json:"user_id"`
}

// BuildFromService converts a service layer user.DBUser to an APIDBUser.
func (s *APIDBUser) BuildFromService(usr user.DBUser) {
	s.DisplayName = utility.ToStringPtr(usr.DispName)
	s.UserID = utility.ToStringPtr(usr.Id)
	s.EmailAddress = utility.ToStringPtr(usr.EmailAddress)
	s.Roles = usr.SystemRoles
	s.OnlyApi = usr.OnlyAPI

	userSettings := APIUserSettings{}
	userSettings.BuildFromService(usr.Settings)
	s.Settings = userSettings

	betaFeatures := APIBetaFeatures{}
	betaFeatures.BuildFromService(usr.BetaFeatures)
	s.BetaFeatures = betaFeatures

	res := []APIParsleyFilter{}
	for _, p := range usr.ParsleyFilters {
		parsleyFilter := APIParsleyFilter{}
		parsleyFilter.BuildFromService(p)
		res = append(res, parsleyFilter)
	}
	s.ParsleyFilters = res

	parsleySettings := APIParsleySettings{}
	parsleySettings.BuildFromService(usr.ParsleySettings)
	s.ParsleySettings = parsleySettings
}

// ToService returns a service layer user.DBUser using the data from APIDBUser.
func (s *APIDBUser) ToService() (*user.DBUser, error) {
	out := &user.DBUser{}
	out.DispName = utility.FromStringPtr(s.DisplayName)
	out.Id = utility.FromStringPtr(s.UserID)
	out.EmailAddress = utility.FromStringPtr(s.EmailAddress)
	out.SystemRoles = s.Roles
	out.OnlyAPI = s.OnlyApi
	out.ParsleySettings = s.ParsleySettings.ToService()
	out.BetaFeatures = s.BetaFeatures.ToService()

	if s.ParsleyFilters != nil {
		filters := []parsley.Filter{}
		for _, f := range s.ParsleyFilters {
			filters = append(filters, f.ToService())
		}
		out.ParsleyFilters = filters
	}

	userSettings, err := s.Settings.ToService()
	if err != nil {
		return nil, errors.Wrapf(err, "converting user settings to service for user '%s'", out.Id)
	}
	out.Settings = userSettings

	return out, nil
}

type APIPubKey struct {
	Name *string `json:"name"`
	Key  *string `json:"key"`
}

// BuildFromService converts from service level structs to an APIPubKey.
func (pk *APIPubKey) BuildFromService(in user.PubKey) {
	pk.Name = utility.ToStringPtr(in.Name)
	pk.Key = utility.ToStringPtr(in.Key)
}

type APIBetaFeatures struct {
	SpruceWaterfallEnabled bool `json:"spruce_waterfall_enabled"`
}

func (b *APIBetaFeatures) BuildFromService(usr user.BetaFeatures) {
	b.SpruceWaterfallEnabled = usr.SpruceWaterfallEnabled
}

func (b *APIBetaFeatures) ToService() user.BetaFeatures {
	return user.BetaFeatures{
		SpruceWaterfallEnabled: b.SpruceWaterfallEnabled,
	}
}

type APIUserSettings struct {
	Timezone         *string                     `json:"timezone"`
	Region           *string                     `json:"region"`
	UseSpruceOptions *APIUseSpruceOptions        `json:"use_spruce_options"`
	GithubUser       *APIGithubUser              `json:"github_user"`
	SlackUsername    *string                     `json:"slack_username"`
	SlackMemberId    *string                     `json:"slack_member_id"`
	Notifications    *APINotificationPreferences `json:"notifications"`
	SpruceFeedback   *APIFeedbackSubmission      `json:"spruce_feedback"`
	DateFormat       *string                     `json:"date_format"`
	TimeFormat       *string                     `json:"time_format"`
}

type APIUseSpruceOptions struct {
	HasUsedSpruceBefore          *bool `json:"has_used_spruce_before" bson:"has_used_spruce_before,omitempty"`
	HasUsedMainlineCommitsBefore *bool `json:"has_used_mainline_commits_before" bson:"has_used_mainline_commits_before,omitempty"`
	SpruceV1                     *bool `json:"spruce_v1" bson:"spruce_v1,omitempty"`
}

func (s *APIUserSettings) BuildFromService(settings user.UserSettings) {
	s.Timezone = utility.ToStringPtr(settings.Timezone)
	s.Region = utility.ToStringPtr(settings.Region)
	s.SlackUsername = utility.ToStringPtr(settings.SlackUsername)
	s.SlackMemberId = utility.ToStringPtr(settings.SlackMemberId)
	s.UseSpruceOptions = &APIUseSpruceOptions{
		HasUsedSpruceBefore:          utility.ToBoolPtr(settings.UseSpruceOptions.HasUsedSpruceBefore),
		HasUsedMainlineCommitsBefore: utility.ToBoolPtr(settings.UseSpruceOptions.HasUsedMainlineCommitsBefore),
		SpruceV1:                     utility.ToBoolPtr(settings.UseSpruceOptions.SpruceV1),
	}
	s.GithubUser = &APIGithubUser{}
	s.GithubUser.BuildFromService(settings.GithubUser)
	s.Notifications = &APINotificationPreferences{}
	s.Notifications.BuildFromService(settings.Notifications)
	s.DateFormat = utility.ToStringPtr(settings.DateFormat)
	s.TimeFormat = utility.ToStringPtr(settings.TimeFormat)
}

func (s *APIUserSettings) ToService() (user.UserSettings, error) {
	githubUser := user.GithubUser{}
	if s.GithubUser != nil {
		githubUser = s.GithubUser.ToService()
	}

	preferences, err := s.Notifications.ToService()
	if err != nil {
		return user.UserSettings{}, err
	}

	useSpruceOptions := user.UseSpruceOptions{}
	if s.UseSpruceOptions != nil {
		useSpruceOptions.HasUsedSpruceBefore = utility.FromBoolPtr(s.UseSpruceOptions.HasUsedSpruceBefore)
		useSpruceOptions.SpruceV1 = utility.FromBoolPtr(s.UseSpruceOptions.SpruceV1)
		useSpruceOptions.HasUsedMainlineCommitsBefore = utility.FromBoolPtr(s.UseSpruceOptions.HasUsedMainlineCommitsBefore)
	}
	return user.UserSettings{
		Timezone:         utility.FromStringPtr(s.Timezone),
		Region:           utility.FromStringPtr(s.Region),
		SlackUsername:    utility.FromStringPtr(s.SlackUsername),
		SlackMemberId:    utility.FromStringPtr(s.SlackMemberId),
		GithubUser:       githubUser,
		Notifications:    preferences,
		UseSpruceOptions: useSpruceOptions,
		DateFormat:       utility.FromStringPtr(s.DateFormat),
		TimeFormat:       utility.FromStringPtr(s.TimeFormat),
	}, nil
}

type APIGithubUser struct {
	UID         int     `json:"uid,omitempty"`
	LastKnownAs *string `json:"last_known_as,omitempty"`
}

func (g *APIGithubUser) BuildFromService(usr user.GithubUser) {
	g.UID = usr.UID
	g.LastKnownAs = utility.ToStringPtr(usr.LastKnownAs)
}

func (g *APIGithubUser) ToService() user.GithubUser {
	return user.GithubUser{
		UID:         g.UID,
		LastKnownAs: utility.FromStringPtr(g.LastKnownAs),
	}
}

type APINotificationPreferences struct {
	BuildBreak            *string `json:"build_break"`
	BuildBreakID          *string `json:"build_break_id,omitempty"`
	PatchFinish           *string `json:"patch_finish"`
	PatchFinishID         *string `json:"patch_finish_id,omitempty"`
	PatchFirstFailure     *string `json:"patch_first_failure"`
	PatchFirstFailureID   *string `json:"patch_first_failure_id,omitempty"`
	SpawnHostExpiration   *string `json:"spawn_host_expiration"`
	SpawnHostExpirationID *string `json:"spawn_host_expiration_id,omitempty"`
	SpawnHostOutcome      *string `json:"spawn_host_outcome"`
	SpawnHostOutcomeID    *string `json:"spawn_host_outcome_id,omitempty"`
	CommitQueue           *string `json:"commit_queue"`
	CommitQueueID         *string `json:"commit_queue_id,omitempty"`
}

func (n *APINotificationPreferences) BuildFromService(in user.NotificationPreferences) {
	n.BuildBreak = utility.ToStringPtr(string(in.BuildBreak))
	n.PatchFinish = utility.ToStringPtr(string(in.PatchFinish))
	n.PatchFirstFailure = utility.ToStringPtr(string(in.PatchFirstFailure))
	n.SpawnHostOutcome = utility.ToStringPtr(string(in.SpawnHostOutcome))
	n.SpawnHostExpiration = utility.ToStringPtr(string(in.SpawnHostExpiration))
	n.CommitQueue = utility.ToStringPtr(string(in.CommitQueue))
	if in.BuildBreakID != "" {
		n.BuildBreakID = utility.ToStringPtr(in.BuildBreakID)
	}
	if in.PatchFinishID != "" {
		n.PatchFinishID = utility.ToStringPtr(in.PatchFinishID)
	}
	if in.PatchFirstFailureID != "" {
		n.PatchFirstFailureID = utility.ToStringPtr(in.PatchFirstFailureID)
	}
	if in.SpawnHostOutcomeID != "" {
		n.SpawnHostOutcomeID = utility.ToStringPtr(in.SpawnHostOutcomeID)
	}
	if in.SpawnHostExpirationID != "" {
		n.SpawnHostExpirationID = utility.ToStringPtr(in.SpawnHostExpirationID)
	}
	if in.CommitQueueID != "" {
		n.CommitQueueID = utility.ToStringPtr(in.CommitQueueID)
	}
}

func (n *APINotificationPreferences) ToService() (user.NotificationPreferences, error) {
	if n == nil {
		return user.NotificationPreferences{}, nil
	}
	buildBreak := utility.FromStringPtr(n.BuildBreak)
	patchFinish := utility.FromStringPtr(n.PatchFinish)
	patchFirstFailure := utility.FromStringPtr(n.PatchFirstFailure)
	spawnHostExpiration := utility.FromStringPtr(n.SpawnHostExpiration)
	spawnHostOutcome := utility.FromStringPtr(n.SpawnHostOutcome)
	commitQueue := utility.FromStringPtr(n.CommitQueue)
	if !user.IsValidSubscriptionPreference(buildBreak) {
		return user.NotificationPreferences{}, errors.Errorf("invalid build break subscription preference '%s'", buildBreak)
	}
	if !user.IsValidSubscriptionPreference(patchFinish) {
		return user.NotificationPreferences{}, errors.Errorf("invalid patch finish subscription preference '%s'", patchFinish)
	}
	if !user.IsValidSubscriptionPreference(patchFirstFailure) {
		return user.NotificationPreferences{}, errors.Errorf("invalid patch first failure subscription preference '%s'", patchFirstFailure)
	}
	if !user.IsValidSubscriptionPreference(spawnHostExpiration) {
		return user.NotificationPreferences{}, errors.Errorf("invalid spawn host expiration subscription preference '%s'", spawnHostExpiration)
	}
	if !user.IsValidSubscriptionPreference(spawnHostOutcome) {
		return user.NotificationPreferences{}, errors.Errorf("invalid spawn host outcome subscription preference '%s'", spawnHostOutcome)
	}
	if !user.IsValidSubscriptionPreference(commitQueue) {
		return user.NotificationPreferences{}, errors.Errorf("invalid commit queue subscription preference '%s'", commitQueue)
	}
	preferences := user.NotificationPreferences{
		BuildBreak:          user.UserSubscriptionPreference(buildBreak),
		PatchFinish:         user.UserSubscriptionPreference(patchFinish),
		PatchFirstFailure:   user.UserSubscriptionPreference(patchFirstFailure),
		SpawnHostOutcome:    user.UserSubscriptionPreference(spawnHostOutcome),
		SpawnHostExpiration: user.UserSubscriptionPreference(spawnHostExpiration),
		CommitQueue:         user.UserSubscriptionPreference(commitQueue),
	}
	preferences.BuildBreakID = utility.FromStringPtr(n.BuildBreakID)
	preferences.PatchFinishID = utility.FromStringPtr(n.PatchFinishID)
	preferences.PatchFirstFailureID = utility.FromStringPtr(n.PatchFirstFailureID)
	preferences.SpawnHostOutcomeID = utility.FromStringPtr(n.SpawnHostOutcomeID)
	preferences.SpawnHostExpirationID = utility.FromStringPtr(n.SpawnHostExpirationID)
	preferences.CommitQueueID = utility.FromStringPtr(n.CommitQueueID)
	return preferences, nil
}

func applyUserChanges(current user.UserSettings, changes APIUserSettings) APIUserSettings {
	oldSettings := APIUserSettings{}
	oldSettings.BuildFromService(current)

	reflectOldSettings := reflect.ValueOf(&oldSettings).Elem()
	reflectNewSettings := reflect.ValueOf(&changes).Elem()
	util.RecursivelySetUndefinedFields(reflectNewSettings, reflectOldSettings)

	return changes
}

type APIFeedbackSubmission struct {
	Type        *string             `json:"type"`
	User        *string             `json:"user"`
	SubmittedAt *time.Time          `json:"submitted_at"`
	Questions   []APIQuestionAnswer `json:"questions"`
}

func (a *APIFeedbackSubmission) ToService() (model.FeedbackSubmission, error) {
	submittedAt, err := FromTimePtr(a.SubmittedAt)
	if err != nil {
		return model.FeedbackSubmission{}, errors.Wrap(err, "parsing 'submitted at' time")
	}
	result := model.FeedbackSubmission{
		Type:        utility.FromStringPtr(a.Type),
		User:        utility.FromStringPtr(a.User),
		SubmittedAt: submittedAt,
	}
	for _, question := range a.Questions {
		result.Questions = append(result.Questions, question.ToService())
	}
	return result, nil
}

type APIQuestionAnswer struct {
	ID     *string `json:"id"`
	Prompt *string `json:"prompt"`
	Answer *string `json:"answer"`
}

func (a *APIQuestionAnswer) ToService() model.QuestionAnswer {
	return model.QuestionAnswer{
		ID:     utility.FromStringPtr(a.ID),
		Prompt: utility.FromStringPtr(a.Prompt),
		Answer: utility.FromStringPtr(a.Answer),
	}
}

// UpdateUserSettings Returns an updated version of the user settings struct
func UpdateUserSettings(ctx context.Context, usr *user.DBUser, userSettings APIUserSettings) (*user.UserSettings, error) {
	changedSettings := applyUserChanges(usr.Settings, userSettings)
	updatedUserSettings, err := changedSettings.ToService()
	if err != nil {
		return nil, errors.Wrapf(err, "converting user settings to service model")
	}

	if len(updatedUserSettings.GithubUser.LastKnownAs) == 0 {
		updatedUserSettings.GithubUser = user.GithubUser{}
	} else if usr.Settings.GithubUser.LastKnownAs != updatedUserSettings.GithubUser.LastKnownAs {
		var ghUser *github.User
		ghUser, err = thirdparty.GetGithubUser(ctx, updatedUserSettings.GithubUser.LastKnownAs)
		if err != nil {
			return nil, errors.Wrapf(err, "getting GitHub user")
		}
		updatedUserSettings.GithubUser.LastKnownAs = *ghUser.Login
		updatedUserSettings.GithubUser.UID = int(*ghUser.ID)
	} else {
		updatedUserSettings.GithubUser.UID = usr.Settings.GithubUser.UID
	}

	return &updatedUserSettings, nil
}
