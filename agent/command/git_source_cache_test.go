package command

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func sourceCacheTestConfig() *internal.TaskConfig {
	return &internal.TaskConfig{
		Task:              task.Task{Id: "t1", Revision: "abc123"},
		SourceCacheBucket: evergreen.BucketConfig{Name: "source-cache"},
		WorkDir:           "/data/mci",
	}
}

func sourceCacheTestOpts() cloneOpts {
	return cloneOpts{owner: "some-org", repo: "some-repo", dir: "src", cloneDepth: 1000}
}

// sourceCacheTestComm returns a mock communicator whose source cache credentials
// grant the base namespace, or the namespaces provided.
func sourceCacheTestComm(namespaces ...string) *client.Mock {
	if len(namespaces) == 0 {
		namespaces = []string{evergreen.SourceCacheBaseNamespace}
	}
	comm := client.NewMock("http://localhost.com")
	comm.SourceCacheCredentialsResponse = &apimodels.SourceCacheCredentialsResponse{
		AWSCredentials: apimodels.AWSCredentials{Expiration: "2030-01-01T00:00:00Z"},
		Namespaces:     namespaces,
	}
	return comm
}

// entriesWithoutKeys returns the cache entries with their computed keys cleared, so
// assertions can compare just the revision and namespace.
func entriesWithoutKeys(t *testing.T, sc *sourceCache) []sourceCacheEntry {
	stripped := []sourceCacheEntry{}
	for _, entry := range sc.entries {
		require.NotEmpty(t, entry.key)
		require.NotEmpty(t, entry.remoteKey)
		stripped = append(stripped, sourceCacheEntry{revision: entry.revision, namespace: entry.namespace})
	}
	return stripped
}

func TestNewSourceCacheSkipsWhenNotOptedInOrSparse(t *testing.T) {
	t.Run("NoBucketConfiguredSkips", func(t *testing.T) {
		conf := sourceCacheTestConfig()
		conf.SourceCacheBucket = evergreen.BucketConfig{}
		sc, reason := newSourceCache(t.Context(), sourceCacheTestComm(), conf, &gitFetchProject{Directory: "src"}, sourceCacheTestOpts(), "linux")
		assert.Nil(t, sc)
		assert.Contains(t, reason, "no source cache bucket")
	})
	t.Run("SparseCheckoutSkips", func(t *testing.T) {
		c := &gitFetchProject{Directory: "src", Filter: "blob:none", SparseCheckoutPaths: []string{"/etc"}}
		sc, reason := newSourceCache(t.Context(), sourceCacheTestComm(), sourceCacheTestConfig(), c, sourceCacheTestOpts(), "linux")
		assert.Nil(t, sc)
		assert.Contains(t, reason, "sparse")
	})
	t.Run("NonLinuxAgentSkips", func(t *testing.T) {
		sc, reason := newSourceCache(t.Context(), sourceCacheTestComm(), sourceCacheTestConfig(), &gitFetchProject{Directory: "src"}, sourceCacheTestOpts(), "windows")
		assert.Nil(t, sc)
		assert.Contains(t, reason, "Linux")
	})
	t.Run("AppServerGrantsNoNamespaceSkips", func(t *testing.T) {
		comm := client.NewMock("http://localhost.com")
		comm.SourceCacheCredentialsResponse = &apimodels.SourceCacheCredentialsResponse{}
		sc, reason := newSourceCache(t.Context(), comm, sourceCacheTestConfig(), &gitFetchProject{Directory: "src"}, sourceCacheTestOpts(), "linux")
		assert.Nil(t, sc)
		assert.Contains(t, reason, "no source cache namespaces")
	})
	t.Run("OptedInProjectIsNotSkipped", func(t *testing.T) {
		sc, reason := newSourceCache(t.Context(), sourceCacheTestComm(), sourceCacheTestConfig(), &gitFetchProject{Directory: "src"}, sourceCacheTestOpts(), "linux")
		require.NotNil(t, sc)
		assert.Empty(t, reason)
	})
}

