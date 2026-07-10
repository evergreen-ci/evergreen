package model

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/singleflight"
)

func resetTranslationCacheForTesting() {
	translationCacheMu.Lock()
	translationCache = nil
	translationCacheMu.Unlock()
	translationGroupPtr.Store(&singleflight.Group{})
	readTranslationGroupPtr.Store(&singleflight.Group{})
	translationCacheEvictions.Store(0)
	translationCacheBytes.Store(0)
	translationCacheBytesLimit.Store(0)
	translationKeyHitsMu.Lock()
	translationKeyHits = map[string]int64{}
	translationKeyHitsMu.Unlock()
}

func TestGetOrComputeTranslation(t *testing.T) {
	t.Run("LRUDisabledSequentialCallsInvokeCompute", func(t *testing.T) {
		t.Cleanup(resetTranslationCacheForTesting)
		// Without LRU, sequential calls each invoke compute (singleflight only
		// coalesces concurrent calls, not sequential ones).
		var callCount int
		compute := func() (*Project, error) {
			callCount++
			return &Project{Identifier: "test"}, nil
		}

		p1, hit1, deduped1, err := getOrComputeTranslation("k", false, compute)
		require.NoError(t, err)
		p2, hit2, deduped2, err := getOrComputeTranslation("k", false, compute)
		require.NoError(t, err)

		assert.Equal(t, 2, callCount)
		assert.False(t, hit1, "cache disabled, never a hit")
		assert.False(t, hit2, "cache disabled, never a hit")
		assert.False(t, deduped1, "uncontended call is never deduped")
		assert.False(t, deduped2, "uncontended call is never deduped")
		assert.Equal(t, "test", p1.Identifier)
		assert.Equal(t, "test", p2.Identifier)
	})

	t.Run("CachesResult", func(t *testing.T) {
		t.Cleanup(resetTranslationCacheForTesting)

		var callCount int
		compute := func() (*Project, error) {
			callCount++
			return &Project{Identifier: "cached"}, nil
		}

		p1, hit1, deduped1, err := getOrComputeTranslation("k", true, compute)
		require.NoError(t, err)
		p2, hit2, deduped2, err := getOrComputeTranslation("k", true, compute)
		require.NoError(t, err)

		assert.Equal(t, 1, callCount, "second call should be served from cache")
		assert.False(t, hit1, "first call computes")
		assert.True(t, hit2, "second call is an LRU hit")
		assert.False(t, deduped1, "uncontended call is never deduped")
		assert.False(t, deduped2, "an LRU hit returns before singleflight is ever invoked")
		assert.Equal(t, "cached", p1.Identifier)
		assert.Equal(t, "cached", p2.Identifier)
	})

	t.Run("ErrorNotCached", func(t *testing.T) {
		t.Cleanup(resetTranslationCacheForTesting)

		var callCount int
		compute := func() (*Project, error) {
			callCount++
			if callCount == 1 {
				return nil, errors.New("compute failed")
			}
			return &Project{Identifier: "retry"}, nil
		}

		_, _, _, err := getOrComputeTranslation("k", true, compute)
		require.Error(t, err)

		p, _, _, err := getOrComputeTranslation("k", true, compute)
		require.NoError(t, err)

		assert.Equal(t, 2, callCount, "error result must not be cached")
		assert.Equal(t, "retry", p.Identifier)
	})

	t.Run("CoalescesConcurrentCallsLRUEnabled", func(t *testing.T) {
		t.Cleanup(resetTranslationCacheForTesting)

		const n = 5
		var calledCount atomic.Int64
		var computeCount atomic.Int64
		compute := func() (*Project, error) {
			// Yield until all goroutines have called getOrComputeTranslation so
			// they are queued in singleflight before compute returns.
			for calledCount.Load() < n {
				runtime.Gosched()
			}
			computeCount.Add(1)
			return &Project{Identifier: "coalesced"}, nil
		}

		projs := make([]*Project, n)
		errs := make([]error, n)
		var wg sync.WaitGroup
		for i := range n {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				calledCount.Add(1)
				projs[i], _, _, errs[i] = getOrComputeTranslation("k", true, compute)
			}(i)
		}

		wg.Wait()

		assert.Equal(t, int64(1), computeCount.Load(), "concurrent calls should coalesce into one compute")
		for i := range n {
			require.NoError(t, errs[i])
			assert.Equal(t, "coalesced", projs[i].Identifier)
		}
	})

	t.Run("CoalescesConcurrentCallsLRUDisabled", func(t *testing.T) {
		t.Cleanup(resetTranslationCacheForTesting)
		// Singleflight coalesces concurrent calls even without the LRU cache.
		// Unlike the LRU-enabled path, there is no re-check inside Do, so the
		// test must ensure all followers have reached singleflight.Do before
		// compute returns. We launch followers only after compute is running,
		// then sleep briefly after all followers signal so they can enter Do.
		const n = 5
		var computeCount atomic.Int64

		computeActive := make(chan struct{})
		var followerCount atomic.Int64

		compute := func() (*Project, error) {
			close(computeActive)
			for followerCount.Load() < n-1 {
				runtime.Gosched()
			}
			// Give followers time to reach singleflight.Do before returning.
			time.Sleep(time.Millisecond)
			computeCount.Add(1)
			return &Project{Identifier: "coalesced-no-lru"}, nil
		}

		projs := make([]*Project, n)
		dedupedFlags := make([]bool, n)
		errs := make([]error, n)
		var wg sync.WaitGroup

		wg.Add(1)
		go func() {
			defer wg.Done()
			projs[0], _, dedupedFlags[0], errs[0] = getOrComputeTranslation("k", false, compute)
		}()

		<-computeActive

		for i := 1; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				followerCount.Add(1)
				projs[i], _, dedupedFlags[i], errs[i] = getOrComputeTranslation("k", false, compute)
			}(i)
		}

		wg.Wait()

		assert.Equal(t, int64(1), computeCount.Load(), "singleflight coalesces even without LRU")
		for i := range n {
			require.NoError(t, errs[i])
			assert.Equal(t, "coalesced-no-lru", projs[i].Identifier)
			// singleflight.Do reports shared=true to the leader too once at least
			// one follower has joined its in-flight call, not just to the followers.
			assert.True(t, dedupedFlags[i], "every caller's request overlapped with another for the same key")
		}
	})

	t.Run("LRUDisabledErrorPropagatedToAllWaiters", func(t *testing.T) {
		t.Cleanup(resetTranslationCacheForTesting)
		// All concurrent singleflight waiters receive the error when compute fails.
		// The next sequential call must retry (singleflight does not cache errors).
		const n = 5
		var computeCount atomic.Int64

		computeActive := make(chan struct{})
		var followerCount atomic.Int64

		compute := func() (*Project, error) {
			close(computeActive)
			for followerCount.Load() < n-1 {
				runtime.Gosched()
			}
			time.Sleep(time.Millisecond)
			computeCount.Add(1)
			return nil, errors.New("compute error")
		}

		errs := make([]error, n)
		var wg sync.WaitGroup

		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _, errs[0] = getOrComputeTranslation("k", false, compute)
		}()

		<-computeActive

		for i := 1; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				followerCount.Add(1)
				_, _, _, errs[i] = getOrComputeTranslation("k", false, compute)
			}(i)
		}

		wg.Wait()

		assert.Equal(t, int64(1), computeCount.Load(), "singleflight should coalesce concurrent failed computes")
		for i := range n {
			require.Error(t, errs[i])
		}

		// Error must not be cached — the next sequential call should retry.
		p, _, _, err := getOrComputeTranslation("k", false, func() (*Project, error) {
			return &Project{Identifier: "retry"}, nil
		})
		require.NoError(t, err)
		assert.Equal(t, "retry", p.Identifier)
	})

	t.Run("CachedPointerIsShared", func(t *testing.T) {
		t.Cleanup(resetTranslationCacheForTesting)
		// The LRU returns the same *Project pointer to all callers. Mutating the
		// result of one call poisons the value seen by future callers. Callers must
		// not mutate the returned project.
		compute := func() (*Project, error) {
			return &Project{Tasks: []ProjectTask{{Name: "t1", DependsOn: []TaskUnitDependency{{Name: "dep"}}}}}, nil
		}

		p1, _, _, err := getOrComputeTranslation("k", true, compute)
		require.NoError(t, err)
		p2, _, _, err := getOrComputeTranslation("k", true, compute)
		require.NoError(t, err)

		p1.Tasks[0].DependsOn = nil

		assert.Nil(t, p2.Tasks[0].DependsOn, "mutating one caller's result affects all other callers that hold the same pointer")
	})

	t.Run("ConcurrentReadersOfSharedPointerNoDataRace", func(t *testing.T) {
		t.Cleanup(resetTranslationCacheForTesting)
		// The LRU cache returns the same *Project pointer to concurrent callers.
		compute := func() (*Project, error) {
			return &Project{Identifier: "shared", Tasks: []ProjectTask{{Name: "t1"}}}, nil
		}

		// Warm the cache so all subsequent callers receive the cached pointer.
		_, _, _, err := getOrComputeTranslation("k", true, compute)
		require.NoError(t, err)

		var wg sync.WaitGroup
		for range 20 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				p, _, _, err := getOrComputeTranslation("k", true, compute)
				require.NoError(t, err)
				_ = p.Identifier
				_ = len(p.Tasks)
			}()
		}
		wg.Wait()
	})
}

