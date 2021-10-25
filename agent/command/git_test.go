package command

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	agentutil "github.com/evergreen-ci/evergreen/agent/internal/testutil"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper"
	"github.com/smartystreets/goconvey/convey/reporting"
	"github.com/stretchr/testify/suite"
)

const (
	globalGitHubToken  = "GLOBALTOKEN"
	projectGitHubToken = "PROJECTTOKEN"
)

type GitGetProjectSuite struct {
	settings    *evergreen.Settings
	jasper      jasper.Manager
	modelData1  *modelutil.TestModelData // test model for TestGitPlugin
	taskConfig1 *internal.TaskConfig
	modelData2  *modelutil.TestModelData // test model for TestValidateGitCommands
	taskConfig2 *internal.TaskConfig
	modelData3  *modelutil.TestModelData
	taskConfig3 *internal.TaskConfig
	modelData4  *modelutil.TestModelData
	taskConfig4 *internal.TaskConfig
	modelData5  *modelutil.TestModelData
	taskConfig5 *internal.TaskConfig
	modelData6  *modelutil.TestModelData
	taskConfig6 *internal.TaskConfig // used for TestMergeMultiplePatches

	suite.Suite
}

func init() {
	reporting.QuietMode()
}

func TestGitGetProjectSuite(t *testing.T) {
	s := new(GitGetProjectSuite)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	settings := env.Settings()

	testutil.ConfigureIntegrationTest(t, settings, "TestGitGetProjectSuite")
	s.settings = settings
	var err error
	s.jasper, err = jasper.NewSynchronizedManager(false)
	s.Require().NoError(err)
	suite.Run(t, s)
}

func (s *GitGetProjectSuite) SetupTest() {
	s.NoError(db.ClearCollections(patch.Collection, build.Collection, task.Collection,
		model.VersionCollection, host.Collection, model.TaskLogCollection))
	var err error
	configPath1 := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "git", "plugin_clone.yml")
	configPath2 := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "git", "test_config.yml")
	configPath3 := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "git", "no_token.yml")
	configPath4 := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "git", "additional_patch.yml")
	patchPath := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "git", "test.patch")

	s.modelData1, err = modelutil.SetupAPITestData(s.settings, "testtask1", "rhel55", configPath1, modelutil.NoPatch)
	s.Require().NoError(err)
	s.taskConfig1, err = agentutil.MakeTaskConfigFromModelData(s.settings, s.modelData1)
	s.Require().NoError(err)
	s.taskConfig1.Expansions = util.NewExpansions(map[string]string{evergreen.GlobalGitHubTokenExpansion: fmt.Sprintf("token " + globalGitHubToken)})
	s.Require().NoError(err)

	s.modelData2, err = modelutil.SetupAPITestData(s.settings, "testtask1", "rhel55", configPath2, modelutil.NoPatch)
	s.Require().NoError(err)
	s.taskConfig2, err = agentutil.MakeTaskConfigFromModelData(s.settings, s.modelData2)
	s.Require().NoError(err)
	s.taskConfig2.Expansions = util.NewExpansions(s.settings.Credentials)
	s.taskConfig2.Expansions.Put("prefixpath", "hello")
	//SetupAPITestData always creates BuildVariant with no modules so this line works around that
	s.taskConfig2.BuildVariant.Modules = []string{"sample"}
	s.modelData2.Task.Requester = evergreen.PatchVersionRequester
	err = setupTestPatchData(s.modelData1, patchPath, s.T())
	s.Require().NoError(err)

	s.modelData3, err = modelutil.SetupAPITestData(s.settings, "testtask1", "rhel55", configPath2, modelutil.NoPatch)
	s.Require().NoError(err)
	s.taskConfig3, err = agentutil.MakeTaskConfigFromModelData(s.settings, s.modelData3)
	s.Require().NoError(err)
	s.taskConfig3.Expansions = util.NewExpansions(s.settings.Credentials)
	s.taskConfig3.GithubPatchData = thirdparty.GithubPatch{
		PRNumber:   9001,
		BaseOwner:  "evergreen-ci",
		BaseRepo:   "evergreen",
		BaseBranch: "main",
		HeadOwner:  "octocat",
		HeadRepo:   "evergreen",
		HeadHash:   "55ca6286e3e4f4fba5d0448333fa99fc5a404a73",
		Author:     "octocat",
	}

	s.modelData4, err = modelutil.SetupAPITestData(s.settings, "testtask1", "rhel55", configPath2, modelutil.MergePatch)
	s.Require().NoError(err)
	s.taskConfig4, err = agentutil.MakeTaskConfigFromModelData(s.settings, s.modelData4)
	s.Require().NoError(err)
	s.taskConfig4.Expansions = util.NewExpansions(s.settings.Credentials)
	s.taskConfig4.GithubPatchData = thirdparty.GithubPatch{
		PRNumber:       9001,
		MergeCommitSHA: "abcdef",
	}
	s.modelData5, err = modelutil.SetupAPITestData(s.settings, "testtask1", "rhel55", configPath3, modelutil.MergePatch)
	s.Require().NoError(err)
	s.taskConfig5, err = agentutil.MakeTaskConfigFromModelData(s.settings, s.modelData5)
	s.Require().NoError(err)

	s.modelData6, err = modelutil.SetupAPITestData(s.settings, "testtask1", "linux-64", configPath4, modelutil.InlinePatch)
	s.Require().NoError(err)
	s.modelData6.Task.Requester = evergreen.MergeTestRequester
	s.Require().NoError(err)
	s.taskConfig6, err = agentutil.MakeTaskConfigFromModelData(s.settings, s.modelData6)
	s.Require().NoError(err)
	s.taskConfig6.Expansions = util.NewExpansions(map[string]string{evergreen.GlobalGitHubTokenExpansion: fmt.Sprintf("token " + globalGitHubToken)})
	s.taskConfig6.BuildVariant.Modules = []string{"evergreen"}
}

