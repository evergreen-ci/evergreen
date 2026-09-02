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

func TestNewSourceCacheSkipsWhenNotOptedInOrSparse(t *testing.T) {
	t.Run("NoBucketConfiguredSkips", func(t *testing.T) {
		conf := sourceCacheTestConfig()
		conf.SourceCacheBucket = evergreen.BucketConfig{}
		sc, reason := newSourceCache(conf, &gitFetchProject{Directory: "src"}, sourceCacheTestOpts(), "linux")
		assert.Nil(t, sc)
		assert.Contains(t, reason, "no source cache bucket")
	})
	t.Run("SparseCheckoutSkips", func(t *testing.T) {
		c := &gitFetchProject{Directory: "src", Filter: "blob:none", SparseCheckoutPaths: []string{"/etc"}}
		sc, reason := newSourceCache(sourceCacheTestConfig(), c, sourceCacheTestOpts(), "linux")
		assert.Nil(t, sc)
		assert.Contains(t, reason, "sparse")
	})
	t.Run("NonLinuxAgentSkips", func(t *testing.T) {
		sc, reason := newSourceCache(sourceCacheTestConfig(), &gitFetchProject{Directory: "src"}, sourceCacheTestOpts(), "windows")
		assert.Nil(t, sc)
		assert.Contains(t, reason, "Linux")
	})
	t.Run("OptedInProjectIsNotSkipped", func(t *testing.T) {
		sc, reason := newSourceCache(sourceCacheTestConfig(), &gitFetchProject{Directory: "src"}, sourceCacheTestOpts(), "linux")
		require.NotNil(t, sc)
		assert.Empty(t, reason)
	})
}

func TestSourceCacheKeyVariesByRevisionAndCloneShape(t *testing.T) {
	c := &gitFetchProject{Directory: "src"}

	base, _ := newSourceCache(sourceCacheTestConfig(), c, sourceCacheTestOpts(), "linux")
	require.NotNil(t, base)
	same, _ := newSourceCache(sourceCacheTestConfig(), c, sourceCacheTestOpts(), "linux")
	require.NotNil(t, same)
	assert.Equal(t, base.key, same.key)
	assert.Equal(t, "source_cache/v1/some-org/some-repo/abc123/"+base.key+".tgz", base.remoteKey)

	otherRevisionConf := sourceCacheTestConfig()
	otherRevisionConf.Task.Revision = "def456"
	otherRevision, _ := newSourceCache(otherRevisionConf, c, sourceCacheTestOpts(), "linux")
	require.NotNil(t, otherRevision)
	assert.NotEqual(t, base.key, otherRevision.key)

	shallowOpts := sourceCacheTestOpts()
	shallowOpts.cloneDepth = 1
	shallow, _ := newSourceCache(sourceCacheTestConfig(), c, shallowOpts, "linux")
	require.NotNil(t, shallow)
	assert.NotEqual(t, base.key, shallow.key)

	// Two project refs on one repo at the same commit must not share an artifact.
	branchOpts := sourceCacheTestOpts()
	branchOpts.branch = "release-v1"
	branch, _ := newSourceCache(sourceCacheTestConfig(), c, branchOpts, "linux")
	require.NotNil(t, branch)
	assert.NotEqual(t, base.key, branch.key)

	otherBranchOpts := sourceCacheTestOpts()
	otherBranchOpts.branch = "release-v2"
	otherBranch, _ := newSourceCache(sourceCacheTestConfig(), c, otherBranchOpts, "linux")
	require.NotNil(t, otherBranch)
	assert.NotEqual(t, branch.key, otherBranch.key)

	// The project directory is deliberately not in the key: the artifact is
	// rooted inside it, so a tree cloned into one directory restores into
	// another. TestSourceCacheArchiveRestoresIntoADifferentDirectory covers the
	// layout half of that contract.
	otherDir, _ := newSourceCache(sourceCacheTestConfig(), &gitFetchProject{Directory: "other"}, sourceCacheTestOpts(), "linux")
	require.NotNil(t, otherDir)
	assert.Equal(t, base.remoteKey, otherDir.remoteKey)
	assert.Equal(t, filepath.Join("/data/mci", "other"), otherDir.projectDir())
}

