package evergreen

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

//Checks that the test settings file can be parsed
//and returns a settings object.
func TestInitSettings(t *testing.T) {
	assert := assert.New(t) //nolint

	settings, err := NewSettings(filepath.Join(FindEvergreenHome(),
		"testdata", "mci_settings.yml"))
	assert.NoError(err, "Parsing a valid settings file should succeed")
	assert.NotNil(settings)
}

//Checks that trying to parse a non existent file returns non-nil err
func TestBadInit(t *testing.T) {
	assert := assert.New(t) //nolint

	settings, err := NewSettings(filepath.Join(FindEvergreenHome(),
		"testdata", "blahblah.yml"))

	assert.Error(err, "Parsing a nonexistent config file should cause an error")
	assert.Nil(settings)
}

func TestGetGithubSettings(t *testing.T) {
	assert := assert.New(t) //nolint

	settings, err := NewSettings(filepath.Join(FindEvergreenHome(),
		"testdata", "mci_settings.yml"))
	assert.NoError(err)
	assert.Empty(settings.Credentials["github"])

	token, err := settings.GetGithubOauthToken()
	assert.Error(err)
	assert.Empty(token)

	settings, err = NewSettings(filepath.Join(FindEvergreenHome(),
		"config_test", "evg_settings.yml"))
	assert.NoError(err)
	assert.NotNil(settings.Credentials["github"])

	token, err = settings.GetGithubOauthToken()
	assert.NoError(err)
	assert.Equal(settings.Credentials["github"], token)

	assert.NotPanics(func() {
		settings := &Settings{}
		assert.Nil(settings.Credentials)

		token, err = settings.GetGithubOauthToken()
		assert.Error(err)
		assert.Empty(token)
	})
}
