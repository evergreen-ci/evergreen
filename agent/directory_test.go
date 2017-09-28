package agent

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func osExists(err error) bool { return !os.IsNotExist(err) }

func TestDirectoryCleanup(t *testing.T) {
	assert := assert.New(t)

	// create a temp directory for the test
	dir, err := ioutil.TempDir("", "dir-cleanup")
	assert.NoError(err)
	stat, err := os.Stat(dir)
	assert.True(stat.IsDir())
	assert.True(osExists(err))

	// create a file in that directory
	fn := filepath.Join(dir, "foo")
	assert.NoError(ioutil.WriteFile(fn, []byte("hello world!"), 0644))
	stat, err = os.Stat(fn)
	assert.False(stat.IsDir())
	assert.True(osExists(err))

	// cannot run the operation on a file, and it will not delete
	// that files
	tryCleanupDirectory(fn)
	_, err = os.Stat(fn)
	assert.True(osExists(err))

	// running the operation on the top level directory does not
	// delete that directory
	tryCleanupDirectory(dir)
	_, err = os.Stat(dir)
	assert.True(osExists(err))

	// does not clean up files in the root directory
	_, err = os.Stat(fn)
	assert.True(osExists(err))

	// create a sub-directory that will need to be deleted
	subdir, err := ioutil.TempDir(dir, "wrapped-dir-cleanup")
	assert.NoError(err)

	tryCleanupDirectory(dir)
	_, err = os.Stat(subdir)
	assert.True(os.IsNotExist(err))

	assert.NoError(os.RemoveAll(dir))
}
