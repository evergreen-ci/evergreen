package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/stretchr/testify/assert"
)

func TestFullUserSettings(t *testing.T) {
	settings := user.UserSettings{
		Timezone:      "east",
		SlackUsername: "me",
		GithubUser: user.GithubUser{
			UID:         5,
			LastKnownAs: "peter",
		},
		Notifications: user.NotificationPreferences{
			BuildBreak:  user.PreferenceEmail,
			PatchFinish: user.PreferenceSlack,
			CommitQueue: user.PreferenceSlack,
		},
	}

	runTests(t, settings)
}

func TestEmptySettings(t *testing.T) {
	settings := user.UserSettings{}

	runTests(t, settings)
}

func TestPartialSettings(t *testing.T) {
	settings := user.UserSettings{
		Notifications: user.NotificationPreferences{
			BuildBreak:  user.PreferenceEmail,
			PatchFinish: user.PreferenceSlack,
			CommitQueue: user.PreferenceEmail,
		},
	}

	runTests(t, settings)
}

func runTests(t *testing.T, in user.UserSettings) {
	assert := assert.New(t)
	apiSettings := APIUserSettings{}
	err := apiSettings.BuildFromService(in)
	assert.NoError(err)

	origSettings, err := apiSettings.ToService()
	assert.NoError(err)
	assert.EqualValues(in, origSettings)
}

func TestAPIUserAuthorInformation(t *testing.T) {
	assert := assert.New(t)
	apiUser := APIUserAuthorInformation{}
	user := &user.DBUser{
		DispName:     "octocat",
		EmailAddress: "octocat@github.com",
	}
	assert.NoError(apiUser.BuildFromService(user))

	_, err := apiUser.ToService()
	assert.Error(err)
}
