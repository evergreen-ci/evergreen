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

	dir, err := ioutil.TempDir("", "dir-cleanup")
	assert.NoError(err)

	stat, err := os.Stat(dir)
	assert.True(stat.IsDir())
	assert.True(osExists(err))

	fn := filepath.Join(dir, "foo")
	assert.NoError(ioutil.WriteFile(fn, []byte("hello world!"), 0644))

	stat, err = os.Stat(fn)
	assert.False(stat.IsDir())
	assert.True(osExists(err))

	tryCleanupDirectory(fn)
	stat, err = os.Stat(fn)
	assert.False(stat.IsDir())
	assert.True(osExists(err))

	tryCleanupDirectory(dir)
	_, err = os.Stat(dir)
	assert.True(os.IsNotExist(err))

	_, err = os.Stat(fn)
	assert.True(os.IsNotExist(err))
}
