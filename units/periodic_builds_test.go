package units

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

func TestPeriodicBuildsJob(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(model.VersionCollection, model.ProjectRefCollection, build.Collection, task.Collection))
	j := makePeriodicBuildsJob()
	ctx := context.Background()
	env := &mock.Environment{}
	j.env = env
	assert.NoError(env.Configure(ctx))
	testutil.ConfigureIntegrationTest(t, j.env.Settings(), "TestPeriodicBuildsJob")

	sampleProject := model.ProjectRef{
		Identifier: "myProject",
		Owner:      "evergreen-ci",
		Repo:       "sample",
		RemotePath: "evergreen.yml",
		Branch:     "master",
		PeriodicBuilds: []model.PeriodicBuildDefinition{
			{IntervalHours: 1, ID: "abc", ConfigFile: "evergreen.yml", Alias: "alias"},
		},
	}
	assert.NoError(sampleProject.Insert())
	j.ProjectID = sampleProject.Identifier

	// test that a version is created the first time
	j.Run(ctx)
	assert.NoError(j.Error())
	createdVersion, err := model.FindLastPeriodicBuild(sampleProject.Identifier, sampleProject.PeriodicBuilds[0].ID)
	assert.NoError(err)
	assert.Equal(evergreen.AdHocRequester, createdVersion.Requester)

	// rerunning the job too soon does not create another build
	j.Run(ctx)
	assert.NoError(j.Error())
	lastVersion, err := model.FindLastPeriodicBuild(sampleProject.Identifier, sampleProject.PeriodicBuilds[0].ID)
	assert.NoError(err)
	assert.Equal(createdVersion.Id, lastVersion.Id)
}
