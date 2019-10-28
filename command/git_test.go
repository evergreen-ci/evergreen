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
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
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
	settings   *evergreen.Settings
	jasper     jasper.Manager
	modelData1 *modelutil.TestModelData // test model for TestGitPlugin
	modelData2 *modelutil.TestModelData // test model for TestValidateGitCommands
	modelData3 *modelutil.TestModelData
	modelData4 *modelutil.TestModelData
	modelData5 *modelutil.TestModelData

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
	patchPath := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "git", "test.patch")

	s.modelData1, err = modelutil.SetupAPITestData(s.settings, "testtask1", "rhel55", configPath1, modelutil.NoPatch)
	s.Require().NoError(err)
	s.modelData1.TaskConfig.Expansions = util.NewExpansions(map[string]string{evergreen.GlobalGitHubTokenExpansion: fmt.Sprintf("token " + globalGitHubToken)})

	s.modelData2, err = modelutil.SetupAPITestData(s.settings, "testtask1", "rhel55", configPath2, modelutil.NoPatch)
	s.Require().NoError(err)
	s.modelData2.TaskConfig.Expansions = util.NewExpansions(s.settings.Credentials)
	//SetupAPITestData always creates BuildVariant with no modules so this line works around that
	s.modelData2.TaskConfig.BuildVariant.Modules = []string{"sample"}
	s.modelData2.Task.Requester = evergreen.PatchVersionRequester
	err = setupTestPatchData(s.modelData1, patchPath, s.T())
	s.Require().NoError(err)

	s.modelData3, err = modelutil.SetupAPITestData(s.settings, "testtask1", "rhel55", configPath2, modelutil.NoPatch)
	s.Require().NoError(err)
	s.modelData3.TaskConfig.Expansions = util.NewExpansions(s.settings.Credentials)
	s.modelData3.TaskConfig.GithubPatchData = patch.GithubPatch{
		PRNumber:   9001,
		BaseOwner:  "evergreen-ci",
		BaseRepo:   "evergreen",
		BaseBranch: "master",
		HeadOwner:  "octocat",
		HeadRepo:   "evergreen",
		HeadHash:   "55ca6286e3e4f4fba5d0448333fa99fc5a404a73",
		Author:     "octocat",
	}

	s.modelData4, err = modelutil.SetupAPITestData(s.settings, "testtask1", "rhel55", configPath2, modelutil.MergePatch)
	s.Require().NoError(err)
	s.modelData4.TaskConfig.Expansions = util.NewExpansions(s.settings.Credentials)
	s.modelData4.TaskConfig.GithubPatchData = patch.GithubPatch{
		PRNumber:       9001,
		MergeCommitSHA: "abcdef",
	}
	s.modelData5, err = modelutil.SetupAPITestData(s.settings, "testtask1", "rhel55", configPath3, modelutil.MergePatch)
	s.Require().NoError(err)
}

func (s *GitGetProjectSuite) TestBuildCloneCommandUsesHTTPS() {
	c := &gitFetchProject{
		Directory: "dir",
		Token:     projectGitHubToken,
	}
	conf := s.modelData1.TaskConfig

	opts := cloneOpts{
		method: distro.CloneMethodOAuth,
		owner:  conf.ProjectRef.Owner,
		repo:   conf.ProjectRef.Repo,
		branch: conf.ProjectRef.Branch,
		dir:    c.Directory,
		token:  c.Token,
	}
	s.Require().NoError(opts.setLocation())
	cmds, _ := c.buildCloneCommand(conf, opts)
	s.Equal("git clone https://PROJECTTOKEN:x-oauth-basic@github.com/deafgoat/mci_test.git 'dir' --branch 'master'", cmds[5])
}

func (s *GitGetProjectSuite) TestBuildCloneCommandWithHTTPSNeedsToken() {
	c := &gitFetchProject{
		Directory: "dir",
	}
	conf := s.modelData1.TaskConfig

	opts := cloneOpts{
		method: distro.CloneMethodOAuth,
		owner:  conf.ProjectRef.Owner,
		repo:   conf.ProjectRef.Repo,
		branch: conf.ProjectRef.Branch,
		dir:    c.Directory,
		token:  "",
	}
	s.Require().NoError(opts.setLocation())
	_, err := c.buildCloneCommand(conf, opts)
	s.Error(err)
}

