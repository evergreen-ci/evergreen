package model

import (
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

		p1, hit1, err := getOrComputeTranslation("k", false, compute)
		require.NoError(t, err)
		p2, hit2, err := getOrComputeTranslation("k", false, compute)
		require.NoError(t, err)

		assert.Equal(t, 2, callCount)
		assert.False(t, hit1, "cache disabled, never a hit")
		assert.False(t, hit2, "cache disabled, never a hit")
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

		p1, hit1, err := getOrComputeTranslation("k", true, compute)
		require.NoError(t, err)
		p2, hit2, err := getOrComputeTranslation("k", true, compute)
		require.NoError(t, err)

		assert.Equal(t, 1, callCount, "second call should be served from cache")
		assert.False(t, hit1, "first call computes")
		assert.True(t, hit2, "second call is an LRU hit")
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

		_, _, err := getOrComputeTranslation("k", true, compute)
		require.Error(t, err)

		p, _, err := getOrComputeTranslation("k", true, compute)
		require.NoError(t, err)

		assert.Equal(t, 2, callCount, "error result must not be cached")
		assert.Equal(t, "retry", p.Identifier)
	})

	t.Run("CoalescesConcurrentCallsLRUEnabled", func(t *testing.T) {
		t.Cleanup(resetTranslationCacheForTesting)

		entered := make(chan struct{})
		release := make(chan struct{})
		var computeCount atomic.Int64
		compute := func() (*Project, error) {
			select {
			case entered <- struct{}{}: // signal first entry
			default:
			}
			<-release
			computeCount.Add(1)
			return &Project{Identifier: "coalesced"}, nil
		}

		const n = 5
		projs := make([]*Project, n)
		errs := make([]error, n)
		var wg sync.WaitGroup
		for i := range n {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				projs[i], _, errs[i] = getOrComputeTranslation("k", true, compute)
			}(i)
		}

		<-entered                        // first goroutine is inside compute
		time.Sleep(5 * time.Millisecond) // let remaining goroutines queue in singleflight
		close(release)
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
		entered := make(chan struct{})
		release := make(chan struct{})
		var computeCount atomic.Int64
		compute := func() (*Project, error) {
			select {
			case entered <- struct{}{}: // signal first entry
			default:
			}
			<-release
			computeCount.Add(1)
			return &Project{Identifier: "coalesced-no-lru"}, nil
		}

		const n = 5
		projs := make([]*Project, n)
		errs := make([]error, n)
		var wg sync.WaitGroup
		for i := range n {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				projs[i], _, errs[i] = getOrComputeTranslation("k", false, compute)
			}(i)
		}

		<-entered
		time.Sleep(5 * time.Millisecond)
		close(release)
		wg.Wait()

		assert.Equal(t, int64(1), computeCount.Load(), "singleflight coalesces even without LRU")
		for i := range n {
			require.NoError(t, errs[i])
			assert.Equal(t, "coalesced-no-lru", projs[i].Identifier)
		}
	})

	t.Run("LRUDisabledErrorPropagatedToAllWaiters", func(t *testing.T) {
		t.Cleanup(resetTranslationCacheForTesting)
		// All concurrent singleflight waiters receive the error when compute fails.
		// The next sequential call must retry (singleflight does not cache errors).
		entered := make(chan struct{})
		release := make(chan struct{})
		var computeCount atomic.Int64
		compute := func() (*Project, error) {
			select {
			case entered <- struct{}{}:
			default:
			}
			<-release
			computeCount.Add(1)
			return nil, errors.New("compute error")
		}

		const n = 5
		errs := make([]error, n)
		var wg sync.WaitGroup
		for i := range n {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_, _, errs[i] = getOrComputeTranslation("k", false, compute)
			}(i)
		}

		<-entered
		time.Sleep(5 * time.Millisecond)
		close(release)
		wg.Wait()

		assert.Equal(t, int64(1), computeCount.Load(), "singleflight should coalesce concurrent failed computes")
		for i := range n {
			require.Error(t, errs[i])
		}

		// Error must not be cached — the next sequential call should retry.
		p, _, err := getOrComputeTranslation("k", false, func() (*Project, error) {
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

		p1, _, err := getOrComputeTranslation("k", true, compute)
		require.NoError(t, err)
		p2, _, err := getOrComputeTranslation("k", true, compute)
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
		_, _, err := getOrComputeTranslation("k", true, compute)
		require.NoError(t, err)

		var wg sync.WaitGroup
		for range 20 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				p, _, err := getOrComputeTranslation("k", true, compute)
				require.NoError(t, err)
				_ = p.Identifier
				_ = len(p.Tasks)
			}()
		}
		wg.Wait()
	})
}

// TestContentTranslationKeySelfInvalidates shows that changed content yields a new key, so a stale
// entry is never served and no invalidation is needed.
func TestContentTranslationKeySelfInvalidates(t *testing.T) {
	t.Cleanup(resetTranslationCacheForTesting)

	identifier := "myproject"
	keyBefore := contentTranslationKey("sha-before-generate", identifier)
	keyAfter := contentTranslationKey("sha-after-generate", identifier)
	require.NotEqual(t, keyBefore, keyAfter, "changed content must produce a different key")

	pBefore, _, err := getOrComputeTranslation(keyBefore, true, func() (*Project, error) {
		return &Project{Identifier: identifier, Tasks: []ProjectTask{{Name: "original"}}}, nil
	})
	require.NoError(t, err)
	require.Len(t, pBefore.Tasks, 1)

	// generate.tasks rewrites the parser project: new content, new key, served fresh.
	pAfter, hit, err := getOrComputeTranslation(keyAfter, true, func() (*Project, error) {
		return &Project{Identifier: identifier, Tasks: []ProjectTask{{Name: "original"}, {Name: "generated"}}}, nil
	})
	require.NoError(t, err)
	assert.False(t, hit, "the new content key is a miss, so the new config is computed")
	assert.Len(t, pAfter.Tasks, 2, "post-generation translation reflects the generated task")

	// The stale pre-generation entry remains under its old key but is never looked up again.
	assert.True(t, getTranslationCache().Contains(keyBefore))
}

func TestTranslationKeysAreDistinctAndStable(t *testing.T) {
	// Distinct prefixes: an identical SHA never collides across the two key namespaces.
	assert.NotEqual(t, contentTranslationKey("sha", "id"), fileTranslationKey("id", "rev", "sha"))
	// Keys are deterministic for identical inputs.
	assert.Equal(t, contentTranslationKey("s", "i"), contentTranslationKey("s", "i"))
	assert.Equal(t, fileTranslationKey("p", "r", "s"), fileTranslationKey("p", "r", "s"))
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