func TestSourceCacheKeyVariesByRevisionAndCloneShape(t *testing.T) {
	c := &gitFetchProject{Directory: "src"}
	comm := sourceCacheTestComm()

	base, _ := newSourceCache(t.Context(), comm, sourceCacheTestConfig(), c, sourceCacheTestOpts(), "linux")
	require.NotNil(t, base)
	same, _ := newSourceCache(t.Context(), comm, sourceCacheTestConfig(), c, sourceCacheTestOpts(), "linux")
	require.NotNil(t, same)
	assert.Equal(t, base.contentKey(), same.contentKey())
	assert.Equal(t, "source_cache/v1/some-org/some-repo/base/abc123/"+base.contentKey()+".tgz", base.saveKey())

	otherRevisionConf := sourceCacheTestConfig()
	otherRevisionConf.Task.Revision = "def456"
	otherRevision, _ := newSourceCache(t.Context(), comm, otherRevisionConf, c, sourceCacheTestOpts(), "linux")
	require.NotNil(t, otherRevision)
	assert.NotEqual(t, base.contentKey(), otherRevision.contentKey())

	shallowOpts := sourceCacheTestOpts()
	shallowOpts.cloneDepth = 1
	shallow, _ := newSourceCache(t.Context(), comm, sourceCacheTestConfig(), c, shallowOpts, "linux")
	require.NotNil(t, shallow)
	assert.NotEqual(t, base.contentKey(), shallow.contentKey())

	// Two project refs on one repo at the same commit must not share an artifact.
	branchOpts := sourceCacheTestOpts()
	branchOpts.branch = "release-v1"
	branch, _ := newSourceCache(t.Context(), comm, sourceCacheTestConfig(), c, branchOpts, "linux")
	require.NotNil(t, branch)
	assert.NotEqual(t, base.contentKey(), branch.contentKey())

	otherBranchOpts := sourceCacheTestOpts()
	otherBranchOpts.branch = "release-v2"
	otherBranch, _ := newSourceCache(t.Context(), comm, sourceCacheTestConfig(), c, otherBranchOpts, "linux")
	require.NotNil(t, otherBranch)
	assert.NotEqual(t, branch.contentKey(), otherBranch.contentKey())

	// The directory is deliberately not in the key; TestSourceCacheArchiveRestoresIntoADifferentDirectory covers the layout half.
	otherDir, _ := newSourceCache(t.Context(), comm, sourceCacheTestConfig(), &gitFetchProject{Directory: "other"}, sourceCacheTestOpts(), "linux")
	require.NotNil(t, otherDir)
	assert.Equal(t, base.saveKey(), otherDir.saveKey())
	assert.Equal(t, filepath.Join("/data/mci", "other"), otherDir.projectDir())
}

// A PR artifact is pinned to the PR head, so the branch would fragment it for no gain.
func TestSourceCachePRKeysIgnoreTheBranch(t *testing.T) {
	c := &gitFetchProject{Directory: "src"}
	comm := sourceCacheTestComm(evergreen.SourceCachePRNamespace)
	const prHead = "55ca6286e3e4f4fba5d0448333fa99fc5a404a73"

	prConf := func() *internal.TaskConfig {
		conf := sourceCacheTestConfig()
		conf.Task.Requester = evergreen.GithubPRRequester
		conf.GithubPatchData.PRNumber = 9001
		conf.GithubPatchData.HeadHash = prHead
		return conf
	}

	branchOpts := sourceCacheTestOpts()
	branchOpts.branch = "release-v1"
	withBranch, _ := newSourceCache(t.Context(), comm, prConf(), c, branchOpts, "linux")
	require.NotNil(t, withBranch)
	withoutBranch, _ := newSourceCache(t.Context(), comm, prConf(), c, sourceCacheTestOpts(), "linux")
	require.NotNil(t, withoutBranch)
	assert.Equal(t, withoutBranch.saveKey(), withBranch.saveKey())

	// The base revision the PR falls back to is keyed on the branch.
	baseKey, _, err := withBranch.cacheKeysForRevision("abc123", evergreen.SourceCacheBaseNamespace)
	require.NoError(t, err)
	otherBaseKey, _, err := withoutBranch.cacheKeysForRevision("abc123", evergreen.SourceCacheBaseNamespace)
	require.NoError(t, err)
	assert.NotEqual(t, otherBaseKey, baseKey)
}