func (s *GitGetProjectSuite) TestBuildCloneCommandUsesHTTPS() {
	c := &gitFetchProject{
		Directory: "dir",
		Token:     projectGitHubToken,
	}
	conf := s.taskConfig1

	opts := cloneOpts{
		method: distro.CloneMethodOAuth,
		owner:  conf.ProjectRef.Owner,
		repo:   conf.ProjectRef.Repo,
		branch: conf.ProjectRef.Branch,
		dir:    c.Directory,
		token:  c.Token,
	}
	s.Require().NoError(opts.setLocation())
	cmds, _ := c.buildCloneCommand(context.Background(), conf, nil, opts)
	s.Equal("git clone https://PROJECTTOKEN:x-oauth-basic@github.com/deafgoat/mci_test.git 'dir' --branch 'master'", cmds[5])
}

func (s *GitGetProjectSuite) TestBuildCloneCommandWithHTTPSNeedsToken() {
	c := &gitFetchProject{
		Directory: "dir",
	}
	conf := s.taskConfig1

	opts := cloneOpts{
		method: distro.CloneMethodOAuth,
		owner:  conf.ProjectRef.Owner,
		repo:   conf.ProjectRef.Repo,
		branch: conf.ProjectRef.Branch,
		dir:    c.Directory,
		token:  "",
	}
	s.Require().NoError(opts.setLocation())
	_, err := c.buildCloneCommand(context.Background(), conf, nil, opts)
	s.Error(err)
}

func (s *GitGetProjectSuite) TestBuildCloneCommandUsesSSH() {
	c := &gitFetchProject{
		Directory: "dir",
		Token:     "",
	}
	conf := s.taskConfig2

	opts := cloneOpts{
		method: distro.CloneMethodLegacySSH,
		owner:  "deafgoat",
		repo:   "mci_test",
		branch: "main",
		dir:    c.Directory,
	}
	s.Require().NoError(opts.setLocation())
	cmds, err := c.buildCloneCommand(context.Background(), conf, nil, opts)
	s.Require().NoError(err)
	s.Equal("git clone 'git@github.com:deafgoat/mci_test.git' 'dir' --branch 'main'", cmds[3])
}

func (s *GitGetProjectSuite) TestBuildCloneCommandDefaultCloneMethodUsesSSH() {
	c := &gitFetchProject{
		Directory: "dir",
	}
	conf := s.taskConfig2

	opts := cloneOpts{
		owner:  conf.ProjectRef.Owner,
		repo:   conf.ProjectRef.Repo,
		branch: conf.ProjectRef.Branch,
		dir:    c.Directory,
	}
	s.Require().NoError(opts.setLocation())
	cmds, err := c.buildCloneCommand(context.Background(), conf, nil, opts)
	s.Require().NoError(err)
	s.Equal("git clone 'git@github.com:evergreen-ci/sample.git' 'dir' --branch 'main'", cmds[3])
}

func (s *GitGetProjectSuite) TestGitPlugin() {
	conf := s.taskConfig1
	token, err := s.settings.GetGithubOauthToken()
	s.Require().NoError(err)
	conf.Expansions.Put("github", token)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	comm := client.NewMock("http://localhost.com")
	logger, err := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, nil)
	s.NoError(err)

	for _, task := range conf.Project.Tasks {
		s.NotEqual(len(task.Commands), 0)
		for _, command := range task.Commands {
			pluginCmds, err := Render(command, conf.Project.Functions)
			s.NoError(err)
			s.NotNil(pluginCmds)
			pluginCmds[0].SetJasperManager(s.jasper)
			err = pluginCmds[0].Execute(ctx, comm, logger, conf)
			s.NoError(err)
		}
	}
}

