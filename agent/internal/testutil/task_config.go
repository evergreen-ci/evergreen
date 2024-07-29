package testutil

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/pkg/errors"
)

// MakeTaskConfigFromModelData converts an API TestModelData to a TaskConfig.
// This function is only used for tests.
func MakeTaskConfigFromModelData(ctx context.Context, settings *evergreen.Settings, data *testutil.TestModelData) (*internal.TaskConfig, error) {
	oauthToken, err := settings.GetGithubOauthToken()
	if err != nil {
		return nil, errors.Wrap(err, "getting global GitHub OAuth token")
	}

	// Arbitrarily pick a long lifetime for the app token so that it's valid for
	// the entire test duration.
	const appTokenLifetime = 30 * time.Minute
	appToken, err := settings.CreateGitHubAppAuth().CreateCachedInstallationToken(ctx, data.ProjectRef.Owner, data.ProjectRef.Repo, appTokenLifetime, nil)
	if err != nil {
		return nil, errors.Wrap(err, "creating GitHub app token")
	}
	knownHosts := settings.Expansions[evergreen.GithubKnownHosts]
	exp, err := model.PopulateExpansions(data.Task, data.Host, oauthToken, appToken, knownHosts)
	if err != nil {
		return nil, errors.Wrap(err, "populating expansions")
	}
	var dv *apimodels.DistroView
	config, err := internal.NewTaskConfig(data.Host.Distro.WorkDir, dv, data.Project, data.Task, data.ProjectRef, nil, &apimodels.ExpansionsAndVars{Expansions: exp})
	if err != nil {
		return nil, errors.Wrap(err, "making task config from test model data")
	}
	return config, nil
}
