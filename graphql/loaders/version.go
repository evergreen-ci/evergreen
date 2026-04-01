package loaders

import (
	"context"
	"errors"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/vikstrous/dataloadgen"
	"go.mongodb.org/mongo-driver/bson"
)

type apiVersionReader struct{}

func (v *apiVersionReader) getAPIVersions(ctx context.Context, versionIDs []string) (map[string]*restModel.APIVersion, error) {
	query := db.Query(bson.M{model.VersionIdKey: bson.M{"$in": versionIDs}}).Project(bson.M{model.VersionBuildVariantsKey: 0})
	versions, err := model.VersionFind(ctx, query)
	if err != nil {
		grip.Error(ctx, message.WrapError(err, message.Fields{
			"message": "error fetching versions in dataloader",
		}))
		return nil, &batchError{err: err}
	}

	apiVersionMap := make(map[string]*restModel.APIVersion, len(versions))
	for i := range versions {
		apiVersion := &restModel.APIVersion{}
		apiVersion.BuildFromService(ctx, versions[i])
		apiVersionMap[versions[i].Id] = apiVersion
	}

	return apiVersionMap, nil
}

// GetAPIVersion returns a single version by ID efficiently using the dataloader.
// Returns nil if the version is not found.
func GetAPIVersion(ctx context.Context, versionID string) (*restModel.APIVersion, error) {
	l := For(ctx)
	result, err := l.APIVersionLoader.Load(ctx, versionID)
	if errors.Is(err, dataloadgen.ErrNotFound) {
		return nil, nil
	}
	return result, err
}
