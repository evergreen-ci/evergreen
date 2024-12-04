package model

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/githubapp"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// TODO (DEVPROD-11883): move all of this logic into githubapp package once all
// project GitHub apps are using Parameter Store and the rollout is stable. The
// functions are only here temporarily to avoid a dependency cycle between
// githubapp and model packages due to the project ref feature flag.

// githubAppCheckAndRunParameterStoreOp checks if the project corresponding to
// the GitHub app auth has Parameter Store enabled, and if so, runs the given
// Parameter Store operation.
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

// kim: TODO: migrate usages of auth.Upsert to use this function temporarily.
// kim: TODO: write test similar to one for vars.Upsert.
// GitHubAppAuthUpsert upserts the GitHub app auth into the database and to
// Parameter Store if enabled.
func GitHubAppAuthUpsert(appAuth *githubapp.GithubAppAuth) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultParameterStoreAccessTimeout)
	defer cancel()

	githubAppCheckAndRunParameterStoreOp(ctx, appAuth, func(ref *ProjectRef, isRepoRef bool) {
		paramName, err := githubAppAuthUpsertParameterStore(ctx, appAuth, ref, isRepoRef)
		grip.Error(message.WrapError(err, message.Fields{
			"message":    "could not upsert GitHub app auth into Parameter Store",
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

// githubAppAuthUpsertParameterStore upserts the GitHub app auth into Parameter
// Store.
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
		return "", errors.Wrapf(err, "marking project/repo ref '%s' as having its GitHub app auth synced to Parameter Store", pRef.Id)
	}

	return paramName, nil
}

// kim: TODO: migrate usages of auth.Remove to use this function temporarily.
// GitHubAppAuthRemove deletes the GitHub app auth from the database and
// Parameter Store if enabled.
// kim: TODO: write test similar to one for vars.Clear.
func GitHubAppAuthRemove(appAuth *githubapp.GithubAppAuth) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultParameterStoreAccessTimeout)
	defer cancel()

	githubAppCheckAndRunParameterStoreOp(ctx, appAuth, func(ref *ProjectRef, isRepoRef bool) {
		// kim: NOTE: no need to check if app is synced here because it uploads
		// the one field rather than doing a smarter diff like project vars have
		// to. Only the sync job needs that info to determine which GitHub apps
		// to sync.
		if err := githubAppAuthRemoveParameterStore(ctx, appAuth); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":    "could not delete GitHub app auth from Parameter Store",
				"op":         "Remove",
				"project_id": appAuth.Id,
				"epic":       "DEVPROD-5552",
			}))
		}
	}, "Remove")
	return githubapp.RemoveGithubAppAuth(appAuth.Id)
}

// githubAppAuthRemoveParameterStore removes the GitHub app auth from Parameter
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
