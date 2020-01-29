package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/stretchr/testify/assert"
)

func TestCommitQueueBuildFromService(t *testing.T) {
	assert := assert.New(t)
	cq := commitqueue.CommitQueue{
		ProjectID: "mci",
		Queue: []commitqueue.CommitQueueItem{
			commitqueue.CommitQueueItem{
				Issue: "1",
				Modules: []commitqueue.Module{
					commitqueue.Module{
						Module: "test_module",
						Issue:  "2",
					},
				},
			},
			commitqueue.CommitQueueItem{
				Issue: "2",
			},
			commitqueue.CommitQueueItem{
				Issue: "3",
			},
		},
	}

	cqAPI := APICommitQueue{}
	assert.NoError(cqAPI.BuildFromService(cq))
	assert.Equal(cq.ProjectID, FromStringPtr(cqAPI.ProjectID))
	assert.Equal(len(cqAPI.Queue), len(cq.Queue))
	for i := range cq.Queue {
		assert.Equal(cq.Queue[i].Issue, FromStringPtr(cqAPI.Queue[i].Issue))
	}
	assert.Equal(cq.Queue[0].Modules[0].Module, FromStringPtr(cqAPI.Queue[0].Modules[0].Module))
	assert.Equal(cq.Queue[0].Modules[0].Issue, FromStringPtr(cqAPI.Queue[0].Modules[0].Issue))
}

func TestParseGitHubCommentModules(t *testing.T) {
	assert := assert.New(t)

	comment := "evergreen merge"
	modules := ParseGitHubCommentModules(comment)
	assert.Len(modules, 0)

	comment = "evergreen merge --unknown-option blah_blah"
	modules = ParseGitHubCommentModules(comment)
	assert.Len(modules, 0)

	comment = "evergreen merge --unknown-option blah_blah --module module1:1234"
	modules = ParseGitHubCommentModules(comment)
	assert.Len(modules, 1)
	assert.Equal(ToStringPtr("module1"), modules[0].Module)
	assert.Equal(ToStringPtr("1234"), modules[0].Issue)

	comment = "evergreen merge --module module1:1234 -m  module2:3456 --module module3:5678"
	modules = ParseGitHubCommentModules(comment)
	assert.Len(modules, 3)
	assert.Equal(ToStringPtr("module1"), modules[0].Module)
	assert.Equal(ToStringPtr("1234"), modules[0].Issue)
	assert.Equal(ToStringPtr("module2"), modules[1].Module)
	assert.Equal(ToStringPtr("3456"), modules[1].Issue)
	assert.Equal(ToStringPtr("module3"), modules[2].Module)
	assert.Equal(ToStringPtr("5678"), modules[2].Issue)

	comment = "evergreen merge -m module1:1234"
	modules = ParseGitHubCommentModules(comment)
	assert.Len(modules, 1)
	assert.Equal(ToStringPtr("module1"), modules[0].Module)
	assert.Equal(ToStringPtr("1234"), modules[0].Issue)

	comment = "evergreen merge -m"
	modules = ParseGitHubCommentModules(comment)
	assert.Len(modules, 0)
}