func (s *GitGetProjectSuite) TestGitFetchRetries() {
	c := gitFetchProject{Directory: "dir"}

	conf := s.taskConfig1
	conf.Distro.CloneMethod = "this is not real!"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	comm := client.NewMock("http://localhost.com")
	logger, err := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, nil)
	s.NoError(err)

	err = c.Execute(ctx, comm, logger, conf)
	s.Error(err)
}

func (s *GitGetProjectSuite) TestTokenScrubbedFromLogger() {
	conf := s.taskConfig1
	conf.ProjectRef.Repo = "doesntexist"
	conf.Distro.CloneMethod = distro.CloneMethodOAuth
	token, err := s.settings.GetGithubOauthToken()
	s.Require().NoError(err)
	conf.Expansions.Put(evergreen.GlobalGitHubTokenExpansion, token)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	comm := client.NewMock("http://localhost.com")
	logger, err := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, nil)
	s.NoError(err)

	for _, task := range conf.Project.Tasks {
		s.NotEqual(len(task.Commands), 0)
		for _, command := range task.Commands {
			pluginCmds, err := Render(command, conf.Project.Functions)
			s.NoError(err)
			s.NotNil(pluginCmds)
			pluginCmds[0].SetJasperManager(s.jasper)
			err = pluginCmds[0].Execute(ctx, comm, logger, conf)
			s.Error(err)
		}
	}

	s.NoError(logger.Close())
	foundCloneCommand := false
	foundCloneErr := false
	for _, msgs := range comm.GetMockMessages() {
		for _, msg := range msgs {
			if strings.Contains(msg.Message, "https://[redacted oauth token]:x-oauth-basic@github.com/deafgoat/doesntexist.git") {
				foundCloneCommand = true
			}
			if strings.Contains(msg.Message, "Repository not found.") {
				foundCloneErr = true
			}
			if strings.Contains(msg.Message, token) {
				s.FailNow("token was leaked")
			}
		}
	}
	s.True(foundCloneCommand)
	s.True(foundCloneErr)
}

func (s *GitGetProjectSuite) TestStdErrLogged() {
	if os.Getenv("IS_DOCKER") == "true" {
		s.T().Skip("TestStdErrLogged will not run on docker since it requires a SSH key")
	}
	conf := s.taskConfig5
	conf.Distro.CloneMethod = distro.CloneMethodLegacySSH
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	comm := client.NewMock("http://localhost.com")
	logger, err := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, nil)
	s.NoError(err)

	for _, task := range conf.Project.Tasks {
		s.NotEqual(len(task.Commands), 0)
		for _, command := range task.Commands {
			pluginCmds, err := Render(command, conf.Project.Functions)
			s.NoError(err)
			s.NotNil(pluginCmds)
			pluginCmds[0].SetJasperManager(s.jasper)
			err = pluginCmds[0].Execute(ctx, comm, logger, conf)
			s.Error(err)
		}
	}

	s.NoError(logger.Close())
	foundCloneCommand := false
	foundCloneErr := false
	foundSSHErr := false
	for _, msgs := range comm.GetMockMessages() {
		for _, msg := range msgs {
			if strings.Contains(msg.Message, "git clone 'git@github.com:evergreen-ci/doesntexist.git' 'src' --branch 'main'") {
				foundCloneCommand = true
			}
			if strings.Contains(msg.Message, "ERROR: Repository not found.") {
				foundCloneErr = true
			}
			if strings.Contains(msg.Message, "Permission denied (publickey)") || strings.Contains(msg.Message, "Host key verification failed.") {
				foundSSHErr = true
			}
		}
	}
	s.True(foundCloneCommand)
	s.True(foundCloneErr || foundSSHErr)
}

func (s *GitGetProjectSuite) TestValidateGitCommands() {
	const refToCompare = "cf46076567e4949f9fc68e0634139d4ac495c89b" //note: also defined in test_config.yml

	conf := s.taskConfig2
	conf.Distro.CloneMethod = distro.CloneMethodOAuth
	token, err := s.settings.GetGithubOauthToken()
	s.Require().NoError(err)
	conf.Expansions.Put(evergreen.GlobalGitHubTokenExpansion, token)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	comm := client.NewMock("http://localhost.com")
	logger, err := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, nil)
	s.NoError(err)
	var pluginCmds []Command

	for _, task := range conf.Project.Tasks {
		for _, command := range task.Commands {
			pluginCmds, err = Render(command, conf.Project.Functions)
			s.NoError(err)
			s.NotNil(pluginCmds)
			pluginCmds[0].SetJasperManager(s.jasper)
			err = pluginCmds[0].Execute(ctx, comm, logger, conf)
			s.NoError(err)
		}
	}
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = conf.WorkDir + "/src/hello/module/sample/"
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	s.NoError(err)
	ref := strings.Trim(out.String(), "\n") // revision that we actually checked out
	s.Equal(refToCompare, ref)
	s.Equal("hello/module", conf.ModulePaths["sample"])
}

