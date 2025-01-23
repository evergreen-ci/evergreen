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
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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
func GetGitHubAppID(projectId string) (*int64, error) {
	githubAppAuth := &GithubAppAuth{}

	q := byGithubAppAuthID(projectId).WithFields(GhAuthAppIdKey)
	err := db.FindOneQ(GitHubAppAuthCollection, q, githubAppAuth)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}

	return &githubAppAuth.AppID, err
}

// githubAppCheckAndRunParameterStoreOp checks if Parameter Store is
// enabled and if so, runs the given Parameter Store operation.
func githubAppCheckAndRunParameterStoreOp(ctx context.Context, op func() error) error {
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return errors.Wrap(err, "getting service flags")
	}
	if flags.ParameterStoreDisabled {
		return nil
	}

	return op()
}

// defaultParameterStoreAccessTimeout is the default timeout for accessing
// Parameter Store. In general, the context timeout should prefer to be
// inherited from a higher-level context (e.g. a REST request's context), so
// this timeout should only be used as a last resort if the context cannot
// easily be passed down.
const defaultParameterStoreAccessTimeout = 30 * time.Second

// FindOneGitHubAppAuth finds the GitHub app auth and retrieves the private key
// from Parameter Store if enabled.
func FindOneGitHubAppAuth(id string) (*GithubAppAuth, error) {
	appAuth, err := findOneGitHubAppAuthDB(id)
	if err != nil {
		return nil, errors.Wrapf(err, "finding GitHub app auth for project '%s'", id)
	}
	if appAuth == nil {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultParameterStoreAccessTimeout)
	defer cancel()

	if err := githubAppCheckAndRunParameterStoreOp(ctx, func() error {
		return findPrivateKeyParameterStore(ctx, appAuth)
	}); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":    "could not find GitHub app private key in Parameter Store",
			"op":         "FindOne",
			"project_id": appAuth.Id,
			"epic":       "DEVPROD-5552",
		}))
		return nil, err
	}

	return appAuth, nil
}

// findOneGitHubAppAuthDB finds the GitHub app auth in the database.
func findOneGitHubAppAuthDB(id string) (*GithubAppAuth, error) {
	appAuth := &GithubAppAuth{}
	err := db.FindOneQ(GitHubAppAuthCollection, byGithubAppAuthID(id), appAuth)
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
func UpsertGitHubAppAuth(appAuth *GithubAppAuth) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultParameterStoreAccessTimeout)
	defer cancel()

	if err := githubAppCheckAndRunParameterStoreOp(ctx, func() error {
		paramName, err := upsertPrivateKeyParameterStore(ctx, appAuth)
		if err != nil {
			return errors.Wrap(err, "upserting GitHub app private key into Parameter Store")
		}

		appAuth.PrivateKeyParameter = paramName

		return nil
	}); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":    "could not upsert GitHub app private key into Parameter Store",
			"op":         "Upsert",
			"project_id": appAuth.Id,
			"epic":       "DEVPROD-5552",
		}))
		return err
	}

	return upsertGitHubAppAuthDB(appAuth)
}

// upsertGitHubAppAuthDB upserts the GitHub app auth into the database.
func upsertGitHubAppAuthDB(appAuth *GithubAppAuth) error {
	_, err := db.Upsert(
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
func RemoveGitHubAppAuth(appAuth *GithubAppAuth) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultParameterStoreAccessTimeout)
	defer cancel()

	if err := githubAppCheckAndRunParameterStoreOp(ctx, func() error {
		return removePrivateKeyParameterStore(ctx, appAuth)
	}); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":    "could not delete GitHub app private key from Parameter Store",
			"op":         "Remove",
			"project_id": appAuth.Id,
			"epic":       "DEVPROD-5552",
		}))
		return err
	}

	return removeGitHubAppAuthDB(appAuth.Id)
}

// removeGitHubAppAuthDB removes the GitHub app auth from the database.
func removeGitHubAppAuthDB(id string) error {
	return db.Remove(
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
