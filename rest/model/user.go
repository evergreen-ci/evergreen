package model

import (
	"reflect"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/pkg/errors"
)

type APIPubKey struct {
	Name *string `json:"name"`
	Key  *string `json:"key"`
}

// BuildFromService converts from service level structs to an APIPubKey.
func (apiPubKey *APIPubKey) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case user.PubKey:
		apiPubKey.Name = ToStringPtr(v.Name)
		apiPubKey.Key = ToStringPtr(v.Key)
	default:
		return errors.Errorf("incorrect type when fetching converting pubkey type")
	}
	return nil
}

// ToService returns a service layer public key using the data from APIPubKey.
func (apiPubKey *APIPubKey) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not impelemented for APIPubKey")
}

type APIUserSettings struct {
	Timezone         *string                     `json:"timezone"`
	UseSpruceOptions *APIUseSpruceOptions        `json:"use_spruce_options"`
	GithubUser       *APIGithubUser              `json:"github_user"`
	SlackUsername    *string                     `json:"slack_username"`
	Notifications    *APINotificationPreferences `json:"notifications"`
	SpruceFeedback   *APIFeedbackSubmission      `json:"spruce_feedback"`
}

type APIUseSpruceOptions struct {
	PatchPage bool `json:"patch_page,omitempty" bson:"patch_page,omitempty"`
}

func (s *APIUserSettings) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case user.UserSettings:
		s.Timezone = ToStringPtr(v.Timezone)
		s.SlackUsername = ToStringPtr(v.SlackUsername)
		s.UseSpruceOptions = &APIUseSpruceOptions{
			PatchPage: v.UseSpruceOptions.PatchPage,
		}
		s.GithubUser = &APIGithubUser{}
		err := s.GithubUser.BuildFromService(v.GithubUser)
		if err != nil {
			return err
		}
		s.Notifications = &APINotificationPreferences{}
		err = s.Notifications.BuildFromService(v.Notifications)
		if err != nil {
			return err
		}
	default:
		return errors.Errorf("incorrect type for APIUserSettings")
	}
	return nil
}

func (s *APIUserSettings) ToService() (interface{}, error) {
	githubUserInterface, err := s.GithubUser.ToService()
	if err != nil {
		return nil, err
	}
	githubUser, ok := githubUserInterface.(user.GithubUser)
	if !ok {
		return nil, errors.New("unable to convert GithubUser")
	}
	preferencesInterface, err := s.Notifications.ToService()
	if err != nil {
		return nil, err
	}
	preferences, ok := preferencesInterface.(user.NotificationPreferences)
	if !ok {
		return nil, errors.New("unable to convert NotificationPreferences")
	}
	useSpruceOptions := user.UseSpruceOptions{}
	if s.UseSpruceOptions != nil {
		useSpruceOptions.PatchPage = s.UseSpruceOptions.PatchPage
	}
	return user.UserSettings{
		Timezone:         FromStringPtr(s.Timezone),
		SlackUsername:    FromStringPtr(s.SlackUsername),
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
		g.LastKnownAs = ToStringPtr(v.LastKnownAs)
	default:
		return errors.Errorf("incorrect type for APIGithubUser")
	}
	return nil
}

func (g *APIGithubUser) ToService() (interface{}, error) {
	if g == nil {
		return user.GithubUser{}, nil
	}
	return user.GithubUser{
		UID:         g.UID,
		LastKnownAs: FromStringPtr(g.LastKnownAs),
	}, nil
}

type APINotificationPreferences struct {
	BuildBreak            *string `json:"build_break"`
	BuildBreakID          *string `json:"build_break_id,omitempty"`
	PatchFinish           *string `json:"patch_finish"`
	PatchFinishID         *string `json:"patch_finish_id,omitempty"`
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
		n.BuildBreak = ToStringPtr(string(v.BuildBreak))
		n.PatchFinish = ToStringPtr(string(v.PatchFinish))
		n.SpawnHostOutcome = ToStringPtr(string(v.SpawnHostOutcome))
		n.SpawnHostExpiration = ToStringPtr(string(v.SpawnHostExpiration))
		n.CommitQueue = ToStringPtr(string(v.CommitQueue))
		if v.BuildBreakID != "" {
			n.BuildBreakID = ToStringPtr(v.BuildBreakID)
		}
		if v.PatchFinishID != "" {
			n.PatchFinishID = ToStringPtr(v.PatchFinishID)
		}
		if v.SpawnHostOutcomeID != "" {
			n.SpawnHostOutcomeID = ToStringPtr(v.SpawnHostOutcomeID)
		}
		if v.SpawnHostExpirationID != "" {
			n.SpawnHostExpirationID = ToStringPtr(v.SpawnHostExpirationID)
		}
		if v.CommitQueueID != "" {
			n.CommitQueueID = ToStringPtr(v.CommitQueueID)
		}
	default:
		return errors.Errorf("incorrect type for APINotificationPreferences")
	}
	return nil
}

