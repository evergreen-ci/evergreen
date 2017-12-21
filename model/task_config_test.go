package model

import (
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

func TestTaskConfig(t *testing.T) {
	assert := assert.New(t) // nolint

	curdir := testutil.GetDirectoryOfFile()

	conf := &TaskConfig{
		WorkDir: curdir,
	}

	// make sure that we fall back to the configured working directory
	out, err := conf.GetWorkingDirectory("")
	assert.NoError(err)
	assert.Equal(conf.WorkDir, out)

	// check for a directory that we know exists
	out, err = conf.GetWorkingDirectory("task")
	assert.NoError(err)
	assert.Equal(out, filepath.Join(curdir, "task"))

	// check for a file not a directory
	out, err = conf.GetWorkingDirectory("project.go")
	assert.Error(err)
	assert.Equal("", out)

	// presumably for a directory that doesn't exist
	out, err = conf.GetWorkingDirectory("does-not-exist")
	assert.Error(err)
	assert.Equal("", out)
}
