package util

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/evergreen-ci/utility"
	. "github.com/smartystreets/goconvey/convey"
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
			require.NoError(t, err, "error writing to temp file %v")
			fileBytes, err := ioutil.ReadFile(filePath)
			require.NoError(t, err, "error reading from temp file %v")
			So(string(fileBytes), ShouldEqual, fileData)
			require.NoError(t, os.Remove(filePath),
				"error removing to temp file %v")
		})
	})
}

func TestFileExists(t *testing.T) {
	tmpfile, err := ioutil.TempFile("", "testFileOne")
	require.NoError(t, err, "error creating test file")
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
