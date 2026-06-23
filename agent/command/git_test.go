package command

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/agent/internal/redactor"
	agenttestutil "github.com/evergreen-ci/evergreen/agent/internal/testutil"
	agentutil "github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	"github.com/smartystreets/goconvey/convey/reporting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	projectGitHubToken   = "PROJECTTOKEN"
	mockedGitHubAppToken = "MOCKEDTOKEN"
)

type GitGetProjectSuite struct {
	settings    *evergreen.Settings
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
	modelData6  *modelutil.TestModelData // GitHub merge queue
	taskConfig6 *internal.TaskConfig     // GitHub merge queue
	modelData7  *modelutil.TestModelData // Multiple modules (parallelized)
	taskConfig7 *internal.TaskConfig     // Multiple modules (parallelized)
	modelData8  *modelutil.TestModelData // Wiki module: clones evergreen-ci/evergreen.wiki
	taskConfig8 *internal.TaskConfig     // Wiki module

	comm   *client.Mock
	jasper jasper.Manager
	ctx    context.Context
	cancel context.CancelFunc
	suite.Suite
}

func init() {
	reporting.QuietMode()
}

func TestGitGetProjectSuite(t *testing.T) {
	s := new(GitGetProjectSuite)
	suite.Run(t, s)
}

func (s *GitGetProjectSuite) SetupSuite() {
	var err error
	s.jasper, err = jasper.NewSynchronizedManager(false)
	s.Require().NoError(err)

	s.comm = client.NewMock("http://localhost.com")

	s.ctx, s.cancel = context.WithCancel(context.Background())
	env := testutil.NewEnvironment(s.ctx, s.T())
	settings := env.Settings()

	testutil.ConfigureIntegrationTest(s.T(), settings)
	s.settings = settings
}

func (s *GitGetProjectSuite) SetupTest() {
	s.NoError(db.ClearCollections(patch.Collection, build.Collection, task.Collection,
		model.VersionCollection, host.Collection))
	var err error

	configPath1 := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "git", "plugin_clone.yml")
	configPath2 := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "git", "test_config.yml")
	configPath3 := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "git", "no_token.yml")
	configPath4 := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "git", "multiple_modules.yml")
	configPath5 := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "git", "wiki_module.yml")
	patchPath := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "git", "test.patch")

	s.modelData1, err = modelutil.SetupAPITestData(s.settings, "testtask1", "rhel55", configPath1, modelutil.NoPatch)
	s.Require().NoError(err)
	s.taskConfig1, err = agenttestutil.MakeTaskConfigFromModelData(s.ctx, s.settings, s.modelData1)
	s.Require().NoError(err)
	s.modelData2, err = modelutil.SetupAPITestData(s.settings, "testtask1", "rhel55", configPath2, modelutil.NoPatch)
	s.Require().NoError(err)
	s.taskConfig2, err = agenttestutil.MakeTaskConfigFromModelData(s.ctx, s.settings, s.modelData2)
	s.Require().NoError(err)
	s.taskConfig2.Expansions.Put("prefixpath", "hello")
	s.taskConfig2.NewExpansions = agentutil.NewDynamicExpansions(s.taskConfig2.Expansions)
	// SetupAPITestData always creates BuildVariant with no modules so this line works around that
	s.taskConfig2.BuildVariant.Modules = []string{"sample"}
	err = setupTestPatchData(s.modelData1, patchPath, s.T())
	s.Require().NoError(err)

	s.modelData3, err = modelutil.SetupAPITestData(s.settings, "testtask1", "rhel55", configPath2, modelutil.NoPatch)
	s.Require().NoError(err)
	s.taskConfig3, err = agenttestutil.MakeTaskConfigFromModelData(s.ctx, s.settings, s.modelData3)
	s.Require().NoError(err)
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
	s.taskConfig3.Task.Requester = evergreen.GithubPRRequester

	s.modelData4, err = modelutil.SetupAPITestData(s.settings, "testtask1", "rhel55", configPath2, modelutil.MergePatch)
	s.Require().NoError(err)
	s.taskConfig4, err = agenttestutil.MakeTaskConfigFromModelData(s.ctx, s.settings, s.modelData4)
	s.Require().NoError(err)
	s.taskConfig4.GithubPatchData = thirdparty.GithubPatch{
		PRNumber: 9001,
	}
	s.modelData5, err = modelutil.SetupAPITestData(s.settings, "testtask1", "rhel55", configPath3, modelutil.MergePatch)
	s.Require().NoError(err)
	s.taskConfig5, err = agenttestutil.MakeTaskConfigFromModelData(s.ctx, s.settings, s.modelData5)
	s.Require().NoError(err)

	s.modelData6, err = modelutil.SetupAPITestData(s.settings, "testtask1", "linux-64", configPath3, modelutil.InlinePatch)
	s.Require().NoError(err)
	s.taskConfig6, err = agenttestutil.MakeTaskConfigFromModelData(s.ctx, s.settings, s.modelData6)
	s.Require().NoError(err)
	s.taskConfig6.BuildVariant.Modules = []string{"evergreen"}
	s.taskConfig6.GithubMergeData = thirdparty.GithubMergeGroup{
		HeadBranch: "gh-readonly-queue/main/pr-515-9cd8a2532bcddf58369aa82eb66ba88e2323c056",
		HeadSHA:    "d2a90288ad96adca4a7d0122d8d4fd1deb24db11",
	}
	s.taskConfig6.Task.Requester = evergreen.GithubMergeRequester

	s.modelData7, err = modelutil.SetupAPITestData(s.settings, "testtask1", "rhel55", configPath4, modelutil.NoPatch)
	s.Require().NoError(err)
	s.taskConfig7, err = agenttestutil.MakeTaskConfigFromModelData(s.ctx, s.settings, s.modelData7)
	s.Require().NoError(err)
	s.taskConfig7.Expansions = *util.NewExpansions(map[string]string{})
	s.taskConfig7.Expansions.Put("prefixpath", "hello")
	// SetupAPITestData always creates BuildVariant with no modules so this line works around that
	s.taskConfig7.BuildVariant.Modules = []string{"sample-1", "sample-2"}

	s.modelData8, err = modelutil.SetupAPITestData(s.settings, "testtask1", "rhel55", configPath5, modelutil.NoPatch)
	s.Require().NoError(err)
	s.taskConfig8, err = agenttestutil.MakeTaskConfigFromModelData(s.ctx, s.settings, s.modelData8)
	s.Require().NoError(err)
	s.taskConfig8.Expansions.Put("prefixpath", "hello")
	s.taskConfig8.NewExpansions = agentutil.NewDynamicExpansions(s.taskConfig8.Expansions)
	s.taskConfig8.BuildVariant.Modules = []string{"evergreen-wiki"}

	s.comm.CreateInstallationTokenResult = mockedGitHubAppToken
	s.comm.CreateInstallationTokenFail = false
}

