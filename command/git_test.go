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
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper"
	"github.com/smartystreets/goconvey/convey/reporting"
	"github.com/stretchr/testify/suite"
)

const (
	globalGitHubToken  = "GLOBAL_GITHUB_TOKEN"
	projectGitHubToken = "PROJECT_GITHUB_TOKEN"
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
	s.jasper, err = jasper.NewLocalManager(false)
	s.Require().NoError(err)
	suite.Run(t, s)
}

func (s *GitGetProjectSuite) SetupTest() {
	s.NoError(db.ClearCollections(patch.Collection, build.Collection, task.Collection,
		model.VersionCollection, host.Collection, model.TaskLogCollection))
	var err error
	s.NoError(err)
	configPath1 := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "git", "plugin_clone.yml")
	configPath2 := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "git", "test_config.yml")
	configPath3 := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "git", "no_token.yml")
	patchPath := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "git", "test.patch")
	settings := &evergreen.Settings{
		Credentials: map[string]string{
			"github": fmt.Sprintf("token %s", globalGitHubToken),
		},
	}
	s.modelData1, err = modelutil.SetupAPITestData(settings, "testtask1", "rhel55", configPath1, modelutil.NoPatch)
	s.NoError(err)
	s.modelData1.TaskConfig.Expansions = util.NewExpansions(map[string]string{"global_github_oauth_token": settings.Credentials["github"]})

	s.modelData2, err = modelutil.SetupAPITestData(settings, "testtask1", "rhel55", configPath2, modelutil.NoPatch)
	s.NoError(err)
	s.modelData2.TaskConfig.Expansions = util.NewExpansions(settings.Credentials)
	//SetupAPITestData always creates BuildVariant with no modules so this line works around that
	s.modelData2.TaskConfig.BuildVariant.Modules = []string{"sample"}
	err = setupTestPatchData(s.modelData1, patchPath, s.T())
	s.NoError(err)

	s.modelData3, err = modelutil.SetupAPITestData(settings, "testtask1", "rhel55", configPath2, modelutil.NoPatch)
	s.NoError(err)
	s.modelData3.TaskConfig.Expansions = util.NewExpansions(settings.Credentials)
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
	s.NoError(err)
	s.modelData4.TaskConfig.Expansions = util.NewExpansions(s.settings.Credentials)
	s.modelData4.TaskConfig.GithubPatchData = patch.GithubPatch{
		PRNumber:       9001,
		MergeCommitSHA: "abcdef",
	}
	s.modelData5, err = modelutil.SetupAPITestData(s.settings, "testtask1", "rhel55", configPath3, modelutil.MergePatch)
	s.NoError(err)
}

func (s *GitGetProjectSuite) TestBuildCloneCommandUsesHTTPSProjectToken() {
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
	s.Equal("git clone https://PROJECT_GITHUB_TOKEN@github.com/deafgoat/mci_test.git 'dir' --branch 'master'", cmds[5])
}

func (s *GitGetProjectSuite) TestBuildCloneCommandUsesHTTPSGlobalToken() {
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
		token:  globalGitHubToken,
	}
	s.Require().NoError(opts.setLocation())
	cmds, _ := c.buildCloneCommand(conf, opts)
	s.Equal("git clone https://GLOBAL_GITHUB_TOKEN@github.com/deafgoat/mci_test.git 'dir' --branch 'master'", cmds[5])
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
		token:  c.Token,
	}
	s.Require().NoError(opts.setLocation())
	cmds, _ := c.buildCloneCommand(conf, opts)
	s.Equal("git clone 'git@github.com:deafgoat/mci_test.git' 'dir' --branch 'master'", cmds[3])
}

func (s *GitGetProjectSuite) TestGitPlugin() {
	conf := s.modelData1.TaskConfig
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

func (s *GitGetProjectSuite) TestTokenScrubbedFromLogger() {
	conf := s.modelData1.TaskConfig
	conf.ProjectRef.Repo = "doesntexist"
	conf.Distro = &distro.Distro{CloneMethod: distro.CloneMethodOAuth}
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
	found := false
	for _, msgs := range comm.GetMockMessages() {
		for _, msg := range msgs {
			grip.Infof("kim: msg = %s", msg)
			if strings.Contains(msg.Message, "https://[redacted oauth token]@github.com/deafgoat/doesntexist.git") {
				found = true
			}
			if strings.Contains(msg.Message, projectGitHubToken) {
				s.FailNow("token was leaked")
			}
		}
	}
	s.True(found)
}

func (s *GitGetProjectSuite) TestStdErrLogged() {
	conf := s.modelData5.TaskConfig
	conf.Distro = &distro.Distro{CloneMethod: distro.CloneMethodOAuth}
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
			if strings.Contains(msg.Message, "git clone https://[redacted oauth token]@github.com/evergreen-ci/doesntexist.git 'src' --branch 'master'") {
				foundCloneCommand = true
			}
			if strings.Contains(msg.Message, "Repository not found.") {
				foundCloneErr = true
			}
		}
	}
	s.True(foundCloneCommand)
	s.True(foundCloneErr)
}

