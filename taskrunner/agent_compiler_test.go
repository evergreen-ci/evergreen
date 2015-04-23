package taskrunner

import (
	"10gen.com/mci"
	"10gen.com/mci/util"
	. "github.com/smartystreets/goconvey/convey"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoxcAgentCompiler(t *testing.T) {

	var agentCompiler *GoxcAgentCompiler

	SkipConvey("When compiling the agent using goxc", t, func() {

		agentCompiler = &GoxcAgentCompiler{}

		Convey("binaries for the specified os targets and architectures"+
			" should be built in the specified destination directory", func() {

			// create a fake go env for building
			mciHome, err := mci.FindMCIHome()
			So(err, ShouldBeNil)
			mockGoPath := filepath.Join(mciHome,
				"src/10gen.com/mci/taskrunner/testdata/tmp/mockGoPath/")
			srcDir := filepath.Join(mockGoPath, "src")
			So(os.MkdirAll(srcDir, 0777), ShouldBeNil)
			binDir := filepath.Join(mockGoPath, "bin")
			So(os.MkdirAll(binDir, 0777), ShouldBeNil)
			pkgDir := filepath.Join(mockGoPath, "pkg")
			So(os.MkdirAll(pkgDir, 0777), ShouldBeNil)

			// add it to the GOPATH
			os.Setenv("GOPATH", os.Getenv("GOPATH")+":"+mockGoPath)

			// create a buildable main package inside of the source dir
			mainPackage := filepath.Join(srcDir, "main")
			So(os.MkdirAll(mainPackage, 0777), ShouldBeNil)
			fileContents := "package main\nimport ()\nfunc main(){}"
			So(ioutil.WriteFile(filepath.Join(mainPackage, "build_me.go"),
				[]byte(fileContents), 0777), ShouldBeNil)

			// create the destination directory
			dstDir := filepath.Join(mockGoPath, "executables")

			// compile the source package
			So(agentCompiler.Compile(mainPackage, dstDir), ShouldBeNil)

			// make sure all of the necessary main files were created
			for _, execType := range []string{"darwin_386", "darwin_amd64",
				"linux_386", "linux_amd64", "windows_386", "windows_amd64"} {

				// windows binaries have .exe at the end
				binFile := "main"
				if strings.HasPrefix(execType, "windows") {
					binFile += ".exe"
				}

				// make sure the binary exists
				exists, err := util.FileExists(filepath.Join(dstDir,
					"snapshot", execType, binFile))
				So(err, ShouldBeNil)
				So(exists, ShouldBeTrue)
			}

			// clean up, by removing the fake go path
			So(os.RemoveAll(mockGoPath), ShouldBeNil)

		})

	})
}