func (s *GitGetProjectSuite) TestBuildCloneCommandUsesSSH() {
	c := &gitFetchProject{
		Directory: "dir",
		Token:     "",
	}
	conf := s.modelData2.TaskConfig

	opts := cloneOpts{
		method: distro.CloneMethodLegacySSH,
		owner:  "deafgoat",
		repo:   "mci_test",
		branch: "master",
		dir:    c.Directory,
	}
	s.Require().NoError(opts.setLocation())
	cmds, err := c.buildCloneCommand(conf, opts)
	s.Require().NoError(err)
	s.Equal("git clone 'git@github.com:deafgoat/mci_test.git' 'dir' --branch 'master'", cmds[3])
}

func (s *GitGetProjectSuite) TestBuildCloneCommandDefaultCloneMethodUsesSSH() {
	c := &gitFetchProject{
		Directory: "dir",
	}
	conf := s.modelData2.TaskConfig

	opts := cloneOpts{
		owner:  conf.ProjectRef.Owner,
		repo:   conf.ProjectRef.Repo,
		branch: conf.ProjectRef.Branch,
		dir:    c.Directory,
	}
	s.Require().NoError(opts.setLocation())
	cmds, err := c.buildCloneCommand(conf, opts)
	s.Require().NoError(err)
	s.Equal("git clone 'git@github.com:evergreen-ci/sample.git' 'dir' --branch 'master'", cmds[3])
}

func (s *GitGetProjectSuite) TestGitPlugin() {
	conf := s.modelData1.TaskConfig
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

	conf := s.modelData1.TaskConfig
	conf.Distro.CloneMethod = "this is not real!"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	comm := client.NewMock("http://localhost.com")
	logger, err := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, nil)
	s.NoError(err)

	err = c.Execute(ctx, comm, logger, conf)
	s.Error(err)
	s.Contains(err.Error(), fmt.Sprintf("after %d retries, operation failed", GitFetchProjectRetries))
}

func (s *GitGetProjectSuite) TestTokenScrubbedFromLogger() {
	conf := s.modelData1.TaskConfig
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
	conf := s.modelData5.TaskConfig
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
			if strings.Contains(msg.Message, "git clone 'git@github.com:evergreen-ci/doesntexist.git' 'src' --branch 'master'") {
				foundCloneCommand = true
			}
			if strings.Contains(msg.Message, "ERROR: Repository not found.") {
				foundCloneErr = true
			}
			if strings.Contains(msg.Message, "Permission denied (publickey)") {
				foundSSHErr = true
			}
		}
	}
	s.True(foundCloneCommand)
	s.True(foundCloneErr || foundSSHErr)
}

func (s *GitGetProjectSuite) TestValidateGitCommands() {
	const refToCompare = "cf46076567e4949f9fc68e0634139d4ac495c89b" //note: also defined in test_config.yml

	conf := s.modelData2.TaskConfig
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
	cmd.Dir = conf.WorkDir + "/src/module/sample/"
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
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
	s.Equal("echo \"git clone https://[redacted oauth token]:x-oauth-basic@github.com/deafgoat/mci_test.git 'dir' --branch 'master'\"", cmds[1])
	s.Equal("git clone https://PROJECTTOKEN:x-oauth-basic@github.com/deafgoat/mci_test.git 'dir' --branch 'master'", cmds[2])
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
	s.Equal("echo \"git clone https://[redacted oauth token]:x-oauth-basic@github.com/deafgoat/mci_test.git 'dir' --branch 'master'\"", cmds[1])
	s.Equal("git clone https://PROJECTTOKEN:x-oauth-basic@github.com/deafgoat/mci_test.git 'dir' --branch 'master'", cmds[2])

	// ensure that we aren't sending the github oauth token to other
	// servers
	opts.location = "http://someothergithost.com/something/else.git"
	cmds, err = opts.buildHTTPCloneCommand()
	s.NoError(err)
	s.Require().Len(cmds, 5)
	s.Equal("echo \"git clone https://[redacted oauth token]:x-oauth-basic@someothergithost.com/deafgoat/mci_test.git 'dir' --branch 'master'\"", cmds[1])
	s.Equal("git clone https://PROJECTTOKEN:x-oauth-basic@someothergithost.com/deafgoat/mci_test.git 'dir' --branch 'master'", cmds[2])
}

