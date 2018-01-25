package subprocess

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalExec(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tmpdir, err := ioutil.TempDir("", "local-exec-test")
	require.NoError(err)
	defer os.RemoveAll(tmpdir)

	cmd, err := NewLocalExec("bash", []string{"-c", "touch foo"}, nil, tmpdir)
	assert.NoError(err)
	assert.NoError(cmd.Stop())
	assert.Equal(-1, cmd.GetPid())
	assert.NoError(cmd.Run(ctx))
	assert.NotEqual(-1, cmd.GetPid())

	_, err = os.Stat(filepath.Join(tmpdir, "foo"))
	assert.False(os.IsNotExist(err))

	// fall back to current working directory if we specify the empty string
	cmd, err = NewLocalExec("bash", []string{"-c", "touch foo"}, nil, "")
	assert.NoError(err)
	assert.NotNil(cmd)
	wd, err := os.Getwd()
	assert.NoError(err)
	assert.Equal(wd, cmd.(*localExec).workingDirectory)

	// check some error cases
	cmd, err = NewLocalExec("", []string{"a", "b"}, nil, tmpdir)
	assert.Error(err)
	assert.Nil(cmd)

	cmd, err = NewLocalExec("bash", []string{"-c", "touch foo"}, nil, filepath.Join(tmpdir, "foo"))
	assert.Error(err)
	assert.Nil(cmd)

	cmd, err = NewLocalExec("bash", []string{"-c", "touch foo"}, nil, filepath.Join(tmpdir, "bar"))
	assert.Error(err)
	assert.Nil(cmd)

	// check envvar
	fn := filepath.Join(tmpdir, "baz")
	_, err = os.Stat(fn)
	assert.True(os.IsNotExist(err))

	cmd, err = NewLocalExec("bash", []string{"-c", "touch $VAL"}, map[string]string{"VAL": "baz"}, tmpdir)
	assert.NoError(err)
	assert.NotNil(cmd)
	assert.NoError(cmd.Run(ctx))
	assert.False(os.IsNotExist(err))

	cmd, err = NewLocalExec("bash", []string{"-c", "sleep 10"}, nil, tmpdir)
	assert.NoError(err)
	assert.NotNil(cmd)
	assert.NoError(cmd.Start(ctx))
	assert.NoError(cmd.Stop())

}