func TestTranslationCacheStats(t *testing.T) {
	t.Cleanup(resetTranslationCacheForTesting)

	getTranslationCache().Resize(1)

	var callCount int
	compute := func() (*Project, error) {
		callCount++
		return &Project{Identifier: "stats"}, nil
	}

	_, _, _, err := getOrComputeTranslation("k1", true, compute)
	require.NoError(t, err)
	size, evictions, _ := translationCacheStats()
	assert.Equal(t, 1, size)
	assert.Equal(t, int64(0), evictions, "no eviction yet, the single entry still fits")

	// Adding a second entry evicts the first because the LRU was resized down to 1.
	_, _, _, err = getOrComputeTranslation("k2", true, compute)
	require.NoError(t, err)
	size, evictions, _ = translationCacheStats()
	assert.Equal(t, 1, size, "capacity is still 1")
	assert.Equal(t, int64(1), evictions, "adding past capacity evicted the older entry")
}

func TestTranslationCacheBytesLimitFallsBackToDefault(t *testing.T) {
	t.Cleanup(resetTranslationCacheForTesting)

	SetTranslationCacheBytesLimit(0)
	assert.Equal(t, int64(defaultTranslationCacheBytesLimit), currentTranslationCacheBytesLimit(), "non-positive limit falls back to the default")

	SetTranslationCacheBytesLimit(-5)
	assert.Equal(t, int64(defaultTranslationCacheBytesLimit), currentTranslationCacheBytesLimit(), "negative limit falls back to the default")

	SetTranslationCacheBytesLimit(4096)
	assert.Equal(t, int64(4096), currentTranslationCacheBytesLimit(), "positive limit is used as-is")
}

