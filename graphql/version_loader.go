package graphql

import (
	"context"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"go.mongodb.org/mongo-driver/bson"
)

type versionReader struct{}

func (v *versionReader) getVersions(ctx context.Context, versionIDs []string) ([]*restModel.APIVersion, []error) {
	query := db.Query(bson.M{model.VersionIdKey: bson.M{"$in": versionIDs}}).Project(bson.M{model.VersionBuildVariantsKey: 0})
	versions, err := model.VersionFind(ctx, query)
	if err != nil {
		// Return the same error for all requested IDs
		errs := make([]error, len(versionIDs))
		for i := range errs {
			errs[i] = err
		}
		return nil, errs
	}

	versionMap := make(map[string]*model.Version, len(versions))
	for i := range versions {
		versionMap[versions[i].Id] = &versions[i]
	}

	// Build results in the same order as input IDs
	// Return nil for versions not found
	results := make([]*restModel.APIVersion, len(versionIDs))
	errs := make([]error, len(versionIDs))
	for i, id := range versionIDs {
		if dbVersion, ok := versionMap[id]; ok {
			apiVersion := &restModel.APIVersion{}
			apiVersion.BuildFromService(ctx, *dbVersion)
			results[i] = apiVersion
		}
		// results[i] remains nil if version not found, errs[i] remains nil
	}

	return results, errs
}

// GetVersion returns a single version by ID efficiently using the dataloader.
// Returns nil if the version is not found.
func GetVersion(ctx context.Context, versionID string) (*restModel.APIVersion, error) {
	loaders := DataloaderFor(ctx)
	return loaders.VersionLoader.Load(ctx, versionID)
}
