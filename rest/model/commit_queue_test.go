package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/utility"
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
	cqAPI.BuildFromService(cq)
	assert.Equal(cq.ProjectID, utility.FromStringPtr(cqAPI.ProjectID))
	assert.Equal(len(cqAPI.Queue), len(cq.Queue))
	for i := range cq.Queue {
		assert.Equal(cq.Queue[i].Issue, utility.FromStringPtr(cqAPI.Queue[i].Issue))
	}
	assert.Equal(cq.Queue[0].Modules[0].Module, utility.FromStringPtr(cqAPI.Queue[0].Modules[0].Module))
	assert.Equal(cq.Queue[0].Modules[0].Issue, utility.FromStringPtr(cqAPI.Queue[0].Modules[0].Issue))
}

func TestParseGitHubComment(t *testing.T) {
	assert := assert.New(t)

	comment := " evergreen merge "
	data := ParseGitHubComment(comment)
	assert.Len(data.Modules, 0)

	comment = " evergreen merge --unknown-option blah_blah "
	data = ParseGitHubComment(comment)
	assert.Len(data.Modules, 0)

	comment = "evergreen merge --unknown-option blah_blah --module module1:1234"
	data = ParseGitHubComment(comment)
	assert.Len(data.Modules, 1)
	assert.Equal(utility.ToStringPtr("module1"), data.Modules[0].Module)
	assert.Equal(utility.ToStringPtr("1234"), data.Modules[0].Issue)

	comment = "evergreen merge --module module1:1234 -m  module2:3456 --module module3:5678"
	data = ParseGitHubComment(comment)
	assert.Len(data.Modules, 3)
	assert.Equal(utility.ToStringPtr("module1"), data.Modules[0].Module)
	assert.Equal(utility.ToStringPtr("1234"), data.Modules[0].Issue)
	assert.Equal(utility.ToStringPtr("module2"), data.Modules[1].Module)
	assert.Equal(utility.ToStringPtr("3456"), data.Modules[1].Issue)
	assert.Equal(utility.ToStringPtr("module3"), data.Modules[2].Module)
	assert.Equal(utility.ToStringPtr("5678"), data.Modules[2].Issue)

	comment = "evergreen merge -m module1:1234"
	data = ParseGitHubComment(comment)
	assert.Len(data.Modules, 1)
	assert.Equal(utility.ToStringPtr("module1"), data.Modules[0].Module)
	assert.Equal(utility.ToStringPtr("1234"), data.Modules[0].Issue)

	comment = "evergreen merge -m"
	data = ParseGitHubComment(comment)
	assert.Len(data.Modules, 0)

	comment = `evergreen merge --unknown-option blah_blah --module module1:1234


this is my commit message
some more lines
    extra whitespace  `
	data = ParseGitHubComment(comment)
	assert.Len(data.Modules, 1)
	assert.Equal(utility.ToStringPtr("module1"), data.Modules[0].Module)
	assert.Equal(utility.ToStringPtr("1234"), data.Modules[0].Issue)
	assert.Equal("this is my commit message some more lines extra whitespace", data.MessageOverride)

	comment = `evergreen merge --unknown-option blah_blah --module module1:1234 
This is a very long string that needs to be wrapped around 72 characters for readability.
Here is another shorter line.
											Whitespace over here.`
	data = ParseGitHubComment(comment)
	assert.Len(data.Modules, 1)
	assert.Equal(utility.ToStringPtr("module1"), data.Modules[0].Module)
	assert.Equal(utility.ToStringPtr("1234"), data.Modules[0].Issue)
	assert.Equal(`This is a very long string that needs to be wrapped around 72 characters
for readability. Here is another shorter line. Whitespace over here.`, data.MessageOverride)
}