func (s *GitGetProjectSuite) TestBuildHTTPCloneCommand() {
	projectRef := &model.ProjectRef{
		Owner:  "deafgoat",
		Repo:   "mci_test",
		Branch: "main",
	}

	// build clone command to clone by http, main branch with token into 'dir'
	opts := cloneOpts{
		method: distro.CloneMethodOAuth,
		owner:  projectRef.Owner,
		repo:   projectRef.Repo,
		branch: projectRef.Branch,
		dir:    "dir",
		token:  projectGitHubToken,
	}
	s.Require().NoError(opts.setLocation())
	cmds, err := opts.buildHTTPCloneCommand()
	s.NoError(err)
	s.Require().Len(cmds, 5)
	s.Equal("set +o xtrace", cmds[0])
	s.Equal("echo \"git clone https://[redacted oauth token]:x-oauth-basic@github.com/deafgoat/mci_test.git 'dir' --branch 'main'\"", cmds[1])
	s.Equal("git clone https://PROJECTTOKEN:x-oauth-basic@github.com/deafgoat/mci_test.git 'dir' --branch 'main'", cmds[2])
	s.Equal("set -o xtrace", cmds[3])
	s.Equal("cd dir", cmds[4])

	// build clone command to clone by http with token into 'dir' w/o specified branch
	opts.branch = ""
	cmds, err = opts.buildHTTPCloneCommand()
	s.NoError(err)
	s.Require().Len(cmds, 5)
	s.Equal("set +o xtrace", cmds[0])
	s.Equal("echo \"git clone https://[redacted oauth token]:x-oauth-basic@github.com/deafgoat/mci_test.git 'dir'\"", cmds[1])
	s.Equal("git clone https://PROJECTTOKEN:x-oauth-basic@github.com/deafgoat/mci_test.git 'dir'", cmds[2])
	s.Equal("set -o xtrace", cmds[3])
	s.Equal("cd dir", cmds[4])

	// build clone command with a URL that uses http, and ensure it's
	// been forced to use https
	opts.location = "http://github.com/deafgoat/mci_test.git"
	opts.branch = projectRef.Branch
	cmds, err = opts.buildHTTPCloneCommand()
	s.NoError(err)
	s.Require().Len(cmds, 5)
	s.Equal("echo \"git clone https://[redacted oauth token]:x-oauth-basic@github.com/deafgoat/mci_test.git 'dir' --branch 'main'\"", cmds[1])
	s.Equal("git clone https://PROJECTTOKEN:x-oauth-basic@github.com/deafgoat/mci_test.git 'dir' --branch 'main'", cmds[2])

	// ensure that we aren't sending the github oauth token to other
	// servers
	opts.location = "http://someothergithost.com/something/else.git"
	cmds, err = opts.buildHTTPCloneCommand()
	s.NoError(err)
	s.Require().Len(cmds, 5)
	s.Equal("echo \"git clone https://[redacted oauth token]:x-oauth-basic@someothergithost.com/deafgoat/mci_test.git 'dir' --branch 'main'\"", cmds[1])
	s.Equal("git clone https://PROJECTTOKEN:x-oauth-basic@someothergithost.com/deafgoat/mci_test.git 'dir' --branch 'main'", cmds[2])
}

func (s *GitGetProjectSuite) TestBuildSSHCloneCommand() {
	// ssh clone command with branch
	opts := cloneOpts{
		method: distro.CloneMethodLegacySSH,
		owner:  "deafgoat",
		repo:   "mci_test",
		branch: "main",
		dir:    "dir",
	}
	s.Require().NoError(opts.setLocation())
	cmds, err := opts.buildSSHCloneCommand()
	s.NoError(err)
	s.Len(cmds, 2)
	s.Equal("git clone 'git@github.com:deafgoat/mci_test.git' 'dir' --branch 'main'", cmds[0])
	s.Equal("cd dir", cmds[1])

	// ssh clone command without branch
	opts.branch = ""
	cmds, err = opts.buildSSHCloneCommand()
	s.NoError(err)
	s.Len(cmds, 2)
	s.Equal("git clone 'git@github.com:deafgoat/mci_test.git' 'dir'", cmds[0])
	s.Equal("cd dir", cmds[1])
}

