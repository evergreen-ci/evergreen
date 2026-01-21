package githubapp

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	// GitHubAppAuthCollection is the name of the collection that contains
	// GitHub app auth credentials.
	GitHubAppAuthCollection = "github_app_auth"
)

var (
	GhAuthIdKey                  = bsonutil.MustHaveTag(GithubAppAuth{}, "Id")
	GhAuthAppIdKey               = bsonutil.MustHaveTag(GithubAppAuth{}, "AppID")
	GhAuthPrivateKeyParameterKey = bsonutil.MustHaveTag(GithubAppAuth{}, "PrivateKeyParameter")
)

// byGithubAppAuthID returns a query that finds a github app auth by the given identifier
// corresponding to the project id
func byGithubAppAuthID(projectId string) db.Q {
	return db.Query(bson.M{GhAuthIdKey: projectId})
}

// GetGitHubAppID returns the app id for the given project id
func GetGitHubAppID(ctx context.Context, projectId string) (*int64, error) {
	githubAppAuth := &GithubAppAuth{}

	q := byGithubAppAuthID(projectId).WithFields(GhAuthAppIdKey)
	err := db.FindOneQ(ctx, GitHubAppAuthCollection, q, githubAppAuth)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}

	return &githubAppAuth.AppID, err
}

// defaultParameterStoreAccessTimeout is the default timeout for accessing
// Parameter Store. In general, the context timeout should prefer to be
// inherited from a higher-level context (e.g. a REST request's context), so
// this timeout should only be used as a last resort if the context cannot
// easily be passed down.
const defaultParameterStoreAccessTimeout = 30 * time.Second

// FindOneGitHubAppAuth finds the GitHub app auth and retrieves the private key
// from Parameter Store if enabled.
func FindOneGitHubAppAuth(ctx context.Context, id string) (*GithubAppAuth, error) {
	appAuth, err := findOneGitHubAppAuthDB(ctx, id)
	if err != nil {
		return nil, errors.Wrapf(err, "finding GitHub app auth for project '%s'", id)
	}
	if appAuth == nil {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(ctx, defaultParameterStoreAccessTimeout)
	defer cancel()

	if err := findPrivateKeyParameterStore(ctx, appAuth); err != nil {
		return nil, errors.Wrapf(err, "finding GitHub app private key for project '%s'", id)
	}

	return appAuth, nil
}

// findOneGitHubAppAuthDB finds the GitHub app auth in the database.
func findOneGitHubAppAuthDB(ctx context.Context, id string) (*GithubAppAuth, error) {
	appAuth := &GithubAppAuth{}
	err := db.FindOneQ(ctx, GitHubAppAuthCollection, byGithubAppAuthID(id), appAuth)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return appAuth, err
}

// findPrivateKeyParameterStore finds the GitHub app private key in Parameter
// Store.
func findPrivateKeyParameterStore(ctx context.Context, appAuth *GithubAppAuth) error {
	paramMgr := evergreen.GetEnvironment().ParameterManager()

	params, err := paramMgr.GetStrict(ctx, appAuth.PrivateKeyParameter)
	if err != nil {
		return errors.Wrapf(err, "getting GitHub app private key from Parameter Store")
	}
	if len(params) != 1 {
		return errors.Errorf("expected to get exactly one parameter '%s', but actually got %d", appAuth.PrivateKeyParameter, len(params))
	}

	privKey := []byte(params[0].Value)
	appAuth.PrivateKey = privKey

	return nil
}

// UpsertGitHubAppAuth upserts the GitHub app auth into the database and upserts
// the private key to Parameter Store if enabled.
func UpsertGitHubAppAuth(ctx context.Context, appAuth *GithubAppAuth) error {
	ctx, cancel := context.WithTimeout(ctx, defaultParameterStoreAccessTimeout)
	defer cancel()

	paramName, err := upsertPrivateKeyParameterStore(ctx, appAuth)
	if err != nil {
		return errors.Wrapf(err, "upserting GitHub app private key into Parameter Store for project '%s'", appAuth.Id)
	}
	appAuth.PrivateKeyParameter = paramName

	return upsertGitHubAppAuthDB(ctx, appAuth)
}

// upsertGitHubAppAuthDB upserts the GitHub app auth into the database.
func upsertGitHubAppAuthDB(ctx context.Context, appAuth *GithubAppAuth) error {
	_, err := db.Upsert(ctx,
		GitHubAppAuthCollection,
		bson.M{
			GhAuthIdKey: appAuth.Id,
		},
		bson.M{
			"$set": bson.M{
				GhAuthAppIdKey:               appAuth.AppID,
				GhAuthPrivateKeyParameterKey: appAuth.PrivateKeyParameter,
			},
		},
	)
	return err
}

// upsertPrivateKeyParameterStore upserts the GitHub app private key into
// Parameter Store.
func upsertPrivateKeyParameterStore(ctx context.Context, appAuth *GithubAppAuth) (string, error) {
	projectID := appAuth.Id
	partialParamName := getPrivateKeyParamName(projectID)

	paramMgr := evergreen.GetEnvironment().ParameterManager()
	paramValue := string(appAuth.PrivateKey)

	param, err := paramMgr.Put(ctx, partialParamName, paramValue)
	if err != nil {
		return "", errors.Wrap(err, "upserting GitHub app private key into Parameter Store")
	}
	paramName := param.Name

	existingParamName := appAuth.PrivateKeyParameter
	if existingParamName != "" && existingParamName != paramName {
		if err := paramMgr.Delete(ctx, existingParamName); err != nil {
			return "", errors.Wrapf(err, "deleting old GitHub app private key parameter '%s' from Parameter Store after it was renamed to '%s'", existingParamName, paramName)
		}
	}

	return paramName, nil
}

// RemoveGitHubAppAuth removes the GitHub app private key from the database and
// from Parameter Store if enabled.
func RemoveGitHubAppAuth(ctx context.Context, appAuth *GithubAppAuth) error {
	ctx, cancel := context.WithTimeout(ctx, defaultParameterStoreAccessTimeout)
	defer cancel()

	if err := removePrivateKeyParameterStore(ctx, appAuth); err != nil {
		return errors.Wrap(err, "removing GitHub app private key from Parameter Store")
	}

	return removeGitHubAppAuthDB(ctx, appAuth.Id)
}

// removeGitHubAppAuthDB removes the GitHub app auth from the database.
func removeGitHubAppAuthDB(ctx context.Context, id string) error {
	return db.Remove(
		ctx,
		GitHubAppAuthCollection,
		bson.M{GhAuthIdKey: id},
	)
}

// removePrivateKeyParameterStore removes the GitHub app private key from
// Parameter Store.
func removePrivateKeyParameterStore(ctx context.Context, appAuth *GithubAppAuth) error {
	if appAuth.PrivateKeyParameter == "" {
		return nil
	}
	paramMgr := evergreen.GetEnvironment().ParameterManager()
	if err := paramMgr.Delete(ctx, appAuth.PrivateKeyParameter); err != nil {
		return errors.Wrapf(err, "deleting GitHub app private key parameter '%s' from Parameter Store", appAuth.PrivateKeyParameter)
	}
	return nil
}

// getPrivateKeyParamName returns the parameter name for the GitHub app's
// private key in Parameter Store for the given project.
func getPrivateKeyParamName(projectID string) string {
	hashedProjectID := util.GetSHA256Hash(projectID)
	return fmt.Sprintf("github_apps/%s/private_key", hashedProjectID)
}
