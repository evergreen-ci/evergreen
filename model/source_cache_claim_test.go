package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaimSourceCacheSave(t *testing.T) {
	ctx := t.Context()
	require.NoError(t, db.Clear(evergreen.SourceCacheClaimsCollection), "clearing prior claims")

	ttl := time.Minute

	t.Run("FirstProducerClaims", func(t *testing.T) {
		claimed, err := ClaimSourceCacheSave(ctx, "bucket/key-a", "task-1", ttl)
		require.NoError(t, err)
		assert.True(t, claimed)
	})

	t.Run("FreshClaimIsDeniedToOtherProducers", func(t *testing.T) {
		claimed, err := ClaimSourceCacheSave(ctx, "bucket/key-a", "task-2", ttl)
		require.NoError(t, err)
		assert.False(t, claimed)
	})

	t.Run("OwnerReclaimsAfterLostResponse", func(t *testing.T) {
		claimed, err := ClaimSourceCacheSave(ctx, "bucket/key-a", "task-1", ttl)
		require.NoError(t, err)
		assert.True(t, claimed)
	})

	t.Run("StaleClaimIsTakenOverByAnotherProducer", func(t *testing.T) {
		require.NoError(t, db.Clear(evergreen.SourceCacheClaimsCollection))
		require.NoError(t, db.Insert(ctx, evergreen.SourceCacheClaimsCollection, SourceCacheClaim{
			ID:    "bucket/key-c",
			Owner: "task-1",
			TS:    time.Now().Add(-2 * ttl),
		}))
		claimed, err := ClaimSourceCacheSave(ctx, "bucket/key-c", "task-2", ttl)
		require.NoError(t, err)
		assert.True(t, claimed)
	})
}

func TestReleaseSourceCacheSave(t *testing.T) {
	ctx := t.Context()

	t.Run("OwnerReleasesTheClaim", func(t *testing.T) {
		require.NoError(t, db.Clear(evergreen.SourceCacheClaimsCollection))
		claimed, err := ClaimSourceCacheSave(ctx, "bucket/key-a", "task-1", time.Minute)
		require.NoError(t, err)
		require.True(t, claimed)

		released, err := ReleaseSourceCacheSave(ctx, "bucket/key-a", "task-1")
		require.NoError(t, err)
		assert.True(t, released)
	})

	t.Run("NonOwnerCannotRelease", func(t *testing.T) {
		require.NoError(t, db.Clear(evergreen.SourceCacheClaimsCollection))
		claimed, err := ClaimSourceCacheSave(ctx, "bucket/key-b", "task-1", time.Minute)
		require.NoError(t, err)
		require.True(t, claimed)

		released, err := ReleaseSourceCacheSave(ctx, "bucket/key-b", "task-2")
		require.NoError(t, err)
		assert.False(t, released)

		claimed, err = ClaimSourceCacheSave(ctx, "bucket/key-b", "task-3", time.Minute)
		require.NoError(t, err)
		assert.False(t, claimed, "the claim must still block other producers")
	})

	t.Run("ReleasingAnUnclaimedKeyIsNotAnError", func(t *testing.T) {
		released, err := ReleaseSourceCacheSave(ctx, "bucket/key-never-claimed", "task-1")
		require.NoError(t, err)
		assert.False(t, released)
	})
}
