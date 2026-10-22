package model

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	adb "github.com/mongodb/anser/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func findClaim(ctx context.Context, id string) (*SourceCacheClaim, error) {
	var claim SourceCacheClaim
	err := db.FindOneQ(ctx, evergreen.SourceCacheClaimsCollection, db.Query(bson.M{"_id": id}), &claim)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &claim, nil
}

func TestClaimSourceCacheSave(t *testing.T) {
	ctx := t.Context()
	ttl := time.Minute

	t.Run("FirstProducerClaims", func(t *testing.T) {
		require.NoError(t, db.Clear(evergreen.SourceCacheClaimsCollection))
		claimed, err := ClaimSourceCacheSave(ctx, "bucket/key-a", "task-1", ttl)
		require.NoError(t, err)
		assert.True(t, claimed)
	})

	t.Run("FreshClaimIsDeniedToOtherProducers", func(t *testing.T) {
		require.NoError(t, db.Clear(evergreen.SourceCacheClaimsCollection))
		claimed, err := ClaimSourceCacheSave(ctx, "bucket/key-a", "task-1", ttl)
		require.NoError(t, err)
		require.True(t, claimed)

		claimed, err = ClaimSourceCacheSave(ctx, "bucket/key-a", "task-2", ttl)
		require.NoError(t, err)
		assert.False(t, claimed)
	})

	t.Run("OwnerReclaimRefreshesTheTimestamp", func(t *testing.T) {
		require.NoError(t, db.Clear(evergreen.SourceCacheClaimsCollection))
		claimed, err := ClaimSourceCacheSave(ctx, "bucket/key-a", "task-1", ttl)
		require.NoError(t, err)
		require.True(t, claimed)
		before, err := findClaim(ctx, "bucket/key-a")
		require.NoError(t, err)
		require.NotNil(t, before)

		claimed, err = ClaimSourceCacheSave(ctx, "bucket/key-a", "task-1", ttl)
		require.NoError(t, err)
		require.True(t, claimed)
		after, err := findClaim(ctx, "bucket/key-a")
		require.NoError(t, err)
		require.NotNil(t, after)
		assert.True(t, after.TS.After(before.TS), "a reclaim must refresh the timestamp so the claim does not expire under an active producer")
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

		claim, err := findClaim(ctx, "bucket/key-c")
		require.NoError(t, err)
		require.NotNil(t, claim)
		assert.Equal(t, "task-2", claim.Owner, "a takeover must store the new owner")

		// The takeover refreshed the claim, so a third producer racing in is denied.
		claimed, err = ClaimSourceCacheSave(ctx, "bucket/key-c", "task-3", ttl)
		require.NoError(t, err)
		assert.False(t, claimed, "a refreshed claim must deny a later producer")
	})

	t.Run("DifferentKeysStayIndependent", func(t *testing.T) {
		require.NoError(t, db.Clear(evergreen.SourceCacheClaimsCollection))
		claimed, err := ClaimSourceCacheSave(ctx, "bucket/key-a", "task-1", ttl)
		require.NoError(t, err)
		require.True(t, claimed)

		claimed, err = ClaimSourceCacheSave(ctx, "bucket/key-b", "task-2", ttl)
		require.NoError(t, err)
		assert.True(t, claimed, "a claim on one key must not block a different key")
	})
}