func (s *GitGetProjectSuite) TestBuildCommand() {
	conf := s.taskConfig1

	c := gitFetchProject{
		Directory: "dir",
		Token:     projectGitHubToken,
	}

	// ensure clone command with legacy SSH contains "git@github.com"
	opts := cloneOpts{
		method: distro.CloneMethodLegacySSH,
		branch: conf.ProjectRef.Branch,
		owner:  conf.ProjectRef.Owner,
		repo:   conf.ProjectRef.Repo,
		dir:    c.Directory,
	}
	s.Require().NoError(opts.setLocation())
	cmds, err := c.buildCloneCommand(context.Background(), conf, nil, opts)
	s.NoError(err)
	s.Require().Len(cmds, 7)
	s.Equal("set -o xtrace", cmds[0])
	s.Equal("set -o errexit", cmds[1])
	s.Equal("rm -rf dir", cmds[2])
	s.Equal("git clone 'git@github.com:deafgoat/mci_test.git' 'dir' --branch 'master'", cmds[3])
	s.Equal("cd dir", cmds[4])
	s.Equal("git reset --hard ", cmds[5])
	s.Equal("git log --oneline -n 10", cmds[6])

	// ensure clone command with location containing "https://github.com" uses
	// HTTPS.
	opts.method = distro.CloneMethodOAuth
	opts.token = c.Token
	s.Require().NoError(opts.setLocation())
	s.Require().NoError(err)
	cmds, err = c.buildCloneCommand(context.Background(), conf, nil, opts)
	s.NoError(err)
	s.Require().Len(cmds, 10)
	s.Equal("set -o xtrace", cmds[0])
	s.Equal("set -o errexit", cmds[1])
	s.Equal("rm -rf dir", cmds[2])
	s.Equal("set +o xtrace", cmds[3])
	s.Equal("echo \"git clone https://[redacted oauth token]:x-oauth-basic@github.com/deafgoat/mci_test.git 'dir' --branch 'master'\"", cmds[4])
	s.Equal("git clone https://PROJECTTOKEN:x-oauth-basic@github.com/deafgoat/mci_test.git 'dir' --branch 'master'", cmds[5])
	s.Equal("set -o xtrace", cmds[6])
	s.Equal("cd dir", cmds[7])
	s.Equal("git reset --hard ", cmds[8])
	s.Equal("git log --oneline -n 10", cmds[9])
}

func (s *GitGetProjectSuite) TestBuildCommandForPullRequests() {
	conf := s.taskConfig3
	c := gitFetchProject{
		Directory: "dir",
	}

	opts := cloneOpts{
		method: distro.CloneMethodLegacySSH,
		branch: conf.ProjectRef.Branch,
		owner:  conf.ProjectRef.Owner,
		repo:   conf.ProjectRef.Repo,
		dir:    c.Directory,
	}
	s.Require().NoError(opts.setLocation())

	cmds, err := c.buildCloneCommand(context.Background(), conf, nil, opts)
	s.NoError(err)
	s.Require().Len(cmds, 9)
	s.True(strings.HasPrefix(cmds[5], "git fetch origin \"pull/9001/head:evg-pr-test-"))
	s.True(strings.HasPrefix(cmds[6], "git checkout \"evg-pr-test-"))
	s.Equal("git reset --hard 55ca6286e3e4f4fba5d0448333fa99fc5a404a73", cmds[7])
	s.Equal("git log --oneline -n 10", cmds[8])
}

func (s *GitGetProjectSuite) TestBuildCommandForCLIMergeTests() {
	conf := s.taskConfig2
	c := gitFetchProject{
		Directory: "dir",
		Token:     projectGitHubToken,
	}

	opts := cloneOpts{
		method:             distro.CloneMethodOAuth,
		branch:             conf.ProjectRef.Branch,
		owner:              conf.ProjectRef.Owner,
		repo:               conf.ProjectRef.Repo,
		dir:                c.Directory,
		token:              c.Token,
		mergeTestRequester: true,
	}
	s.Require().NoError(opts.setLocation())

	s.taskConfig2.Task.Requester = evergreen.MergeTestRequester
	cmds, err := c.buildCloneCommand(context.Background(), conf, nil, opts)
	s.NoError(err)
	s.Len(cmds, 9)
	s.True(strings.HasSuffix(cmds[5], fmt.Sprintf("--branch '%s'", s.taskConfig2.ProjectRef.Branch)))
}

