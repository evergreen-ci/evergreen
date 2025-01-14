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

// TODO (DEVPROD-11883): move all of this logic into the githubapp package once
// all project GitHub apps are using Parameter Store and the rollout is stable.
// The functions are only here temporarily to avoid a dependency cycle between
// the githubapp and model packages due to the project ref feature flag.

// githubAppCheckAndRunParameterStoreOp checks if the project corresponding to
// the GitHub app private key has Parameter Store enabled, and if so, runs the
// given Parameter Store operation.
func githubAppCheckAndRunParameterStoreOp(ctx context.Context, appAuth *githubapp.GithubAppAuth, op func() error) error {
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return errors.Wrap(err, "getting service flags")
	}
	if flags.ParameterStoreDisabled {
		return nil
	}

	return op()
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

	if err := githubAppCheckAndRunParameterStoreOp(ctx, appAuth, func() error {
		return githubAppAuthFindParameterStore(ctx, appAuth)
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
	appAuth.PrivateKey = privKey

	return nil
}

// GitHubAppAuthUpsert upserts the GitHub app auth into the database and upserts
// the private key to Parameter Store if enabled.
func GitHubAppAuthUpsert(appAuth *githubapp.GithubAppAuth) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultParameterStoreAccessTimeout)
	defer cancel()

	if err := githubAppCheckAndRunParameterStoreOp(ctx, appAuth, func() error {
		paramName, err := githubAppAuthUpsertParameterStore(ctx, appAuth)
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

	return githubapp.UpsertGithubAppAuth(appAuth)
}

// githubAppAuthUpsertParameterStore upserts the GitHub app private key into
// Parameter Store.
func githubAppAuthUpsertParameterStore(ctx context.Context, appAuth *githubapp.GithubAppAuth) (string, error) {
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

// GitHubAppAuthRemove removes the GitHub app private key from the database and
// from Parameter Store if enabled.
func GitHubAppAuthRemove(appAuth *githubapp.GithubAppAuth) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultParameterStoreAccessTimeout)
	defer cancel()

	if err := githubAppCheckAndRunParameterStoreOp(ctx, appAuth, func() error {
		return githubAppAuthRemoveParameterStore(ctx, appAuth)
	}); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":    "could not delete GitHub app private key from Parameter Store",
			"op":         "Remove",
			"project_id": appAuth.Id,
			"epic":       "DEVPROD-5552",
		}))
		return err
	}
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
