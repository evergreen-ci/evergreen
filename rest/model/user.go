package model

import (
	"context"
	"reflect"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v34/github"
	"github.com/pkg/errors"
)

type APIPubKey struct {
	Name *string `json:"name"`
	Key  *string `json:"key"`
}

// BuildFromService converts from service level structs to an APIPubKey.
func (pk *APIPubKey) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case user.PubKey:
		pk.Name = utility.ToStringPtr(v.Name)
		pk.Key = utility.ToStringPtr(v.Key)
	default:
		return errors.Errorf("programmatic error: expected public key but got type %T", h)
	}
	return nil
}

// ToService returns a service layer public key using the data from APIPubKey.
func (pk *APIPubKey) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not implemented for APIPubKey")
}

type APIUserSettings struct {
	Timezone         *string                     `json:"timezone"`
	Region           *string                     `json:"region"`
	UseSpruceOptions *APIUseSpruceOptions        `json:"use_spruce_options"`
	GithubUser       *APIGithubUser              `json:"github_user"`
	SlackUsername    *string                     `json:"slack_username"`
	Notifications    *APINotificationPreferences `json:"notifications"`
	SpruceFeedback   *APIFeedbackSubmission      `json:"spruce_feedback"`
}

type APIUseSpruceOptions struct {
	HasUsedSpruceBefore          *bool `json:"has_used_spruce_before" bson:"has_used_spruce_before,omitempty"`
	HasUsedMainlineCommitsBefore *bool `json:"has_used_mainline_commits_before" bson:"has_used_mainline_commits_before,omitempty"`
	SpruceV1                     *bool `json:"spruce_v1" bson:"spruce_v1,omitempty"`
}

func (s *APIUserSettings) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case user.UserSettings:
		s.Timezone = utility.ToStringPtr(v.Timezone)
		s.Region = utility.ToStringPtr(v.Region)
		s.SlackUsername = utility.ToStringPtr(v.SlackUsername)
		s.UseSpruceOptions = &APIUseSpruceOptions{
			HasUsedSpruceBefore:          utility.ToBoolPtr(v.UseSpruceOptions.HasUsedSpruceBefore),
			HasUsedMainlineCommitsBefore: utility.ToBoolPtr(v.UseSpruceOptions.HasUsedMainlineCommitsBefore),
			SpruceV1:                     utility.ToBoolPtr(v.UseSpruceOptions.SpruceV1),
		}
		s.GithubUser = &APIGithubUser{}
		err := s.GithubUser.BuildFromService(v.GithubUser)
		if err != nil {
			return errors.Wrap(err, "converting GitHub user settings to API model")
		}
		s.Notifications = &APINotificationPreferences{}
		err = s.Notifications.BuildFromService(v.Notifications)
		if err != nil {
			return errors.Wrap(err, "converting GitHub user settings to API model")
		}
	default:
		return errors.Errorf("programmatic error: expected user settings but got type %T", h)
	}
	return nil
}

func (s *APIUserSettings) ToService() (interface{}, error) {
	githubUserInterface, err := s.GithubUser.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "converting GitHub user settings to service model")
	}
	githubUser, ok := githubUserInterface.(user.GithubUser)
	if !ok {
		return nil, errors.Errorf("programmatic error: expected GitHub user settings but got type %T", githubUserInterface)
	}
	preferencesInterface, err := s.Notifications.ToService()
	if err != nil {
		return nil, err
	}
	preferences, ok := preferencesInterface.(user.NotificationPreferences)
	if !ok {
		return nil, errors.Errorf("programmatic error: expected notification preferences but got type %T", preferencesInterface)
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
		GithubUser:       githubUser,
		Notifications:    preferences,
		UseSpruceOptions: useSpruceOptions,
	}, nil
}

type APIGithubUser struct {
	UID         int     `json:"uid,omitempty"`
	LastKnownAs *string `json:"last_known_as,omitempty"`
}

func (g *APIGithubUser) BuildFromService(h interface{}) error {
	if g == nil {
		return errors.New("APIGithubUser has not been instantiated")
	}
	switch v := h.(type) {
	case user.GithubUser:
		g.UID = v.UID
		g.LastKnownAs = utility.ToStringPtr(v.LastKnownAs)
	default:
		return errors.Errorf("programmatic error: expected GitHub user settings but got type %T", h)
	}
	return nil
}