func (s *GitGetProjectSuite) TestBuildSourceCommandUsesHTTPS() {
	c := &gitFetchProject{
		Directory: "dir",
		Token:     projectGitHubToken,
	}
	conf := s.taskConfig1

	opts := cloneOpts{
		owner:  conf.ProjectRef.Owner,
		repo:   conf.ProjectRef.Repo,
		branch: conf.ProjectRef.Branch,
		dir:    c.Directory,
		token:  c.Token,
	}
	cmds, _ := c.buildSourceCloneCommand(conf, opts)
	s.True(utility.StringSliceContains(cmds, "git clone https://x-access-token:PROJECTTOKEN@github.com/evergreen-ci/sample.git 'dir' --branch 'main'"), cmds)
}

func (s *GitGetProjectSuite) TestRetryFetchAttemptsFiveTimesOnError() {
	c := &gitFetchProject{
		Directory: "dir",
		Token:     projectGitHubToken,
	}
	conf := s.taskConfig2
	logger, err := s.comm.GetLoggerProducer(s.ctx, &conf.Task, nil)
	s.Require().NoError(err)

	opts := cloneOpts{}

	attempt := 0
	err = c.retryFetch(s.ctx, logger, s.comm, conf, false, opts, func(o cloneOpts) error {
		attempt++
		return errors.New("failed to fetch")
	})

	s.Equal(5, attempt)
	s.Require().Error(err)
	s.Contains(err.Error(), "failed to fetch")
}

func (s *GitGetProjectSuite) TestRetryFetchAttemptsOnceOnSuccess() {
	c := &gitFetchProject{
		Directory: "dir",
		Token:     projectGitHubToken,
	}
	conf := s.taskConfig2
	logger, err := s.comm.GetLoggerProducer(s.ctx, &conf.Task, nil)
	s.Require().NoError(err)

	opts := cloneOpts{}

	attempt := 0
	err = c.retryFetch(s.ctx, logger, s.comm, conf, false, opts, func(o cloneOpts) error {
		attempt++
		return nil
	})

	s.Equal(1, attempt)
	s.Require().NoError(err)
}

func (s *GitGetProjectSuite) TestRetryFetchStopsOnInvalidGitHubMergeQueueRef() {
	c := &gitFetchProject{
		Directory: "dir",
		Token:     projectGitHubToken,
	}
	conf := s.taskConfig2
	logger, err := s.comm.GetLoggerProducer(s.ctx, &conf.Task, nil)
	s.Require().NoError(err)

	opts := cloneOpts{}

	attempt := 0
	err = c.retryFetch(s.ctx, logger, s.comm, conf, true, opts, func(o cloneOpts) error {
		attempt++
		c.refNotFound = true
		return errors.New("the GitHub merge SHA is not available most likely because the merge completed or was aborted")
	})

	s.Equal(1, attempt)
	s.Require().ErrorContains(err, "the GitHub merge SHA is not available most likely because the merge completed or was aborted")
}

func (s *GitGetProjectSuite) TestBuildSourceCommandCloneDepth() {
	c := &gitFetchProject{
		Directory: "dir",
	}
	conf := s.taskConfig2

	opts := cloneOpts{
		token:      projectGitHubToken,
		owner:      conf.ProjectRef.Owner,
		repo:       conf.ProjectRef.Repo,
		branch:     conf.ProjectRef.Branch,
		dir:        c.Directory,
		cloneDepth: 50,
	}
	cmds, err := c.buildSourceCloneCommand(conf, opts)
	s.Require().NoError(err)
	combined := strings.Join(cmds, " ")
	s.Contains(combined, "--depth 50")
	s.Contains(combined, "git log HEAD..")
}

func (s *GitGetProjectSuite) TestBuildSourceCommandSparseCheckout() {
	c := &gitFetchProject{
		Directory: "dir",
	}
	conf := s.taskConfig2

	opts := cloneOpts{
		token:               projectGitHubToken,
		owner:               conf.ProjectRef.Owner,
		repo:                conf.ProjectRef.Repo,
		branch:              conf.ProjectRef.Branch,
		dir:                 c.Directory,
		filter:              "blob:none",
		sparseCheckoutPaths: []string{"scripts/a.sh", "scripts/b.sh"},
	}
	cmds, err := c.buildSourceCloneCommand(conf, opts)
	s.Require().NoError(err)
	combined := strings.Join(cmds, " ")
	// The clone is partial (blob-filtered) and does not check out the working
	// tree until the sparse set is applied.
	s.Contains(combined, "--filter='blob:none'")
	s.Contains(combined, "--sparse --no-checkout")
	// The sparse set is narrowed before the revision is materialized, so only the
	// listed paths land on disk.
	s.Contains(combined, "git sparse-checkout set --no-cone 'scripts/a.sh' 'scripts/b.sh'")
	s.True(utility.StringSliceContainsOrderedPrefixSubset(cmds, []string{
		"git sparse-checkout set --no-cone 'scripts/a.sh' 'scripts/b.sh'",
		"git reset --hard ",
		"git log --oneline -n 10",
	}), cmds)
}