// TestByteBoundedCacheEvictsLRUToBudget shows the cache stays within the configured byte budget by
// evicting the least-recently-used entries, keeping the most recent ones regardless of size.
func TestByteBoundedCacheEvictsLRUToBudget(t *testing.T) {
	t.Cleanup(resetTranslationCacheForTesting)
	resetTranslationCacheForTesting()

	makeProject := func(id string) *Project {
		p := &Project{Identifier: id}
		for i := 0; i < 50; i++ {
			p.Tasks = append(p.Tasks, ProjectTask{Name: fmt.Sprintf("%s-task-%d", id, i)})
		}
		return p
	}
	entrySize, ok := estimateProjectSize(makeProject("sample"))
	require.True(t, ok)
	require.Positive(t, entrySize)

	// Budget holds ~3 entries at a time.
	budget := entrySize*3 + entrySize/2
	SetTranslationCacheBytesLimit(budget)

	for _, k := range []string{"k1", "k2", "k3", "k4", "k5"} {
		_, _, _, err := getOrComputeTranslation(k, true, func() (*Project, error) {
			return makeProject(k), nil
		})
		require.NoError(t, err)
	}

	_, _, bytes := translationCacheStats()
	assert.LessOrEqual(t, bytes, budget, "running byte total stays within budget")
	assert.False(t, getTranslationCache().Contains("k1"), "oldest entry evicted to stay within budget")
	assert.False(t, getTranslationCache().Contains("k2"), "second-oldest entry evicted to stay within budget")
	assert.True(t, getTranslationCache().Contains("k5"), "most-recently-used entry retained")
}