func (s *GitGetProjectSuite) TestBuildSSHCloneCommand() {
	// ssh clone command with branch
	opts := cloneOpts{
		method: distro.CloneMethodLegacySSH,
		owner:  "deafgoat",
		repo:   "mci_test",
		branch: "master",
		dir:    "dir",
	}
	s.Require().NoError(opts.setLocation())
	cmds, err := opts.buildSSHCloneCommand()
	s.NoError(err)
	s.Len(cmds, 2)
	s.Equal("git clone 'git@github.com:deafgoat/mci_test.git' 'dir' --branch 'master'", cmds[0])
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
	conf := s.modelData1.TaskConfig

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
	cmds, err := c.buildCloneCommand(conf, opts)
	s.NoError(err)
	s.Require().Len(cmds, 6)
	s.Equal("set -o xtrace", cmds[0])
	s.Equal("set -o errexit", cmds[1])
	s.Equal("rm -rf dir", cmds[2])
	s.Equal("git clone 'git@github.com:deafgoat/mci_test.git' 'dir' --branch 'master'", cmds[3])
	s.Equal("cd dir", cmds[4])
	s.Equal("git reset --hard ", cmds[5])

	// ensure clone command with location containing "https://github.com" uses
	// HTTPS.
	opts.method = distro.CloneMethodOAuth
	opts.token = c.Token
	s.Require().NoError(opts.setLocation())
	s.Require().NoError(err)
	cmds, err = c.buildCloneCommand(conf, opts)
	s.NoError(err)
	s.Require().Len(cmds, 9)
	s.Equal("set -o xtrace", cmds[0])
	s.Equal("set -o errexit", cmds[1])
	s.Equal("rm -rf dir", cmds[2])
	s.Equal("set +o xtrace", cmds[3])
	s.Equal("echo \"git clone https://[redacted oauth token]:x-oauth-basic@github.com/deafgoat/mci_test.git 'dir' --branch 'master'\"", cmds[4])
	s.Equal("git clone https://PROJECTTOKEN:x-oauth-basic@github.com/deafgoat/mci_test.git 'dir' --branch 'master'", cmds[5])
	s.Equal("set -o xtrace", cmds[6])
	s.Equal("cd dir", cmds[7])
	s.Equal("git reset --hard ", cmds[8])
}

func (s *GitGetProjectSuite) TestBuildCommandForPullRequests() {
	conf := s.modelData3.TaskConfig
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

	cmds, err := c.buildCloneCommand(conf, opts)
	s.NoError(err)
	s.Require().Len(cmds, 8)
	s.True(strings.HasPrefix(cmds[5], "git fetch origin \"pull/9001/head:evg-pr-test-"))
	s.True(strings.HasPrefix(cmds[6], "git checkout \"evg-pr-test-"))
	s.Equal("git reset --hard 55ca6286e3e4f4fba5d0448333fa99fc5a404a73", cmds[7])
}

func (s *GitGetProjectSuite) TestBuildCommandForPRMergeTests() {
	conf := s.modelData4.TaskConfig
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
	cmds, err := c.buildCloneCommand(conf, opts)
	s.NoError(err)
	s.Require().Len(cmds, 8)
	s.True(strings.HasPrefix(cmds[5], "git fetch origin \"pull/9001/merge:evg-merge-test-"))
	s.True(strings.HasPrefix(cmds[6], "git checkout \"evg-merge-test-"))
	s.Equal("git reset --hard abcdef", cmds[7])
}

