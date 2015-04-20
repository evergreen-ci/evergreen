package taskrunner

import (
	"10gen.com/mci"
	"10gen.com/mci/command"
	"10gen.com/mci/db"
	"10gen.com/mci/model/host"
	"10gen.com/mci/util"
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

var (
	hostGatewayTestConf = mci.TestConfig()
)

// mock AgentCompiler implementations
type FailingAgentCompiler struct{}

func (self *FailingAgentCompiler) Compile(sourceDir string,
	destinationDir string) error {
	return fmt.Errorf("compile failed")
}

func (self *FailingAgentCompiler) ExecutableSubPath(distroName string) (
	string, error) {
	return "", fmt.Errorf("ExecutableSubPath not implemented")
}

type SucceedingAgentCompiler struct{}

func (self *SucceedingAgentCompiler) Compile(sourceDir string,
	destinationDir string) error {
	return nil
}

func (self *SucceedingAgentCompiler) ExecutableSubPath(distroName string) (
	string, error) {
	return "main", nil
}

func TestAgentBasedHostGateway(t *testing.T) {

	var hostGateway *AgentBasedHostGateway

	SkipConvey("When checking if the agent needs to be built", t, func() {

		hostGateway = &AgentBasedHostGateway{}

		Convey("a non-existent executables directory should cause the agent"+
			" to need to be built", func() {

			hostGateway.ExecutablesDir = "/foo/bar/baz/foo/bar"
			needsBuild, err := hostGateway.AgentNeedsBuild()
			So(err, ShouldBeNil)
			So(needsBuild, ShouldBeTrue)

		})

		Convey("an out-of-date last-built version of the agent should cause"+
			" the agent to need to be built", func() {

			hostGateway.ExecutablesDir = "/" // just needs to exist

			// store the "last built hash" as something different than the
			// current one
			So(db.StoreLastAgentBuild("fedcba"), ShouldBeNil)

			needsBuild, err := hostGateway.AgentNeedsBuild()
			So(err, ShouldBeNil)
			So(needsBuild, ShouldBeTrue)

		})

		Convey("an up-to-date last-built version of the agent should cause"+
			" the agent to not need to be built", func() {

			hostGateway.ExecutablesDir = "/" // just needs to exist

			// compute and cache the current hash of the agent package
			agentHash, err := util.CurrentGitHash(hostGateway.AgentPackageDir)
			So(err, ShouldBeNil)

			// store the "last built hash" as the same as the current one
			So(db.StoreLastAgentBuild(agentHash), ShouldBeNil)

			needsBuild, err := hostGateway.AgentNeedsBuild()
			So(err, ShouldBeNil)
			So(needsBuild, ShouldBeFalse)
		})

	})

	SkipConvey("When building the agent", t, func() {

		hostGateway = &AgentBasedHostGateway{}
		So(db.StoreLastAgentBuild("abcdef"), ShouldBeNil)

		Convey("a nil AgentCompiler should cause a panic", func() {
			So(func() { hostGateway.buildAgent() }, ShouldPanic)
		})

		Convey("a failed compilation should error and cause the build to not"+
			" be recorded as successful", func() {

			// attempt to compile
			hostGateway.currentAgentHash = "fedcba"
			hostGateway.Compiler = &FailingAgentCompiler{}
			So(hostGateway.buildAgent(), ShouldNotBeNil)

			// make sure the last built hash was not updated
			lastBuiltHash, err := db.GetLastAgentBuild()
			So(err, ShouldBeNil)
			So(lastBuiltHash, ShouldEqual, "abcdef")
		})

		Convey("a successful compilation should cause the build to be"+
			" recorded as successful", func() {

			// attempt to compile
			hostGateway.currentAgentHash = "fedcba"
			hostGateway.Compiler = &SucceedingAgentCompiler{}
			So(hostGateway.buildAgent(), ShouldBeNil)

			// make sure the last built hash was updated
			lastBuiltHash, err := db.GetLastAgentBuild()
			So(err, ShouldBeNil)
			So(lastBuiltHash, ShouldEqual, "fedcba")
		})
	})

	Convey("When prepping the remote machine", t, func() {

		Convey("the remote shell should be created, and the config directory"+
			" and agent binaries should be copied over to it", func() {

			hostGateway = &AgentBasedHostGateway{
				Compiler: &SucceedingAgentCompiler{},
			}

			// create a mock config directory, mock executables
			// directory, and mock remote shell
			mciHome, err := mci.FindMCIHome()
			So(err, ShouldBeNil)
			tmpBase := filepath.Join(mciHome, "taskrunner/testdata/tmp")
			mockConfigDir := filepath.Join(tmpBase, "mock_config_dir")
			hostGatewayTestConf.ConfigDir = mockConfigDir
			mockExecutablesDir := filepath.Join(tmpBase, "mock_executables_dir")
			hostGateway.ExecutablesDir = mockExecutablesDir
			mockExecutable := filepath.Join(mockExecutablesDir, "main")
			mockRemoteShell := filepath.Join(tmpBase, "mock_remote_shell")
			mci.RemoteShell = mockRemoteShell

			// prevent permissions issues
			syscall.Umask(0000)

			// remove the directories, if they exist (start clean)
			exists, err := util.FileExists(tmpBase)
			So(err, ShouldBeNil)
			if exists {
				So(os.RemoveAll(tmpBase), ShouldBeNil)
			}
			So(os.MkdirAll(tmpBase, 0777), ShouldBeNil)

			// create the config and executables directories, as well as a
			// mock executable, to copy over
			So(os.Mkdir(mockConfigDir, 0777), ShouldBeNil)
			So(os.Mkdir(mockExecutablesDir, 0777), ShouldBeNil)
			So(ioutil.WriteFile(mockExecutable, []byte("run me"), 0777),
				ShouldBeNil)

			// mock up a host for localhost
			localhost := host.Host{
				Host: command.TestRemote,
				User: command.TestRemoteUser,
			}

			// prep the "remote" host
			_, err = hostGateway.prepRemoteHost(hostGatewayTestConf,
				localhost, []string{"-i", command.TestRemoteKey}, "")
			So(err, ShouldBeNil)

			// make sure the correct files were created and copied over
			exists, err = util.FileExists(filepath.Join(mockRemoteShell,
				"mock_config_dir"))
			So(err, ShouldBeNil)
			So(exists, ShouldBeTrue)
			exists, err = util.FileExists(filepath.Join(mockRemoteShell,
				"main"))
			So(err, ShouldBeNil)
			So(exists, ShouldBeTrue)

		})

	})

}