// A PR artifact is pinned to the PR head, so the branch would fragment it for no gain.
func TestSourceCachePRKeysIgnoreTheBranch(t *testing.T) {
	c := &gitFetchProject{Directory: "src"}
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
	withBranch, _ := newSourceCache(prConf(), c, branchOpts, "linux")
	require.NotNil(t, withBranch)
	withoutBranch, _ := newSourceCache(prConf(), c, sourceCacheTestOpts(), "linux")
	require.NotNil(t, withoutBranch)
	assert.Equal(t, withoutBranch.remoteKey, withBranch.remoteKey)

	// The base revision the PR falls back to is keyed on the branch.
	baseKey, _, err := withBranch.cacheKeysForRevision("abc123")
	require.NoError(t, err)
	otherBaseKey, _, err := withoutBranch.cacheKeysForRevision("abc123")
	require.NoError(t, err)
	assert.NotEqual(t, otherBaseKey, baseKey)
}

func TestSourceCacheKeysPRCheckoutsInTheirOwnNamespace(t *testing.T) {
	c := &gitFetchProject{Directory: "src"}
	const prHead = "55ca6286e3e4f4fba5d0448333fa99fc5a404a73"

	for _, tc := range []struct {
		name    string
		mutate  func(*internal.TaskConfig)
		wantRev string
	}{
		{
			name:    "MainlineTaskKeysOnItsOwnRevision",
			mutate:  func(*internal.TaskConfig) {},
			wantRev: "abc123",
		},
		{
			name: "PullRequestKeysOnThePRHead",
			mutate: func(conf *internal.TaskConfig) {
				conf.Task.Requester = evergreen.GithubPRRequester
				conf.GithubPatchData.PRNumber = 9001
				conf.GithubPatchData.HeadHash = prHead
			},
			wantRev: prHead,
		},
		{
			name: "MergeQueueKeysOnTheQueueHead",
			mutate: func(conf *internal.TaskConfig) {
				conf.Task.Requester = evergreen.GithubMergeRequester
				conf.GithubMergeData.HeadSHA = prHead
				conf.GithubMergeData.HeadBranch = "gh-readonly-queue/main/pr-9001"
			},
			wantRev: prHead,
		},
		{
			name: "ParentPRCheckoutKeysOnTheParentPRHead",
			mutate: func(conf *internal.TaskConfig) {
				conf.GitHubParentPRCheckout = &patch.GitHubParentPRCheckout{ForSource: true, PRNumber: 9001, HeadHash: prHead}
			},
			wantRev: prHead,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conf := sourceCacheTestConfig()
			tc.mutate(conf)
			sc, reason := newSourceCache(conf, c, sourceCacheTestOpts(), "linux")
			require.NotNil(t, sc, reason)

			assert.Equal(t, tc.wantRev, sc.revision, "a task must save under whatever its own clone leaves HEAD at")
			isPR := tc.wantRev != "abc123"
			if isPR {
				// The PR artifact is tried first, then the base revision one a
				// mainline build produces.
				assert.Equal(t, []string{prHead, "abc123"}, sc.restoreRevisions())
				assert.Equal(t, "source_cache/v1/some-org/some-repo/pr/"+prHead+"/"+sc.key+".tgz", sc.remoteKey)
			} else {
				assert.Equal(t, []string{"abc123"}, sc.restoreRevisions())
				assert.Equal(t, "source_cache/v1/some-org/some-repo/abc123/"+sc.key+".tgz", sc.remoteKey)
			}

			// The namespace is the security boundary: whatever a PR task saves,
			// its base revision key must stay byte-identical to the key a
			// mainline task on that same commit computes.
			mainline, _ := newSourceCache(sourceCacheTestConfig(), c, sourceCacheTestOpts(), "linux")
			require.NotNil(t, mainline)
			_, baseKey, err := sc.cacheKeysForRevision("abc123")
			require.NoError(t, err)
			assert.Equal(t, mainline.remoteKey, baseKey)
			if isPR {
				assert.NotEqual(t, mainline.remoteKey, sc.remoteKey)
			}
		})
	}
}