// TestByteBoundedCacheKeepsSingleOversizedEntry shows an entry larger than the whole budget is kept
// rather than evicted immediately, so the budget can be exceeded transiently in that degenerate case.
func TestByteBoundedCacheKeepsSingleOversizedEntry(t *testing.T) {
	t.Cleanup(resetTranslationCacheForTesting)
	resetTranslationCacheForTesting()

	SetTranslationCacheBytesLimit(1)
	_, _, _, err := getOrComputeTranslation("big", true, func() (*Project, error) {
		return &Project{Identifier: "big", Tasks: []ProjectTask{{Name: "t"}}}, nil
	})
	require.NoError(t, err)
	assert.True(t, getTranslationCache().Contains("big"), "a single entry larger than the budget is kept")
}

// TestContentTranslationKeySelfInvalidates shows that changed content yields a new key, so a stale
// entry is never served and no invalidation is needed.
func TestContentTranslationKeySelfInvalidates(t *testing.T) {
	t.Cleanup(resetTranslationCacheForTesting)

	identifier := "myproject"
	keyBefore := contentTranslationKey("sha-before-generate", identifier)
	keyAfter := contentTranslationKey("sha-after-generate", identifier)
	require.NotEqual(t, keyBefore, keyAfter, "changed content must produce a different key")

	pBefore, _, _, err := getOrComputeTranslation(keyBefore, true, func() (*Project, error) {
		return &Project{Identifier: identifier, Tasks: []ProjectTask{{Name: "original"}}}, nil
	})
	require.NoError(t, err)
	require.Len(t, pBefore.Tasks, 1)

	// generate.tasks rewrites the parser project: new content, new key, served fresh.
	pAfter, hit, _, err := getOrComputeTranslation(keyAfter, true, func() (*Project, error) {
		return &Project{Identifier: identifier, Tasks: []ProjectTask{{Name: "original"}, {Name: "generated"}}}, nil
	})
	require.NoError(t, err)
	assert.False(t, hit, "the new content key is a miss, so the new config is computed")
	assert.Len(t, pAfter.Tasks, 2, "post-generation translation reflects the generated task")

	// The stale pre-generation entry remains under its old key but is never looked up again.
	assert.True(t, getTranslationCache().Contains(keyBefore))
}

func TestTranslationKeysAreDistinctAndStable(t *testing.T) {
	// Distinct prefixes: content and version keys never collide across namespaces.
	assert.NotEqual(t, contentTranslationKey("sha", "id"), versionTranslationKey("id", false))
	// Keys are deterministic for identical inputs.
	assert.Equal(t, contentTranslationKey("s", "i"), contentTranslationKey("s", "i"))
	// Different identifiers with identical content yield distinct keys.
	assert.NotEqual(t, contentTranslationKey("s", "a"), contentTranslationKey("s", "b"))
}

func TestFindAndTranslateProjectForVersion(t *testing.T) {
	ctx := t.Context()
	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))
	t.Cleanup(resetTranslationCacheForTesting)
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(ParserProjectCollection, VersionCollection))
	})
	require.NoError(t, db.ClearCollections(ParserProjectCollection, VersionCollection))

	versionID := "version_id"
	projectID := "project_id"
	pp := ParserProject{
		Id:         versionID,
		Identifier: utility.ToStringPtr(projectID),
	}
	require.NoError(t, pp.Insert(ctx))

	project, _, err := FindAndTranslateProjectForVersion(ctx, env.Settings(), &Version{
		Id:                   versionID,
		ProjectStorageMethod: evergreen.ProjectStorageMethodDB,
	}, false)
	require.NoError(t, err)
	require.NotNil(t, project)

	assert.Equal(t, projectID, project.Identifier)
}

