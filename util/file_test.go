package util

import (
	. "github.com/smartystreets/goconvey/convey"
	"io/ioutil"
	"os"
	"testing"
)

func TestWriteToTempFile(t *testing.T) {
	Convey("When writing content to a temp file", t, func() {
		Convey("ensure the exact contents passed are written", func() {
			fileData := "data"
			filePath, err := WriteToTempFile(fileData)
			HandleTestingErr(err, t, "error writing to temp file %v")
			fileBytes, err := ioutil.ReadFile(filePath)
			HandleTestingErr(err, t, "error reading from temp file %v")
			So(string(fileBytes), ShouldEqual, fileData)
			HandleTestingErr(os.Remove(filePath), t,
				"error removing to temp file %v")
		})
	})
}

func TestFileExists(t *testing.T) {

	_, err := os.Create("testFile1")
	HandleTestingErr(err, t, "error creating test file")
	defer func() {
		HandleTestingErr(os.Remove("testFile1"), t, "error removing test file")
	}()

	Convey("When testing that a file exists", t, func() {

		Convey("an existing file should be reported as existing", func() {
			exists, err := FileExists("testFile1")
			So(err, ShouldBeNil)
			So(exists, ShouldBeTrue)
		})

		Convey("a nonexistent file should be reported as missing", func() {
			exists, err := FileExists("testFileThatDoesNotExist1234567")
			So(err, ShouldBeNil)
			So(exists, ShouldBeFalse)
		})
	})
}
