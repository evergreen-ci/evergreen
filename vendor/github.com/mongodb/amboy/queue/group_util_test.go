package queue

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGroupCache(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	queue := NewLocalLimitedSize(2, 128)
	require.NotNil(t, queue)

	for _, impl := range []struct {
		name    string
		factory func() GroupCache
	}{
		{
			name:    "BaseMinute",
			factory: func() GroupCache { return NewGroupCache(time.Minute) },
		},
		{
			name:    "BaseZero",
			factory: func() GroupCache { return NewGroupCache(0) },
		},
		{
			name:    "BaseHour",
			factory: func() GroupCache { return NewGroupCache(time.Hour) },
		},
	} {
		t.Run(impl.name, func(t *testing.T) {
			for _, test := range []struct {
				name string
				test func(*testing.T, GroupCache)
			}{
				{
					name: "ValidateFixture",
					test: func(t *testing.T, cache GroupCache) {
						require.Len(t, cache.Names(), 0)
						require.Zero(t, cache.Len())
					},
				},
				{
					name: "GetNilCase",
					test: func(t *testing.T, cache GroupCache) {
						require.Nil(t, cache.Get("foo"))
					},
				},
				{
					name: "SetNilCase",
					test: func(t *testing.T, cache GroupCache) {
						require.Error(t, cache.Set("foo", nil, 0))
					},
				},
				{
					name: "SetZero",
					test: func(t *testing.T, cache GroupCache) {
						require.NoError(t, cache.Set("foo", queue, 0))

						require.Len(t, cache.Names(), 1)
						require.Equal(t, 1, cache.Len())
					},
				},
				{
					name: "DoubleSet",
					test: func(t *testing.T, cache GroupCache) {
						require.NoError(t, cache.Set("foo", queue, 0))
						require.Error(t, cache.Set("foo", queue, 0))
					},
				},
				{
					name: "RoundTrip",
					test: func(t *testing.T, cache GroupCache) {
						require.NoError(t, cache.Set("foo", queue, 0))
						require.Equal(t, queue, cache.Get("foo"))
					},
				},
				{
					name: "RemoveNonExistantQueue",
					test: func(t *testing.T, cache GroupCache) {
						require.NoError(t, cache.Remove(ctx, "foo"))
					},
				},
				{
					name: "RemoveSteadyQueue",
					test: func(t *testing.T, cache GroupCache) {
						require.NoError(t, cache.Set("foo", queue, 0))
						require.Equal(t, 1, cache.Len())
						require.NoError(t, cache.Remove(ctx, "foo"))
						require.Equal(t, 0, cache.Len())
					},
				},
				{
					name: "RemoveQueueWithWork",
					test: func(t *testing.T, cache GroupCache) {
						q := NewLocalLimitedSize(1, 128)
						require.NoError(t, q.Start(ctx))
						require.NoError(t, q.Put(ctx, &sleepJob{Sleep: time.Minute}))

						require.NoError(t, cache.Set("foo", q, 1))
						require.Equal(t, 1, cache.Len())
						require.Error(t, cache.Remove(ctx, "foo"))
						require.Equal(t, 1, cache.Len())
					},
				},
				{
					name: "PruneNil",
					test: func(t *testing.T, cache GroupCache) {
						require.NoError(t, cache.Prune(ctx))
					},
				},
				{
					name: "PruneOne",
					test: func(t *testing.T, cache GroupCache) {
						require.NoError(t, cache.Set("foo", queue, time.Millisecond))
						time.Sleep(2 * time.Millisecond)
						require.NoError(t, cache.Prune(ctx))
						require.Zero(t, cache.Len())
					},
				},
				{
					name: "PruneWithCanceledContext",
					test: func(t *testing.T, cache GroupCache) {
						tctx, cancel := context.WithCancel(ctx)
						cancel()

						require.NoError(t, cache.Set("foo", queue, time.Hour))
						require.Equal(t, 1, cache.Len())
						require.Error(t, cache.Prune(tctx))
						require.Equal(t, 1, cache.Len())
					},
				},
				{
					name: "PruneWithUnexpiredTTL",
					test: func(t *testing.T, cache GroupCache) {
						require.NoError(t, cache.Set("foo", queue, time.Hour))
						require.Equal(t, 1, cache.Len())
						require.NoError(t, cache.Prune(ctx))
						require.Equal(t, 1, cache.Len())
					},
				},
				{
					name: "CloseNoop",
					test: func(t *testing.T, cache GroupCache) {
						require.NoError(t, cache.Close(ctx))
					},
				},
				{
					name: "CloseErrorsCtxCancel",
					test: func(t *testing.T, cache GroupCache) {
						tctx, cancel := context.WithCancel(ctx)
						cancel()

						require.NoError(t, cache.Set("foo", queue, time.Hour))
						require.Equal(t, 1, cache.Len())
						require.Error(t, cache.Close(tctx))
						require.Equal(t, 1, cache.Len())
					},
				},
				{
					name: "CloseEmptyErrorsCtxCancel",
					test: func(t *testing.T, cache GroupCache) {
						tctx, cancel := context.WithCancel(ctx)
						cancel()

						require.Error(t, cache.Close(tctx))
					},
				},
				{
					name: "ClosingClearsQueue",
					test: func(t *testing.T, cache GroupCache) {
						require.NoError(t, cache.Set("foo", queue, time.Hour))
						require.Equal(t, 1, cache.Len())
						require.NoError(t, cache.Close(ctx))
						require.Equal(t, 0, cache.Len())

					},
				},
				// {
				//	name: "",
				//	test: func(t *testing.T, cache GroupCache) {
				//	},
				// },
				// {
				//	name: "",
				//	test: func(t *testing.T, cache GroupCache) {
				//	},
				// },
			} {
				t.Run(test.name, func(t *testing.T) {
					cache := impl.factory()
					require.NotNil(t, cache)

					test.test(t, cache)
				})
			}
		})
	}

}