// TestFindAndTranslateProjectForVersionReturnedCopyIsolatesCallerMutations reproduces the manifest
// path's real in-place mutation of a returned project (rest/route/agent.go's CreateManifest expands
// Module fields via util.ExpandValues, which writes into the Module struct in place) and confirms it
// cannot corrupt what a later call for the same version sees, with the cache both off and on.
func TestFindAndTranslateProjectForVersionReturnedCopyIsolatesCallerMutations(t *testing.T) {
	ctx := t.Context()
	t.Cleanup(resetTranslationCacheForTesting)
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(ParserProjectCollection, VersionCollection))
	})

	for _, cacheEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("CacheEnabled=%v", cacheEnabled), func(t *testing.T) {
			require.NoError(t, db.ClearCollections(ParserProjectCollection, VersionCollection))
			t.Cleanup(resetTranslationCacheForTesting)

			versionID := "version_id"
			projectID := "project_id"
			pp := ParserProject{
				Id:         versionID,
				Identifier: utility.ToStringPtr(projectID),
				Modules: []Module{
					{Name: "m1", Owner: "orig-owner", Repo: "orig-repo", Ref: "orig-ref"},
				},
			}
			require.NoError(t, pp.Insert(ctx))

			settings := &evergreen.Settings{}
			settings.ServiceFlags.ProjectTranslationCacheEnabled = cacheEnabled
			v := &Version{Id: versionID, ProjectStorageMethod: evergreen.ProjectStorageMethodDB}

			first, _, err := FindAndTranslateProjectForVersion(ctx, settings, v, false)
			require.NoError(t, err)
			require.Len(t, first.Modules, 1)

			// Reproduce util.ExpandValues writing into the Module struct in place, as the manifest
			// path does.
			first.Modules[0].Owner = "expanded-owner"
			first.Modules[0].Repo = "expanded-repo"
			first.Modules[0].Ref = "expanded-ref"

			second, _, err := FindAndTranslateProjectForVersion(ctx, settings, v, false)
			require.NoError(t, err)
			require.Len(t, second.Modules, 1)
			assert.Equal(t, "orig-owner", second.Modules[0].Owner, "a later call must not see the first caller's in-place expansion")
			assert.Equal(t, "orig-repo", second.Modules[0].Repo)
			assert.Equal(t, "orig-ref", second.Modules[0].Ref)
		})
	}
}

func TestFindProjectFromVersionID(t *testing.T) {
	ctx := t.Context()
	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))
	t.Cleanup(resetTranslationCacheForTesting)
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(ParserProjectCollection, VersionCollection))
	})
	require.NoError(t, db.ClearCollections(ParserProjectCollection, VersionCollection))

	versionID := "version_id"
	projectID := "project_id"
	require.NoError(t, (&Version{
		Id:                   versionID,
		Identifier:           projectID,
		ProjectStorageMethod: evergreen.ProjectStorageMethodDB,
	}).Insert(ctx))
	require.NoError(t, (&ParserProject{
		Id: versionID,
	}).Insert(ctx))

	project, err := FindProjectFromVersionID(ctx, versionID)
	require.NoError(t, err)
	require.NotNil(t, project)

	assert.Equal(t, projectID, project.Identifier)
}

