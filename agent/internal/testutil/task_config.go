package testutil

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// MakeTaskConfigFromModelData converts an API TestModelData to a TaskConfig.
// This function is only used for tests.
func MakeTaskConfigFromModelData(ctx context.Context, settings *evergreen.Settings, data *testutil.TestModelData) (*internal.TaskConfig, error) {
	oauthToken, err := settings.GetGithubOauthToken()
	if err != nil {
		return nil, errors.Wrap(err, "getting global GitHub OAuth token")
	}
	appToken, err := settings.CreateInstallationToken(ctx, data.ProjectRef.Owner, data.ProjectRef.Repo, nil)
	if err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"ticket":  "EVG-19966",
			"message": "error creating GitHub app token",
			"caller":  "MakeTaskConfigFromModelData",
			"task":    data.Task.Id,
		}))
	}
	exp, err := model.PopulateExpansions(data.Task, data.Host, oauthToken, appToken)
	if err != nil {
		return nil, errors.Wrap(err, "populating expansions")
	}
	var dv *apimodels.DistroView
	if data.Host != nil {
		dv = &apimodels.DistroView{
			CloneMethod: data.Host.Distro.CloneMethod,
		}
	}
	config, err := internal.NewTaskConfig(data.Host.Distro.WorkDir, dv, data.Project, data.Task, data.ProjectRef, nil, exp)
	if err != nil {
		return nil, errors.Wrap(err, "making task config from test model data")
	}
	return config, nil
}
