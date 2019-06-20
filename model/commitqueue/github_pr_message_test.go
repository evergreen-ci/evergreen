package commitqueue

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/mongodb/grip/level"
	"github.com/stretchr/testify/assert"
)

func TestGithubPRMerge(t *testing.T) {
	assert := assert.New(t)
	pr := GithubMergePR{
		ProjectID: "mci",
		PRs: []event.PRInfo{
			{
				Owner:       "evergreen-ci",
				Repo:        "evergreen",
				Ref:         "deadbeef",
				CommitTitle: "PR (#1)",
				PRNum:       1,
			},
		},
		Item:   "1",
		Status: evergreen.PatchFailed,
	}
	c := NewGithubMergePRMessage(level.Info, pr)
	assert.NotNil(c)
	assert.True(c.Loggable())

	raw, ok := c.Raw().(*GithubMergePR)
	assert.True(ok)

	assert.Equal(pr, *raw)

	assert.Equal("GitHub commit queue merge '1'", c.String())
}

func TestGithubMergePRMessageValidator(t *testing.T) {
	assert := assert.New(t)

	missingPRNum := GithubMergePR{
		ProjectID:   "mci",
		Status:      evergreen.PatchSucceeded,
		Item:        "1",
		MergeMethod: githubMergeMethodSquash,
		PRs: []event.PRInfo{
			{
				Owner: "evergreen-ci",
				Repo:  "evergreen",
				Ref:   "deadbeef",
			},
		},
	}
	c := NewGithubMergePRMessage(level.Info, missingPRNum)
	assert.False(c.Loggable())

	missingMergeMethod := GithubMergePR{
		ProjectID: "mci",
		Status:    evergreen.PatchSucceeded,
		Item:      "1",
		PRs: []event.PRInfo{
			{
				Owner: "evergreen-ci",
				Repo:  "evergreen",
				Ref:   "deadbeef",
				PRNum: 1,
			},
		},
	}
	c = NewGithubMergePRMessage(level.Info, missingMergeMethod)
	assert.True(c.Loggable())

}
