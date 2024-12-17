package model

import (
	"bytes"
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/githubapp"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// TODO (DEVPROD-11883): move all of this logic into the githubapp package once
// all project GitHub apps are using Parameter Store and the rollout is stable.
// The functions are only here temporarily to avoid a dependency cycle between
// the githubapp and model packages due to the project ref feature flag.

// githubAppCheckAndRunParameterStoreOp checks if the project corresponding to
// the GitHub app private key has Parameter Store enabled, and if so, runs the
// given Parameter Store operation.
func githubAppCheckAndRunParameterStoreOp(ctx context.Context, appAuth *githubapp.GithubAppAuth, op func(ref *ProjectRef, isRepoRef bool), opName string) {
	ref, isRepoRef, err := findProjectRef(appAuth.Id)
	grip.Error(message.WrapError(err, message.Fields{
		"message":    "could not check if Parameter Store is enabled for project; assuming it's disabled and will not use Parameter Store",
		"op":         opName,
		"project_id": appAuth.Id,
		"epic":       "DEVPROD-5552",
	}))
	isPSEnabled, err := isParameterStoreEnabledForProject(ctx, ref)
	grip.Error(message.WrapError(err, message.Fields{
		"message":    "could not check if Parameter Store is enabled for project; assuming it's disabled and will not use Parameter Store",
		"op":         opName,
		"project_id": appAuth.Id,
		"epic":       "DEVPROD-5552",
	}))
	if !isPSEnabled {
		return
	}

	op(ref, isRepoRef)
}

// GitHubAppAuthFindOne finds the GitHub app auth and retrieves the private key
// from Parameter Store if enabled.
func GitHubAppAuthFindOne(id string) (*githubapp.GithubAppAuth, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultParameterStoreAccessTimeout)
	defer cancel()

	appAuth, err := githubapp.FindOneGithubAppAuth(id)
	if err != nil {
		return nil, err
	}
	if appAuth == nil {
		return nil, nil
	}

	githubAppCheckAndRunParameterStoreOp(ctx, appAuth, func(ref *ProjectRef, isRepoRef bool) {
		if ref.ParameterStoreGitHubAppSynced {
			if err := githubAppAuthFindParameterStore(ctx, appAuth); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message":    "could not find GitHub app private key in Parameter Store",
					"op":         "FindOne",
					"project_id": appAuth.Id,
					"epic":       "DEVPROD-5552",
				}))
			}
		}
	}, "FindOne")

	return appAuth, nil
}

func githubAppAuthFindParameterStore(ctx context.Context, appAuth *githubapp.GithubAppAuth) error {
	paramMgr := evergreen.GetEnvironment().ParameterManager()

	params, err := paramMgr.GetStrict(ctx, appAuth.PrivateKeyParameter)
	if err != nil {
		return errors.Wrapf(err, "getting GitHub app private key from Parameter Store")
	}
	if len(params) != 1 {
		return errors.Errorf("expected to get exactly one parameter '%s', but actually got %d", appAuth.PrivateKeyParameter, len(params))
	}

	privKey := []byte(params[0].Value)

	// Check that the private key retrieved from Parameter Store is identical to
	// the private key stored in the DB. This is a data consistency check and
	// doubles as a fallback. By checking the private key retrieved from
	// Parameter Store, Evergreen can automatically detect if the Parameter
	// Store integration is returning incorrect information and if so, fall back
	// to using the private key stored in the DB rather than Parameter Store,
	// which avoids using potentially the wrong private key while the rollout is
	// ongoing.
	// TODO (DEVPROD-9441): remove this consistency check once the rollout is
	// complete and everything is prepared to remove the GitHub app private keys
	// from the DB.
	if !bytes.Equal(privKey, appAuth.PrivateKey) {
		grip.Error(message.Fields{
			"message": "private key in Parameter Store does not match private key in the DB",
			"project": appAuth.Id,
			"epic":    "DEVPROD-5552",
		})
	} else {
		appAuth.PrivateKey = privKey
	}

	return nil
}

// GitHubAppAuthUpsert upserts the GitHub app auth into the database and upserts
// the private key to Parameter Store if enabled.
func GitHubAppAuthUpsert(appAuth *githubapp.GithubAppAuth) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultParameterStoreAccessTimeout)
	defer cancel()

	githubAppCheckAndRunParameterStoreOp(ctx, appAuth, func(ref *ProjectRef, isRepoRef bool) {
		paramName, err := githubAppAuthUpsertParameterStore(ctx, appAuth, ref, isRepoRef)
		grip.Error(message.WrapError(err, message.Fields{
			"message":    "could not upsert GitHub app private key into Parameter Store",
			"op":         "Upsert",
			"project_id": appAuth.Id,
			"epic":       "DEVPROD-5552",
		}))
		if paramName != "" {
			appAuth.PrivateKeyParameter = paramName
		}
	}, "Upsert")

	return githubapp.UpsertGithubAppAuth(appAuth)
}

// githubAppAuthUpsertParameterStore upserts the GitHub app private key into
// Parameter Store.
func githubAppAuthUpsertParameterStore(ctx context.Context, appAuth *githubapp.GithubAppAuth, pRef *ProjectRef, isRepoRef bool) (string, error) {
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

	if err := pRef.setParameterStoreGitHubAppAuthSynced(true, isRepoRef); err != nil {
		return "", errors.Wrapf(err, "marking project/repo ref '%s' as having its GitHub app private key synced to Parameter Store", pRef.Id)
	}

	return paramName, nil
}

// GitHubAppAuthRemove removes the GitHub app private key from the database and
// from Parameter Store if enabled.
func GitHubAppAuthRemove(appAuth *githubapp.GithubAppAuth) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultParameterStoreAccessTimeout)
	defer cancel()

	githubAppCheckAndRunParameterStoreOp(ctx, appAuth, func(ref *ProjectRef, isRepoRef bool) {
		if err := githubAppAuthRemoveParameterStore(ctx, appAuth); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":    "could not delete GitHub app private key from Parameter Store",
				"op":         "Remove",
				"project_id": appAuth.Id,
				"epic":       "DEVPROD-5552",
			}))
		}
	}, "Remove")
	return githubapp.RemoveGithubAppAuth(appAuth.Id)
}

// githubAppAuthRemoveParameterStore removes the GitHub app private key from Parameter
// Store.
func githubAppAuthRemoveParameterStore(ctx context.Context, appAuth *githubapp.GithubAppAuth) error {
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