func (s *GitGetProjectSuite) TestBuildModuleCommand() {
	conf := s.taskConfig2
	c := gitFetchProject{
		Directory: "dir",
		Token:     projectGitHubToken,
	}

	opts := cloneOpts{
		method: distro.CloneMethodLegacySSH,
		owner:  "deafgoat",
		repo:   "mci_test",
		dir:    "module",
	}
	s.Require().NoError(opts.setLocation())

	// ensure module clone command with ssh URL does not inject token
	cmds, err := c.buildModuleCloneCommand(conf, opts, "main", nil)
	s.NoError(err)
	s.Require().Len(cmds, 5)
	s.Equal("set -o xtrace", cmds[0])
	s.Equal("set -o errexit", cmds[1])
	s.Equal("git clone 'git@github.com:deafgoat/mci_test.git' 'module'", cmds[2])
	s.Equal("cd module", cmds[3])
	s.Equal("git checkout 'main'", cmds[4])

	// ensure module clone command with http URL injects token
	opts.method = distro.CloneMethodOAuth
	opts.token = c.Token
	s.Require().NoError(opts.setLocation())
	cmds, err = c.buildModuleCloneCommand(conf, opts, "main", nil)
	s.NoError(err)
	s.Require().Len(cmds, 8)
	s.Equal("set -o xtrace", cmds[0])
	s.Equal("set -o errexit", cmds[1])
	s.Equal("set +o xtrace", cmds[2])
	s.Equal("echo \"git clone https://[redacted oauth token]:x-oauth-basic@github.com/deafgoat/mci_test.git 'module'\"", cmds[3])
	s.Equal("git clone https://PROJECTTOKEN:x-oauth-basic@github.com/deafgoat/mci_test.git 'module'", cmds[4])
	s.Equal("set -o xtrace", cmds[5])
	s.Equal("cd module", cmds[6])
	s.Equal("git checkout 'main'", cmds[7])

	// ensure insecure github url is forced to use https
	opts.location = "http://github.com/deafgoat/mci_test.git"
	cmds, err = c.buildModuleCloneCommand(conf, opts, "main", nil)
	s.NoError(err)
	s.Require().Len(cmds, 8)
	s.Equal("echo \"git clone https://[redacted oauth token]:x-oauth-basic@github.com/deafgoat/mci_test.git 'module'\"", cmds[3])
	s.Equal("git clone https://PROJECTTOKEN:x-oauth-basic@github.com/deafgoat/mci_test.git 'module'", cmds[4])

	conf = s.taskConfig4
	// with merge test-commit checkout
	module := &patch.ModulePatch{
		ModuleName: "test-module",
		Githash:    "1234abcd",
		PatchSet: patch.PatchSet{
			Patch: "1234",
		},
	}
	opts.method = distro.CloneMethodLegacySSH
	s.Require().NoError(opts.setLocation())
	cmds, err = c.buildModuleCloneCommand(conf, opts, "main", module)
	s.NoError(err)
	s.Require().Len(cmds, 7)
	s.Equal("set -o xtrace", cmds[0])
	s.Equal("set -o errexit", cmds[1])
	s.Equal("git clone 'git@github.com:deafgoat/mci_test.git' 'module'", cmds[2])
	s.Equal("cd module", cmds[3])
	s.Regexp("^git fetch origin \"pull/1234/merge:evg-merge-test-", cmds[4])
	s.Regexp("^git checkout 'evg-merge-test-", cmds[5])
	s.Equal("git reset --hard 1234abcd", cmds[6])
}

func (s *GitGetProjectSuite) TestGetApplyCommand() {
	c := &gitFetchProject{
		Directory:      "dir",
		Token:          projectGitHubToken,
		CommitterName:  "octocat",
		CommitterEmail: "octocat@github.com",
	}

	// regular patch
	patchPath := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "git", "test.patch")
	applyCommand, err := c.getApplyCommand(patchPath)
	s.NoError(err)
	s.Equal(fmt.Sprintf("git apply --binary --index < '%s'", patchPath), applyCommand)

	// mbox patch
	patchPath = filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "git", "test_mbox.patch")
	applyCommand, err = c.getApplyCommand(patchPath)
	s.NoError(err)
	s.Equal(fmt.Sprintf(`GIT_COMMITTER_NAME="%s" GIT_COMMITTER_EMAIL="%s" git am --keep-cr --keep < "%s"`, c.CommitterName, c.CommitterEmail, patchPath), applyCommand)
}

func (s *GitGetProjectSuite) TestCorrectModuleRevisionSetModule() {
	const correctHash = "b27779f856b211ffaf97cbc124b7082a20ea8bc0"
	conf := s.taskConfig2
	ctx := context.WithValue(context.Background(), "patch", &patch.Patch{
		Patches: []patch.ModulePatch{
			{
				ModuleName: "sample",
				Githash:    correctHash,
			},
		},
	})
	comm := client.NewMock("http://localhost.com")
	logger, err := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, nil)
	s.NoError(err)

	for _, task := range conf.Project.Tasks {
		s.NotEqual(len(task.Commands), 0)
		for _, command := range task.Commands {
			var pluginCmds []Command
			pluginCmds, err = Render(command, conf.Project.Functions)
			s.NoError(err)
			s.NotNil(pluginCmds)
			pluginCmds[0].SetJasperManager(s.jasper)
			err = pluginCmds[0].Execute(ctx, comm, logger, conf)
			s.NoError(err)
		}
	}

	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = conf.WorkDir + "/src/hello/module/sample/"
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	s.NoError(err)
	ref := strings.Trim(out.String(), "\n")
	s.Equal(correctHash, ref) // this revision is defined in the patch, returned by GetTaskPatch
	s.NoError(logger.Close())
	toCheck := `Using revision/ref 'b27779f856b211ffaf97cbc124b7082a20ea8bc0' for module 'sample' (reason: specified in set-module)`
	foundMsg := false
	for _, task := range comm.GetMockMessages() {
		for _, msg := range task {
			if msg.Message == toCheck {
				foundMsg = true
			}
		}
	}
	s.True(foundMsg)
	s.Equal("hello/module", conf.ModulePaths["sample"])
}

