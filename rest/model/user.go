package model

import (
	"reflect"

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
	Timezone      APIString                   `json:"timezone"`
	GithubUser    *APIGithubUser              `json:"github_user"`
	SlackUsername APIString                   `json:"slack_username"`
	Notifications *APINotificationPreferences `json:"notifications"`
}

func (s *APIUserSettings) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case user.UserSettings:
		s.Timezone = ToAPIString(v.Timezone)
		s.SlackUsername = ToAPIString(v.SlackUsername)
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
	return user.UserSettings{
		Timezone:      FromAPIString(s.Timezone),
		SlackUsername: FromAPIString(s.SlackUsername),
		GithubUser:    githubUser,
		Notifications: preferences,
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
}

func (n *APINotificationPreferences) BuildFromService(h interface{}) error {
	if n == nil {
		return errors.New("APINotificationPreferences has not been instantiated")
	}
	switch v := h.(type) {
	case user.NotificationPreferences:
		n.BuildBreak = ToAPIString(string(v.BuildBreak))
		n.PatchFinish = ToAPIString(string(v.PatchFinish))
		n.SpawnHostExpiration = ToAPIString(string(v.SpawnHostExpiration))
		if v.BuildBreakID != "" {
			n.BuildBreakID = ToAPIString(v.BuildBreakID.Hex())
		}
		if v.PatchFinishID != "" {
			n.PatchFinishID = ToAPIString(v.PatchFinishID.Hex())
		}
		if v.SpawnHostExpirationID != "" {
			n.SpawnHostExpirationID = ToAPIString(v.SpawnHostExpirationID.Hex())
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
	if !user.IsValidSubscriptionPreference(buildbreak) {
		return nil, errors.New("Build break preference is not a valid type")
	}
	if !user.IsValidSubscriptionPreference(patchFinish) {
		return nil, errors.New("Patch finish preference is not a valid type")
	}
	preferences := user.NotificationPreferences{
		BuildBreak:          user.UserSubscriptionPreference(buildbreak),
		PatchFinish:         user.UserSubscriptionPreference(patchFinish),
		SpawnHostExpiration: user.UserSubscriptionPreference(spawnHostExpiration),
	}
	var err error
	if n.BuildBreakID != nil {
		preferences.BuildBreakID, err = user.FormatObjectID(FromAPIString(n.BuildBreakID))
		if err != nil {
			return nil, err
		}
	}
	if n.PatchFinishID != nil {
		preferences.PatchFinishID, err = user.FormatObjectID(FromAPIString(n.PatchFinishID))
		if err != nil {
			return nil, err
		}
	}
	if n.SpawnHostExpirationID != nil {
		preferences.SpawnHostExpirationID, err = user.FormatObjectID(FromAPIString(n.SpawnHostExpirationID))
		if err != nil {
			return nil, err
		}
	}
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