func (s *GitGetProjectSuite) TestValidateGitCommands() {
	const refToCompare = "cf46076567e4949f9fc68e0634139d4ac495c89b" //note: also defined in test_config.yml

	conf := s.modelData2.TaskConfig
	conf.Distro = &distro.Distro{CloneMethod: distro.CloneMethodOAuth}
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
	cmds, err := buildHTTPCloneCommand(opts)
	s.NoError(err)
	s.Require().Len(cmds, 5)
	s.Equal("set +o xtrace", cmds[0])
	s.Equal("echo \"git clone https://[redacted oauth token]@github.com/deafgoat/mci_test.git 'dir' --branch 'master'\"", cmds[1])
	s.Equal("git clone https://PROJECT_GITHUB_TOKEN@github.com/deafgoat/mci_test.git 'dir' --branch 'master'", cmds[2])
	s.Equal("set -o xtrace", cmds[3])
	s.Equal("cd dir", cmds[4])

	// build clone command to clone by http with token into 'dir' w/o specified branch
	opts.branch = ""
	cmds, err = buildHTTPCloneCommand(opts)
	s.NoError(err)
	s.Require().Len(cmds, 5)
	s.Equal("set +o xtrace", cmds[0])
	s.Equal("echo \"git clone https://[redacted oauth token]@github.com/deafgoat/mci_test.git 'dir'\"", cmds[1])
	s.Equal("git clone https://PROJECT_GITHUB_TOKEN@github.com/deafgoat/mci_test.git 'dir'", cmds[2])
	s.Equal("set -o xtrace", cmds[3])
	s.Equal("cd dir", cmds[4])

	// build clone command with a URL that uses http, and ensure it's
	// been forced to use https
	opts.location = "http://github.com/deafgoat/mci_test.git"
	opts.branch = projectRef.Branch
	cmds, err = buildHTTPCloneCommand(opts)
	s.NoError(err)
	s.Require().Len(cmds, 5)
	s.Equal("echo \"git clone https://[redacted oauth token]@github.com/deafgoat/mci_test.git 'dir' --branch 'master'\"", cmds[1])
	s.Equal("git clone https://PROJECT_GITHUB_TOKEN@github.com/deafgoat/mci_test.git 'dir' --branch 'master'", cmds[2])

	// ensure that we aren't sending the github oauth token to other
	// servers
	opts.location = "http://someothergithost.com/something/else.git"
	cmds, err = buildHTTPCloneCommand(opts)
	s.NoError(err)
	s.Require().Len(cmds, 5)
	s.Equal("echo \"git clone https://[redacted oauth token]@someothergithost.com/deafgoat/mci_test.git 'dir' --branch 'master'\"", cmds[1])
	s.Equal("git clone https://PROJECT_GITHUB_TOKEN@someothergithost.com/deafgoat/mci_test.git 'dir' --branch 'master'", cmds[2])
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
	cmds, err := buildSSHCloneCommand(opts)
	s.NoError(err)
	s.Len(cmds, 2)
	s.Equal("git clone 'git@github.com:deafgoat/mci_test.git' 'dir' --branch 'master'", cmds[0])
	s.Equal("cd dir", cmds[1])

	// ssh clone command without branch
	opts.branch = ""
	cmds, err = buildSSHCloneCommand(opts)
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
	s.Equal("echo \"git clone https://[redacted oauth token]@github.com/deafgoat/mci_test.git 'dir' --branch 'master'\"", cmds[4])
	s.Equal("git clone https://PROJECT_GITHUB_TOKEN@github.com/deafgoat/mci_test.git 'dir' --branch 'master'", cmds[5])
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

	cmds, err := c.buildCloneCommand(conf, opts)
	s.NoError(err)
	s.Len(cmds, 8)
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
	cmds, err := c.buildCloneCommand(conf, opts)
	s.NoError(err)
	s.Len(cmds, 8)
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
	s.Require().Len(cmds, 9)
	s.True(strings.HasPrefix(cmds[8], fmt.Sprintf("git checkout %s", s.modelData2.TaskConfig.ProjectRef.Branch)))
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
	s.Equal("echo \"git clone https://[redacted oauth token]@github.com/deafgoat/mci_test.git 'module'\"", cmds[3])
	s.Equal("git clone https://PROJECT_GITHUB_TOKEN@github.com/deafgoat/mci_test.git 'module'", cmds[4])
	s.Equal("set -o xtrace", cmds[5])
	s.Equal("cd module", cmds[6])
	s.Equal("git checkout 'master'", cmds[7])

	// ensure insecure github url is forced to use https
	opts.location = "http://github.com/deafgoat/mci_test.git"
	cmds, err = c.buildModuleCloneCommand(conf, opts, "master", nil)
	s.NoError(err)
	s.Require().Len(cmds, 8)
	s.Equal("echo \"git clone https://[redacted oauth token]@github.com/deafgoat/mci_test.git 'module'\"", cmds[3])
	s.Equal("git clone https://PROJECT_GITHUB_TOKEN@github.com/deafgoat/mci_test.git 'module'", cmds[4])

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

func (s *GitGetProjectSuite) TestCloneOptsSetsLocationBasedOnCloneMethod() {
	opts := cloneOpts{
		method: distro.CloneMethodLegacySSH,
		owner:  "foo",
		repo:   "bar",
	}
	s.Require().NoError(opts.setLocation())
	s.Equal("git@github.com:foo/bar.git", opts.location)

	opts.method = distro.CloneMethodOAuth
	opts.token = globalGitHubToken
	s.Require().NoError(opts.setLocation())
	s.Equal("https://github.com/foo/bar.git", opts.location)
}

func (s *GitGetProjectSuite) TestCloneOptsChecksValidCloneMethod() {
	opts := cloneOpts{
		method: "invalid",
		owner:  "foo",
		repo:   "bar",
	}

	s.Error(opts.setLocation())
}

// kim: TODO:
// * OAuth distro with token == pass, https://github.com, doesn't leak token
// * OAuth distro but no project token available == pass, use global OAuth token
// * OAuth distro but no project or global token available == fail
// * Legacy SSH without token == pass, git@github.com
// * Legacy SSH distro with token == pass, git@github.com
// * Unspecified distro with token == pass, git@github.com
// * Unspecified distro without token == pass, git@github.com
