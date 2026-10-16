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

// ClaimSourceCacheSave claims the save right for claimID on behalf of owner; the owner re-claims its own live claim, and a different producer only takes over a claim older than ttl.
func ClaimSourceCacheSave(ctx context.Context, claimID, owner string, ttl time.Duration) (bool, error) {
	now := time.Now()
	err := db.Insert(ctx, evergreen.SourceCacheClaimsCollection, SourceCacheClaim{ID: claimID, Owner: owner, TS: now})
	if err == nil {
		return true, nil
	}
	if !db.IsDuplicateKey(err) {
		return false, errors.Wrapf(err, "claiming source cache save for '%s'", claimID)
	}

	var claim SourceCacheClaim
	changeInfo, err := db.FindAndModify(ctx, evergreen.SourceCacheClaimsCollection,
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
			ReturnNew: true,
		}, &claim)
	if err != nil {
		// A fresh claim held by another producer matches neither branch.
		if adb.ResultsNotFound(err) {
			return false, nil
		}
		return false, errors.Wrapf(err, "taking over source cache claim for '%s'", claimID)
	}
	return changeInfo != nil && changeInfo.Updated > 0, nil
}

// ReleaseSourceCacheSave deletes the claim for claimID when owner holds it and reports whether a claim was deleted.
func ReleaseSourceCacheSave(ctx context.Context, claimID, owner string) (bool, error) {
	res, err := evergreen.GetEnvironment().DB().Collection(evergreen.SourceCacheClaimsCollection).DeleteOne(ctx,
		bson.M{sourceCacheClaimIDKey: claimID, sourceCacheClaimOwnerKey: owner})
	if err != nil {
		return false, errors.Wrapf(err, "releasing source cache claim for '%s'", claimID)
	}
	return res.DeletedCount > 0, nil
}