func TestSourceCacheKeysPRCheckoutsInTheirOwnNamespace(t *testing.T) {
	c := &gitFetchProject{Directory: "src"}
	const prHead = "55ca6286e3e4f4fba5d0448333fa99fc5a404a73"

	for _, tc := range []struct {
		name      string
		mutate    func(*internal.TaskConfig)
		wantRev   string
		namespace string
	}{
		{
			name:      "MainlineTaskKeysOnItsOwnRevision",
			mutate:    func(*internal.TaskConfig) {},
			wantRev:   "abc123",
			namespace: evergreen.SourceCacheBaseNamespace,
		},
		{
			name: "PullRequestKeysOnThePRHead",
			mutate: func(conf *internal.TaskConfig) {
				conf.Task.Requester = evergreen.GithubPRRequester
				conf.GithubPatchData.PRNumber = 9001
				conf.GithubPatchData.HeadHash = prHead
			},
			wantRev:   prHead,
			namespace: evergreen.SourceCachePRNamespace,
		},
		{
			name: "MergeQueueKeysOnTheQueueHead",
			mutate: func(conf *internal.TaskConfig) {
				conf.Task.Requester = evergreen.GithubMergeRequester
				conf.GithubMergeData.HeadSHA = prHead
				conf.GithubMergeData.HeadBranch = "gh-readonly-queue/main/pr-9001"
			},
			wantRev:   prHead,
			namespace: evergreen.SourceCachePRNamespace,
		},
		{
			name: "ParentPRCheckoutKeysOnTheParentPRHead",
			mutate: func(conf *internal.TaskConfig) {
				conf.GitHubParentPRCheckout = &patch.GitHubParentPRCheckout{ForSource: true, PRNumber: 9001, HeadHash: prHead}
			},
			wantRev:   prHead,
			namespace: evergreen.SourceCachePRNamespace,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conf := sourceCacheTestConfig()
			tc.mutate(conf)
			sc, reason := newSourceCache(t.Context(), sourceCacheTestComm(tc.namespace), conf, c, sourceCacheTestOpts(), "linux")
			require.NotNil(t, sc, reason)

			assert.Equal(t, tc.wantRev, sc.revision, "a task must save under whatever its own clone leaves HEAD at")
			isPR := tc.wantRev != "abc123"
			if isPR {
				// The PR artifact is tried first, then the one a mainline build produces.
				assert.Equal(t, []sourceCacheEntry{
					{revision: prHead, namespace: evergreen.SourceCachePRNamespace},
					{revision: "abc123", namespace: evergreen.SourceCacheBaseNamespace},
				}, entriesWithoutKeys(t, sc))
				assert.Equal(t, "source_cache/v1/some-org/some-repo/pr/"+prHead+"/"+sc.contentKey()+".tgz", sc.saveKey())
			} else {
				assert.Equal(t, []sourceCacheEntry{{revision: "abc123", namespace: evergreen.SourceCacheBaseNamespace}}, entriesWithoutKeys(t, sc))
				assert.Equal(t, "source_cache/v1/some-org/some-repo/base/abc123/"+sc.contentKey()+".tgz", sc.saveKey())
			}

			// The namespace is the security boundary, so a PR task's base revision key must match a mainline task's exactly.
			mainline, _ := newSourceCache(t.Context(), sourceCacheTestComm(), sourceCacheTestConfig(), c, sourceCacheTestOpts(), "linux")
			require.NotNil(t, mainline)
			_, baseKey, err := sc.cacheKeysForRevision("abc123", evergreen.SourceCacheBaseNamespace)
			require.NoError(t, err)
			assert.Equal(t, mainline.saveKey(), baseKey)
			if isPR {
				assert.NotEqual(t, mainline.saveKey(), sc.saveKey())
			}
		})
	}
}

