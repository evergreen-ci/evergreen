package graphql

import (
	"context"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/vikstrous/dataloadgen"
	"go.mongodb.org/mongo-driver/bson"
)

type ctxKey string

const loadersKey = ctxKey("dataloaders")

type Loaders struct {
	UserLoader    *dataloadgen.Loader[string, *restModel.APIDBUser]
	VersionLoader *dataloadgen.Loader[string, *restModel.APIVersion]
}

type userReader struct{}

func (u *userReader) getUsers(ctx context.Context, userIDs []string) ([]*restModel.APIDBUser, []error) {
	query := db.Query(bson.M{user.IdKey: bson.M{"$in": userIDs}})
	users, err := user.Find(ctx, query)
	if err != nil {
		// Return the same error for all requested IDs
		errs := make([]error, len(userIDs))
		for i := range errs {
			errs[i] = err
		}
		return nil, errs
	}

	userMap := make(map[string]*user.DBUser, len(users))
	for i := range users {
		userMap[users[i].Id] = &users[i]
	}

	// Build results in the same order as input IDs
	// Return nil for users not found (e.g. service users)
	results := make([]*restModel.APIDBUser, len(userIDs))
	errs := make([]error, len(userIDs))
	for i, id := range userIDs {
		if dbUser, ok := userMap[id]; ok {
			apiUser := &restModel.APIDBUser{}
			apiUser.BuildFromService(*dbUser)
			results[i] = apiUser
		}
		// results[i] remains nil if user not found, errs[i] remains nil
	}

	return results, errs
}

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

// NewLoaders instantiates data loaders for the middleware.
func NewLoaders() *Loaders {
	ur := &userReader{}
	vr := &versionReader{}
	return &Loaders{
		UserLoader:    dataloadgen.NewLoader(ur.getUsers, dataloadgen.WithWait(time.Millisecond)),
		VersionLoader: dataloadgen.NewLoader(vr.getVersions, dataloadgen.WithWait(time.Millisecond)),
	}
}

// Middleware injects data loaders into the request context.
func DataloaderMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loader := NewLoaders()
		r = r.WithContext(context.WithValue(r.Context(), loadersKey, loader))
		next.ServeHTTP(w, r)
	})
}

// For returns the dataloader for a given context.
func DataloaderFor(ctx context.Context) *Loaders {
	return ctx.Value(loadersKey).(*Loaders)
}

// GetUser returns a single user by ID efficiently using the dataloader.
// Returns nil if the user is not found (e.g., reaped users).
func GetUser(ctx context.Context, userID string) (*restModel.APIDBUser, error) {
	loaders := DataloaderFor(ctx)
	return loaders.UserLoader.Load(ctx, userID)
}

// GetVersion returns a single version by ID efficiently using the dataloader.
// Returns nil if the version is not found.
func GetVersion(ctx context.Context, versionID string) (*restModel.APIVersion, error) {
	loaders := DataloaderFor(ctx)
	return loaders.VersionLoader.Load(ctx, versionID)
}
