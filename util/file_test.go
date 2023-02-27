package util

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/evergreen-ci/utility"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getDirectoryOfFile() string {
	// returns the directory of the file of the calling
	// function. duplicated from testutil to avoid a cycle.
	_, file, _, _ := runtime.Caller(1)

	return filepath.Dir(file)
}

func TestWriteToTempFile(t *testing.T) {
	Convey("When writing content to a temp file", t, func() {
		Convey("ensure the exact contents passed are written", func() {
			fileData := "data"
			filePath, err := WriteToTempFile(fileData)
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, os.Remove(filePath))
			}()
			fileBytes, err := os.ReadFile(filePath)
			require.NoError(t, err)
			So(string(fileBytes), ShouldEqual, fileData)
		})
	})
}

func TestFileExists(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "testFileOne")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name())

	Convey("When testing that a file exists", t, func() {

		Convey("an existing file should be reported as existing", func() {
			exists := utility.FileExists(tmpfile.Name())
			So(exists, ShouldBeTrue)
		})

		Convey("a nonexistent file should be reported as missing", func() {
			exists := utility.FileExists("testFileThatDoesNotExist1234567")
			So(exists, ShouldBeFalse)
		})
	})
}
