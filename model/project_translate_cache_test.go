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
	translationGroupPtr.Store(&singleflight.Group{})
}

func TestGetOrComputeTranslation(t *testing.T) {
	t.Run("SequentialCallsEachInvokeCompute", func(t *testing.T) {
		t.Cleanup(resetTranslationCacheForTesting)
		// Singleflight coalesces concurrent calls, not sequential ones.
		var callCount int
		compute := func() (*Project, error) {
			callCount++
			return &Project{Identifier: "test"}, nil
		}

		p1, err := getOrComputeTranslation("k", compute)
		require.NoError(t, err)
		p2, err := getOrComputeTranslation("k", compute)
		require.NoError(t, err)

		assert.Equal(t, 2, callCount)
		assert.Equal(t, "test", p1.Identifier)
		assert.Equal(t, "test", p2.Identifier)
	})

	t.Run("CoalescesConcurrentCalls", func(t *testing.T) {
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
				projs[i], errs[i] = getOrComputeTranslation("k", compute)
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

	t.Run("ErrorPropagatedToAllConcurrentWaiters", func(t *testing.T) {
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
				_, errs[i] = getOrComputeTranslation("k", compute)
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
		p, err := getOrComputeTranslation("k", func() (*Project, error) {
			return &Project{Identifier: "retry"}, nil
		})
		require.NoError(t, err)
		assert.Equal(t, "retry", p.Identifier)
	})
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
