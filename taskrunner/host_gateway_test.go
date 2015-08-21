package taskrunner

import (
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

var (
	hostGatewayTestConf = evergreen.TestConfig()
)

// mock AgentCompiler implementations
type FailingAgentCompiler struct{}

func (self *FailingAgentCompiler) Compile(sourceDir string,
	destinationDir string) error {
	return fmt.Errorf("compile failed")
}

func (self *FailingAgentCompiler) ExecutableSubPath(distroId string) (
	string, error) {
	return "", fmt.Errorf("ExecutableSubPath not implemented")
}

type SucceedingAgentCompiler struct{}

func (self *SucceedingAgentCompiler) Compile(sourceDir string,
	destinationDir string) error {
	return nil
}

func (self *SucceedingAgentCompiler) ExecutableSubPath(distroId string) (
	string, error) {
	return "main", nil
}

func TestAgentBasedHostGateway(t *testing.T) {

	var hostGateway *AgentBasedHostGateway

	Convey("When prepping the remote machine", t, func() {

		Convey("the remote shell should be created, and the config directory"+
			" and agent binaries should be copied over to it", func() {

			hostGateway = &AgentBasedHostGateway{
				Compiler: &SucceedingAgentCompiler{},
			}

			// create a mock config directory, mock executables
			// directory, and mock remote shell
			evgHome := evergreen.FindEvergreenHome()
			tmpBase := filepath.Join(evgHome, "taskrunner/testdata/tmp")
			mockConfigDir := filepath.Join(tmpBase, "mock_config_dir")
			hostGatewayTestConf.ConfigDir = mockConfigDir
			mockExecutablesDir := filepath.Join(tmpBase, "mock_executables_dir")
			hostGateway.ExecutablesDir = mockExecutablesDir
			mockExecutable := filepath.Join(mockExecutablesDir, "main")
			mockRemoteShell := filepath.Join(tmpBase, "mock_remote_shell")
			evergreen.RemoteShell = mockRemoteShell

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