func TestBuildPostRestoreCommand(t *testing.T) {
	c := &gitFetchProject{Directory: "src"}
	const prHead = "55ca6286e3e4f4fba5d0448333fa99fc5a404a73"

	prConf := func() *internal.TaskConfig {
		conf := sourceCacheTestConfig()
		conf.Task.Requester = evergreen.GithubPRRequester
		conf.GithubPatchData.PRNumber = 9001
		conf.GithubPatchData.HeadHash = prHead
		return conf
	}

	t.Run("SkipsPRCheckoutOnAPRNamespacedHit", func(t *testing.T) {
		conf := prConf()
		joined := strings.Join(c.buildPostRestoreCommand(conf, cloneOpts{owner: "some-org", repo: "some-repo"}, prHead, false), "\n")
		assert.Contains(t, joined, `test "$(git rev-parse HEAD)" = "`+prHead+`"`)
		assert.NotContains(t, joined, "git fetch origin")
	})

	t.Run("VerifiesHeadBeforeTrustingTree", func(t *testing.T) {
		conf := sourceCacheTestConfig()
		joined := strings.Join(c.buildPostRestoreCommand(conf, cloneOpts{owner: "some-org", repo: "some-repo", token: projectGitHubToken}, conf.Task.Revision, false), "\n")
		assert.Contains(t, joined, `test "$(git rev-parse HEAD)" = "abc123"`)
	})

	// A restored tree keeps the producer's scrubbed, tokenless origin, so the
	// restore must put the task's own credential back. Otherwise a hit would
	// leave a remote that can't fetch from a private repo, where a clone can.
	t.Run("RestoresAuthenticatedOriginSoHitMatchesClone", func(t *testing.T) {
		conf := sourceCacheTestConfig()
		joined := strings.Join(c.buildPostRestoreCommand(conf, cloneOpts{owner: "some-org", repo: "some-repo", token: projectGitHubToken}, conf.Task.Revision, false), "\n")
		assert.Contains(t, joined, "git remote set-url origin https://x-access-token:"+projectGitHubToken+"@github.com/some-org/some-repo.git")
	})

	// Renaming the branch here would mislabel a PR hit, whose tree is at the PR head.
	t.Run("LeavesTheRestoredBranchAlone", func(t *testing.T) {
		conf := sourceCacheTestConfig()
		joined := strings.Join(c.buildPostRestoreCommand(conf, cloneOpts{owner: "some-org", repo: "some-repo", branch: "release-v1"}, conf.Task.Revision, false), "\n")
		assert.NotContains(t, joined, "git checkout -B")

		prJoined := strings.Join(c.buildPostRestoreCommand(prConf(), cloneOpts{owner: "some-org", repo: "some-repo", branch: "release-v1"}, prHead, false), "\n")
		assert.NotContains(t, prJoined, "git checkout -B")
	})

	t.Run("FetchesPRRefWithAuthenticatedOrigin", func(t *testing.T) {
		conf := prConf()
		joined := strings.Join(c.buildPostRestoreCommand(conf, cloneOpts{owner: "some-org", repo: "some-repo", token: projectGitHubToken}, conf.Task.Revision, true), "\n")
		assert.Contains(t, joined, "git remote set-url origin https://x-access-token:"+projectGitHubToken+"@github.com/some-org/some-repo.git")
		assert.Contains(t, joined, `git fetch origin "pull/9001/head:evg-pr-test-`)
		assert.Contains(t, joined, "git reset --hard "+prHead)
	})
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

	// The branch the producer left behind is kept as is, since the key pins it.
	branchOpts := cloneOpts{owner: "some-org", repo: "some-repo", branch: "release-v1"}
	require.NoError(t, c.runCommands(ctx, logger, conf, c.buildPostRestoreCommand(conf, branchOpts, conf.Task.Revision, false)))
	branchBytes, err := exec.CommandContext(ctx, "git", "-C", repoDir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	require.NoError(t, err)
	assert.Equal(t, "main", strings.TrimSpace(string(branchBytes)))
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

	// The producer's directory name must not survive anywhere in the restored
	// tree, which is what a workDir-rooted archive would leave behind.
	_, err = os.Stat(filepath.Join(consumer.projectDir(), "src"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(consumerWorkDir, "src"))
	assert.True(t, os.IsNotExist(err))
}

// TestPreSaveCommandScrubsSubmoduleTokens runs the pre-save script over a real
// repo, since the submodule scrub is a shell pipeline whose behavior a string
// assertion would not catch.
func TestPreSaveCommandScrubsSubmoduleTokens(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	ctx := t.Context()

	workDir := t.TempDir()
	repoDir := filepath.Join(workDir, "src")
	for _, args := range [][]string{
		{"init", "--initial-branch=main", repoDir},
		{"-C", repoDir, "remote", "add", "origin", "https://x-access-token:" + projectGitHubToken + "@github.com/some-org/some-repo.git"},
	} {
		require.NoError(t, exec.CommandContext(ctx, "git", args...).Run())
	}
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, ".git", "modules", "sub"), 0755))
	tokenURL := "https://x-access-token:" + projectGitHubToken + "@github.com/some-org/some-sub.git"
	// Git resolves a relative submodule URL with the parent's credential and
	// records it in both the parent config and the submodule's own config.
	parentConfig := filepath.Join(repoDir, ".git", "config")
	existing, err := os.ReadFile(parentConfig)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(parentConfig,
		append(existing, []byte("[submodule \"sub\"]\n\turl = "+tokenURL+"\n")...), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, ".git", "modules", "sub", "config"),
		[]byte("[remote \"origin\"]\n\turl = "+tokenURL+"\n"), 0644))

	c := &gitFetchProject{Directory: "src"}
	script := strings.Join(c.buildPreSaveCommand(cloneOpts{owner: "some-org", repo: "some-repo", token: projectGitHubToken}), "\n")
	cmd := exec.CommandContext(ctx, "bash", "-c", script)
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	for _, config := range []string{
		filepath.Join(repoDir, ".git", "config"),
		filepath.Join(repoDir, ".git", "modules", "sub", "config"),
	} {
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
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	ctx := t.Context()

	workDir := t.TempDir()
	repoDir := filepath.Join(workDir, "src")
	opts := cloneOpts{owner: "some-org", repo: "some-repo", token: projectGitHubToken}
	tokenURL := "https://x-access-token:" + projectGitHubToken + "@github.com/some-org/some-sub.git"
	for _, args := range [][]string{
		{"init", "--initial-branch=main", repoDir},
		{"-C", repoDir, "remote", "add", "origin", "https://x-access-token:" + projectGitHubToken + "@github.com/some-org/some-repo.git"},
	} {
		require.NoError(t, exec.CommandContext(ctx, "git", args...).Run())
	}
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, ".git", "modules", "sub"), 0755))
	parentConfig := filepath.Join(repoDir, ".git", "config")
	existing, err := os.ReadFile(parentConfig)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(parentConfig,
		append(existing, []byte("[submodule \"sub\"]\n\turl = "+tokenURL+"\n")...), 0644))
	moduleConfig := filepath.Join(repoDir, ".git", "modules", "sub", "config")
	require.NoError(t, os.WriteFile(moduleConfig,
		[]byte("[remote \"origin\"]\n\turl = "+tokenURL+"\n"), 0644))

	c := &gitFetchProject{Directory: "src"}
	runScript := func(cmds []string) {
		cmd := exec.CommandContext(ctx, "bash", "-c", strings.Join(cmds, "\n"))
		cmd.Dir = workDir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	runScript(c.buildPreSaveCommand(opts))
	runScript(c.buildPostSaveCommand(opts))

	for _, config := range []string{parentConfig, moduleConfig} {
		contents, err := os.ReadFile(config)
		require.NoError(t, err)
		assert.Contains(t, string(contents), tokenURL, "submodule credential not restored in '%s'", config)
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
	comm := client.NewMock("http://localhost.com")
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

// TestSourceCacheHealsOnlyTheKeyItWrites covers the decision to overwrite an
// existing artifact. Overwriting is normally forbidden so concurrent producers
// don't re-upload the same tree, so a task may only replace an object it proved
// corrupt, and only when that object is the one it saves under anyway.
func TestSourceCacheHealsOnlyTheKeyItWrites(t *testing.T) {
	const prHead = "55ca6286e3e4f4fba5d0448333fa99fc5a404a73"
	c := &gitFetchProject{Directory: "src"}

	prConfig := func() *internal.TaskConfig {
		conf := sourceCacheTestConfig()
		conf.Task.Requester = evergreen.GithubPRRequester
		conf.GithubPatchData.PRNumber = 9001
		conf.GithubPatchData.HeadHash = prHead
		return conf
	}

	t.Run("NoCorruptionDoesNotHeal", func(t *testing.T) {
		sc, reason := newSourceCache(sourceCacheTestConfig(), c, sourceCacheTestOpts(), "linux")
		require.NotNil(t, sc, reason)
		assert.False(t, sc.healsCorruptArtifact())
	})

	t.Run("MainlineTaskHealsItsOwnRevision", func(t *testing.T) {
		sc, reason := newSourceCache(sourceCacheTestConfig(), c, sourceCacheTestOpts(), "linux")
		require.NotNil(t, sc, reason)
		sc.corruptRemoteKey = sc.remoteKey
		assert.True(t, sc.healsCorruptArtifact())
	})

	t.Run("PRTaskHealsItsOwnPRArtifact", func(t *testing.T) {
		sc, reason := newSourceCache(prConfig(), c, sourceCacheTestOpts(), "linux")
		require.NotNil(t, sc, reason)
		sc.corruptRemoteKey = sc.remoteKey
		assert.True(t, sc.healsCorruptArtifact())
	})

	// A PR task's tree has the PR applied on top of the base revision, so it
	// must never overwrite the shared mainline artifact, even after proving that
	// artifact is corrupt.
	t.Run("PRTaskDoesNotHealTheSharedBaseArtifact", func(t *testing.T) {
		sc, reason := newSourceCache(prConfig(), c, sourceCacheTestOpts(), "linux")
		require.NotNil(t, sc, reason)
		_, baseKey, err := sc.cacheKeysForRevision(sc.baseRevision)
		require.NoError(t, err)
		require.NotEqual(t, sc.remoteKey, baseKey)

		sc.corruptRemoteKey = baseKey
		assert.False(t, sc.healsCorruptArtifact())
	})
}

func TestSourceCacheExtractMarksAnUndecodableArtifactCorrupt(t *testing.T) {
	ctx := t.Context()
	logger := grip.NewJournaler("test")
	const remoteKey = "source_cache/v2/10gen/mongo/abc123/key.tgz"

	newCache := func(t *testing.T) *sourceCache {
		sc := &sourceCache{workDir: t.TempDir(), dir: "src", remoteKey: remoteKey}
		require.NoError(t, os.MkdirAll(sc.projectDir(), 0755))
		return sc
	}

	t.Run("UndecodableArchiveIsMarkedAndHealed", func(t *testing.T) {
		sc := newCache(t)
		require.Error(t, sc.extractArchive(ctx, strings.NewReader("this is not a gzip stream"), remoteKey))
		assert.Equal(t, remoteKey, sc.corruptRemoteKey)
		assert.True(t, sc.healsCorruptArtifact())
	})

	t.Run("ValidArchiveIsNotMarked", func(t *testing.T) {
		producer := &sourceCache{workDir: t.TempDir(), dir: "src"}
		require.NoError(t, os.MkdirAll(producer.projectDir(), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(producer.projectDir(), "top.txt"), []byte("top"), 0644))
		archive := filepath.Join(t.TempDir(), "source"+cacheArchiveSuffix)
		require.NoError(t, makeCacheArchive(ctx, producer.projectDir(), []string{producer.projectDir()}, archive, logger, true))
		f, err := os.Open(archive)
		require.NoError(t, err)
		t.Cleanup(func() { assert.NoError(t, f.Close()) })

		sc := newCache(t)
		require.NoError(t, sc.extractArchive(ctx, f, remoteKey))
		assert.Empty(t, sc.corruptRemoteKey)
		assert.False(t, sc.healsCorruptArtifact())
	})
}