func (s *GitGetProjectSuite) TestCorrectModuleRevisionManifest() {
	const correctHash = "3585388b1591dfca47ac26a5b9a564ec8f138a5e"
	conf := s.taskConfig2
	conf.Expansions.Put(moduleRevExpansionName("sample"), correctHash)
	ctx := context.Background()
	comm := client.NewMock("http://localhost.com")
	logger, err := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, nil)
	s.NoError(err)

	for _, task := range conf.Project.Tasks {
		s.NotEqual(len(task.Commands), 0)
		for _, command := range task.Commands {
			var pluginCmds []Command
			pluginCmds, err = Render(command, conf.Project.Functions)
			s.NoError(err)
			s.NotNil(pluginCmds)
			pluginCmds[0].SetJasperManager(s.jasper)
			err = pluginCmds[0].Execute(ctx, comm, logger, conf)
			s.NoError(err)
		}
	}

	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = conf.WorkDir + "/src/hello/module/sample/"
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	s.NoError(err)
	ref := strings.Trim(out.String(), "\n")
	s.Equal(correctHash, ref)
	s.NoError(logger.Close())
	toCheck := `Using revision/ref '3585388b1591dfca47ac26a5b9a564ec8f138a5e' for module 'sample' (reason: from manifest)`
	foundMsg := false
	for _, task := range comm.GetMockMessages() {
		for _, msg := range task {
			if msg.Message == toCheck {
				foundMsg = true
			}
		}
	}
	s.True(foundMsg)
	s.Equal("hello/module", conf.ModulePaths["sample"])
}

func (s *GitGetProjectSuite) TearDownSuite() {
	if s.taskConfig1 != nil {
		s.NoError(os.RemoveAll(s.taskConfig1.WorkDir))
	}
	if s.taskConfig2 != nil {
		s.NoError(os.RemoveAll(s.taskConfig2.WorkDir))
	}
}

func (s *GitGetProjectSuite) TestAllowsEmptyPatches() {
	dir, err := ioutil.TempDir("", "evg-test")
	s.Require().NoError(err)
	defer func() {
		s.NoError(os.RemoveAll(dir))
	}()

	c := gitFetchProject{
		Directory: dir,
		Token:     projectGitHubToken,
	}

	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "init", dir)
	s.NotNil(cmd)
	s.NoError(cmd.Run())

	sender := send.MakeInternalLogger()
	logger := client.NewSingleChannelLogHarness("", sender)

	conf := internal.TaskConfig{
		WorkDir: dir,
	}

	s.NoError(c.applyPatch(ctx, logger, &conf, []patch.ModulePatch{{}}))
	s.Equal(1, sender.Len())

	msg := sender.GetMessage()
	s.Require().NotNil(msg)
	s.Equal(level.Info, msg.Priority)
	s.Equal("Skipping empty patch file...", msg.Message.String())
}

func (s *GitGetProjectSuite) TestCloneOptsSetLocationGitHub() {
	opts := cloneOpts{
		method: "",
		owner:  "foo",
		repo:   "bar",
		token:  "",
	}
	s.Require().NoError(opts.setLocation())
	s.Equal("git@github.com:foo/bar.git", opts.location)

	opts.method = distro.CloneMethodLegacySSH
	s.Require().NoError(opts.setLocation())
	s.Equal("git@github.com:foo/bar.git", opts.location)

	opts.method = distro.CloneMethodOAuth
	s.Require().NoError(opts.setLocation())
	s.Equal("https://github.com/foo/bar.git", opts.location)

	opts.method = distro.CloneMethodLegacySSH
	opts.token = globalGitHubToken
	s.Require().NoError(opts.setLocation())
	s.Equal("git@github.com:foo/bar.git", opts.location)

	opts.method = "foo"
	opts.token = ""
	s.Error(opts.setLocation())
}

