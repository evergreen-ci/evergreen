package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

func TestPeriodicBuildsJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	assert := assert.New(t)
	now := time.Now().Truncate(time.Second)
	assert.NoError(db.ClearCollections(model.VersionCollection, model.ProjectRefCollection, build.Collection, task.Collection))
	j := makePeriodicBuildsJob()
	env := evergreen.GetEnvironment()
	_ = env.DB().RunCommand(nil, map[string]string{"create": model.VersionCollection})
	_ = env.DB().RunCommand(nil, map[string]string{"create": build.Collection})
	_ = env.DB().RunCommand(nil, map[string]string{"create": task.Collection})
	j.env = env
	testutil.ConfigureIntegrationTest(t, j.env.Settings(), "TestPeriodicBuildsJob")

	sampleProject := model.ProjectRef{
		Id:         "myProject",
		Owner:      "evergreen-ci",
		Repo:       "sample",
		RemotePath: "evergreen.yml",
		Branch:     "main",
		PeriodicBuilds: []model.PeriodicBuildDefinition{
			{IntervalHours: 1, ID: "abc", ConfigFile: "evergreen.yml", Alias: "alias", NextRunTime: now.Add(time.Hour)},
		},
	}
	assert.NoError(sampleProject.Insert())
	j.ProjectID = sampleProject.Id
	j.DefinitionID = "abc"

	prevVersion := model.Version{
		Id:         "version",
		Identifier: sampleProject.Id,
		Requester:  evergreen.RepotrackerVersionRequester,
		Revision:   "88dcc12106a40cb4917f552deab7574ececd9a3e",
		Author:     "test author",
	}
	assert.NoError(prevVersion.Insert())

	// test that a version is created when the job runs
	j.Run(ctx)
	assert.NoError(j.Error())
	createdVersion, err := model.FindLastPeriodicBuild(sampleProject.Id, sampleProject.PeriodicBuilds[0].ID)
	assert.NoError(err)
	assert.Equal(evergreen.AdHocRequester, createdVersion.Requester)
	assert.Equal(prevVersion.Revision, createdVersion.Revision)
	assert.Equal(prevVersion.Author, createdVersion.Author)
	tasks, err := task.Find(task.ByVersion(createdVersion.Id))
	assert.NoError(err)
	assert.True(tasks[0].Activated)
	assert.Equal(tasks[0].ActivatedBy, prevVersion.Author)
	dbProject, err := model.FindBranchProjectRef(sampleProject.Id)
	assert.NoError(err)
	assert.True(sampleProject.PeriodicBuilds[0].NextRunTime.Add(time.Hour).Equal(dbProject.PeriodicBuilds[0].NextRunTime))
}
