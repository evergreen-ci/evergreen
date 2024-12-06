package data

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type VersionConnectorSuite struct {
	suite.Suite
}

//----------------------------------------------------------------------------//
//   Initialize the ConnectorSuites                                           //
//----------------------------------------------------------------------------//

func TestDBVersionConnectorSuite(t *testing.T) {
	s := new(VersionConnectorSuite)
	suite.Run(t, s)
}

func (s *VersionConnectorSuite) SetupTest() {
	s.Require().NoError(db.Clear(task.Collection))
	s.Require().NoError(db.Clear(task.OldCollection))
	s.Require().NoError(db.Clear(model.VersionCollection))
	s.Require().NoError(db.Clear(build.Collection))
	s.Require().NoError(db.Clear(model.ParserProjectCollection))

	// Insert data for the test paths
	versions := []*model.Version{
		{Id: "version1"},
		{Id: "version2"},
		{Id: "version3"},
	}

	tasks := []*task.Task{
		{Id: "task1", Version: "version1", Aborted: false, Status: evergreen.TaskStarted},
		{Id: "task2", Version: "version1", Aborted: false, Status: evergreen.TaskDispatched},
		{Id: "task3", Version: "version1", Aborted: true, Status: evergreen.TaskInactive},
		{Id: "task4", Version: "version2", Aborted: false, Status: evergreen.TaskStarted},
		{Id: "task5", Version: "version3", Aborted: false, Status: evergreen.TaskSucceeded, BuildId: "build1"},
	}

	builds := []*build.Build{
		{Id: "build1"},
	}

	for _, item := range versions {
		s.Require().NoError(item.Insert())
	}
	for _, item := range tasks {
		s.Require().NoError(item.Insert())
	}
	for _, item := range builds {
		s.Require().NoError(item.Insert())
	}
}

//----------------------------------------------------------------------------//
//   Test cases                                                               //
//----------------------------------------------------------------------------//