func TestSourceCacheWriteNamespaceComesFromTheServer(t *testing.T) {
	c := &gitFetchProject{Directory: "src"}
	conf := sourceCacheTestConfig()
	conf.Task.Requester = evergreen.GithubMergeRequester

	// The server grants a merge queue task the PR namespace even when it checks
	// out no PR head, so the agent saves only where that grant allows.
	sc, reason := newSourceCache(t.Context(), sourceCacheTestComm(evergreen.SourceCachePRNamespace), conf, c, sourceCacheTestOpts(), "linux")
	require.NotNil(t, sc, reason)

	assert.Equal(t, "source_cache/v1/some-org/some-repo/pr/abc123/"+sc.contentKey()+".tgz", sc.saveKey())
	assert.Equal(t, []sourceCacheEntry{
		{revision: "abc123", namespace: evergreen.SourceCachePRNamespace},
		{revision: "abc123", namespace: evergreen.SourceCacheBaseNamespace},
	}, entriesWithoutKeys(t, sc))

	mainline, _ := newSourceCache(t.Context(), sourceCacheTestComm(), sourceCacheTestConfig(), c, sourceCacheTestOpts(), "linux")
	require.NotNil(t, mainline)
	require.Len(t, sc.entries, 2)
	assert.Equal(t, mainline.saveKey(), sc.entries[1].remoteKey)
}

func TestBuildPostRestoreCommand(t *testing.T) {
	c := &gitFetchProject{Directory: "src"}
	const prHead = "55ca6286e3e4f4fba5d0448333fa99fc5a404a73"
	const authedOrigin = "git remote set-url origin https://x-access-token:" + projectGitHubToken + "@github.com/some-org/some-repo.git"

	prConf := func() *internal.TaskConfig {
		conf := sourceCacheTestConfig()
		conf.Task.Requester = evergreen.GithubPRRequester
		conf.GithubPatchData.PRNumber = 9001
		conf.GithubPatchData.HeadHash = prHead
		return conf
	}

	for _, tc := range []struct {
		name           string
		conf           *internal.TaskConfig
		opts           cloneOpts
		revision       string
		runPRCheckout  bool
		wantContains   []string
		wantNotContain []string
	}{
		{
			// A restored tree keeps the producer's scrubbed origin, so the restore must put the task's own credential back.
			name:         "VerifiesHeadAndRestoresAuthenticatedOrigin",
			conf:         sourceCacheTestConfig(),
			opts:         cloneOpts{owner: "some-org", repo: "some-repo", token: projectGitHubToken},
			revision:     "abc123",
			wantContains: []string{`test "$(git rev-parse HEAD)" = "abc123"`, authedOrigin},
		},
		{
			name:           "SkipsPRCheckoutOnAPRNamespacedHit",
			conf:           prConf(),
			opts:           cloneOpts{owner: "some-org", repo: "some-repo"},
			revision:       prHead,
			wantContains:   []string{`test "$(git rev-parse HEAD)" = "` + prHead + `"`},
			wantNotContain: []string{"git fetch origin"},
		},
		{
			// The branch is part of the cache key, so a hit already came from the same branch and the restored branch is left alone.
			name:           "LeavesTheRestoredBranchAlone",
			conf:           sourceCacheTestConfig(),
			opts:           cloneOpts{owner: "some-org", repo: "some-repo", branch: "release-v1"},
			revision:       "abc123",
			wantNotContain: []string{"git checkout -B"},
		},
		{
			name:          "FetchesPRRefWithAuthenticatedOrigin",
			conf:          prConf(),
			opts:          cloneOpts{owner: "some-org", repo: "some-repo", token: projectGitHubToken},
			revision:      "abc123",
			runPRCheckout: true,
			wantContains:  []string{authedOrigin, `git fetch origin "pull/9001/head:evg-pr-test-`, "git reset --hard " + prHead},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			joined := strings.Join(c.buildPostRestoreCommand(tc.conf, tc.opts, tc.revision, tc.runPRCheckout), "\n")
			for _, want := range tc.wantContains {
				assert.Contains(t, joined, want)
			}
			for _, unwanted := range tc.wantNotContain {
				assert.NotContains(t, joined, unwanted)
			}
		})
	}
}

