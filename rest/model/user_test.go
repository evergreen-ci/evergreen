package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/stretchr/testify/assert"
)

func TestFullUserSettings(t *testing.T) {
	settings := user.UserSettings{
		Timezone:         "east",
		Region:           "us-west-1",
		SlackUsername:    "me",
		UseSpruceOptions: user.UseSpruceOptions{},
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
	apiSettings.BuildFromService(in)

	origSettings, err := apiSettings.ToService()
	assert.NoError(err)
	assert.EqualValues(in, origSettings)

	finalAPISettings := applyUserChanges(user.UserSettings{}, apiSettings)
	assert.EqualValues(apiSettings, finalAPISettings)
}
