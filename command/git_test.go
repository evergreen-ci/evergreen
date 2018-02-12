package command

import (
	"bytes"
	"context"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/plugin/plugintest"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/smartystreets/goconvey/convey/reporting"
	"github.com/stretchr/testify/suite"
)

type GitGetProjectSuite struct {
	suite.Suite

	modelData1 *modelutil.TestModelData // test model for TestGitPlugin
	modelData2 *modelutil.TestModelData // test model for TestValidateGitCommands
}

func init() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	reporting.QuietMode()
}

func TestGitGetProjectSuite(t *testing.T) {
	suite.Run(t, new(GitGetProjectSuite))
}

func (s *GitGetProjectSuite) SetupTest() {
	var err error
	testConfig := testutil.TestConfig()
	s.NoError(err)
	configPath1 := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "git", "plugin_clone.yml")
	configPath2 := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "git", "test_config.yml")
	patchPath := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "git", "test.patch")
	s.modelData1, err = modelutil.SetupAPITestData(testConfig, "test", "rhel55", configPath1, modelutil.NoPatch)
	s.NoError(err)

	s.modelData2, err = modelutil.SetupAPITestData(testConfig, "test", "rhel55", configPath2, modelutil.NoPatch)
	s.NoError(err)
	//SetupAPITestData always creates BuildVariant with no modules so this line works around that
	s.modelData2.TaskConfig.BuildVariant.Modules = []string{"sample"}
	err = plugintest.SetupPatchData(s.modelData1, patchPath, s.T())
	s.NoError(err)
}

func (s *GitGetProjectSuite) TestGitPlugin() {
	conf := s.modelData1.TaskConfig
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	comm := client.NewMock("http://localhost.com")
	logger := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret})

	for _, task := range conf.Project.Tasks {
		s.NotEqual(len(task.Commands), 0)
		for _, command := range task.Commands {

			pluginCmds, err := Render(command, conf.Project.Functions)
			s.NoError(err)
			s.NotNil(pluginCmds)
			err = pluginCmds[0].Execute(ctx, comm, logger, conf)
			s.NoError(err)
		}
	}
}

func (s *GitGetProjectSuite) TestValidateGitCommands() {
	const refToCompare = "cf46076567e4949f9fc68e0634139d4ac495c89b" //note: also defined in test_config.yml

	conf := s.modelData2.TaskConfig
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	comm := client.NewMock("http://localhost.com")
	logger := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret})

	for _, task := range conf.Project.Tasks {
		for _, command := range task.Commands {
			pluginCmds, err := Render(command, conf.Project.Functions)
			s.NoError(err)
			s.NotNil(pluginCmds)
			err = pluginCmds[0].Execute(ctx, comm, logger, conf)
			s.NoError(err)
		}
	}
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = conf.WorkDir + "/src/module/sample/"
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	s.NoError(err)
	ref := strings.Trim(out.String(), "\n") // revision that we actually checked out
	s.Equal(refToCompare, ref)
}