func TestBuildPreSaveCommandScrubsHooksAndToken(t *testing.T) {
	c := &gitFetchProject{Directory: "src"}
	joined := strings.Join(c.buildPreSaveCommand(cloneOpts{owner: "some-org", repo: "some-repo", token: projectGitHubToken}), "\n")
	assert.Contains(t, joined, "rm -rf .git/hooks")
	assert.Contains(t, joined, "git remote set-url origin https://github.com/some-org/some-repo.git")
	assert.NotContains(t, joined, projectGitHubToken)
}

// TestPostRestoreCommandFailsOnWrongRevision verifies a restored tree at the wrong revision is rejected.
func TestPostRestoreCommandFailsOnWrongRevision(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	ctx := t.Context()

	workDir := t.TempDir()
	repoDir := filepath.Join(workDir, "src")
	for _, args := range [][]string{
		{"init", "--initial-branch=main", repoDir},
		{"-C", repoDir, "commit", "--allow-empty", "-m", "initial", "--author=t <t@t>"},
		// A restored tree always came from a clone, so it always has an origin
		// for the post-restore script to point back at the task's own URL.
		{"-C", repoDir, "remote", "add", "origin", "https://github.com/some-org/some-repo.git"},
	} {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Env = append(cmd.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		require.NoError(t, cmd.Run())
	}

	jpm, err := jasper.NewSynchronizedManager(false)
	require.NoError(t, err)
	c := &gitFetchProject{Directory: "src"}
	c.SetJasperManager(jpm)
	comm := client.NewMock("http://localhost.com")
	conf := sourceCacheTestConfig()
	conf.WorkDir = workDir
	logger, err := comm.GetLoggerProducer(ctx, &conf.Task, nil)
	require.NoError(t, err)

	opts := cloneOpts{owner: "some-org", repo: "some-repo"}

	// The revision the task asked for isn't the one in the restored tree.
	assert.Error(t, c.runCommands(ctx, logger, conf, c.buildPostRestoreCommand(conf, opts, conf.Task.Revision, false)))

	headBytes, err := exec.CommandContext(ctx, "git", "-C", repoDir, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	conf.Task.Revision = strings.TrimSpace(string(headBytes))
	assert.NoError(t, c.runCommands(ctx, logger, conf, c.buildPostRestoreCommand(conf, opts, conf.Task.Revision, false)))

	// The branch is part of the cache key, so a restored tree keeps the branch
	// the producer left in it.
	branchOpts := cloneOpts{owner: "some-org", repo: "some-repo", branch: "release-v1"}
	require.NoError(t, c.runCommands(ctx, logger, conf, c.buildPostRestoreCommand(conf, branchOpts, conf.Task.Revision, false)))
	branchBytes, err := exec.CommandContext(ctx, "git", "-C", repoDir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	require.NoError(t, err)
	assert.Equal(t, "main", strings.TrimSpace(string(branchBytes)))
	headAfter, err := exec.CommandContext(ctx, "git", "-C", repoDir, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	assert.Equal(t, conf.Task.Revision, strings.TrimSpace(string(headAfter)))
}

func TestSourceCacheSpanDurationsAreNumericMilliseconds(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	ctx, span := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)).Tracer("test").Start(t.Context(), "git.get_project")
	setSourceCacheSpanDuration(ctx, sourceCacheCloneDurationAttribute, 1500*time.Millisecond)
	setSourceCacheSpanDuration(ctx, sourceCacheDownloadDurationAttribute, 250*time.Microsecond)
	span.End()

	want := map[string]float64{
		"evergreen.command.git_get_project.source_cache.clone_duration_ms":    1500,
		"evergreen.command.git_get_project.source_cache.download_duration_ms": 0.25,
	}
	ended := recorder.Ended()
	require.Len(t, ended, 1)
	for _, attr := range ended[0].Attributes() {
		expected, ok := want[string(attr.Key)]
		require.True(t, ok, "unexpected attribute '%s'", attr.Key)
		assert.Equal(t, attribute.FLOAT64, attr.Value.Type())
		assert.InDelta(t, expected, attr.Value.AsFloat64(), 0.001)
		delete(want, string(attr.Key))
	}
	assert.Empty(t, want)
}

func TestSourceCacheArchiveRestoresIntoADifferentDirectory(t *testing.T) {
	ctx := t.Context()
	logger := grip.NewJournaler("test")

	producerWorkDir := t.TempDir()
	producer := &sourceCache{workDir: producerWorkDir, dir: "src"}
	require.NoError(t, os.MkdirAll(filepath.Join(producer.projectDir(), "subdir"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(producer.projectDir(), "top.txt"), []byte("top"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(producer.projectDir(), "subdir", "nested.txt"), []byte("nested"), 0644))

	archive := filepath.Join(t.TempDir(), "source"+cacheArchiveSuffix)
	require.NoError(t, makeCacheArchive(ctx, producer.projectDir(), []string{producer.projectDir()}, archive, logger, true))

	consumerWorkDir := t.TempDir()
	consumer := &sourceCache{workDir: consumerWorkDir, dir: "other"}
	require.NoError(t, os.MkdirAll(consumer.projectDir(), 0755))
	f, err := os.Open(archive)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, f.Close()) })
	require.NoError(t, extractTarball(ctx, f, consumer.projectDir(), []string{}, true))

	top, err := os.ReadFile(filepath.Join(consumer.projectDir(), "top.txt"))
	require.NoError(t, err)
	assert.Equal(t, "top", string(top))
	nested, err := os.ReadFile(filepath.Join(consumer.projectDir(), "subdir", "nested.txt"))
	require.NoError(t, err)
	assert.Equal(t, "nested", string(nested))

	// The producer's directory name must not survive anywhere in the restored tree.
	_, err = os.Stat(filepath.Join(consumer.projectDir(), "src"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(consumerWorkDir, "src"))
	assert.True(t, os.IsNotExist(err))
}

// newRepoWithSubmoduleCredentials builds a repo whose origin, parent config and
// submodule config all carry the producer's token, and returns the work
// directory plus the two config paths. Git resolves a relative submodule URL
// with the parent's credential and records it in both configs.
func newRepoWithSubmoduleCredentials(t *testing.T) (workDir, parentConfig, moduleConfig string) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	workDir = t.TempDir()
	repoDir := filepath.Join(workDir, "src")
	for _, args := range [][]string{
		{"init", "--initial-branch=main", repoDir},
		{"-C", repoDir, "remote", "add", "origin", "https://x-access-token:" + projectGitHubToken + "@github.com/some-org/some-repo.git"},
	} {
		require.NoError(t, exec.CommandContext(t.Context(), "git", args...).Run())
	}
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, ".git", "modules", "sub"), 0755))

	parentConfig = filepath.Join(repoDir, ".git", "config")
	existing, err := os.ReadFile(parentConfig)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(parentConfig,
		append(existing, []byte("[submodule \"sub\"]\n\turl = "+submoduleTokenURL+"\n")...), 0644))

	moduleConfig = filepath.Join(repoDir, ".git", "modules", "sub", "config")
	require.NoError(t, os.WriteFile(moduleConfig,
		[]byte("[remote \"origin\"]\n\turl = "+submoduleTokenURL+"\n"), 0644))
	return workDir, parentConfig, moduleConfig
}

