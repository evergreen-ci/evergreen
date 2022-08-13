package testutil

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/pkg/errors"
)

// MakeTaskConfigFromModelData converts an API TestModelData to a TaskConfig.
func MakeTaskConfigFromModelData(settings *evergreen.Settings, data *testutil.TestModelData) (*internal.TaskConfig, error) {
	oauthToken, err := settings.GetGithubOauthToken()
	if err != nil {
		return nil, errors.Wrap(err, "error getting oauth token")
	}
	exp, err := model.PopulateExpansions(data.Task, data.Host, oauthToken)
	if err != nil {
		return nil, errors.Wrap(err, "error populating expansions")
	}
	var dv *apimodels.DistroView
	if data.Host != nil {
		dv = &apimodels.DistroView{
			CloneMethod: data.Host.Distro.CloneMethod,
		}
	}
	config, err := internal.NewTaskConfig(data.Host.Distro.WorkDir, dv, data.Project, data.Task, data.ProjectRef, nil, exp)
	if err != nil {
		return nil, errors.Wrap(err, "could not make task config from test model data")
	}
	return config, nil
}
