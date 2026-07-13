package model

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setParserProjectReadForTesting swaps the parser-project read seam and restores it on cleanup.
func setParserProjectReadForTesting(t *testing.T, fn func(ctx context.Context, settings *evergreen.Settings, method evergreen.ParserProjectStorageMethod, id string) (*ParserProject, error)) {
	orig := parserProjectFindOneByID
	parserProjectFindOneByID = fn
	t.Cleanup(func() { parserProjectFindOneByID = orig })
}

func TestReadCoalescing(t *testing.T) {
	t.Run("ConcurrentSameVersionReadsPerformExactlyOneRead", func(t *testing.T) {
		t.Cleanup(resetTranslationCacheForTesting)
		resetTranslationCacheForTesting()

		var reads atomic.Int64
		release := make(chan struct{})
		started := make(chan struct{}, 1)
		setParserProjectReadForTesting(t, func(ctx context.Context, _ *evergreen.Settings, _ evergreen.ParserProjectStorageMethod, id string) (*ParserProject, error) {
			reads.Add(1)
			started <- struct{}{}
			<-release
			return &ParserProject{Id: id}, nil
		})

		const n = 5
		var dedupedCount, leaderCount atomic.Int64
		var wg sync.WaitGroup

		// Launch the leader and wait until its read is in flight, so the followers coalesce.
		_, leaderSpan := tracer.Start(t.Context(), "leader")
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, deduped := readAndTranslateProjectForVersionCoalesced(t.Context(), leaderSpan, &evergreen.Settings{}, "v1", "", evergreen.ProjectStorageMethodDB, "", false, true)
			require.NoError(t, res.err)
			if deduped {
				dedupedCount.Add(1)
			} else {
				leaderCount.Add(1)
			}
		}()
		<-started

		for range n - 1 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, span := tracer.Start(t.Context(), "follower")
				res, deduped := readAndTranslateProjectForVersionCoalesced(t.Context(), span, &evergreen.Settings{}, "v1", "", evergreen.ProjectStorageMethodDB, "", false, true)
				require.NoError(t, res.err)
				if deduped {
					dedupedCount.Add(1)
				} else {
					leaderCount.Add(1)
				}
			}()
		}

		// Give followers time to join the in-flight singleflight before releasing the read.
		time.Sleep(100 * time.Millisecond)
		close(release)
		wg.Wait()

		assert.Equal(t, int64(1), reads.Load(), "a burst of same-version reads performs exactly one read")
		assert.Equal(t, int64(1), leaderCount.Load(), "exactly one leader")
		assert.Equal(t, int64(n-1), dedupedCount.Load(), "all followers report read_deduped")
	})

	t.Run("DifferentVersionsAreNotCoalesced", func(t *testing.T) {
		t.Cleanup(resetTranslationCacheForTesting)
		resetTranslationCacheForTesting()

		var reads atomic.Int64
		release := make(chan struct{})
		var startedWG sync.WaitGroup
		startedWG.Add(2)
		setParserProjectReadForTesting(t, func(ctx context.Context, _ *evergreen.Settings, _ evergreen.ParserProjectStorageMethod, id string) (*ParserProject, error) {
			reads.Add(1)
			startedWG.Done()
			<-release
			return &ParserProject{Id: id}, nil
		})

		var wg sync.WaitGroup
		for _, versionID := range []string{"v1", "v2"} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, span := tracer.Start(t.Context(), "call")
				res, deduped := readAndTranslateProjectForVersionCoalesced(t.Context(), span, &evergreen.Settings{}, versionID, "", evergreen.ProjectStorageMethodDB, "", false, true)
				require.NoError(t, res.err)
				assert.False(t, deduped, "different versions never coalesce")
			}()
		}
		startedWG.Wait()
		close(release)
		wg.Wait()

		assert.Equal(t, int64(2), reads.Load(), "different versions each read")
	})

	t.Run("OptedOutReadsAreNotShared", func(t *testing.T) {
		t.Cleanup(resetTranslationCacheForTesting)
		resetTranslationCacheForTesting()

		release := make(chan struct{})
		started := make(chan struct{}, 2)
		setParserProjectReadForTesting(t, func(ctx context.Context, _ *evergreen.Settings, _ evergreen.ParserProjectStorageMethod, id string) (*ParserProject, error) {
			started <- struct{}{}
			<-release
			return &ParserProject{Id: id, Tasks: []parserTask{{Name: "t"}}}, nil
		})

		var mu sync.Mutex
		var pps []*ParserProject
		var wg sync.WaitGroup
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				// coalesceRead=false, as the generate path uses, so each caller gets its own pp.
				_, pp, err := FindAndTranslateProjectForVersionWithOpts(t.Context(), &evergreen.Settings{}, &Version{Id: "v1", ProjectStorageMethod: evergreen.ProjectStorageMethodDB}, false, false)
				require.NoError(t, err)
				mu.Lock()
				pps = append(pps, pp)
				mu.Unlock()
			}()
		}
		<-started
		<-started
		close(release)
		wg.Wait()

		require.Len(t, pps, 2)
		assert.NotSame(t, pps[0], pps[1], "opted-out callers must not share a parser project pointer")
		// Mutating one must not affect the other, as the generate path mutates pp in place.
		pps[0].Tasks = append(pps[0].Tasks, parserTask{Name: "generated"})
		assert.Len(t, pps[1].Tasks, 1, "a mutation of one caller's pp must not be visible to another")
	})

	t.Run("LeaderContextCancellationDoesNotFailFollower", func(t *testing.T) {
		t.Cleanup(resetTranslationCacheForTesting)
		resetTranslationCacheForTesting()

		release := make(chan struct{})
		started := make(chan struct{}, 1)
		setParserProjectReadForTesting(t, func(ctx context.Context, _ *evergreen.Settings, _ evergreen.ParserProjectStorageMethod, id string) (*ParserProject, error) {
			started <- struct{}{}
			<-release
			// The detached read context must stay live even after a caller cancels.
			require.NoError(t, ctx.Err())
			return &ParserProject{Id: id}, nil
		})

		leaderCtx, cancelLeader := context.WithCancel(t.Context())
		var wg sync.WaitGroup
		var leaderErr, followerErr error

		_, leaderSpan := tracer.Start(leaderCtx, "leader")
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, _ := readAndTranslateProjectForVersionCoalesced(leaderCtx, leaderSpan, &evergreen.Settings{}, "v1", "", evergreen.ProjectStorageMethodDB, "", false, true)
			leaderErr = res.err
		}()
		<-started

		_, followerSpan := tracer.Start(t.Context(), "follower")
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, _ := readAndTranslateProjectForVersionCoalesced(t.Context(), followerSpan, &evergreen.Settings{}, "v1", "", evergreen.ProjectStorageMethodDB, "", false, true)
			followerErr = res.err
		}()

		time.Sleep(100 * time.Millisecond)
		cancelLeader()
		close(release)
		wg.Wait()

		assert.NoError(t, leaderErr, "detached context lets the leader finish despite cancellation")
		assert.NoError(t, followerErr, "a follower with a live context is not failed by the leader cancelling")
	})

	t.Run("ReadErrorIsSurfacedToAllCallersAndNotCached", func(t *testing.T) {
		t.Cleanup(resetTranslationCacheForTesting)
		resetTranslationCacheForTesting()

		readErr := errors.New("transient read failure")
		var reads atomic.Int64
		release := make(chan struct{})
		started := make(chan struct{}, 1)
		setParserProjectReadForTesting(t, func(ctx context.Context, _ *evergreen.Settings, _ evergreen.ParserProjectStorageMethod, id string) (*ParserProject, error) {
			if reads.Add(1) == 1 {
				started <- struct{}{}
				<-release
			}
			return nil, readErr
		})

		const n = 3
		var wg sync.WaitGroup
		var errCount atomic.Int64
		_, leaderSpan := tracer.Start(t.Context(), "leader")
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, _ := readAndTranslateProjectForVersionCoalesced(t.Context(), leaderSpan, &evergreen.Settings{}, "v1", "", evergreen.ProjectStorageMethodDB, "", false, true)
			if res.err != nil {
				errCount.Add(1)
			}
		}()
		<-started

		for range n - 1 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, span := tracer.Start(t.Context(), "follower")
				res, _ := readAndTranslateProjectForVersionCoalesced(t.Context(), span, &evergreen.Settings{}, "v1", "", evergreen.ProjectStorageMethodDB, "", false, true)
				if res.err != nil {
					errCount.Add(1)
				}
			}()
		}
		time.Sleep(100 * time.Millisecond)
		close(release)
		wg.Wait()

		assert.Equal(t, int64(n), errCount.Load(), "the read error is surfaced to every coalesced caller")
		// The bounded retry fires within the coalesced closure (MaxAttempts=2), so the leader read twice.
		assert.Equal(t, int64(2), reads.Load(), "the inline bounded retry fires for the leader")

		// A failure is not cached: the next request reads fresh.
		reads.Store(0)
		setParserProjectReadForTesting(t, func(ctx context.Context, _ *evergreen.Settings, _ evergreen.ParserProjectStorageMethod, id string) (*ParserProject, error) {
			reads.Add(1)
			return &ParserProject{Id: id, Identifier: utility.ToStringPtr("proj")}, nil
		})
		_, retrySpan := tracer.Start(t.Context(), "retry")
		res, _ := readAndTranslateProjectForVersionCoalesced(t.Context(), retrySpan, &evergreen.Settings{}, "v1", "", evergreen.ProjectStorageMethodDB, "", false, true)
		require.NoError(t, res.err)
		assert.Equal(t, int64(1), reads.Load(), "a failed read is not cached; the next request reads fresh")
	})
}