func TestParserProjectContentSHA(t *testing.T) {
	t.Run("StableAcrossRepeatedCalls", func(t *testing.T) {
		pp := &ParserProject{
			Identifier: utility.ToStringPtr("my-project"),
			Functions: map[string]*YAMLCommandSet{
				"fn1": {SingleCommand: &PluginCommandConf{Command: "shell.exec"}},
				"fn2": {SingleCommand: &PluginCommandConf{Command: "s3.put"}},
			},
		}
		sha1, err := parserProjectContentSHA(pp)
		require.NoError(t, err)
		sha2, err := parserProjectContentSHA(pp)
		require.NoError(t, err)
		assert.Equal(t, sha1, sha2, "same struct must hash identically on every call")
	})

	t.Run("StableAcrossMapInsertionOrder", func(t *testing.T) {
		// encoding/json sorts map keys, so two maps with the same entries produce the same
		// hash regardless of which order the keys were inserted.
		cmd1 := &PluginCommandConf{Command: "shell.exec"}
		cmd2 := &PluginCommandConf{Command: "s3.put"}
		pp1 := &ParserProject{
			Functions: map[string]*YAMLCommandSet{
				"alpha": {SingleCommand: cmd1},
				"beta":  {SingleCommand: cmd2},
			},
		}
		pp2 := &ParserProject{
			Functions: map[string]*YAMLCommandSet{
				"beta":  {SingleCommand: cmd2},
				"alpha": {SingleCommand: cmd1},
			},
		}
		sha1, err := parserProjectContentSHA(pp1)
		require.NoError(t, err)
		sha2, err := parserProjectContentSHA(pp2)
		require.NoError(t, err)
		assert.Equal(t, sha1, sha2, "map insertion order must not affect the hash")
	})

	t.Run("SensitiveToFieldChanges", func(t *testing.T) {
		pp := &ParserProject{Identifier: utility.ToStringPtr("project-a")}
		sha1, err := parserProjectContentSHA(pp)
		require.NoError(t, err)
		pp.Identifier = utility.ToStringPtr("project-b")
		sha2, err := parserProjectContentSHA(pp)
		require.NoError(t, err)
		assert.NotEqual(t, sha1, sha2, "changed field must produce a different hash")
	})
}

// TestParserProjectContentSHASelfInvalidation verifies that content-hash keying gives a cache hit
// for unchanged content and a miss after content changes, with no invalidation call.
func TestParserProjectContentSHASelfInvalidation(t *testing.T) {
	t.Cleanup(resetTranslationCacheForTesting)

	identifier := "my-project"

	pp := &ParserProject{
		Identifier: utility.ToStringPtr(identifier),
		Tasks:      []parserTask{{Name: "original-task"}},
	}
	sha, err := parserProjectContentSHA(pp)
	require.NoError(t, err)
	key := contentTranslationKey(sha, identifier)

	_, hit, _, err := getOrComputeTranslation(key, true, func() (*Project, error) {
		return &Project{Identifier: identifier, Tasks: []ProjectTask{{Name: "original-task"}}}, nil
	})
	require.NoError(t, err)
	assert.False(t, hit, "first call is a cache miss")

	_, hit, _, err = getOrComputeTranslation(key, true, func() (*Project, error) {
		t.Fatal("compute must not be called on a cache hit")
		return nil, nil
	})
	require.NoError(t, err)
	assert.True(t, hit, "unchanged content key is a cache hit")

	// Simulate generate.tasks: changed content → different hash → new key → miss, no invalidation.
	ppGenerated := &ParserProject{
		Identifier: utility.ToStringPtr(identifier),
		Tasks:      []parserTask{{Name: "original-task"}, {Name: "generated-task"}},
	}
	shaGenerated, err := parserProjectContentSHA(ppGenerated)
	require.NoError(t, err)
	require.NotEqual(t, sha, shaGenerated, "new content must produce a different hash")
	keyGenerated := contentTranslationKey(shaGenerated, identifier)

	pGenerated, hit, _, err := getOrComputeTranslation(keyGenerated, true, func() (*Project, error) {
		return &Project{Identifier: identifier, Tasks: []ProjectTask{{Name: "original-task"}, {Name: "generated-task"}}}, nil
	})
	require.NoError(t, err)
	assert.False(t, hit, "changed content produces a new key that is a cache miss")
	require.Len(t, pGenerated.Tasks, 2)
}
