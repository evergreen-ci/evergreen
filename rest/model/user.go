package model

import (
	"reflect"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/pkg/errors"
)

type APIPubKey struct {
	Name APIString `json:"name"`
	Key  APIString `json:"key"`
}

// BuildFromService converts from service level structs to an APIPubKey.
func (apiPubKey *APIPubKey) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case user.PubKey:
		apiPubKey.Name = ToAPIString(v.Name)
		apiPubKey.Key = ToAPIString(v.Key)
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
	Timezone         APIString                   `json:"timezone"`
	UseSpruceOptions *APIUseSpruceOptions        `json:"use_spruce_options"`
	GithubUser       *APIGithubUser              `json:"github_user"`
	SlackUsername    APIString                   `json:"slack_username"`
	Notifications    *APINotificationPreferences `json:"notifications"`
	SpruceFeedback   *APIFeedbackSubmission      `json:"spruce_feedback"`
}

type APIUseSpruceOptions struct {
	PatchPage bool `json:"patch_page,omitempty" bson:"patch_page,omitempty"`
}

func (s *APIUserSettings) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case user.UserSettings:
		s.Timezone = ToAPIString(v.Timezone)
		s.SlackUsername = ToAPIString(v.SlackUsername)
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
		Timezone:         FromAPIString(s.Timezone),
		SlackUsername:    FromAPIString(s.SlackUsername),
		GithubUser:       githubUser,
		Notifications:    preferences,
		UseSpruceOptions: useSpruceOptions,
	}, nil
}

type APIGithubUser struct {
	UID         int       `json:"uid,omitempty"`
	LastKnownAs APIString `json:"last_known_as,omitempty"`
}

func (g *APIGithubUser) BuildFromService(h interface{}) error {
	if g == nil {
		return errors.New("APIGithubUser has not been instantiated")
	}
	switch v := h.(type) {
	case user.GithubUser:
		g.UID = v.UID
		g.LastKnownAs = ToAPIString(v.LastKnownAs)
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
		LastKnownAs: FromAPIString(g.LastKnownAs),
	}, nil
}

type APINotificationPreferences struct {
	BuildBreak            APIString `json:"build_break"`
	BuildBreakID          APIString `json:"build_break_id,omitempty"`
	PatchFinish           APIString `json:"patch_finish"`
	PatchFinishID         APIString `json:"patch_finish_id,omitempty"`
	SpawnHostExpiration   APIString `json:"spawn_host_expiration"`
	SpawnHostExpirationID APIString `json:"spawn_host_expiration_id,omitempty"`
	SpawnHostOutcome      APIString `json:"spawn_host_outcome"`
	SpawnHostOutcomeID    APIString `json:"spawn_host_outcome_id,omitempty"`
	CommitQueue           APIString `json:"commit_queue"`
	CommitQueueID         APIString `json:"commit_queue_id,omitempty"`
}