func (g *APIGithubUser) ToService() (interface{}, error) {
	if g == nil {
		return user.GithubUser{}, nil
	}
	return user.GithubUser{
		UID:         g.UID,
		LastKnownAs: utility.FromStringPtr(g.LastKnownAs),
	}, nil
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

func (n *APINotificationPreferences) BuildFromService(h interface{}) error {
	if n == nil {
		return errors.New("APINotificationPreferences has not been instantiated")
	}
	switch v := h.(type) {
	case user.NotificationPreferences:
		n.BuildBreak = utility.ToStringPtr(string(v.BuildBreak))
		n.PatchFinish = utility.ToStringPtr(string(v.PatchFinish))
		n.PatchFirstFailure = utility.ToStringPtr(string(v.PatchFirstFailure))
		n.SpawnHostOutcome = utility.ToStringPtr(string(v.SpawnHostOutcome))
		n.SpawnHostExpiration = utility.ToStringPtr(string(v.SpawnHostExpiration))
		n.CommitQueue = utility.ToStringPtr(string(v.CommitQueue))
		if v.BuildBreakID != "" {
			n.BuildBreakID = utility.ToStringPtr(v.BuildBreakID)
		}
		if v.PatchFinishID != "" {
			n.PatchFinishID = utility.ToStringPtr(v.PatchFinishID)
		}
		if v.PatchFirstFailureID != "" {
			n.PatchFirstFailureID = utility.ToStringPtr(v.PatchFirstFailureID)
		}
		if v.SpawnHostOutcomeID != "" {
			n.SpawnHostOutcomeID = utility.ToStringPtr(v.SpawnHostOutcomeID)
		}
		if v.SpawnHostExpirationID != "" {
			n.SpawnHostExpirationID = utility.ToStringPtr(v.SpawnHostExpirationID)
		}
		if v.CommitQueueID != "" {
			n.CommitQueueID = utility.ToStringPtr(v.CommitQueueID)
		}
	default:
		return errors.Errorf("programmatic error: expected notification preferences but got type %T", h)
	}
	return nil
}

func (n *APINotificationPreferences) ToService() (interface{}, error) {
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
		return nil, errors.Errorf("invalid build break subscription preference '%s'", buildBreak)
	}
	if !user.IsValidSubscriptionPreference(patchFinish) {
		return nil, errors.Errorf("invalid patch finish subscription preference '%s'", patchFinish)
	}
	if !user.IsValidSubscriptionPreference(patchFirstFailure) {
		return nil, errors.Errorf("invalid patch first failure subscription preference '%s'", patchFirstFailure)
	}
	if !user.IsValidSubscriptionPreference(spawnHostExpiration) {
		return nil, errors.Errorf("invalid spawn host expiration subscription preference '%s'", spawnHostExpiration)
	}
	if !user.IsValidSubscriptionPreference(spawnHostOutcome) {
		return nil, errors.Errorf("invalid spawn host outcome subscription preference '%s'", spawnHostOutcome)
	}
	if !user.IsValidSubscriptionPreference(commitQueue) {
		return nil, errors.Errorf("invalid commit queue subscription preference '%s'", commitQueue)
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

func ApplyUserChanges(current user.UserSettings, changes APIUserSettings) (APIUserSettings, error) {
	oldSettings := APIUserSettings{}
	if err := oldSettings.BuildFromService(current); err != nil {
		return oldSettings, errors.Wrap(err, "converting user settings to API model")
	}

	reflectOldSettings := reflect.ValueOf(&oldSettings).Elem()
	reflectNewSettings := reflect.ValueOf(&changes).Elem()
	util.RecursivelySetUndefinedFields(reflectNewSettings, reflectOldSettings)

	return changes, nil
}

type APIFeedbackSubmission struct {
	Type        *string             `json:"type"`
	User        *string             `json:"user"`
	SubmittedAt *time.Time          `json:"submitted_at"`
	Questions   []APIQuestionAnswer `json:"questions"`
}

func (a *APIFeedbackSubmission) BuildFromService(h interface{}) error {
	return errors.New("BuildFromService not implemented for APIFeedbackSubmission")
}

func (a *APIFeedbackSubmission) ToService() (interface{}, error) {
	submittedAt, err := FromTimePtr(a.SubmittedAt)
	if err != nil {
		return nil, errors.Wrap(err, "parsing 'submitted at' time")
	}
	result := model.FeedbackSubmission{
		Type:        utility.FromStringPtr(a.Type),
		User:        utility.FromStringPtr(a.User),
		SubmittedAt: submittedAt,
	}
	for _, question := range a.Questions {
		answerInterface, _ := question.ToService()
		answer := answerInterface.(model.QuestionAnswer)
		result.Questions = append(result.Questions, answer)
	}
	return result, nil
}

type APIQuestionAnswer struct {
	ID     *string `json:"id"`
	Prompt *string `json:"prompt"`
	Answer *string `json:"answer"`
}

func (a *APIQuestionAnswer) BuildFromService(h interface{}) error {
	return errors.New("BuildFromService not implemented for APIQuestionAnswer")
}

func (a *APIQuestionAnswer) ToService() (interface{}, error) {
	return model.QuestionAnswer{
		ID:     utility.FromStringPtr(a.ID),
		Prompt: utility.FromStringPtr(a.Prompt),
		Answer: utility.FromStringPtr(a.Answer),
	}, nil
}

// UpdateUserSettings Returns an updated version of the user settings struct
func UpdateUserSettings(ctx context.Context, usr *user.DBUser, userSettings APIUserSettings) (*user.UserSettings, error) {
	adminSettings, err := evergreen.GetConfig()
	if err != nil {
		return nil, errors.Wrapf(err, "getting admin settings")
	}
	changedSettings, err := ApplyUserChanges(usr.Settings, userSettings)
	if err != nil {
		return nil, errors.Wrapf(err, "applying user changes")
	}
	userSettingsInterface, err := changedSettings.ToService()
	if err != nil {
		return nil, errors.Wrapf(err, "converting user settings to service model")
	}
	updatedUserSettings, ok := userSettingsInterface.(user.UserSettings)
	if !ok {
		return nil, errors.Errorf("programmatic error: expected user settings but got type %T", userSettingsInterface)
	}

	if len(updatedUserSettings.GithubUser.LastKnownAs) == 0 {
		updatedUserSettings.GithubUser = user.GithubUser{}
	} else if usr.Settings.GithubUser.LastKnownAs != updatedUserSettings.GithubUser.LastKnownAs {
		var token string
		var ghUser *github.User
		token, err = adminSettings.GetGithubOauthToken()
		if err != nil {
			return nil, errors.Wrapf(err, "getting GitHub OAuth token")
		}
		ghUser, err = thirdparty.GetGithubUser(ctx, token, updatedUserSettings.GithubUser.LastKnownAs)
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