func (s *GitGetProjectSuite) TestSparseCheckoutIgnoredWithoutFilter() {
	// The filter is the master switch: sparse_checkout_paths set with no filter
	// is a no-op (a sparse checkout without a partial-clone filter saves nothing),
	// so the clone is a plain full clone.
	opts := cloneOpts{
		owner:               "evergreen-ci",
		repo:                "sample",
		dir:                 "dir",
		token:               projectGitHubToken,
		sparseCheckoutPaths: []string{"scripts/a.sh"},
	}
	cmds, err := opts.getCloneCommand()
	s.Require().NoError(err)
	s.Require().Len(cmds, 5)
	combined := strings.Join(cmds, " ")
	s.NotContains(combined, "--sparse")
	s.NotContains(combined, "--no-checkout")
	s.NotContains(combined, "sparse-checkout")
}

func (s *GitGetProjectSuite) TestGitPlugin() {
	conf := s.taskConfig1
	logger, err := s.comm.GetLoggerProducer(s.ctx, &conf.Task, nil)
	s.Require().NoError(err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.comm.CreateInstallationTokenResult = "token"
	s.comm.CreateGitHubDynamicAccessTokenResult = "token"
	for _, task := range conf.Project.Tasks {
		s.NotEmpty(task.Commands)
		for _, command := range task.Commands {
			pluginCmds, err := Render(command, &conf.Project, BlockInfo{})
			s.NoError(err)
			s.NotNil(pluginCmds)
			pluginCmds[0].SetJasperManager(s.jasper)
			err = pluginCmds[0].Execute(ctx, s.comm, logger, conf)
			s.NoError(err)
		}
	}
}

func (s *GitGetProjectSuite) TestGitFetchRetries() {
	c := gitFetchProject{Directory: ""}

	conf := s.taskConfig1
	c.SetJasperManager(s.jasper)
	s.comm.CreateInstallationTokenFail = true

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger, err := s.comm.GetLoggerProducer(s.ctx, &conf.Task, nil)
	s.Require().NoError(err)

	err = c.Execute(ctx, s.comm, logger, conf)
	s.Error(err)
}

func (s *GitGetProjectSuite) TestTokenIsRedactedWhenGenerated() {
	conf := s.taskConfig5
	conf.ProjectRef.Repo = "invalidRepo"
	conf.Distro = nil
	token := "abcdefghij"
	s.comm.CreateInstallationTokenResult = token
	s.comm.CreateInstallationTokenFail = false
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runCommands := func(logger client.LoggerProducer) {
		for _, task := range conf.Project.Tasks {
			s.NotEmpty(task.Commands)
			for _, command := range task.Commands {
				pluginCmds, err := Render(command, &conf.Project, BlockInfo{})
				s.NoError(err)
				s.NotNil(pluginCmds)
				pluginCmds[0].SetJasperManager(s.jasper)
				err = pluginCmds[0].Execute(ctx, s.comm, logger, conf)
				s.Error(err)
			}
		}
	}

	findTokenInLogs := func() bool {
		for _, line := range s.comm.GetTaskLogs(conf.Task.Id) {
			if strings.Contains(line.Data, token) {
				return true
			}
		}
		return false
	}

	findTokenInRedacted := func() bool {
		for _, redacted := range conf.NewExpansions.GetRedacted() {
			if redacted.Key == generatedTokenKey && redacted.Value == token {
				return true
			}
		}
		return false
	}

	// This is to ensure that the token would be leaked if not redacted.
	s.Run("WithoutRedactorShouldLeak", func() {
		logger, err := s.comm.GetLoggerProducer(s.ctx, &conf.Task, nil)
		s.Require().NoError(err)
		runCommands(logger)
		s.NoError(logger.Close())

		// Token should be leaked in logs.
		s.True(findTokenInLogs())

		// Token should be in the redacted list (the
		// redactor logger sender is just not using it).
		s.True(findTokenInRedacted())
	})

	s.Run("WithRedactorShouldNotLeak", func() {
		logger, err := s.comm.GetLoggerProducer(s.ctx, &conf.Task, &client.LoggerConfig{
			RedactorOpts: redactor.RedactionOptions{
				Expansions: conf.NewExpansions,
			},
		})
		s.Require().NoError(err)

		runCommands(logger)
		s.NoError(logger.Close())

		// Token should not be leaked in the logs.
		s.False(findTokenInLogs())

		// Token should be in redacted list.
		s.True(findTokenInRedacted())
	})
}

func (s *GitGetProjectSuite) TestStdErrLogged() {
	if os.Getenv("IS_DOCKER") == "true" {
		s.T().Skip("TestStdErrLogged will not run on docker since it requires a SSH key")
	}
	conf := s.taskConfig5
	logger, err := s.comm.GetLoggerProducer(s.ctx, &conf.Task, nil)
	s.Require().NoError(err)
	conf.ProjectRef.Repo = "invalidRepo"
	s.comm.CreateInstallationTokenResult = "unauthed-token"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, task := range conf.Project.Tasks {
		s.NotEmpty(task.Commands)
		for _, command := range task.Commands {
			pluginCmds, err := Render(command, &conf.Project, BlockInfo{})
			s.NoError(err)
			s.NotNil(pluginCmds)
			pluginCmds[0].SetJasperManager(s.jasper)
			err = pluginCmds[0].Execute(ctx, s.comm, logger, conf)
			s.Error(err)
		}
	}

	s.NoError(logger.Close())
	foundCloneCommand := false
	foundCloneErr := false
	for _, line := range s.comm.GetTaskLogs(conf.Task.Id) {
		if strings.Contains(line.Data, "/invalidRepo.git 'src' --branch 'main'") {
			foundCloneCommand = true
		}
		if strings.Contains(line.Data, "git source clone failed") {
			foundCloneErr = true
		}
	}
	s.True(foundCloneCommand)
	s.True(foundCloneErr)
}

func (s *GitGetProjectSuite) TestValidateGitCommands() {
	const refToCompare = "cf46076567e4949f9fc68e0634139d4ac495c89b" // Note: also defined in test_config.yml

	conf := s.taskConfig2
	logger, err := s.comm.GetLoggerProducer(s.ctx, &conf.Task, nil)
	s.Require().NoError(err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var pluginCmds []Command

	for _, task := range conf.Project.Tasks {
		for _, command := range task.Commands {
			pluginCmds, err = Render(command, &conf.Project, BlockInfo{})
			s.NoError(err)
			s.NotNil(pluginCmds)
			pluginCmds[0].SetJasperManager(s.jasper)
			err = pluginCmds[0].Execute(ctx, s.comm, logger, conf)
			s.NoError(err)
		}
	}
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = conf.WorkDir + "/src/hello/module/sample/"
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	s.NoError(err)
	ref := strings.Trim(out.String(), "\n") // Revision that we actually checked out
	s.Equal(refToCompare, ref)
	s.Equal("hello/module", conf.ModulePaths["sample"])
}

func (s *GitGetProjectSuite) TestGetCloneCommand() {
	projectRef := &model.ProjectRef{
		Owner:  "evergreen-ci",
		Repo:   "sample",
		Branch: "main",
	}

	// build clone command to clone by http, main branch with token into 'dir'
	opts := cloneOpts{
		owner:  projectRef.Owner,
		repo:   projectRef.Repo,
		branch: projectRef.Branch,
		dir:    "dir",
		token:  projectGitHubToken,
	}
	cmds, err := opts.getCloneCommand()
	s.NoError(err)
	s.Require().Len(cmds, 5)
	s.True(utility.ContainsOrderedSubset(cmds, []string{
		"set +o xtrace",
		fmt.Sprintf("echo \"git clone https://x-access-token:%s@github.com/evergreen-ci/sample.git 'dir' --branch 'main'\"", projectGitHubToken),
		fmt.Sprintf("git clone https://x-access-token:%s@github.com/evergreen-ci/sample.git 'dir' --branch 'main'", projectGitHubToken),
		"set -o xtrace",
		"cd dir",
	}), cmds)
	// build clone command to clone by http with token into 'dir' w/o specified branch
	opts.branch = ""
	cmds, err = opts.getCloneCommand()
	s.NoError(err)
	s.Require().Len(cmds, 5)
	s.True(utility.ContainsOrderedSubset(cmds, []string{
		"set +o xtrace",
		fmt.Sprintf("echo \"git clone https://x-access-token:%s@github.com/evergreen-ci/sample.git 'dir'\"", projectGitHubToken),
		fmt.Sprintf("git clone https://x-access-token:%s@github.com/evergreen-ci/sample.git 'dir'", projectGitHubToken),
		"set -o xtrace",
		"cd dir",
	}), cmds)

	// build clone command with a URL that uses http, and ensure it's
	// been forced to use https
	opts.owner = "evergreen-ci"
	opts.repo = "sample"
	opts.branch = projectRef.Branch
	cmds, err = opts.getCloneCommand()
	s.NoError(err)
	s.Require().Len(cmds, 5)
	s.True(utility.ContainsOrderedSubset(cmds, []string{
		fmt.Sprintf("echo \"git clone https://x-access-token:%s@github.com/evergreen-ci/sample.git 'dir' --branch 'main'\"", projectGitHubToken),
		fmt.Sprintf("git clone https://x-access-token:%s@github.com/evergreen-ci/sample.git 'dir' --branch 'main'", projectGitHubToken),
	}), cmds)

	// a partial, sparse clone filters blobs, defers checkout, and appends a
	// sparse-checkout step that narrows the working tree to the listed paths
	opts.filter = "blob:none"
	opts.sparseCheckoutPaths = []string{"scripts/a.sh", "scripts/b.sh"}
	cmds, err = opts.getCloneCommand()
	s.NoError(err)
	s.Require().Len(cmds, 6)
	s.True(utility.ContainsOrderedSubset(cmds, []string{
		fmt.Sprintf("git clone https://x-access-token:%s@github.com/evergreen-ci/sample.git 'dir' --filter='blob:none' --sparse --no-checkout --branch 'main'", projectGitHubToken),
		"set -o xtrace",
		"cd dir",
		"git sparse-checkout set --no-cone 'scripts/a.sh' 'scripts/b.sh'",
	}), cmds)
}

func (s *GitGetProjectSuite) TestBuildSourceCommand() {
	conf := s.taskConfig1

	c := gitFetchProject{
		Directory: "dir",
		Token:     projectGitHubToken,
	}

	opts := cloneOpts{
		branch: conf.ProjectRef.Branch,
		owner:  conf.ProjectRef.Owner,
		repo:   conf.ProjectRef.Repo,
		dir:    c.Directory,
	}

	// ensure clone command with location containing "https://github.com" uses
	// HTTPS.
	opts.token = c.Token
	cmds, err := c.buildSourceCloneCommand(conf, opts)
	s.NoError(err)
	s.Require().Len(cmds, 11)
	s.True(utility.ContainsOrderedSubset([]string{
		"set -o xtrace",
		"chmod -R 755 dir",
		"set -o errexit",
		"rm -rf dir",
		"set +o xtrace",
		fmt.Sprintf("echo \"git clone https://x-access-token:%s@github.com/evergreen-ci/sample.git 'dir' --branch 'main'\"", projectGitHubToken),
		fmt.Sprintf("git clone https://x-access-token:%s@github.com/evergreen-ci/sample.git 'dir' --branch 'main'", projectGitHubToken),
		"set -o xtrace",
		"cd dir",
		"git reset --hard ",
		"git log --oneline -n 10",
	}, cmds))
}

func (s *GitGetProjectSuite) TestBuildSourceCommandForPullRequests() {
	conf := s.taskConfig3

	c := gitFetchProject{
		Directory: "dir",
	}

	opts := cloneOpts{
		token:  projectGitHubToken,
		branch: conf.ProjectRef.Branch,
		owner:  conf.ProjectRef.Owner,
		repo:   conf.ProjectRef.Repo,
		dir:    c.Directory,
	}

	cmds, err := c.buildSourceCloneCommand(conf, opts)
	s.NoError(err)
	s.Require().Len(cmds, 13)
	s.True(utility.StringSliceContainsOrderedPrefixSubset(cmds, []string{
		"git fetch origin \"pull/9001/head:evg-pr-test-",
		"git checkout \"evg-pr-test-",
		"git reset --hard 55ca6286e3e4f4fba5d0448333fa99fc5a404a73",
		"git log --oneline -n 10",
	}), cmds)
}
func (s *GitGetProjectSuite) TestBuildSourceCommandForGitHubMergeQueue() {
	conf := s.taskConfig6

	c := gitFetchProject{
		Directory: "dir",
	}

	opts := cloneOpts{
		token:  projectGitHubToken,
		branch: conf.ProjectRef.Branch,
		owner:  conf.ProjectRef.Owner,
		repo:   conf.ProjectRef.Repo,
		dir:    c.Directory,
	}

	cmds, err := c.buildSourceCloneCommand(conf, opts)
	s.NoError(err)
	s.Len(cmds, 13)
	s.True(utility.StringSliceContainsOrderedPrefixSubset(cmds, []string{
		"git fetch origin \"gh-readonly-queue/main/pr-515-9cd8a2532bcddf58369aa82eb66ba88e2323c056:evg-mg-test-",
		"git checkout \"evg-mg-test-",
		"git reset --hard d2a90288ad96adca4a7d0122d8d4fd1deb24db11",
		"git log --oneline -n 10",
	}), cmds)
}

func (s *GitGetProjectSuite) TestBuildModuleCommand() {
	conf := s.taskConfig2
	c := gitFetchProject{
		Directory: "dir",
		Token:     projectGitHubToken,
	}

	opts := cloneOpts{
		token: c.Token,
		owner: "evergreen-ci",
		repo:  "sample",
		dir:   "module",
	}

	// ensure module clone command with http URL injects token
	cmds, err := c.buildModuleCloneCommand(conf, opts, "main", nil)
	s.NoError(err)
	s.Require().Len(cmds, 8)
	// [2025/09/08 12:39:15.053]         	Messages:   	[set -o xtrace set -o errexit set +o xtrace echo "git clone https://x-access-token:PROJECTTOKEN@github.com/evergreen-ci/sample.git 'module'" git clone https://x-access-token:PROJECTTOKEN@github.com/evergreen-ci/sample.git 'module' set -o xtrace cd module git checkout 'main']

	s.True(utility.ContainsOrderedSubset(cmds, []string{
		"set -o xtrace",
		"set -o errexit",
		"set +o xtrace",
		fmt.Sprintf("echo \"git clone https://x-access-token:%s@github.com/evergreen-ci/sample.git 'module'\"", projectGitHubToken),
		fmt.Sprintf("git clone https://x-access-token:%s@github.com/evergreen-ci/sample.git 'module'", projectGitHubToken),
		"set -o xtrace",
		"cd module",
		"git checkout 'main'",
	}), cmds)

	// ensure insecure github url is forced to use https
	opts.owner = "evergreen-ci"
	opts.repo = "sample"
	cmds, err = c.buildModuleCloneCommand(conf, opts, "main", nil)
	s.NoError(err)
	s.Require().Len(cmds, 8)
	s.True(utility.ContainsOrderedSubset(cmds, []string{
		fmt.Sprintf("echo \"git clone https://x-access-token:%s@github.com/evergreen-ci/sample.git 'module'\"", projectGitHubToken),
		fmt.Sprintf("git clone https://x-access-token:%s@github.com/evergreen-ci/sample.git 'module'", projectGitHubToken),
	}), cmds)

	conf = s.taskConfig3
	opts.owner = "evergreen-ci"
	opts.repo = "evergreen"
	cmds, err = c.buildModuleCloneCommand(conf, opts, "main", nil)
	s.NoError(err)
	s.Require().Len(cmds, 8)
	s.Contains(cmds[len(cmds)-1], "git checkout 'main'")

	// A module in a different repo than the PR should use a normal ref checkout.
	conf = s.taskConfig3
	opts.owner = "evergreen-ci"
	opts.repo = "sample"
	cmds, err = c.buildModuleCloneCommand(conf, opts, "main", nil)
	s.NoError(err)
	s.Require().Len(cmds, 8)
	s.Contains(cmds[len(cmds)-1], "git checkout 'main'")
}

func TestModuleUsesGitHubParentPRCheckout(t *testing.T) {
	conf := &internal.TaskConfig{
		Task: task.Task{Requester: evergreen.PatchVersionRequester, ParentPatchID: "parent-patch-id"},
		GitHubParentPRCheckout: &patch.GitHubParentPRCheckout{
			PRNumber:  42,
			BaseOwner: "evergreen-ci",
			BaseRepo:  "evergreen",
			HeadOwner: "octocat",
			HeadRepo:  "evergreen",
			HeadHash:  "abc123",
			ForModule: "module1",
		},
	}
	assert.True(t, moduleUsesGitHubParentPRCheckout(conf, &patch.ModulePatch{ModuleName: "module1"}))
	assert.False(t, moduleUsesGitHubParentPRCheckout(conf, &patch.ModulePatch{ModuleName: "module2"}))
	assert.False(t, moduleUsesGitHubParentPRCheckout(conf, nil))
	assert.False(t, moduleUsesGitHubParentPRCheckout(&internal.TaskConfig{
		Task: task.Task{Requester: evergreen.PatchVersionRequester, ParentPatchID: "parent-patch-id"},
		GithubPatchData: thirdparty.GithubPatch{
			PRNumber: 42,
			HeadHash: "abc123",
		},
	}, &patch.ModulePatch{ModuleName: "module1"}))
}

func TestUsesGitHubParentPRCheckout(t *testing.T) {
	gh := thirdparty.GithubPatch{PRNumber: 42, HeadHash: "abc123"}
	prConf := &internal.TaskConfig{
		Task:            task.Task{Requester: evergreen.GithubPRRequester},
		GithubPatchData: gh,
	}
	assert.True(t, usesGitHubParentPRCheckout(prConf))
	childConf := &internal.TaskConfig{
		Task:            task.Task{Requester: evergreen.PatchVersionRequester, ParentPatchID: "parent"},
		GithubPatchData: gh,
	}
	assert.False(t, usesGitHubParentPRCheckout(childConf))
	assert.False(t, shouldSkipApplyingPatches(childConf))
	childSourceConf := &internal.TaskConfig{
		Task: task.Task{Requester: evergreen.PatchVersionRequester, ParentPatchID: "parent"},
		GitHubParentPRCheckout: &patch.GitHubParentPRCheckout{
			PRNumber:  42,
			HeadHash:  "abc123",
			ForSource: true,
		},
	}
	assert.True(t, usesGitHubParentPRCheckout(childSourceConf))
	assert.True(t, shouldSkipApplyingPatches(childSourceConf))
	assert.False(t, shouldSkipApplyingPatches(&internal.TaskConfig{
		Task: task.Task{Requester: evergreen.PatchVersionRequester},
	}))
	mqConf := &internal.TaskConfig{
		Task: task.Task{Requester: evergreen.GithubMergeRequester},
		GithubMergeData: thirdparty.GithubMergeGroup{
			HeadSHA: "abc123",
		},
	}
	assert.True(t, usesGitHubParentPRCheckout(mqConf))
	assert.Equal(t, 0, mqConf.GithubPatchData.PRNumber)
}

func TestBuildSourceCloneCommandChildPatchUsesTaskRevision(t *testing.T) {
	c := &gitFetchProject{Directory: "dir", Token: projectGitHubToken}
	conf := &internal.TaskConfig{
		Task: task.Task{
			Requester:     evergreen.PatchVersionRequester,
			ParentPatchID: "parent-patch-id",
			Revision:      "child-mainline-sha",
		},
		GithubPatchData: thirdparty.GithubPatch{
			PRNumber: 9001,
			HeadHash: "55ca6286e3e4f4fba5d0448333fa99fc5a404a73",
		},
		ProjectRef: model.ProjectRef{Owner: "evergreen-ci", Repo: "evergreen", Branch: "main"},
	}
	opts := cloneOpts{
		token:  projectGitHubToken,
		branch: conf.ProjectRef.Branch,
		owner:  conf.ProjectRef.Owner,
		repo:   conf.ProjectRef.Repo,
		dir:    c.Directory,
	}
	cmds, err := c.buildSourceCloneCommand(conf, opts)
	require.NoError(t, err)
	joined := strings.Join(cmds, "\n")
	assert.Contains(t, joined, "git reset --hard child-mainline-sha")
	assert.NotContains(t, joined, "pull/9001/head")
}

func TestBuildSourceCloneCommandChildPatchUsesGitHubParentPRCheckout(t *testing.T) {
	c := &gitFetchProject{Directory: "dir", Token: projectGitHubToken}
	conf := &internal.TaskConfig{
		Task: task.Task{
			Requester:     evergreen.PatchVersionRequester,
			ParentPatchID: "parent-patch-id",
			Revision:      "child-mainline-sha",
		},
		GitHubParentPRCheckout: &patch.GitHubParentPRCheckout{
			PRNumber:  9001,
			HeadHash:  "55ca6286e3e4f4fba5d0448333fa99fc5a404a73",
			ForSource: true,
		},
		ProjectRef: model.ProjectRef{Owner: "evergreen-ci", Repo: "evergreen", Branch: "main"},
	}
	opts := cloneOpts{
		token:  projectGitHubToken,
		branch: conf.ProjectRef.Branch,
		owner:  conf.ProjectRef.Owner,
		repo:   conf.ProjectRef.Repo,
		dir:    c.Directory,
	}
	cmds, err := c.buildSourceCloneCommand(conf, opts)
	require.NoError(t, err)
	joined := strings.Join(cmds, "\n")
	assert.Contains(t, joined, `git fetch origin "pull/9001/head:evg-pr-test-`)
	assert.Contains(t, joined, `git reset --hard 55ca6286e3e4f4fba5d0448333fa99fc5a404a73`)
	assert.NotContains(t, joined, "git reset --hard child-mainline-sha")
}

func TestBuildModuleCloneCommandGithubPRHead(t *testing.T) {
	c := &gitFetchProject{Directory: "dir", Token: projectGitHubToken}
	conf := &internal.TaskConfig{
		Task: task.Task{Requester: evergreen.PatchVersionRequester, ParentPatchID: "parent-patch-id"},
		GitHubParentPRCheckout: &patch.GitHubParentPRCheckout{
			PRNumber:  9001,
			BaseOwner: "evergreen-ci",
			BaseRepo:  "evergreen",
			HeadOwner: "octocat",
			HeadRepo:  "evergreen",
			HeadHash:  "55ca6286e3e4f4fba5d0448333fa99fc5a404a73",
			ForModule: "module1",
		},
	}
	opts := cloneOpts{
		token: projectGitHubToken,
		owner: "evergreen-ci",
		repo:  "evergreen",
		dir:   "module",
	}
	cmds, err := c.buildModuleCloneCommand(conf, opts, "", &patch.ModulePatch{ModuleName: "module1"})
	require.NoError(t, err)
	joined := strings.Join(cmds, "\n")
	assert.Contains(t, joined, `git fetch origin "pull/9001/head:evg-pr-test-`)
	assert.Contains(t, joined, `git reset --hard 55ca6286e3e4f4fba5d0448333fa99fc5a404a73`)
	assert.NotContains(t, joined, "/merge:")
}

func TestBuildModuleCloneCommandWiki(t *testing.T) {
	c := &gitFetchProject{Directory: "dir", Token: projectGitHubToken}
	conf := &internal.TaskConfig{}
	opts := cloneOpts{
		token: projectGitHubToken,
		owner: "myorg",
		repo:  "parent.wiki",
		dir:   "wiki_dir",
	}
	cmds, err := c.buildModuleCloneCommand(conf, opts, "deadbeef0000", nil)
	require.NoError(t, err)
	joined := strings.Join(cmds, "\n")
	assert.NotContains(t, joined, "git checkout")
	assert.NotContains(t, joined, "deadbeef")
	assert.Contains(t, joined, "git clone")
	assert.Contains(t, joined, "myorg/parent.wiki")
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
	s.Equal(fmt.Sprintf("GIT_TRACE=1 git apply --binary --index < '%s'", patchPath), applyCommand)
}

func (s *GitGetProjectSuite) TestCorrectModuleRevisionSetModule() {
	const correctHash = "b27779f856b211ffaf97cbc124b7082a20ea8bc0"
	conf := s.taskConfig2
	logger, err := s.comm.GetLoggerProducer(s.ctx, &conf.Task, nil)
	s.Require().NoError(err)
	s.modelData2.Task.Requester = evergreen.PatchVersionRequester
	s.taskConfig2.Task.Requester = evergreen.PatchVersionRequester
	s.comm.GetTaskPatchResponse = &patch.Patch{
		Patches: []patch.ModulePatch{
			{
				ModuleName: "sample",
				Githash:    correctHash,
			},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, task := range conf.Project.Tasks {
		s.NotEmpty(task.Commands)
		for _, command := range task.Commands {
			var pluginCmds []Command
			pluginCmds, err = Render(command, &conf.Project, BlockInfo{})
			s.NoError(err)
			s.NotNil(pluginCmds)
			pluginCmds[0].SetJasperManager(s.jasper)
			err = pluginCmds[0].Execute(ctx, s.comm, logger, conf)
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
	toCheck := `Using revision/ref 'b27779f856b211ffaf97cbc124b7082a20ea8bc0' for module 'sample' (reason: specified in set-module).`
	foundMsg := false
	for _, line := range s.comm.GetTaskLogs(conf.Task.Id) {
		if line.Data == toCheck {
			foundMsg = true
		}
	}
	s.True(foundMsg)
	s.Equal("hello/module", conf.ModulePaths["sample"])
}

func (s *GitGetProjectSuite) TestMultipleModules() {
	const sample1Hash = "cf46076567e4949f9fc68e0634139d4ac495c89b"
	const sample2Hash = "9bdedd0990e83e328e42f7bb8c2771cab6ae0145"
	conf := s.taskConfig7

	logger, err := s.comm.GetLoggerProducer(s.ctx, &conf.Task, nil)
	s.Require().NoError(err)

	var pluginCmds []Command

	conf.Expansions.Put(moduleRevExpansionName("sample-1"), sample1Hash)
	conf.Expansions.Put(moduleRevExpansionName("sample-2"), sample2Hash)

	s.comm.CreateInstallationTokenResult = mockedGitHubAppToken
	s.comm.CreateGitHubDynamicAccessTokenResult = mockedGitHubAppToken

	for _, task := range conf.Project.Tasks {
		s.NotEmpty(task.Commands)
		for _, command := range task.Commands {
			pluginCmds, err = Render(command, &conf.Project, BlockInfo{})
			s.NoError(err)
			s.NotNil(pluginCmds)
			pluginCmds[0].SetJasperManager(s.jasper)
			err = pluginCmds[0].Execute(s.ctx, s.comm, logger, conf)
			s.NoError(err)
		}
	}

	// Test module 1.
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = conf.WorkDir + "/src/hello/module-1/sample-1/"
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	s.NoError(err)
	ref := strings.Trim(out.String(), "\n")
	s.Equal(sample1Hash, ref)
	s.NoError(logger.Close())
	toCheck := fmt.Sprintf("Using revision/ref '%s' for module 'sample-1' (reason: from manifest).", sample1Hash)
	foundMsg := false
	for _, line := range s.comm.GetTaskLogs(conf.Task.Id) {
		if line.Data == toCheck {
			foundMsg = true
		}
	}
	s.True(foundMsg)
	s.Equal("hello/module-1", conf.ModulePaths["sample-1"])

	// Test module 2.
	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = conf.WorkDir + "/src/hello/module-2/sample-2/"
	out = bytes.Buffer{}
	cmd.Stdout = &out
	err = cmd.Run()
	s.NoError(err)
	ref = strings.Trim(out.String(), "\n")
	s.Equal(sample2Hash, ref)
	s.NoError(logger.Close())
	toCheck = fmt.Sprintf("Using revision/ref '%s' for module 'sample-2' (reason: from manifest).", sample2Hash)
	foundMsg = false
	for _, line := range s.comm.GetTaskLogs(conf.Task.Id) {
		if line.Data == toCheck {
			foundMsg = true
		}
	}
	s.True(foundMsg)
	s.Equal("hello/module-2", conf.ModulePaths["sample-2"])
}

func (s *GitGetProjectSuite) TestCloningWikiModule() {
	if testing.Short() {
		s.T().Skip("skipping network integration test in short mode")
	}
	const ignoredPatchHash = "7b817a1908f7505cb9c05ac5601d4692793e1c0a"
	const ignoredYAMLRef = "0000000000000000000000000000000000000001"

	conf := s.taskConfig8
	logger, err := s.comm.GetLoggerProducer(s.ctx, &conf.Task, nil)
	s.Require().NoError(err)

	// set-module and YAML ref must not control the clone for wikis
	s.modelData8.Task.Requester = evergreen.PatchVersionRequester
	s.taskConfig8.Task.Requester = evergreen.PatchVersionRequester
	s.comm.GetTaskPatchResponse = &patch.Patch{
		Patches: []patch.ModulePatch{
			{ModuleName: "evergreen-wiki", Githash: ignoredPatchHash},
		},
	}

	for _, task := range conf.Project.Tasks {
		s.NotEmpty(task.Commands)
		for _, command := range task.Commands {
			var pluginCmds []Command
			pluginCmds, err = Render(command, &conf.Project, BlockInfo{})
			s.NoError(err)
			s.NotNil(pluginCmds)
			pluginCmds[0].SetJasperManager(s.jasper)
			err = pluginCmds[0].Execute(s.ctx, s.comm, logger, conf)
			s.NoError(err)
		}
	}

	wikiDir := filepath.Join(conf.WorkDir, "src", "hello", "w", "evergreen-wiki")
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = wikiDir
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	s.NoError(err)
	ref := strings.TrimSpace(out.String())
	s.Len(ref, 40, "wiki clone should yield a full commit SHA at HEAD")
	s.NotEqual(ignoredPatchHash, ref, "set-module must not pin wiki revision")
	s.NotEqual(ignoredYAMLRef, ref, "YAML ref must not pin wiki revision")
	s.NoError(logger.Close())
	s.Equal("hello/w", conf.ModulePaths["evergreen-wiki"])
}

func (s *GitGetProjectSuite) TestCorrectModuleRevisionManifest() {
	const correctHash = "3585388b1591dfca47ac26a5b9a564ec8f138a5e"
	conf := s.taskConfig2
	logger, err := s.comm.GetLoggerProducer(s.ctx, &conf.Task, nil)
	s.Require().NoError(err)
	conf.Expansions.Put(moduleRevExpansionName("sample"), correctHash)

	for _, task := range conf.Project.Tasks {
		s.NotEmpty(task.Commands)
		for _, command := range task.Commands {
			var pluginCmds []Command
			pluginCmds, err = Render(command, &conf.Project, BlockInfo{})
			s.NoError(err)
			s.NotNil(pluginCmds)
			pluginCmds[0].SetJasperManager(s.jasper)
			err = pluginCmds[0].Execute(s.ctx, s.comm, logger, conf)
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
	toCheck := `Using revision/ref '3585388b1591dfca47ac26a5b9a564ec8f138a5e' for module 'sample' (reason: from manifest).`
	foundMsg := false
	for _, line := range s.comm.GetTaskLogs(conf.Task.Id) {
		if line.Data == toCheck {
			foundMsg = true
		}
	}
	s.True(foundMsg)
	s.Equal("hello/module", conf.ModulePaths["sample"])
}

func (s *GitGetProjectSuite) TestCorrectModuleRevisionManifestWithExpansion() {
	const correctHash = "3585388b1591dfca47ac26a5b9a564ec8f138a5e"
	conf := s.taskConfig2
	logger, err := s.comm.GetLoggerProducer(s.ctx, &conf.Task, nil)
	s.Require().NoError(err)
	conf.BuildVariant.Modules = []string{"sample"}
	conf.Expansions.Put(moduleRevExpansionName("sample"), correctHash)

	for _, task := range conf.Project.Tasks {
		s.NotEmpty(task.Commands)
		for _, command := range task.Commands {
			var pluginCmds []Command
			pluginCmds, err = Render(command, &conf.Project, BlockInfo{})
			s.NoError(err)
			s.NotNil(pluginCmds)
			pluginCmds[0].SetJasperManager(s.jasper)
			err = pluginCmds[0].Execute(s.ctx, s.comm, logger, conf)
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
	toCheck := `Using revision/ref '3585388b1591dfca47ac26a5b9a564ec8f138a5e' for module 'sample' (reason: from manifest).`
	foundMsg := false
	for _, line := range s.comm.GetTaskLogs(conf.Task.Id) {
		if line.Data == toCheck {
			foundMsg = true
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
	s.cancel()
}

func (s *GitGetProjectSuite) TestAllowsEmptyPatches() {
	dir := s.T().TempDir()

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

func (s *GitGetProjectSuite) TestGetCloneToken() {
	var token string
	var err error

	conf := &internal.TaskConfig{
		Task: s.taskConfig1.Task,
		ProjectRef: model.ProjectRef{
			Owner: "valid-owner",
			Repo:  "valid-repo",
		},
		Expansions:    map[string]string{},
		NewExpansions: agentutil.NewDynamicExpansions(map[string]string{}),
	}

	token, err = getCloneToken(s.ctx, s.comm, conf, projectGitHubToken)
	s.NoError(err)
	s.Equal(projectGitHubToken, token)

	token, err = getCloneToken(s.ctx, s.comm, conf, "")
	s.NoError(err)
	s.Equal(mockedGitHubAppToken, token)

	s.comm.CreateInstallationTokenFail = true

	_, err = getCloneToken(s.ctx, s.comm, conf, "")
	s.Error(err)

	_, err = getCloneToken(s.ctx, s.comm, conf, "token this is not a real token")
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

func TestParentRepoForGitHubAppToken(t *testing.T) {
	assert.Equal(t, "mongo", parentRepoForGitHubAppToken("mongo.wiki"))
	assert.Equal(t, "mongo", parentRepoForGitHubAppToken("mongo.wiki.git"))
	assert.Equal(t, "other", parentRepoForGitHubAppToken("other"))
}