func (n *APINotificationPreferences) ToService() (interface{}, error) {
	if n == nil {
		return user.NotificationPreferences{}, nil
	}
	buildbreak := FromStringPtr(n.BuildBreak)
	patchFinish := FromStringPtr(n.PatchFinish)
	spawnHostExpiration := FromStringPtr(n.SpawnHostExpiration)
	spawnHostOutcome := FromStringPtr(n.SpawnHostOutcome)
	commitQueue := FromStringPtr(n.CommitQueue)
	if !user.IsValidSubscriptionPreference(buildbreak) {
		return nil, errors.New("Build break preference is not a valid type")
	}
	if !user.IsValidSubscriptionPreference(patchFinish) {
		return nil, errors.New("Patch finish preference is not a valid type")
	}
	if !user.IsValidSubscriptionPreference(spawnHostExpiration) {
		return nil, errors.New("Spawn Host Expiration preference is not a valid type")
	}
	if !user.IsValidSubscriptionPreference(spawnHostOutcome) {
		return nil, errors.New("Spawn Host Outcome preference is not a valid type")
	}
	if !user.IsValidSubscriptionPreference(commitQueue) {
		return nil, errors.New("Commit Queue preference is not a valid type")
	}
	preferences := user.NotificationPreferences{
		BuildBreak:          user.UserSubscriptionPreference(buildbreak),
		PatchFinish:         user.UserSubscriptionPreference(patchFinish),
		SpawnHostOutcome:    user.UserSubscriptionPreference(spawnHostOutcome),
		SpawnHostExpiration: user.UserSubscriptionPreference(spawnHostExpiration),
		CommitQueue:         user.UserSubscriptionPreference(commitQueue),
	}
	preferences.BuildBreakID = FromStringPtr(n.BuildBreakID)
	preferences.PatchFinishID = FromStringPtr(n.PatchFinishID)
	preferences.SpawnHostOutcomeID = FromStringPtr(n.PatchFinishID)
	preferences.SpawnHostExpirationID = FromStringPtr(n.SpawnHostExpirationID)
	preferences.CommitQueueID = FromStringPtr(n.CommitQueueID)
	return preferences, nil
}

func ApplyUserChanges(current user.UserSettings, changes APIUserSettings) (APIUserSettings, error) {
	oldSettings := APIUserSettings{}
	if err := oldSettings.BuildFromService(current); err != nil {
		return oldSettings, errors.Wrap(err, "problem applying update to user settings")
	}

	reflectOldSettings := reflect.ValueOf(&oldSettings)
	reflectNewSettings := reflect.ValueOf(changes)
	for i := 0; i < reflectNewSettings.NumField(); i++ {
		propName := reflectNewSettings.Type().Field(i).Name
		changedVal := reflectNewSettings.FieldByName(propName)
		if changedVal.IsNil() {
			continue
		}
		reflectOldSettings.Elem().FieldByName(propName).Set(changedVal)
	}

	return oldSettings, nil
}

type APIUserAuthorInformation struct {
	DisplayName *string
	Email       *string
}

func (u *APIUserAuthorInformation) BuildFromService(h interface{}) error {
	v, ok := h.(*user.DBUser)
	if !ok {
		return errors.Errorf("incorrect type '%T' for user", h)
	}
	if v == nil {
		return errors.New("can't build from nil user")
	}

	u.DisplayName = ToStringPtr(v.DisplayName())
	u.Email = ToStringPtr(v.Email())

	return nil
}

func (u *APIUserAuthorInformation) ToService() (interface{}, error) {
	return nil, errors.New("not implemented for read-only route")
}

type APIFeedbackSubmission struct {
	Type        *string             `json:"type"`
	User        *string             `json:"user"`
	SubmittedAt time.Time           `json:"submitted_at"`
	Questions   []APIQuestionAnswer `json:"questions"`
}

func (a *APIFeedbackSubmission) BuildFromService(h interface{}) error {
	return errors.New("BuildFromService not implemented for APIFeedbackSubmission")
}

func (a *APIFeedbackSubmission) ToService() (interface{}, error) {
	result := model.FeedbackSubmission{
		Type:        FromStringPtr(a.Type),
		User:        FromStringPtr(a.User),
		SubmittedAt: a.SubmittedAt,
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
		ID:     FromStringPtr(a.ID),
		Prompt: FromStringPtr(a.Prompt),
		Answer: FromStringPtr(a.Answer),
	}, nil
}
