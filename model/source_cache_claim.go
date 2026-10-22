package model

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	sourceCacheClaimIDKey    = "_id"
	sourceCacheClaimOwnerKey = "owner"
	sourceCacheClaimTSKey    = "ts"
)

// SourceCacheClaim records that one producer owns saving a source cache object.
type SourceCacheClaim struct {
	ID    string    `bson:"_id"`
	Owner string    `bson:"owner"`
	TS    time.Time `bson:"ts"`
}

// ClaimSourceCacheSave claims the save right for claimID on behalf of owner; the owner re-claims its own live claim, and a different producer only takes over a claim older than ttl. The upsert is atomic, so racing producers cannot both win.
func ClaimSourceCacheSave(ctx context.Context, claimID, owner string, ttl time.Duration) (bool, error) {
	now := time.Now()
	_, err := db.FindAndModify(ctx, evergreen.SourceCacheClaimsCollection,
		bson.M{
			sourceCacheClaimIDKey: claimID,
			"$or": []bson.M{
				{sourceCacheClaimOwnerKey: owner},
				{sourceCacheClaimTSKey: bson.M{"$lte": now.Add(-ttl)}},
			},
		},
		[]string{},
		adb.Change{
			Update:    bson.M{"$set": bson.M{sourceCacheClaimOwnerKey: owner, sourceCacheClaimTSKey: now}},
			Upsert:    true,
			ReturnNew: true,
		}, &SourceCacheClaim{})
	if err != nil {
		// A fresh claim held by another producer matches neither branch, so the upsert collides on the key and this producer is denied.
		if db.IsDuplicateKey(err) {
			return false, nil
		}
		return false, errors.Wrapf(err, "claiming source cache save for '%s'", claimID)
	}
	return true, nil
}
