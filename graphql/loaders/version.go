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
