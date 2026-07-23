package scheduler

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/mongo"
)

type SchedulerSuite struct {
	suite.Suite
}

func TestSchedulerSpawnSuite(t *testing.T) {
	suite.Run(t, new(SchedulerSuite))
}

func (s *SchedulerSuite) SetupTest() {
	s.NoError(db.ClearCollections("hosts"))
	s.NoError(db.ClearCollections("distro"))
	s.NoError(db.ClearCollections("tasks"))
}

func TestCreateIntentHosts(t *testing.T) {
	ctx := t.Context()

	Convey("When spawning hosts", t, func() {

		distroIds := []string{"d1", "d2", "d3"}
		Convey("if there are no hosts to be spawned, the Scheduler should not"+
			" make any calls to the Manager", func() {

			newHostsSpawned, err := CreateIntentHosts(ctx, distro.Distro{}, 0)
			So(err, ShouldBeNil)
			So(len(newHostsSpawned), ShouldEqual, 0)
		})

		Convey("if there are hosts to be spawned, the Scheduler should make"+
			" one call to the Manager for each host, and return the"+
			" results bucketed by distro", func() {

			newHostsNeeded := map[string]int{
				distroIds[0]: 3,
				distroIds[1]: 0,
				distroIds[2]: 1,
			}

			for _, id := range distroIds {
				d := distro.Distro{
					Id:       id,
					Provider: evergreen.ProviderNameMock,
					HostAllocatorSettings: distro.HostAllocatorSettings{
						MaximumHosts: 3,
					},
				}

				newHostsSpawned, err := CreateIntentHosts(ctx, d, newHostsNeeded[id])
				So(err, ShouldBeNil)

				So(newHostsNeeded[id], ShouldEqual, len(newHostsSpawned))
			}
		})
	})
}

func TestUnderwaterUnschedule(t *testing.T) {
	assert := assert.New(t)

	ctx := t.Context()

	require.NoError(t, db.ClearCollections(task.Collection, distro.Collection, build.Collection, model.VersionCollection))
	require.NoError(t, db.EnsureIndex(task.Collection,
		mongo.IndexModel{Keys: task.ActivatedTasksByDistroIndex}))

	t1 := task.Task{
		Id:            "t1",
		Status:        evergreen.TaskUndispatched,
		Activated:     true,
		Priority:      0,
		ActivatedTime: time.Time{},
		DistroId:      "d",
		BuildId:       "b",
		Version:       "v",
	}
	assert.NoError(t1.Insert(ctx))

	t2 := task.Task{
		Id:            "t2",
		Status:        evergreen.TaskUndispatched,
		Activated:     true,
		Priority:      0,
		ActivatedTime: time.Time{},
		DistroId:      "d",
		BuildId:       "b",
		Version:       "v",
	}
	assert.NoError(t2.Insert(ctx))

	t3 := task.Task{
		Id:            "t3",
		Status:        evergreen.TaskUndispatched,
		Activated:     true,
		Priority:      0,
		ActivatedTime: time.Now(),
		DistroId:      "d",
		BuildId:       "b",
		Version:       "v",
	}
	assert.NoError(t3.Insert(ctx))

	dt := task.Task{
		Id:             "dt",
		DisplayOnly:    true,
		ExecutionTasks: []string{"et1", "et2"},
		Status:         evergreen.TaskStarted,
		Activated:      true,
		Priority:       0,
		ActivatedTime:  time.Now(),
		DistroId:       "d",
		BuildId:        "b",
		Version:        "v",
	}
	assert.NoError(dt.Insert(ctx))
	execTasks := []task.Task{
		{
			Id:            "et1",
			Status:        evergreen.TaskSucceeded,
			Activated:     true,
			Priority:      0,
			ActivatedTime: time.Time{},
			DistroId:      "d",
			BuildId:       "b",
			Version:       "v",
		},
		{
			Id:            "et2",
			Status:        evergreen.TaskUndispatched,
			Activated:     true,
			Priority:      0,
			ActivatedTime: time.Time{},
			DistroId:      "d",
			BuildId:       "b",
			Version:       "v",
		},
	}
	for _, et := range execTasks {
		assert.NoError(et.Insert(ctx))
	}

	d := distro.Distro{
		Id: "d",
	}
	b := build.Build{
		Id:        "b",
		Activated: true,
		Status:    evergreen.BuildStarted,
		Version:   "v",
	}
	v := model.Version{
		Id:     "v",
		Status: evergreen.VersionStarted,
	}
	assert.NoError(d.Insert(ctx))
	assert.NoError(b.Insert(ctx))
	assert.NoError(v.Insert(ctx))

	err := underwaterUnschedule(ctx, "d")
	assert.NoError(err)

	foundBuild, err := build.FindOneId(t.Context(), b.Id)
	assert.NoError(err)
	require.NotNil(t, foundBuild)
	foundVersion, err := model.VersionFindOneId(t.Context(), v.Id)
	assert.NoError(err)
	require.NotNil(t, foundVersion)
	foundT1, err := task.FindOneId(ctx, t1.Id)
	assert.NoError(err)
	require.NotNil(t, foundT1)
	foundT2, err := task.FindOneId(ctx, t2.Id)
	assert.NoError(err)
	require.NotNil(t, foundT2)
	foundT3, err := task.FindOneId(ctx, t3.Id)
	assert.NoError(err)
	require.NotNil(t, foundT3)

	assert.Equal(evergreen.VersionStarted, foundVersion.Status)
	assert.Equal(evergreen.DisabledTaskPriority, foundT1.Priority)
	assert.Equal(evergreen.DisabledTaskPriority, foundT2.Priority)
	assert.Equal(int64(0), foundT3.Priority)
	assert.False(foundT1.Activated)
	assert.False(foundT2.Activated)
	assert.True(foundT3.Activated)

	foundDisplayTask, err := task.FindOneId(ctx, dt.Id)
	assert.NoError(err)
	require.NotNil(t, foundDisplayTask)
	assert.Equal(evergreen.TaskSucceeded, foundDisplayTask.Status)
}
