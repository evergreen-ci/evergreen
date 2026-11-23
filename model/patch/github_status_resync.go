package patch

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
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
	// Contexts are the GitHub status contexts that failed to send.
	Contexts []string `bson:"contexts,omitempty"`
	// LastAttemptedAt is when resending the patch's statuses last failed.
	LastAttemptedAt time.Time `bson:"last_attempted_at,omitempty"`
}

var (
	gitHubStatusResyncPatchIDKey         = bsonutil.MustHaveTag(GitHubStatusResync{}, "PatchID")
	gitHubStatusResyncIsMergeQueueKey    = bsonutil.MustHaveTag(GitHubStatusResync{}, "IsMergeQueue")
	gitHubStatusResyncDirtySinceKey      = bsonutil.MustHaveTag(GitHubStatusResync{}, "DirtySince")
	gitHubStatusResyncLastFailedAtKey    = bsonutil.MustHaveTag(GitHubStatusResync{}, "LastFailedAt")
	gitHubStatusResyncContextsKey        = bsonutil.MustHaveTag(GitHubStatusResync{}, "Contexts")
	gitHubStatusResyncLastAttemptedAtKey = bsonutil.MustHaveTag(GitHubStatusResync{}, "LastAttemptedAt")
)

// MarkGitHubStatusResync records that the patch's GitHub status for the given
// context failed to send.
func MarkGitHubStatusResync(ctx context.Context, patchID string, isMergeQueue bool, statusContext string) error {
	now := time.Now()
	_, err := db.Upsert(ctx, GitHubStatusResyncCollection, bson.M{gitHubStatusResyncPatchIDKey: patchID}, bson.M{
		"$min": bson.M{gitHubStatusResyncDirtySinceKey: now},
		"$set": bson.M{
			gitHubStatusResyncIsMergeQueueKey: isMergeQueue,
			gitHubStatusResyncLastFailedAtKey: now,
		},
		"$addToSet": bson.M{gitHubStatusResyncContextsKey: statusContext},
	})
	return errors.Wrapf(err, "marking GitHub statuses for patch '%s' to be resent", patchID)
}

// MarkGitHubStatusResyncAttempted records that resending the patch's statuses
// failed, so patches that keep failing don't hold up the others.
func MarkGitHubStatusResyncAttempted(ctx context.Context, patchID string) error {
	_, err := db.UpdateAll(ctx, GitHubStatusResyncCollection,
		bson.M{gitHubStatusResyncPatchIDKey: patchID},
		bson.M{"$set": bson.M{gitHubStatusResyncLastAttemptedAtKey: time.Now()}},
	)
	return errors.Wrapf(err, "recording GitHub status resync attempt for patch '%s'", patchID)
}

// FindGitHubStatusResync returns the patch's resync record, or nil if the
// patch is not marked.
func FindGitHubStatusResync(ctx context.Context, patchID string) (*GitHubStatusResync, error) {
	resync := &GitHubStatusResync{}
	err := db.FindOneQ(ctx, GitHubStatusResyncCollection, db.Query(bson.M{gitHubStatusResyncPatchIDKey: patchID}), resync)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrapf(err, "finding GitHub status resync for patch '%s'", patchID)
	}
	return resync, nil
}

// FindDueGitHubStatusResyncs returns up to limit patches whose statuses need to
// be resent, with merge queue patches first, then patches that haven't failed a
// resend before, then the patches that have been out of date the longest.
func FindDueGitHubStatusResyncs(ctx context.Context, limit int) ([]GitHubStatusResync, error) {
	q := db.Query(bson.M{gitHubStatusResyncLastFailedAtKey: bson.M{"$gte": time.Now().Add(-GitHubStatusResyncMaxAge)}}).
		Sort([]string{"-" + gitHubStatusResyncIsMergeQueueKey, gitHubStatusResyncLastAttemptedAtKey, gitHubStatusResyncDirtySinceKey}).
		Limit(limit)
	resyncs := []GitHubStatusResync{}
	err := db.FindAllQ(ctx, GitHubStatusResyncCollection, q, &resyncs)
	return resyncs, errors.Wrap(err, "finding due GitHub status resyncs")
}

// ClearGitHubStatusResync removes the patch's mark unless a send failed after
// the given time, so a failure that happens while the patch's statuses are
// being resent is not lost.
func ClearGitHubStatusResync(ctx context.Context, patchID string, lastFailedAt time.Time) error {
	return errors.Wrapf(db.Remove(ctx, GitHubStatusResyncCollection, bson.M{
		gitHubStatusResyncPatchIDKey:      patchID,
		gitHubStatusResyncLastFailedAtKey: bson.M{"$lte": lastFailedAt},
	}), "clearing GitHub status resync for patch '%s'", patchID)
}