func (s *GitGetProjectSuite) TestBuildHTTPCloneCommand() {
	projectRef := &model.ProjectRef{
		Owner:  "deafgoat",
		Repo:   "mci_test",
		Branch: "master",
	}

	// build clone command to clone by http, master branch with token into 'dir'
	location, err := projectRef.HTTPLocation()
	s.Require().NoError(err)
	cmds, err := buildHTTPCloneCommand(location, projectRef.Branch, "dir", "GITHUBTOKEN")
	s.NoError(err)
	s.Require().Len(cmds, 5)
	s.Equal("set +o xtrace", cmds[0])
	s.Equal("echo \"GIT_ASKPASS='true' git -c '[redacted oauth token]' clone 'https://github.com/deafgoat/mci_test.git' 'dir' --branch 'master'\"", cmds[1])
	s.Equal("GIT_ASKPASS='true' git -c 'credential.https://github.com.username=GITHUBTOKEN' clone 'https://github.com/deafgoat/mci_test.git' 'dir' --branch 'master'", cmds[2])
	s.Equal("set -o xtrace", cmds[3])
	s.Equal("cd dir", cmds[4])

	// build clone command to clone by http with token into 'dir' w/o specified branch
	location, err = projectRef.HTTPLocation()
	s.Require().NoError(err)
	cmds, err = buildHTTPCloneCommand(location, "", "dir", "GITHUBTOKEN")
	s.NoError(err)
	s.Require().Len(cmds, 5)
	s.Equal("set +o xtrace", cmds[0])
	s.Equal("echo \"GIT_ASKPASS='true' git -c '[redacted oauth token]' clone 'https://github.com/deafgoat/mci_test.git' 'dir'\"", cmds[1])
	s.Equal("GIT_ASKPASS='true' git -c 'credential.https://github.com.username=GITHUBTOKEN' clone 'https://github.com/deafgoat/mci_test.git' 'dir'", cmds[2])
	s.Equal("set -o xtrace", cmds[3])
	s.Equal("cd dir", cmds[4])

	// build clone command with a URL that uses http, and ensure it's
	// been forced to use https
	location, err = url.Parse("http://github.com/deafgoat/mci_test.git")
	s.Require().NoError(err)
	s.Require().NotNil(location)
	cmds, err = buildHTTPCloneCommand(location, projectRef.Branch, "dir", "GITHUBTOKEN")
	s.NoError(err)
	s.Require().Len(cmds, 5)
	s.Equal("echo \"GIT_ASKPASS='true' git -c '[redacted oauth token]' clone 'https://github.com/deafgoat/mci_test.git' 'dir' --branch 'master'\"", cmds[1])
	s.Equal("GIT_ASKPASS='true' git -c 'credential.https://github.com.username=GITHUBTOKEN' clone 'https://github.com/deafgoat/mci_test.git' 'dir' --branch 'master'", cmds[2])
	s.Equal("https", location.Scheme)

	// ensure that we aren't sending the github oauth token to other
	// servers
	location, err = url.Parse("http://someothergithost.com/something/else.git")
	s.Require().NoError(err)
	s.Require().NotNil(location)
	cmds, err = buildHTTPCloneCommand(location, projectRef.Branch, "dir", "")
	s.NoError(err)
	s.Require().Len(cmds, 5)
	s.Equal("echo \"GIT_ASKPASS='true' git  clone 'https://someothergithost.com/something/else.git' 'dir' --branch 'master'\"", cmds[1])
	s.Equal("GIT_ASKPASS='true' git  clone 'https://someothergithost.com/something/else.git' 'dir' --branch 'master'", cmds[2])
	s.Equal("https", location.Scheme)
}

func (s *GitGetProjectSuite) TestBuildSSHCloneCommand() {
	projectRef := &model.ProjectRef{
		Owner:  "deafgoat",
		Repo:   "mci_test",
		Branch: "master",
	}

	// ssh clone command with branch
	location, err := projectRef.Location()
	s.NoError(err)
	cmds, err := buildSSHCloneCommand(location, projectRef.Branch, "dir")
	s.NoError(err)
	s.Len(cmds, 2)
	s.Equal("git clone 'git@github.com:deafgoat/mci_test.git' 'dir' --branch 'master'", cmds[0])
	s.Equal("cd dir", cmds[1])

	// ssh clone command without branch
	projectRef.Branch = ""
	location, err = projectRef.Location()
	s.NoError(err)
	cmds, err = buildSSHCloneCommand(location, projectRef.Branch, "dir")
	s.NoError(err)
	s.Len(cmds, 2)
	s.Equal("git clone 'git@github.com:deafgoat/mci_test.git' 'dir'", cmds[0])
	s.Equal("cd dir", cmds[1])
}

