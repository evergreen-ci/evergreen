package commitqueue

import (
	"testing"

	"github.com/mongodb/grip/level"
	"github.com/stretchr/testify/assert"
)

func TestGithubPRMerge(t *testing.T) {
	assert := assert.New(t)
	pr := GithubMergePR{
		ProjectID:     "mci",
		Owner:         "evergreen-ci",
		Repo:          "evergreen",
		Ref:           "deadbeef",
		CommitMessage: "merged by cq",
		PRNum:         1,
	}
	c := NewGithubMergePRMessage(level.Info, pr)
	assert.NotNil(c)
	assert.True(c.Loggable())

	raw, ok := c.Raw().(*GithubMergePR)
	assert.True(ok)

	assert.Equal(pr, *raw)

	assert.Equal("Merge Pull Request #1 (Ref: deadbeef) on evergreen-ci/evergreen: merged by cq", c.String())
}

func TestGithubMergePRMessageValidator(t *testing.T) {
	assert := assert.New(t)

	missingPRNum := GithubMergePR{
		ProjectID: "mci",
		Owner:     "evergreen-ci",
		Repo:      "evergreen",
		Ref:       "deadbeef",
	}
	c := NewGithubMergePRMessage(level.Info, missingPRNum)
	assert.False(c.Loggable())

	missingMergeMethod := GithubMergePR{
		ProjectID: "mci",
		Owner:     "evergreen-ci",
		Repo:      "evergreen",
		Ref:       "deadbeef",
		PRNum:     1,
	}
	c = NewGithubMergePRMessage(level.Info, missingMergeMethod)
	assert.True(c.Loggable())

}