func (s *GitGetProjectSuite) TestBuildCommandForCLIMergeTests() {
	conf := s.modelData2.TaskConfig
	c := gitFetchProject{
		Directory: "dir",
		Token:     projectGitHubToken,
	}

	opts := cloneOpts{
		method: distro.CloneMethodOAuth,
		branch: conf.ProjectRef.Branch,
		owner:  conf.ProjectRef.Owner,
		repo:   conf.ProjectRef.Repo,
		dir:    c.Directory,
		token:  c.Token,
	}
	s.Require().NoError(opts.setLocation())

	s.modelData2.TaskConfig.Task.Requester = evergreen.MergeTestRequester
	cmds, err := c.buildCloneCommand(conf, opts)
	s.NoError(err)
	s.Len(cmds, 9)
	s.True(strings.HasPrefix(cmds[8], fmt.Sprintf("git checkout '%s'", s.modelData2.TaskConfig.ProjectRef.Branch)))
}

func (s *GitGetProjectSuite) TestBuildModuleCommand() {
	conf := s.modelData2.TaskConfig
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
	cmds, err := c.buildModuleCloneCommand(conf, opts, "master", nil)
	s.NoError(err)
	s.Require().Len(cmds, 5)
	s.Equal("set -o xtrace", cmds[0])
	s.Equal("set -o errexit", cmds[1])
	s.Equal("git clone 'git@github.com:deafgoat/mci_test.git' 'module'", cmds[2])
	s.Equal("cd module", cmds[3])
	s.Equal("git checkout 'master'", cmds[4])

	// ensure module clone command with http URL injects token
	opts.method = distro.CloneMethodOAuth
	opts.token = c.Token
	s.Require().NoError(opts.setLocation())
	cmds, err = c.buildModuleCloneCommand(conf, opts, "master", nil)
	s.NoError(err)
	s.Require().Len(cmds, 8)
	s.Equal("set -o xtrace", cmds[0])
	s.Equal("set -o errexit", cmds[1])
	s.Equal("set +o xtrace", cmds[2])
	s.Equal("echo \"git clone https://[redacted oauth token]:x-oauth-basic@github.com/deafgoat/mci_test.git 'module'\"", cmds[3])
	s.Equal("git clone https://PROJECTTOKEN:x-oauth-basic@github.com/deafgoat/mci_test.git 'module'", cmds[4])
	s.Equal("set -o xtrace", cmds[5])
	s.Equal("cd module", cmds[6])
	s.Equal("git checkout 'master'", cmds[7])

	// ensure insecure github url is forced to use https
	opts.location = "http://github.com/deafgoat/mci_test.git"
	cmds, err = c.buildModuleCloneCommand(conf, opts, "master", nil)
	s.NoError(err)
	s.Require().Len(cmds, 8)
	s.Equal("echo \"git clone https://[redacted oauth token]:x-oauth-basic@github.com/deafgoat/mci_test.git 'module'\"", cmds[3])
	s.Equal("git clone https://PROJECTTOKEN:x-oauth-basic@github.com/deafgoat/mci_test.git 'module'", cmds[4])

	conf = s.modelData4.TaskConfig
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
	cmds, err = c.buildModuleCloneCommand(conf, opts, "master", module)
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

func (s *GitGetProjectSuite) TestCorrectModuleRevisionSetModule() {
	const correctHash = "b27779f856b211ffaf97cbc124b7082a20ea8bc0"
	conf := s.modelData2.TaskConfig
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
	cmd.Dir = conf.WorkDir + "/src/module/sample/"
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
}

func (s *GitGetProjectSuite) TestCorrectModuleRevisionManifest() {
	const correctHash = "3585388b1591dfca47ac26a5b9a564ec8f138a5e"
	conf := s.modelData2.TaskConfig
	conf.Expansions.Put(moduleExpansionName("sample"), correctHash)
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
	cmd.Dir = conf.WorkDir + "/src/module/sample/"
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
	s.NoError(err)
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

	p := patch.Patch{
		Patches: []patch.ModulePatch{
			{},
		},
	}
	conf := model.TaskConfig{
		WorkDir: dir,
	}

	s.NoError(c.applyPatch(ctx, logger, &conf, &p))
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
	s.Equal("", method)

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