const submoduleTokenURL = "https://x-access-token:" + projectGitHubToken + "@github.com/some-org/some-sub.git"

// runScriptInWorkDir runs a built command script the way the agent would.
func runScriptInWorkDir(t *testing.T, workDir string, cmds []string) {
	cmd := exec.CommandContext(t.Context(), "bash", "-c", strings.Join(cmds, "\n"))
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

// TestPreSaveCommandScrubsSubmoduleTokens runs the pre-save script over a real
// repo, since the submodule scrub is a shell pipeline whose behavior a string
// assertion would not catch.
func TestPreSaveCommandScrubsSubmoduleTokens(t *testing.T) {
	workDir, parentConfig, moduleConfig := newRepoWithSubmoduleCredentials(t)

	c := &gitFetchProject{Directory: "src"}
	runScriptInWorkDir(t, workDir, c.buildPreSaveCommand(cloneOpts{owner: "some-org", repo: "some-repo", token: projectGitHubToken}))

	for _, config := range []string{parentConfig, moduleConfig} {
		contents, err := os.ReadFile(config)
		require.NoError(t, err)
		assert.NotContains(t, string(contents), projectGitHubToken, "token left in '%s'", config)
		assert.Contains(t, string(contents), "https://github.com/some-org/some-sub.git")
	}
}

// TestPostSaveCommandRestoresSubmoduleTokens runs the scrub and then the
// restore over a real repo, since together they have to leave the submodule
// remotes usable again for the rest of the producer's task.
func TestPostSaveCommandRestoresSubmoduleTokens(t *testing.T) {
	workDir, parentConfig, moduleConfig := newRepoWithSubmoduleCredentials(t)

	c := &gitFetchProject{Directory: "src"}
	opts := cloneOpts{owner: "some-org", repo: "some-repo", token: projectGitHubToken}
	runScriptInWorkDir(t, workDir, c.buildPreSaveCommand(opts))
	runScriptInWorkDir(t, workDir, c.buildPostSaveCommand(opts))

	for _, config := range []string{parentConfig, moduleConfig} {
		contents, err := os.ReadFile(config)
		require.NoError(t, err)
		assert.Contains(t, string(contents), submoduleTokenURL, "submodule credential not restored in '%s'", config)
	}
	// The origin is set explicitly rather than by the rewrite, so it must not
	// have picked up a second credential.
	origin, err := os.ReadFile(parentConfig)
	require.NoError(t, err)
	assert.NotContains(t, string(origin), "x-access-token:"+projectGitHubToken+"@x-access-token:")
}

// mergeQueueTestConfig returns a merge queue task config keyed on its cached queue head.
func mergeQueueTestConfig() *internal.TaskConfig {
	conf := sourceCacheTestConfig()
	conf.Task.Requester = evergreen.GithubMergeRequester
	conf.GithubMergeData.HeadSHA = "55ca6286e3e4f4fba5d0448333fa99fc5a404a73"
	conf.GithubMergeData.HeadBranch = "gh-readonly-queue/main/pr-9001"
	return conf
}

// stubMergeQueueRefExists swaps the GitHub lookup for the duration of a test.
func stubMergeQueueRefExists(t *testing.T, exists bool, err error) *int {
	calls := 0
	original := mergeQueueRefExists
	mergeQueueRefExists = func(ctx context.Context, owner, repo, ref, token string) (bool, error) {
		calls++
		return exists, err
	}
	t.Cleanup(func() { mergeQueueRefExists = original })
	return &calls
}

func TestMergeQueueRefDeletedOnlyChecksMergeQueueTasks(t *testing.T) {
	opts := sourceCacheTestOpts()
	opts.token = "a-token"

	for _, tc := range []struct {
		name   string
		mutate func(*internal.TaskConfig, *cloneOpts)
	}{
		{
			name:   "MainlineTaskIsNotChecked",
			mutate: func(conf *internal.TaskConfig, _ *cloneOpts) { *conf = *sourceCacheTestConfig() },
		},
		{
			name: "PullRequestTaskIsNotChecked",
			mutate: func(conf *internal.TaskConfig, _ *cloneOpts) {
				*conf = *sourceCacheTestConfig()
				conf.Task.Requester = evergreen.GithubPRRequester
			},
		},
		{
			name:   "MergeQueueTaskWithoutAHeadBranchIsNotChecked",
			mutate: func(conf *internal.TaskConfig, _ *cloneOpts) { conf.GithubMergeData.HeadBranch = "" },
		},
		{
			name:   "MergeQueueTaskWithoutATokenIsNotChecked",
			mutate: func(_ *internal.TaskConfig, opts *cloneOpts) { opts.token = "" },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := stubMergeQueueRefExists(t, false, nil)
			conf, tcOpts := mergeQueueTestConfig(), opts
			tc.mutate(conf, &tcOpts)

			deleted, err := mergeQueueRefDeleted(t.Context(), client.NewMock("http://localhost.com"), conf, "", tcOpts)
			assert.NoError(t, err)
			// A task that is never checked has no definite answer, so it must not fail fast.
			assert.False(t, deleted)
			assert.Zero(t, *calls)
		})
	}
}

