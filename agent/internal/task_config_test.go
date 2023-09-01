package internal

import (
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskConfigGetWorkingDirectory(t *testing.T) {
	curdir := testutil.GetDirectoryOfFile()

	conf := &TaskConfig{
		WorkDir: curdir,
	}

	// make sure that we fall back to the configured working directory
	out, err := conf.GetWorkingDirectory("")
	assert.NoError(t, err)
	assert.Equal(t, conf.WorkDir, out)

	// check for a directory that we know exists
	out, err = conf.GetWorkingDirectory("testutil")
	require.NoError(t, err)
	assert.Equal(t, out, filepath.Join(curdir, "testutil"))

	// check for a file not a directory
	out, err = conf.GetWorkingDirectory("task_config.go")
	assert.Error(t, err)
	assert.Equal(t, "", out)

	// presumably for a directory that doesn't exist
	out, err = conf.GetWorkingDirectory("does-not-exist")
	assert.Error(t, err)
	assert.Equal(t, "", out)
}