func (s *GitGetProjectSuite) TestGetProjectMethodAndToken() {
	var token string
	var method string
	var err error

	method, token, err = getProjectMethodAndToken(projectGitHubToken, globalGitHubToken, distro.CloneMethodOAuth)
	s.NoError(err)
	s.Equal(projectGitHubToken, token)
	s.Equal(distro.CloneMethodOAuth, method)

	method, token, err = getProjectMethodAndToken(projectGitHubToken, globalGitHubToken, distro.CloneMethodLegacySSH)
	s.NoError(err)
	s.Equal(projectGitHubToken, token)
	s.Equal(distro.CloneMethodOAuth, method)

	method, token, err = getProjectMethodAndToken(projectGitHubToken, "", distro.CloneMethodOAuth)
	s.NoError(err)
	s.Equal(projectGitHubToken, token)
	s.Equal(distro.CloneMethodOAuth, method)

	method, token, err = getProjectMethodAndToken(projectGitHubToken, "", distro.CloneMethodLegacySSH)
	s.NoError(err)
	s.Equal(projectGitHubToken, token)
	s.Equal(distro.CloneMethodOAuth, method)

	method, token, err = getProjectMethodAndToken("", globalGitHubToken, distro.CloneMethodOAuth)
	s.NoError(err)
	s.Equal(globalGitHubToken, token)
	s.Equal(distro.CloneMethodOAuth, method)

	method, token, err = getProjectMethodAndToken("", "", distro.CloneMethodLegacySSH)
	s.NoError(err)
	s.Equal("", token)
	s.Equal(distro.CloneMethodLegacySSH, method)

	method, token, err = getProjectMethodAndToken("", "", distro.CloneMethodOAuth)
	s.Error(err)
	s.Equal("", token)
	s.Equal(distro.CloneMethodLegacySSH, method)

	method, token, err = getProjectMethodAndToken("", "", distro.CloneMethodLegacySSH)
	s.NoError(err)
	s.Equal("", token)
	s.Equal(distro.CloneMethodLegacySSH, method)

	method, token, err = getProjectMethodAndToken("", "", "")
	s.NoError(err)
	s.Equal("", token)
	s.Equal(distro.CloneMethodLegacySSH, method)

	method, token, err = getProjectMethodAndToken("", "", "foobar")
	s.Error(err)
	s.Equal("", token)
	s.Equal("", method)

	_, _, err = getProjectMethodAndToken("", "token this is an invalid token", distro.CloneMethodOAuth)
	s.Error(err)

	_, _, err = getProjectMethodAndToken("token this is an invalid token", "", distro.CloneMethodOAuth)
	s.Error(err)
}

func (s *GitGetProjectSuite) TestReorderPatches() {
	patches := []patch.ModulePatch{{ModuleName: ""}}
	patches = reorderPatches(patches)
	s.Equal("", patches[0].ModuleName)

	patches = []patch.ModulePatch{
		{ModuleName: ""},
		{ModuleName: "m0"},
		{ModuleName: "m1"},
	}
	patches = reorderPatches(patches)
	s.Equal("m0", patches[0].ModuleName)
	s.Equal("m1", patches[1].ModuleName)
	s.Equal("", patches[2].ModuleName)
}

func (s *GitGetProjectSuite) TestMergeMultiplePatches() {
	conf := s.taskConfig6
	token, err := s.settings.GetGithubOauthToken()
	s.Require().NoError(err)
	conf.Expansions.Put("github", token)
	ctx := context.WithValue(context.Background(), "patch", &patch.Patch{
		Id: "p",
		Patches: []patch.ModulePatch{
			{Githash: "d0d878e81b303fd2abbf09331e54af41d6cd0c7d", PatchSet: patch.PatchSet{PatchFileId: "patchfile1"}, ModuleName: "evergreen"},
		},
	})
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	comm := client.NewMock("http://localhost.com")
	comm.PatchFiles["patchfile1"] = `
diff --git a/README.md b/README.md
index edc0c34..8e82862 100644
--- a/README.md
+++ b/README.md
@@ -1,2 +1,3 @@
 mci_test
 ========
+another line
`

	sender := send.MakeInternalLogger()
	logger := client.NewSingleChannelLogHarness("test", sender)

	for _, task := range conf.Project.Tasks {
		s.NotEqual(len(task.Commands), 0)
		for _, command := range task.Commands {
			pluginCmds, err := Render(command, conf.Project.Functions)
			s.NoError(err)
			s.NotNil(pluginCmds)
			pluginCmds[0].SetJasperManager(s.jasper)
			err = pluginCmds[0].Execute(ctx, comm, logger, conf)
			// this command will error because it'll apply the same patch twice. We are just testing that
			// there was an attempt to apply the patch the second time
			grip.Debug(err)
			s.Error(err)
		}
	}

	successMessage := "applied changes from previous commit queue patch '555555555555555555555555'"
	foundSuccessMessage := false
	for msg, ok := sender.GetMessageSafe(); ok; msg, ok = sender.GetMessageSafe() {
		if strings.Contains(msg.Message.String(), successMessage) {
			foundSuccessMessage = true
		}
	}
	s.True(foundSuccessMessage, "did not see the following in task output: %s", successMessage)
}
