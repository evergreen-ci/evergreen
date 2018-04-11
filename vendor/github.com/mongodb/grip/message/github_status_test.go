package message

import (
	"testing"

	"github.com/mongodb/grip/level"
	"github.com/stretchr/testify/assert"
)

func TestGithubStatus(t *testing.T) {
	assert := assert.New(t) //nolint: vetshadow

	c := NewGithubStatusMessage(level.Info, "example", GithubStatePending, "https://example.com/hi", "description")
	assert.NotNil(c)
	assert.True(c.Loggable())

	raw, ok := c.Raw().(*GithubStatus)
	assert.True(ok)

	assert.NotPanics(func() {
		assert.Equal("example", raw.Context)
		assert.Equal(GithubStatePending, raw.State)
		assert.Equal("https://example.com/hi", raw.URL)
		assert.Equal("description", raw.Description)
	})

	assert.Equal("example pending: description (https://example.com/hi)", c.String())
}

func TestGithubStatusInvalidStatusesAreNotLoggable(t *testing.T) {
	assert := assert.New(t) //nolint: vetshadow

	c := NewGithubStatusMessage(level.Info, "", GithubStatePending, "https://example.com/hi", "description")
	assert.False(c.Loggable())
	c = NewGithubStatusMessage(level.Info, "example", "nope", "https://example.com/hi", "description")
	assert.False(c.Loggable())
	c = NewGithubStatusMessage(level.Info, "example", GithubStatePending, ":foo", "description")
	assert.False(c.Loggable())

	p := GithubStatus{
		Owner:       "",
		Repo:        "grip",
		Ref:         "master",
		Context:     "example",
		State:       GithubStatePending,
		URL:         "https://example.com/hi",
		Description: "description",
	}
	c = NewGithubStatusMessageWithRepo(level.Info, p)
	assert.False(c.Loggable())

	p.Owner = "mongodb"
	p.Repo = ""
	c = NewGithubStatusMessageWithRepo(level.Info, p)
	assert.False(c.Loggable())

	p.Repo = "grip"
	p.Ref = ""
	c = NewGithubStatusMessageWithRepo(level.Info, p)
	assert.False(c.Loggable())
}
