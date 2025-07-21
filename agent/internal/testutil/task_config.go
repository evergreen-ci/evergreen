package testutil

import (
	"context"

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
	knownHosts := settings.Expansions[evergreen.GithubKnownHosts]
	exp, err := model.PopulateExpansions(ctx, data.Task, data.Host, knownHosts)
	if err != nil {
		return nil, errors.Wrap(err, "populating expansions")
	}
	tcOpts := internal.TaskConfigOptions{
		WorkDir:           data.Host.Distro.WorkDir,
		Distro:            &apimodels.DistroView{},
		Host:              &apimodels.HostView{},
		Project:           data.Project,
		ProjectRef:        data.ProjectRef,
		Task:              data.Task,
		ExpansionsAndVars: &apimodels.ExpansionsAndVars{Expansions: exp},
	}
	config, err := internal.NewTaskConfig(tcOpts)
	if err != nil {
		return nil, errors.Wrap(err, "making task config from test model data")
	}
	return config, nil
}
