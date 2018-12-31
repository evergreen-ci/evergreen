package githubpr

import (
	"testing"

	"github.com/mongodb/grip/level"
	"github.com/stretchr/testify/assert"
)

func TestGithubPRMerge(t *testing.T) {
	assert := assert.New(t)
	pr := GithubMergePR{
		Owner:     "evergreen-ci",
		Repo:      "evergreen",
		Ref:       "deadbeef",
		CommitMsg: "merged by cq",
		PRNum:     1,
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
		Owner:     "evergreen-ci",
		Repo:      "evergreen",
		Ref:       "deadbeef",
		CommitMsg: "merged by cq",
	}
	c := NewGithubMergePRMessage(level.Info, missingPRNum)
	assert.False(c.Loggable())

	missingMergeMethod := GithubMergePR{
		Owner:     "evergreen-ci",
		Repo:      "evergreen",
		Ref:       "deadbeef",
		CommitMsg: "merged by cq",
		PRNum:     1,
	}
	c = NewGithubMergePRMessage(level.Info, missingMergeMethod)
	assert.True(c.Loggable())

}