func (n *APINotificationPreferences) BuildFromService(h interface{}) error {
	if n == nil {
		return errors.New("APINotificationPreferences has not been instantiated")
	}
	switch v := h.(type) {
	case user.NotificationPreferences:
		n.BuildBreak = ToAPIString(string(v.BuildBreak))
		n.PatchFinish = ToAPIString(string(v.PatchFinish))
		n.SpawnHostOutcome = ToAPIString(string(v.SpawnHostOutcome))
		n.SpawnHostExpiration = ToAPIString(string(v.SpawnHostExpiration))
		n.CommitQueue = ToAPIString(string(v.CommitQueue))
		if v.BuildBreakID != "" {
			n.BuildBreakID = ToAPIString(v.BuildBreakID)
		}
		if v.PatchFinishID != "" {
			n.PatchFinishID = ToAPIString(v.PatchFinishID)
		}
		if v.SpawnHostOutcomeID != "" {
			n.SpawnHostOutcomeID = ToAPIString(v.SpawnHostOutcomeID)
		}
		if v.SpawnHostExpirationID != "" {
			n.SpawnHostExpirationID = ToAPIString(v.SpawnHostExpirationID)
		}
		if v.CommitQueueID != "" {
			n.CommitQueueID = ToAPIString(v.CommitQueueID)
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
	buildbreak := FromAPIString(n.BuildBreak)
	patchFinish := FromAPIString(n.PatchFinish)
	spawnHostExpiration := FromAPIString(n.SpawnHostExpiration)
	spawnHostOutcome := FromAPIString(n.SpawnHostOutcome)
	commitQueue := FromAPIString(n.CommitQueue)
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
	preferences.BuildBreakID = FromAPIString(n.BuildBreakID)
	preferences.PatchFinishID = FromAPIString(n.PatchFinishID)
	preferences.SpawnHostOutcomeID = FromAPIString(n.PatchFinishID)
	preferences.SpawnHostExpirationID = FromAPIString(n.SpawnHostExpirationID)
	preferences.CommitQueueID = FromAPIString(n.CommitQueueID)
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
	DisplayName APIString
	Email       APIString
}

func (u *APIUserAuthorInformation) BuildFromService(h interface{}) error {
	v, ok := h.(*user.DBUser)
	if !ok {
		return errors.Errorf("incorrect type '%T' for user", h)
	}
	if v == nil {
		return errors.New("can't build from nil user")
	}

	u.DisplayName = ToAPIString(v.DisplayName())
	u.Email = ToAPIString(v.Email())

	return nil
}

func (u *APIUserAuthorInformation) ToService() (interface{}, error) {
	return nil, errors.New("not implemented for read-only route")
}

type APIFeedbackSubmission struct {
	Type        APIString           `json:"type"`
	User        APIString           `json:"user"`
	SubmittedAt time.Time           `json:"submitted_at"`
	Questions   []APIQuestionAnswer `json:"questions"`
}

func (a *APIFeedbackSubmission) BuildFromService(h interface{}) error {
	return errors.New("BuildFromService not implemented for APIFeedbackSubmission")
}

func (a *APIFeedbackSubmission) ToService() (interface{}, error) {
	result := model.FeedbackSubmission{
		Type:        FromAPIString(a.Type),
		User:        FromAPIString(a.User),
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
	ID     APIString `json:"id"`
	Prompt APIString `json:"prompt"`
	Answer APIString `json:"answer"`
}

func (a *APIQuestionAnswer) BuildFromService(h interface{}) error {
	return errors.New("BuildFromService not implemented for APIQuestionAnswer")
}

func (a *APIQuestionAnswer) ToService() (interface{}, error) {
	return model.QuestionAnswer{
		ID:     FromAPIString(a.ID),
		Prompt: FromAPIString(a.Prompt),
		Answer: FromAPIString(a.Answer),
	}, nil
}

type APIRole struct {
	Id          APIString         `json:"id"`
	Name        APIString         `json:"name"`
	ScopeType   APIString         `json:"scope_type"`
	Scope       APIString         `json:"scope"`
	Permissions map[string]string `json:"permissions"`
}

func (r *APIRole) BuildFromService(h interface{}) error {
	v, ok := h.(*user.Role)
	if !ok {
		return errors.Errorf("incorrect type '%T' for role", h)
	}
	if v == nil {
		return errors.New("can't build from nil role")
	}

	r.Id = ToAPIString(v.Id)
	r.Name = ToAPIString(v.Name)
	r.ScopeType = ToAPIString(string(v.ScopeType))
	r.Scope = ToAPIString(v.Scope)
	r.Permissions = v.Permissions

	return nil
}

func (r *APIRole) ToService() (interface{}, error) {
	return user.Role{
		Id:          FromAPIString(r.Id),
		Name:        FromAPIString(r.Name),
		ScopeType:   user.ScopeType(FromAPIString(r.ScopeType)),
		Scope:       FromAPIString(r.Scope),
		Permissions: r.Permissions,
	}, nil
}
