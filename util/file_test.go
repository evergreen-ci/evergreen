package util

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func TestWriteToTempFile(t *testing.T) {
	Convey("When writing content to a temp file", t, func() {
		Convey("ensure the exact contents passed are written", func() {
			fileData := "data"
			filePath, err := WriteToTempFile(fileData)
			testutil.HandleTestingErr(err, t, "error writing to temp file %v")
			fileBytes, err := ioutil.ReadFile(filePath)
			testutil.HandleTestingErr(err, t, "error reading from temp file %v")
			So(string(fileBytes), ShouldEqual, fileData)
			testutil.HandleTestingErr(os.Remove(filePath), t,
				"error removing to temp file %v")
		})
	})
}

func TestFileExists(t *testing.T) {
	tmpfile, err := ioutil.TempFile("", "testFileOne")
	testutil.HandleTestingErr(err, t, "error creating test file")
	defer os.Remove(tmpfile.Name())

	Convey("When testing that a file exists", t, func() {

		Convey("an existing file should be reported as existing", func() {
			exists, err := FileExists(tmpfile.Name())
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

func TestBuildFileList(t *testing.T) {
	wd, err := ioutil.TempDir("", "evg")

	testutil.HandleTestingErr(err, t, "error getting working directory")

	fnames := []string{
		"testFile1",
		"testFile2",
		"testFile.go",
		"testFile2.go",
		"testFile3.yml",
		"built.yml",
		"built.go",
		"built.cpp",
	}
	dirNames := []string{
		"dir1",
		"dir2",
	}
	// create all the files in the current directory
	for _, fname := range fnames {
		_, err := os.Create(filepath.Join(wd, fname))
		testutil.HandleTestingErr(err, t, "error creating test file")
	}

	// create all the files in the sub directories
	for _, dirName := range dirNames {
		err := os.Mkdir(filepath.Join(wd, dirName), 0777)
		testutil.HandleTestingErr(err, t, "error creating test directory")
		for _, fname := range fnames {
			path := filepath.Join(wd, dirName, fname)
			_, err := os.Create(path)
			testutil.HandleTestingErr(err, t, "error creating test file")
		}
	}
	defer func() {
		testutil.HandleTestingErr(os.RemoveAll(wd), t, "error removing test directory")
	}()
	Convey("When files and directories exists", t, func() {
		Convey("With an absolute start dir", func() {
			Convey("Should just match one file when expression is a name", func() {
				files, err := BuildFileList(wd, fnames[0])
				So(err, ShouldBeNil)
				So(files[0], ShouldContainSubstring, fnames[0])
				for i := 1; i < len(fnames); i++ {
					So(files, ShouldNotContain, fnames[i])
				}
			})
			Convey("Should match all files in base directory with prefix", func() {
				files, err := BuildFileList(wd, "/testFile*")
				So(err, ShouldBeNil)
				for i, fname := range fnames {
					if i <= 4 {
						So(files, ShouldContain, fname)
					} else {
						So(files, ShouldNotContain, fname)
					}
				}
			})
			Convey("Should match all files in base directory with suffix", func() {
				files, err := BuildFileList(wd, "/*.go")
				So(err, ShouldBeNil)
				for i, fname := range fnames {
					if i == 2 || i == 3 || i == 6 {
						So(files, ShouldContain, fname)
					} else {
						So(files, ShouldNotContain, fname)
					}
				}
			})
			Convey("Should match all files in a specified sub directory with suffix", func() {
				files, err := BuildFileList(wd, "/dir1/*.go")
				So(err, ShouldBeNil)
				for i, fname := range fnames {
					path := fmt.Sprintf("%s%v%s", "dir1", string(os.PathSeparator), fname)
					if i == 2 || i == 3 || i == 6 {
						So(files, ShouldContain, path)
						So(files, ShouldNotContain, fname)
					} else {
						So(files, ShouldNotContain, path)
						So(files, ShouldNotContain, fname)
					}
				}
			})
			Convey("Should match all files in multiple wildcard sub directory with suffix", func() {
				files, err := BuildFileList(wd, "/*/*.go")
				So(err, ShouldBeNil)
				for i, fname := range fnames {
					for _, dirName := range dirNames {
						path := fmt.Sprintf("%s%v%s", dirName, string(os.PathSeparator), fname)
						if i == 2 || i == 3 || i == 6 {
							So(files, ShouldContain, path)
							So(files, ShouldNotContain, fname)
						} else {
							So(files, ShouldNotContain, path)
							So(files, ShouldNotContain, fname)
						}
					}
				}
			})
		})
	})
}
