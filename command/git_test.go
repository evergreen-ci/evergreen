package git_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/comm"
	agentutil "github.com/evergreen-ci/evergreen/agent/testutil"
	"github.com/evergreen-ci/evergreen/db"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/plugin"
	. "github.com/evergreen-ci/evergreen/plugin/builtin/git"
	"github.com/evergreen-ci/evergreen/plugin/plugintest"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip/slogger"
	"github.com/stretchr/testify/suite"
)

type GitGetProjectSuite struct {
	suite.Suite

	registry   *plugin.SimpleRegistry
	server     *service.TestServer
	modelData1 *modelutil.TestModelData // test model for TestGitPlugin
	modelData2 *modelutil.TestModelData // test model for TestValidateGitCommands
	httpCom    *comm.HTTPCommunicator
	logger     *comm.StreamLogger
}

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testutil.TestConfig()))
}

func TestGitGetProjectSuite(t *testing.T) {
	suite.Run(t, new(GitGetProjectSuite))
}

func (s *GitGetProjectSuite) SetupSuite() {
	testConfig := testutil.TestConfig()
	s.registry = plugin.NewSimpleRegistry()
	gitPlugin := &GitPlugin{}
	err := s.registry.Register(gitPlugin)
	s.NoError(err)
	configPath1 := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "plugin_clone.yml")
	configPath2 := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "test_config.yml")
	patchPath := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "test.patch")
	s.modelData1, err = modelutil.SetupAPITestData(testConfig, "test", "rhel55", configPath1, modelutil.NoPatch)
	s.NoError(err)
	s.modelData2, err = modelutil.SetupAPITestData(testConfig, "test", "rhel55", configPath2, modelutil.NoPatch)
	s.NoError(err)
	//SetupAPITestData always creates BuildVariant with no modules so this line works around that
	s.modelData2.TaskConfig.BuildVariant.Modules = []string{"sample"}
	err = plugintest.SetupPatchData(s.modelData1, patchPath, s.T())
	s.NoError(err)
	s.logger = agentutil.NewTestLogger(slogger.StdOutAppender())
	s.server, err = service.CreateTestServer(testConfig, nil)
	s.NoError(err)
	s.httpCom = plugintest.TestAgentCommunicator(s.modelData1, s.server.URL)
}

func (s *GitGetProjectSuite) TestGitPlugin() {
	taskConfig := s.modelData1.TaskConfig

	for _, task := range taskConfig.Project.Tasks {
		s.NotEqual(len(task.Commands), 0)
		for _, command := range task.Commands {
			pluginCmds, err := s.registry.GetCommands(command, taskConfig.Project.Functions)
			s.NoError(err)
			s.NotNil(pluginCmds)
			pluginCom := &comm.TaskJSONCommunicator{pluginCmds[0].Plugin(), s.httpCom}
			err = pluginCmds[0].Execute(s.logger, pluginCom, taskConfig, make(chan bool))
			s.NoError(err)
		}
	}
}

func (s *GitGetProjectSuite) TestValidateGitCommands() {
	taskConfig := s.modelData2.TaskConfig
	const refToCompare = "cf46076567e4949f9fc68e0634139d4ac495c89b" //note: also defined in test_config.yml

	for _, task := range taskConfig.Project.Tasks {
		for _, command := range task.Commands {
			pluginCmds, err := s.registry.GetCommands(command, taskConfig.Project.Functions)
			s.NoError(err)
			s.NotNil(pluginCmds)
			pluginCom := &comm.TaskJSONCommunicator{pluginCmds[0].Plugin(), s.httpCom}
			err = pluginCmds[0].Execute(s.logger, pluginCom, taskConfig, make(chan bool))
			s.NoError(err)
		}
	}
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = taskConfig.WorkDir + "/src/module/sample/"
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	s.NoError(err)
	ref := strings.Trim(out.String(), "\n") // revision that we actually checked out
	s.Equal(refToCompare, ref)
}

func (s *GitGetProjectSuite) TearDownSuite() {
	s.server.Close()
	if s.modelData1.TaskConfig != nil {
		s.NoError(os.RemoveAll(s.modelData1.TaskConfig.WorkDir))
	}
	if s.modelData2.TaskConfig != nil {
		s.NoError(os.RemoveAll(s.modelData2.TaskConfig.WorkDir))
	}
}
