package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetCommitCommands(t *testing.T) {
	assert := assert.New(t)

	cmds := getCommitCommands("John Doe", "john@doe.com", "message", "master")
	assert.Equal("set -o xtrace", cmds[0])
	assert.Equal("set -o errexit", cmds[1])
	assert.Equal("git add -A", cmds[2])
	assert.Equal(`git -c "user.name=Evergreen Agent" -c "user.email=no-reply@evergreen.mongodb.com" commit -m "message" --author="John Doe <john@doe.com>"`, cmds[3])
	assert.Equal(`git push origin "master"`, cmds[4])
}

func TestCheckoutBranchCommands(t *testing.T) {
	assert := assert.New(t)

	cmds := getCheckoutBranchCommands("master")
	assert.Equal("set -o xtrace", cmds[0])
	assert.Equal("set -o errexit", cmds[1])
	assert.Equal(`git checkout "master"`, cmds[2])
}
