package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/mongodb/jasper/mock"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func osExists(err error) bool { return !os.IsNotExist(err) }

func TestRemoveAll(t *testing.T) {
	t.Run("SucceedsOnFirstAttempt", func(t *testing.T) {
		dir := t.TempDir()
		a := Agent{}
		require.NoError(t, a.removeAll(t.Context(), dir))
		assert.NoDirExists(t, dir)
	})
}

func TestRemoveTaskDirectory(t *testing.T) {
	// make a long directory name to test working around https://github.com/golang/go/issues/36375
	a := ""
	b := ""
	for i := 0; i < 150; i++ {
		a += "a"
		b += "b"
	}
	wd, err := os.Getwd()
	require.NoError(t, err)
	tmpDir, err := os.MkdirTemp(wd, "test-remove")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "foo", "bar", a, b), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "read.txt"), []byte("haha can't delete me!"), 0444))

	agent := Agent{
		opts: Options{
			WorkingDirectory: tmpDir,
		},
	}

	tc := &taskContext{
		taskConfig: &internal.TaskConfig{
			WorkDir: filepath.Base(tmpDir),
		},
		oomTracker: &mock.OOMTracker{},
	}

	agent.removeTaskDirectory(t.Context(), tc)
	_, err = os.Stat(tmpDir)
	require.True(t, os.IsNotExist(err), "directory should have been deleted")
}
func TestDirectoryCleanup(t *testing.T) {
	assert := assert.New(t)

	// create a temp directory for the test
	dir := t.TempDir()

	// create a file in that directory
	fn := filepath.Join(dir, "foo")
	require.NoError(t, os.WriteFile(fn, []byte("hello world!"), 0644))
	stat, err := os.Stat(fn)
	require.NoError(t, err)
	require.NotNil(t, stat)
	assert.False(stat.IsDir())

	// cannot run the operation on a file, and it will not delete
	// that file
	a := Agent{}
	a.tryCleanupDirectory(t.Context(), fn)
	_, err = os.Stat(fn)
	assert.True(osExists(err))

	// running the operation on the top level directory does not
	// delete that directory but does delete the files within it
	a.tryCleanupDirectory(t.Context(), dir)
	_, err = os.Stat(dir)
	assert.True(osExists(err))

	// verify a subdirectory containing a read-only file is deleted
	toDelete := filepath.Join(dir, "wrapped-dir-cleanup")
	require.NoError(t, os.Mkdir(toDelete, 0777))
	readOnlyFileToDelete := filepath.Join(toDelete, "read-only")
	require.NoError(t, os.WriteFile(readOnlyFileToDelete, []byte("cookies"), 0644))
	require.NoError(t, os.Chmod(readOnlyFileToDelete, 0444))
	a.tryCleanupDirectory(t.Context(), dir)
	_, err = os.Stat(readOnlyFileToDelete)
	assert.True(os.IsNotExist(err))
	_, err = os.Stat(toDelete)
	assert.True(os.IsNotExist(err))

	// should delete nothing if we hit .git first
	gitDir := filepath.Join(dir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0777))
	shouldNotDelete := filepath.Join(dir, "dir1", "delete-me")
	require.NoError(t, os.MkdirAll(shouldNotDelete, 0777))
	a.tryCleanupDirectory(t.Context(), dir)
	_, err = os.Stat(gitDir)
	assert.False(os.IsNotExist(err))
	_, err = os.Stat(shouldNotDelete)
	assert.False(os.IsNotExist(err))
}

func TestCheckDataDirectoryHealthWithUsage(t *testing.T) {
	t.Run("DiskUsageBelowThreshold", func(t *testing.T) {
		mockComm := &client.Mock{}
		agent := Agent{
			opts: Options{
				HostID: "test-host-id",
			},
			comm: mockComm,
		}

		lowUsage := &disk.UsageStat{
			UsedPercent: 40.0, // Below threshold
		}

		err := agent.checkDataDirectoryHealthWithUsage(t.Context(), lowUsage)
		assert.NoError(t, err)
	})

	t.Run("DiskUsageAboveThreshold", func(t *testing.T) {
		mockComm := &client.Mock{}
		agent := Agent{
			opts: Options{
				HostID: "test-host-id",
			},
			comm: mockComm,
		}

		highUsage := &disk.UsageStat{
			UsedPercent: 95.0, // Above threshold
		}

		err := agent.checkDataDirectoryHealthWithUsage(t.Context(), highUsage)
		assert.NoError(t, err)
	})
}
