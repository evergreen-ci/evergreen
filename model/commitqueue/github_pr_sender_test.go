package commitqueue

import (
	"testing"

	"github.com/mongodb/grip/level"
	"github.com/stretchr/testify/assert"
)

func TestGithubPRLogger(t *testing.T) {
	assert := assert.New(t)
	errLogger := &mockErrorLogger{}
	ghPRLogger, err := NewMockGithubPRLogger("mock_gh_pr_logger", errLogger)
	assert.NoError(err)

	msg := GithubMergePR{
		PatchSucceeded: true,
		ProjectID:      "mci",
		Owner:          "evergreen-ci",
		Repo:           "evergreen",
		Ref:            "deadbeef",
		CommitMessage:  "merged by cq",
		PRNum:          1,
	}
	c := NewGithubMergePRMessage(level.Info, msg)
	ghPRLogger.Send(c)
	assert.Empty(errLogger.errList)
}
