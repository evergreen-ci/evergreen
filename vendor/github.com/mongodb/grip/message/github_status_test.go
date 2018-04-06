package message

import (
	"testing"

	"github.com/google/go-github/github"
	"github.com/mongodb/grip/level"
	"github.com/stretchr/testify/assert"
)

func TestGithubStatus(t *testing.T) {
	assert := assert.New(t) //nolint: vetshadow

	c := NewGithubStatus(level.Info, "example", GithubStatePending, "https://example.com/hi", "description")
	assert.NotNil(c)
	assert.True(c.Loggable())

	raw, ok := c.Raw().(*github.RepoStatus)
	assert.True(ok)

	assert.NotPanics(func() {
		assert.Equal("example", *raw.Context)
		assert.Equal(string(GithubStatePending), *raw.State)
		assert.Equal("https://example.com/hi", *raw.URL)
		assert.Equal("description", *raw.Description)
	})

	assert.Equal("example pending: description (https://example.com/hi)", c.String())

	c = NewGithubStatus(level.Info, "example", GithubStatePending, "https://example.com/hi", "")
	assert.True(c.Loggable())
	raw, ok = c.Raw().(*github.RepoStatus)
	assert.True(ok)
	assert.Nil(raw.Description)
	assert.Equal("example pending (https://example.com/hi)", c.String())
}

func TestGithubStatusInvalidStatusesAreNotLoggable(t *testing.T) {
	assert := assert.New(t) //nolint: vetshadow

	c := NewGithubStatus(level.Info, "", GithubStatePending, "https://example.com/hi", "description")
	assert.False(c.Loggable())
	c = NewGithubStatus(level.Info, "example", "nope", "https://example.com/hi", "description")
	assert.False(c.Loggable())
	c = NewGithubStatus(level.Info, "example", GithubStatePending, ":foo", "description")
	assert.False(c.Loggable())
}
