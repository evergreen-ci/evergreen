package patch

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// GitHubStatusResyncCollection stores patches whose GitHub statuses failed to
// send and need to be sent again. Records expire 6 hours after the last failed
// send.
const GitHubStatusResyncCollection = "github_status_resyncs"

// GitHubStatusResyncMaxAge is how long after the last failed send a patch's
// statuses are still resent. It matches the TTL index on last_failed_at.
const GitHubStatusResyncMaxAge = 6 * time.Hour

// GitHubStatusResync marks a patch whose GitHub statuses are out of date
// because sending one of them failed.
type GitHubStatusResync struct {
	PatchID      string    `bson:"_id"`
	IsMergeQueue bool      `bson:"is_merge_queue"`
	DirtySince   time.Time `bson:"dirty_since"`
	LastFailedAt time.Time `bson:"last_failed_at"`
}

var (
	gitHubStatusResyncPatchIDKey      = bsonutil.MustHaveTag(GitHubStatusResync{}, "PatchID")
	gitHubStatusResyncIsMergeQueueKey = bsonutil.MustHaveTag(GitHubStatusResync{}, "IsMergeQueue")
	gitHubStatusResyncDirtySinceKey   = bsonutil.MustHaveTag(GitHubStatusResync{}, "DirtySince")
	gitHubStatusResyncLastFailedAtKey = bsonutil.MustHaveTag(GitHubStatusResync{}, "LastFailedAt")
)

// MarkGitHubStatusResync records that a GitHub status for the patch failed to
// send.
func MarkGitHubStatusResync(ctx context.Context, patchID string, isMergeQueue bool) error {
	now := time.Now()
	_, err := db.Upsert(ctx, GitHubStatusResyncCollection, bson.M{gitHubStatusResyncPatchIDKey: patchID}, bson.M{
		"$min": bson.M{gitHubStatusResyncDirtySinceKey: now},
		"$set": bson.M{
			gitHubStatusResyncIsMergeQueueKey: isMergeQueue,
			gitHubStatusResyncLastFailedAtKey: now,
		},
	})
	return errors.Wrapf(err, "marking GitHub statuses for patch '%s' to be resent", patchID)
}

// FindDueGitHubStatusResyncs returns up to limit patches whose statuses need to
// be resent, with merge queue patches first and then the patches that have been
// out of date the longest.
func FindDueGitHubStatusResyncs(ctx context.Context, limit int) ([]GitHubStatusResync, error) {
	q := db.Query(bson.M{gitHubStatusResyncLastFailedAtKey: bson.M{"$gte": time.Now().Add(-GitHubStatusResyncMaxAge)}}).
		Sort([]string{"-" + gitHubStatusResyncIsMergeQueueKey, gitHubStatusResyncDirtySinceKey}).
		Limit(limit)
	resyncs := []GitHubStatusResync{}
	err := db.FindAllQ(ctx, GitHubStatusResyncCollection, q, &resyncs)
	return resyncs, errors.Wrap(err, "finding due GitHub status resyncs")
}

// ClearGitHubStatusResync removes the patch's mark unless a send failed at or
// after the given time, so a failure that happens while the patch's statuses
// are being resent is not lost.
func ClearGitHubStatusResync(ctx context.Context, patchID string, before time.Time) error {
	return errors.Wrapf(db.Remove(ctx, GitHubStatusResyncCollection, bson.M{
		gitHubStatusResyncPatchIDKey:      patchID,
		gitHubStatusResyncLastFailedAtKey: bson.M{"$lt": before},
	}), "clearing GitHub status resync for patch '%s'", patchID)
}
