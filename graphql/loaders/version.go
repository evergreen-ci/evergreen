package loaders

import (
	"context"
	"errors"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/vikstrous/dataloadgen"
	"go.mongodb.org/mongo-driver/bson"
)

type versionReader struct{}

func (v *versionReader) getVersions(ctx context.Context, versionIDs []string) (map[string]*model.Version, error) {
	query := db.Query(bson.M{model.VersionIdKey: bson.M{"$in": versionIDs}}).Project(bson.M{model.VersionBuildVariantsKey: 0})
	versions, err := model.VersionFind(ctx, query)
	if err != nil {
		grip.Error(ctx, message.WrapError(err, message.Fields{
			"message": "error fetching versions in dataloader",
		}))
		return nil, &batchError{err: err}
	}

	versionMap := make(map[string]*model.Version, len(versions))
	for i := range versions {
		versionMap[versions[i].Id] = &versions[i]
	}

	return versionMap, nil
}

// GetVersion returns a single version by ID efficiently using the dataloader.
// Returns nil if the version is not found.
func GetVersion(ctx context.Context, versionID string) (*model.Version, error) {
	l := For(ctx)
	result, err := l.VersionLoader.Load(ctx, versionID)
	if errors.Is(err, dataloadgen.ErrNotFound) {
		return nil, nil
	}
	return result, err
}

// PreloadVersions enqueues every version ID into the dataloader's current batch
// in a single synchronous loop, guaranteeing that subsequent GetVersion calls
// for these IDs are served from the loader's thunk cache without any additional
// MongoDB queries. Use this when a resolver knows up front that it will need
// many versions whose loads would otherwise be split across multiple batches
// due to the wait-time window (e.g. the TaskHistory resolver, where hundreds of
// inactive tasks each request their own version).
//
// Errors are intentionally discarded here; per-key errors are still surfaced to
// the individual GetVersion callers via the cached thunks.
func PreloadVersions(ctx context.Context, versionIDs []string) {
	if len(versionIDs) == 0 {
		return
	}
	l := For(ctx)
	_, _ = l.VersionLoader.LoadAll(ctx, versionIDs)
}
