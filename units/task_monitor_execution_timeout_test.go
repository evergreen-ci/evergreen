package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/amboy"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanupTask(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	Convey("When cleaning up a task", t, func() {
		// reset the db
		require.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, build.Collection), "error clearing tasks collection")
		require.NoError(t, db.ClearCollections(host.Collection), "error clearing hosts collection")

		Convey("an error should be thrown if the passed-in projects slice"+
			" does not contain the task's project", func() {

			err := cleanUpTimedOutTask(ctx, env, t.Name(), &task.Task{
				Project: "proj",
			})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "not found")

		})

		Convey("if the task's heartbeat timed out", func() {

			// reset the db
			require.NoError(t, db.ClearCollections(task.Collection), "error clearing tasks collection")
			require.NoError(t, db.ClearCollections(host.Collection), "error clearing hosts collection")
			require.NoError(t, db.ClearCollections(build.Collection), "error clearing builds collection")
			require.NoError(t, db.ClearCollections(task.OldCollection), "error clearing old tasks collection")
			require.NoError(t, db.ClearCollections(model.VersionCollection), "error clearing versions collection")

			vID := "v1"
			bID := "b1"
			tID := "t1"
			hID := "h1"

			Convey("the task should be reset", func() {
				newTask := &task.Task{
					Id:       tID,
					Status:   "started",
					HostId:   hID,
					BuildId:  bID,
					Project:  "proj",
					Restarts: 1,
					Version:  vID,
				}
				require.NoError(t, newTask.Insert(), "error inserting task")

				host := &host.Host{
					Id:          hID,
					RunningTask: tID,
					Distro:      distro.Distro{Provider: evergreen.ProviderNameMock},
					Status:      evergreen.HostRunning,
				}
				So(host.Insert(), ShouldBeNil)
				cloud.GetMockProvider().Set(host.Id, cloud.MockInstance{
					Status: cloud.StatusRunning,
				})

				b := &build.Build{
					Id:      bID,
					Version: vID,
				}
				So(b.Insert(), ShouldBeNil)

				v := &model.Version{
					Id: vID,
				}
				So(v.Insert(), ShouldBeNil)

				// cleaning up the task should work
				So(cleanUpTimedOutTask(ctx, env, t.Name(), newTask), ShouldBeNil)

				// refresh the task - it should be reset
				newTask, err := task.FindOne(task.ById(tID))
				So(err, ShouldBeNil)
				So(newTask.Status, ShouldEqual, evergreen.TaskUndispatched)
				So(newTask.Restarts, ShouldEqual, 2)

				Convey("an execution task should be cleaned up", func() {
					dt := &task.Task{
						Id:             "dt",
						Status:         evergreen.TaskStarted,
						HostId:         hID,
						BuildId:        "b2",
						Project:        "proj",
						Restarts:       0,
						DisplayOnly:    true,
						ExecutionTasks: []string{"et1", "et2"},
					}
					et1 := &task.Task{
						Id:        "et1",
						Status:    evergreen.TaskStarted,
						HostId:    hID,
						BuildId:   "b2",
						Project:   "proj",
						Restarts:  0,
						Requester: evergreen.PatchVersionRequester,
						Activated: true,
						Version:   vID,
					}
					et2 := &task.Task{
						Id:        "et2",
						Status:    evergreen.TaskStarted,
						Activated: true,
						Version:   vID,
					}
					So(dt.Insert(), ShouldBeNil)
					So(et1.Insert(), ShouldBeNil)
					So(et2.Insert(), ShouldBeNil)
					b := &build.Build{
						Id:      "b2",
						Version: vID,
					}
					So(b.Insert(), ShouldBeNil)

					So(cleanUpTimedOutTask(ctx, env, t.Name(), et1), ShouldBeNil)
					et1, err := task.FindOneId(et1.Id)
					So(err, ShouldBeNil)
					So(et1.Status, ShouldEqual, evergreen.TaskFailed)
					dt, err = task.FindOneId(dt.Id)
					So(err, ShouldBeNil)
					So(dt.Status, ShouldEqual, evergreen.TaskStarted) // et2 is still running
					So(dt.ResetWhenFinished, ShouldBeTrue)
				})
			})

			Convey("given a running host running a task", func() {
				require.NoError(t, db.ClearCollections(task.Collection), "error clearing tasks collection")
				require.NoError(t, db.ClearCollections(host.Collection), "error clearing hosts collection")
				require.NoError(t, db.ClearCollections(build.Collection), "error clearing builds collection")
				require.NoError(t, db.ClearCollections(task.OldCollection), "error clearing old tasks collection")
				require.NoError(t, db.ClearCollections(model.VersionCollection), "error clearing versions collection")

				newTask := &task.Task{
					Id:       tID,
					Status:   "started",
					HostId:   hID,
					BuildId:  bID,
					Project:  "proj",
					Restarts: 1,
					Version:  vID,
				}
				require.NoError(t, newTask.Insert(), "error inserting task")

				h := &host.Host{
					Id:          hID,
					RunningTask: tID,
					Distro:      distro.Distro{Provider: evergreen.ProviderNameMock},
					Provider:    evergreen.ProviderNameMock,
					StartedBy:   evergreen.User,
					Status:      evergreen.HostRunning,
				}
				So(h.Insert(), ShouldBeNil)

				build := &build.Build{
					Id:      bID,
					Version: vID,
				}
				So(build.Insert(), ShouldBeNil)

				v := &model.Version{Id: vID}
				So(v.Insert(), ShouldBeNil)

				Convey("a running host should have its running task cleared", func() {
					cloud.GetMockProvider().Set(h.Id, cloud.MockInstance{
						Status: cloud.StatusRunning,
					})

					// cleaning up the task should work
					So(cleanUpTimedOutTask(ctx, env, t.Name(), newTask), ShouldBeNil)

					// refresh the host, make sure its running task field has
					// been reset
					var err error
					h, err = host.FindOne(host.ById(hID))
					So(err, ShouldBeNil)
					So(h.RunningTask, ShouldEqual, "")

				})
				Convey("an externally terminated host should be marked terminated and its task should be reset", func() {
					cloud.GetMockProvider().Set(h.Id, cloud.MockInstance{
						Status: cloud.StatusTerminated,
					})

					So(cleanUpTimedOutTask(ctx, env, t.Name(), newTask), ShouldBeNil)

					So(amboy.WaitInterval(ctx, env.RemoteQueue(), 100*time.Millisecond), ShouldBeTrue)

					var err error
					h, err = host.FindOneId(hID)
					So(err, ShouldBeNil)
					So(h.RunningTask, ShouldEqual, "")
					So(h.Status, ShouldEqual, evergreen.HostTerminated)
				})
			})
		})
	})
}

func TestCleanupTimedOutTaskWithTaskGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	require.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, build.Collection, host.Collection, model.VersionCollection))

	t1 := &task.Task{
		Id:                "t1",
		DisplayName:       "display_t1",
		Status:            evergreen.TaskStarted,
		HostId:            "h1",
		BuildId:           "b1",
		Version:           "v1",
		TaskGroup:         "my_task_group",
		TaskGroupMaxHosts: 1,
		Project:           "proj",
		Restarts:          1,
	}
	t2 := &task.Task{
		Id:                "t2",
		DisplayName:       "display_t2",
		Status:            evergreen.TaskSucceeded,
		HostId:            "h2",
		BuildId:           "b1",
		Version:           "v1",
		TaskGroup:         "my_task_group",
		TaskGroupMaxHosts: 1,
		Project:           "proj",
		Restarts:          1,
	}
	t3 := &task.Task{
		Id:                "t3",
		DisplayName:       "display_t3",
		Status:            evergreen.TaskUndispatched,
		BuildId:           "b1",
		Version:           "v1",
		TaskGroup:         "my_task_group",
		TaskGroupMaxHosts: 1,
		Project:           "proj",
		Restarts:          1,
	}
	b := &build.Build{
		Id:      "b1",
		Version: "v1",
	}
	h := &host.Host{
		Id:          "h1",
		RunningTask: "t1",
		Distro:      distro.Distro{Provider: evergreen.ProviderNameMock},
		Status:      evergreen.HostRunning,
	}
	yml := `
task_groups:
- name: my_task_group
  tasks: [display_t1, display_t2, display_t3]
tasks:
- name: display_t1
- name: display_t2
- name: display_t3
`
	v := &model.Version{
		Id:     "v1",
		Config: yml,
	}
	assert.NoError(t, t1.Insert())
	assert.NoError(t, t2.Insert())
	assert.NoError(t, t3.Insert())
	assert.NoError(t, b.Insert())
	assert.NoError(t, h.Insert())
	assert.NoError(t, v.Insert())
	cloud.GetMockProvider().Set(h.Id, cloud.MockInstance{
		Status: cloud.StatusRunning,
	})

	assert.NoError(t, cleanUpTimedOutTask(ctx, env, t.Name(), t1))

	// refresh the host, make sure its running task field has been reset
	var err error
	h, err = host.FindOneId("h1")
	assert.NoError(t, err)
	assert.Empty(t, h.RunningTask)

	t2, err = task.FindOneId(t2.Id)
	assert.NoError(t, err)
	assert.NotNil(t, t2)
	assert.NotEqual(t, evergreen.TaskSucceeded, t2.Status)
}