func (s *GitGetProjectSuite) TestBuildCommand() {
	conf := s.modelData1.TaskConfig

	c := gitFetchProject{
		Directory: "dir",
	}

	// ensure clone command without specified token uses ssh
	cmds, err := c.buildCloneCommand(conf)
	s.NoError(err)
	s.Require().Len(cmds, 6)
	s.Equal("set -o xtrace", cmds[0])
	s.Equal("set -o errexit", cmds[1])
	s.Equal("rm -rf dir", cmds[2])
	s.Equal("git clone 'git@github.com:deafgoat/mci_test.git' 'dir' --branch 'master'", cmds[3])
	s.Equal("cd dir", cmds[4])
	s.Equal("git reset --hard ", cmds[5])

	// ensure clone command with a token uses http
	c.Token = "GITHUBTOKEN"
	cmds, err = c.buildCloneCommand(conf)
	s.NoError(err)
	s.Require().Len(cmds, 9)
	s.Equal("set -o xtrace", cmds[0])
	s.Equal("set -o errexit", cmds[1])
	s.Equal("rm -rf dir", cmds[2])
	s.Equal("set +o xtrace", cmds[3])
	s.Equal("echo \"GIT_ASKPASS='true' git -c '[redacted oauth token]' clone 'https://github.com/deafgoat/mci_test.git' 'dir' --branch 'master'\"", cmds[4])
	s.Equal("GIT_ASKPASS='true' git -c 'credential.https://github.com.username=GITHUBTOKEN' clone 'https://github.com/deafgoat/mci_test.git' 'dir' --branch 'master'", cmds[5])
	s.Equal("set -o xtrace", cmds[6])
	s.Equal("cd dir", cmds[7])
	s.Equal("git reset --hard ", cmds[8])

	// ensure clone command cannot be built if projectref has no owner
	conf.ProjectRef.Owner = ""
	cmds, err = c.buildCloneCommand(conf)
	s.Error(err)
	s.Nil(cmds)
}

func (s *GitGetProjectSuite) TestBuildModuleCommand() {
	c := gitFetchProject{
		Directory: "dir",
		Token:     "GITHUBTOKEN",
	}

	// ensure module clone command with ssh URL does not inject token
	cmds, err := c.buildModuleCloneCommand("git@github.com:deafgoat/mci_test.git", "module", "master")
	s.NoError(err)
	s.Require().Len(cmds, 5)
	s.Equal("set -o xtrace", cmds[0])
	s.Equal("set -o errexit", cmds[1])
	s.Equal("git clone 'git@github.com:deafgoat/mci_test.git' 'module'", cmds[2])
	s.Equal("cd module", cmds[3])
	s.Equal("git checkout 'master'", cmds[4])

	// ensure module clone command with http URL injects token
	cmds, err = c.buildModuleCloneCommand("https://github.com/deafgoat/mci_test.git", "module", "master")
	s.NoError(err)
	s.Require().Len(cmds, 8)
	s.Equal("set -o xtrace", cmds[0])
	s.Equal("set -o errexit", cmds[1])
	s.Equal("set +o xtrace", cmds[2])
	s.Equal("echo \"GIT_ASKPASS='true' git -c '[redacted oauth token]' clone 'https://github.com/deafgoat/mci_test.git' 'module'\"", cmds[3])
	s.Equal("GIT_ASKPASS='true' git -c 'credential.https://github.com.username=GITHUBTOKEN' clone 'https://github.com/deafgoat/mci_test.git' 'module'", cmds[4])
	s.Equal("set -o xtrace", cmds[5])
	s.Equal("cd module", cmds[6])
	s.Equal("git checkout 'master'", cmds[7])

	// ensure insecure github url is force to use https
	cmds, err = c.buildModuleCloneCommand("http://github.com/deafgoat/mci_test.git", "module", "master")
	s.NoError(err)
	s.Require().Len(cmds, 8)
	s.Equal("echo \"GIT_ASKPASS='true' git -c '[redacted oauth token]' clone 'https://github.com/deafgoat/mci_test.git' 'module'\"", cmds[3])
	s.Equal("GIT_ASKPASS='true' git -c 'credential.https://github.com.username=GITHUBTOKEN' clone 'https://github.com/deafgoat/mci_test.git' 'module'", cmds[4])
}

func (s *GitGetProjectSuite) TestIsMailboxPatch() {
	isMBP, err := isMailboxPatch(filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "filethatdoesntexist.txt"))
	s.Error(err)
	s.False(isMBP)

	isMBP, err = isMailboxPatch(filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "test.patch"))
	s.NoError(err)
	s.True(isMBP)

	isMBP, err = isMailboxPatch(filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "test.diff"))
	s.NoError(err)
	s.False(isMBP)

	isMBP, err = isMailboxPatch(filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "emptyfile.txt"))
	s.Error(err)
	s.Equal("patch file appears to be empty", err.Error())
	s.False(isMBP)
}

func (s *GitGetProjectSuite) TearDownSuite() {
	if s.modelData1.TaskConfig != nil {
		s.NoError(os.RemoveAll(s.modelData1.TaskConfig.WorkDir))
	}
	if s.modelData2.TaskConfig != nil {
		s.NoError(os.RemoveAll(s.modelData2.TaskConfig.WorkDir))
	}
}
