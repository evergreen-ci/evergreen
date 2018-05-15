package model

import (
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
	Timezone      APIString                  `json:"timezone"`
	GithubUser    APIGithubUser              `json:"github_user"`
	SlackUsername APIString                  `json:"slack_username"`
	Notifications APINotificationPreferences `json:"notifications"`
}

func (s *APIUserSettings) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case user.UserSettings:
		s.Timezone = ToAPIString(v.Timezone)
		s.SlackUsername = ToAPIString(v.SlackUsername)
		err := s.GithubUser.BuildFromService(v.GithubUser)
		if err != nil {
			return err
		}
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
	return user.GithubUser{
		UID:         g.UID,
		LastKnownAs: FromAPIString(g.LastKnownAs),
	}, nil
}

type APINotificationPreferences struct {
	BuildBreak  APIString `json:"build_break"`
	PatchFinish APIString `json:"patch_finish"`
}

func (n *APINotificationPreferences) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case user.NotificationPreferences:
		n.BuildBreak = ToAPIString(string(v.BuildBreak))
		n.PatchFinish = ToAPIString(string(v.PatchFinish))
	default:
		return errors.Errorf("incorrect type for APINotificationPreferences")
	}
	return nil
}

func (n *APINotificationPreferences) ToService() (interface{}, error) {
	buildbreak := FromAPIString(n.BuildBreak)
	patchFinish := FromAPIString(n.PatchFinish)
	if !user.IsValidSubscriptionPreference(buildbreak) {
		return nil, errors.New("Build break preference is not a valid type")
	}
	if !user.IsValidSubscriptionPreference(patchFinish) {
		return nil, errors.New("Patch finish preference is not a valid type")
	}
	return user.NotificationPreferences{
		BuildBreak:  user.UserSubscriptionPreference(buildbreak),
		PatchFinish: user.UserSubscriptionPreference(patchFinish),
	}, nil

}