func TestMergeQueueRefDeletedReportsGitHubsAnswer(t *testing.T) {
	opts := sourceCacheTestOpts()
	opts.token = "a-token"
	comm := client.NewMock("http://localhost.com")

	t.Run("DeletedRefIsReported", func(t *testing.T) {
		calls := stubMergeQueueRefExists(t, false, nil)
		deleted, err := mergeQueueRefDeleted(t.Context(), comm, mergeQueueTestConfig(), "", opts)
		assert.NoError(t, err)
		assert.True(t, deleted)
		assert.Equal(t, 1, *calls)
	})

	t.Run("ExistingRefIsNotReported", func(t *testing.T) {
		stubMergeQueueRefExists(t, true, nil)
		deleted, err := mergeQueueRefDeleted(t.Context(), comm, mergeQueueTestConfig(), "", opts)
		assert.NoError(t, err)
		assert.False(t, deleted)
	})

	t.Run("LookupErrorIsReturnedWithoutADeletedVerdict", func(t *testing.T) {
		stubMergeQueueRefExists(t, false, errors.New("GitHub is down"))
		deleted, err := mergeQueueRefDeleted(t.Context(), comm, mergeQueueTestConfig(), "", opts)
		assert.ErrorContains(t, err, "GitHub is down")
		assert.False(t, deleted)
	})
}

func TestFetchOrRestoreSourceFailsFastWhenTheMergeQueueRefIsDeleted(t *testing.T) {
	if runtime.GOOS != "linux" {
		// The check lives behind the source cache, which only runs on Linux.
		t.Skip("the source cache is only enabled on Linux agents")
	}
	ctx := t.Context()
	opts := sourceCacheTestOpts()
	opts.token = "a-token"

	calls := stubMergeQueueRefExists(t, false, nil)
	comm := sourceCacheTestComm(evergreen.SourceCachePRNamespace)
	conf := mergeQueueTestConfig()
	logger, err := comm.GetLoggerProducer(ctx, &conf.Task, nil)
	require.NoError(t, err)
	c := &gitFetchProject{Directory: "src"}

	// No Jasper manager is set, so reaching a restore or a clone panics rather than quietly passing.
	err = c.fetchOrRestoreSource(ctx, comm, logger, conf, opts)

	require.ErrorContains(t, err, mergeQueueRefGoneMessage)
	assert.Equal(t, 1, *calls)
	assert.True(t, c.refNotFound)
	assert.True(t, comm.MarkedMergeQueueGitRefNotFound)
}
