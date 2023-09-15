package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func osExists(err error) bool { return !os.IsNotExist(err) }

func TestRemoveTaskDirectory(t *testing.T) {
	require := require.New(t)

	// make a long directory name to test working around https://github.com/golang/go/issues/36375
	a := ""
	b := ""
	for i := 0; i < 150; i++ {
		a += "a"
		b += "b"

	}
	wd, err := os.Getwd()
	require.NoError(err)
	tmpDir, err := os.MkdirTemp(wd, "test-remove")
	require.NoError(err)
	err = os.MkdirAll(filepath.Join(tmpDir, "foo", "bar", a, b), 0755)
	require.NoError(err)
	// verify files with read-only permissions get deleted
	require.NoError(os.WriteFile(filepath.Join(tmpDir, "read.txt"), []byte("haha can't delete me!"), 0444))

	// remove the task directory
	agent := Agent{
		opts: Options{
			WorkingDirectory: tmpDir,
		},
	}

	tc := &taskContext{
		taskConfig: &internal.TaskConfig{
			WorkDir: filepath.Base(tmpDir),
		},
	}

	agent.removeTaskDirectory(tc)
	_, err = os.Stat(tmpDir)
	require.True(os.IsNotExist(err), "directory should have been deleted")
}
func TestDirectoryCleanup(t *testing.T) {
	assert := assert.New(t)

	// create a temp directory for the test
	dir := t.TempDir()

	// create a file in that directory
	fn := filepath.Join(dir, "foo")
	assert.NoError(os.WriteFile(fn, []byte("hello world!"), 0644))
	stat, err := os.Stat(fn)
	assert.False(stat.IsDir())
	assert.True(osExists(err))

	// cannot run the operation on a file, and it will not delete
	// that file
	a := Agent{}
	a.tryCleanupDirectory(fn)
	_, err = os.Stat(fn)
	assert.True(osExists(err))

	// running the operation on the top level directory does not
	// delete that directory but does delete the files within it
	a.tryCleanupDirectory(dir)
	_, err = os.Stat(dir)
	assert.True(osExists(err))

	// verify a subdirectory containing a read-only file is deleted
	toDelete := filepath.Join(dir, "wrapped-dir-cleanup")
	assert.NoError(os.Mkdir(toDelete, 0777))
	readOnlyFileToDelete := filepath.Join(toDelete, "read-only")
	assert.NoError(os.WriteFile(readOnlyFileToDelete, []byte("cookies"), 0644))
	assert.NoError(os.Chmod(readOnlyFileToDelete, 0444))
	a.tryCleanupDirectory(dir)
	_, err = os.Stat(readOnlyFileToDelete)
	assert.True(os.IsNotExist(err))
	_, err = os.Stat(toDelete)
	assert.True(os.IsNotExist(err))

	// should delete nothing if we hit .git first
	gitDir := filepath.Join(dir, ".git")
	assert.NoError(os.MkdirAll(gitDir, 0777))
	shouldNotDelete := filepath.Join(dir, "dir1", "delete-me")
	assert.NoError(os.MkdirAll(shouldNotDelete, 0777))
	a.tryCleanupDirectory(dir)
	_, err = os.Stat(gitDir)
	assert.False(os.IsNotExist(err))
	_, err = os.Stat(shouldNotDelete)
	assert.False(os.IsNotExist(err))
}
