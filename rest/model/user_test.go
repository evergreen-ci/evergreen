package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/stretchr/testify/assert"
)

func TestUserSettings(t *testing.T) {
	assert := assert.New(t)
	settings := user.UserSettings{
		Timezone:      "east",
		SlackUsername: "me",
		GithubUser: user.GithubUser{
			UID:         5,
			LastKnownAs: "peter",
		},
		Notifications: user.NotificationPreferences{
			BuildBreak:  event.PreferenceEmail,
			PatchFinish: event.PreferenceNone,
		},
	}

	apiSettings := APIUserSettings{}
	err := apiSettings.BuildFromService(settings)
	assert.NoError(err)

	origSettings, err := apiSettings.ToService()
	assert.NoError(err)
	assert.EqualValues(settings, origSettings)
}
