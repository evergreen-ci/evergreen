package commitqueue

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/grip/level"
	"github.com/stretchr/testify/assert"
)

func TestGithubPRLogger(t *testing.T) {
	assert := assert.New(t)

	dbSessionFactory, err := getDBSessionFactory()
	assert.NoError(err)
	db.SetGlobalSessionProvider(dbSessionFactory)
	assert.NoError(db.ClearCollections(Collection))
	cq := &CommitQueue{
		ProjectID: "mci",
		Queue:     []string{"1", "2"},
	}
	assert.NoError(InsertQueue(cq))

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