func (s *VersionConnectorSuite) TestGetVersionsAndVariants() {
	s.NoError(db.ClearCollections(model.ProjectRefCollection))

	projRef := model.ProjectRef{
		Id: "proj",
	}
	s.NoError(projRef.Insert())
	proj := model.Project{
		Identifier: projRef.Id,
		BuildVariants: model.BuildVariants{
			{
				Name:        "bv1",
				DisplayName: "bv1",
			},
			{
				Name:        "bv2",
				DisplayName: "bv2",
			},
		},
	}
	v1 := model.Version{
		Id:                  "v1",
		Revision:            "abcd1",
		RevisionOrderNumber: 1,
		Identifier:          proj.Identifier,
		Status:              evergreen.VersionFailed,
		Requester:           evergreen.RepotrackerVersionRequester,
		Message:             "I am v1",
	}
	v2 := model.Version{
		Id:                  "v2",
		Revision:            "abcd2",
		RevisionOrderNumber: 2,
		Identifier:          proj.Identifier,
		Status:              evergreen.VersionCreated,
		Requester:           evergreen.RepotrackerVersionRequester,
		Message:             "I am v2",
	}
	s.NoError(v2.Insert())
	s.NoError(v1.Insert())
	b11 := build.Build{
		Id:           "b11",
		Activated:    true,
		Version:      v1.Id,
		Project:      proj.Identifier,
		Revision:     v1.Revision,
		BuildVariant: "bv1",
		Tasks: []build.TaskCache{
			{Id: "t111"},
			{Id: "t112"},
		},
	}
	s.NoError(b11.Insert())
	b12 := build.Build{
		Id:           "b12",
		Activated:    true,
		Version:      v1.Id,
		Project:      proj.Identifier,
		Revision:     v1.Revision,
		BuildVariant: "bv2",
		Tasks: []build.TaskCache{
			{Id: "t121"},
			{Id: "t122"},
		},
	}
	s.NoError(b12.Insert())
	b21 := build.Build{
		Id:           "b21",
		Version:      v2.Id,
		Project:      proj.Identifier,
		Revision:     v2.Revision,
		BuildVariant: "bv1",
		Tasks: []build.TaskCache{
			{Id: "t211"},
			{Id: "t212"},
		},
	}
	s.NoError(b21.Insert())
	b22 := build.Build{
		Id:           "b22",
		Version:      v2.Id,
		Project:      proj.Identifier,
		Revision:     v2.Revision,
		BuildVariant: "bv2",
		Tasks: []build.TaskCache{
			{Id: "t221"},
			{Id: "t222"},
		},
	}
	s.NoError(b22.Insert())
	tasks := []task.Task{
		{
			Id:        "t111",
			Version:   v1.Id,
			Activated: true,
			Status:    evergreen.TaskFailed,
			Details: apimodels.TaskEndDetail{
				Status:      evergreen.TaskFailed,
				Type:        evergreen.CommandTypeSystem,
				TimedOut:    true,
				Description: evergreen.TaskDescriptionHeartbeat,
			},
		},
		{
			Id:        "t112",
			Version:   v1.Id,
			Activated: true,
			Status:    evergreen.TaskSucceeded,
			Details: apimodels.TaskEndDetail{
				Status: evergreen.TaskSucceeded,
				Type:   "test",
			},
		},
		{
			Id:        "t121",
			Version:   v1.Id,
			Activated: true,
			Status:    evergreen.TaskSucceeded,
			Details: apimodels.TaskEndDetail{
				Status: evergreen.TaskSucceeded,
				Type:   "test",
			},
		},
		{
			Id:        "t122",
			Version:   v1.Id,
			Activated: true,
			Status:    evergreen.TaskSucceeded,
			Details: apimodels.TaskEndDetail{
				Status: evergreen.TaskSucceeded,
				Type:   "test",
			},
		},
		{
			Id:      "t211",
			Version: v2.Id,
			Status:  evergreen.TaskUnscheduled,
		},
		{
			Id:      "t212",
			Version: v2.Id,
			Status:  evergreen.TaskUnscheduled,
		},
		{
			Id:      "t221",
			Version: v2.Id,
			Status:  evergreen.TaskUnscheduled,
		},
		{
			Id:      "t222",
			Version: v2.Id,
			Status:  evergreen.TaskUnscheduled,
		},
	}
	for _, t := range tasks {
		s.NoError(t.Insert())
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results, err := GetVersionsAndVariants(ctx, 0, 10, &proj)
	s.NoError(err)

	bv1 := results.Rows["bv1"]
	s.Equal("bv1", bv1.BuildVariant)
	resultb11 := bv1.Builds["v1"]
	s.EqualValues(utility.ToStringPtr("b11"), resultb11.Id)
	s.Len(resultb11.Tasks, 2)
	s.Equal(1, resultb11.StatusCounts.Succeeded)
	s.Equal(1, resultb11.StatusCounts.TimedOut)

	bv2 := results.Rows["bv2"]
	s.Equal("bv2", bv2.BuildVariant)
	resultb12 := bv2.Builds["v1"]
	s.EqualValues(utility.ToStringPtr("b12"), resultb12.Id)
	s.Len(resultb12.Tasks, 2)
	s.Equal(2, resultb12.StatusCounts.Succeeded)

	inactiveVersions := results.Versions[0]
	s.True(inactiveVersions.RolledUp)
	s.EqualValues(utility.ToStringPtr("v2"), inactiveVersions.Versions[0].Id)
	s.EqualValues(utility.ToStringPtr("I am v2"), inactiveVersions.Versions[0].Message)

	activeVersions := results.Versions[1]
	s.False(activeVersions.RolledUp)
	s.EqualValues(utility.ToStringPtr("v1"), activeVersions.Versions[0].Id)
	s.EqualValues(utility.ToStringPtr("I am v1"), activeVersions.Versions[0].Message)
}

func TestCreateVersionFromConfig(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	assert := assert.New(t)
	require.NoError(t, db.ClearCollections(model.ProjectRefCollection, model.ParserProjectCollection, model.VersionCollection, distro.Collection, task.Collection, build.Collection, user.Collection))
	require.NoError(t, db.CreateCollections(model.ParserProjectCollection))

	ref := model.ProjectRef{
		Id: "mci",
	}
	assert.NoError(ref.Insert())
	d := distro.Distro{
		Id: "d",
	}
	assert.NoError(d.Insert(ctx))
	u := user.DBUser{
		Id:          "u",
		PatchNumber: 5,
	}
	assert.NoError(u.Insert())
	config1 := `{
			"stepback": true,
			"buildvariants": [{
				"name": "v1",
				"display_name": "v1_display",
				"run_on": "d",
				"tasks": [
					{"name": "t1"},
				]
			}],
			"tasks": [
				{"name": "t1"}
			]
		}`

	p := &model.Project{}
	pp, err := model.LoadProjectInto(ctx, []byte(config1), nil, ref.Id, p)
	assert.NoError(err)
	projectInfo := &model.ProjectInfo{
		Project:             p,
		IntermediateProject: pp,
		Ref:                 &ref,
	}
	metadata := model.VersionMetadata{
		Message:  "my message",
		User:     &u,
		IsAdHoc:  true,
		Activate: true,
	}
	dc := DBVersionConnector{}
	newVersion, err := dc.CreateVersionFromConfig(ctx, projectInfo, metadata)
	assert.NoError(err)
	assert.Equal("my message", newVersion.Message)
	assert.Equal(evergreen.VersionCreated, newVersion.Status)
	assert.Equal(ref.Id, newVersion.Identifier)
	assert.Equal(1, newVersion.RevisionOrderNumber)

	ppStorage, err := model.GetParserProjectStorage(ctx, env.Settings(), newVersion.ProjectStorageMethod)
	require.NoError(t, err)

	pp, err = ppStorage.FindOneByID(ctx, newVersion.Id)
	assert.NoError(err)
	assert.NotNil(pp)
	assert.True(utility.FromBoolPtr(pp.Stepback))

	b, err := build.FindOneId(newVersion.BuildIds[0])
	assert.NoError(err)
	assert.Equal(evergreen.BuildCreated, b.Status)
	assert.True(b.Activated)
	assert.Len(b.Tasks, 1)

	dbTask, err := task.FindOneId(b.Tasks[0].Id)
	assert.NoError(err)
	assert.Equal(evergreen.TaskUndispatched, dbTask.Status)
	assert.True(dbTask.Activated)

	config2 := `
stepback: true
buildvariants:
- name: v1
  display_name: "v1_display"
  run_on: d
  tasks:
  - name: t1
tasks:
- name: t1
`
	p = &model.Project{}
	pp, err = model.LoadProjectInto(ctx, []byte(config2), nil, ref.Id, p)
	assert.NoError(err)
	projectInfo.Project = p
	projectInfo.IntermediateProject = pp
	metadata = model.VersionMetadata{
		Message:         "message 2",
		User:            &u,
		IsAdHoc:         true,
		Activate:        true,
		PeriodicBuildID: "abc",
	}
	newVersion, err = dc.CreateVersionFromConfig(context.Background(), projectInfo, metadata)
	assert.NoError(err)
	assert.Equal("message 2", newVersion.Message)
	assert.Equal(evergreen.VersionCreated, newVersion.Status)
	assert.Equal(ref.Id, newVersion.Identifier)
	assert.Equal(2, newVersion.RevisionOrderNumber)
	assert.Equal(evergreen.AdHocRequester, newVersion.Requester)

	pp, err = ppStorage.FindOneByID(ctx, newVersion.Id)
	assert.NoError(err)
	assert.NotNil(pp)
	assert.True(utility.FromBoolPtr(pp.Stepback))

	b, err = build.FindOneId(newVersion.BuildIds[0])
	assert.NoError(err)
	assert.Equal(evergreen.BuildCreated, b.Status)
	assert.True(b.Activated)
	assert.Len(b.Tasks, 1)

	dbTask, err = task.FindOneId(b.Tasks[0].Id)
	assert.NoError(err)
	assert.Equal(evergreen.TaskUndispatched, dbTask.Status)
	assert.True(dbTask.Activated)
}
