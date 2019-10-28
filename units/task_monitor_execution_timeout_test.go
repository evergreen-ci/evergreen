package units

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
)

func TestCleanupTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)

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

			Convey("the task should be reset", func() {

				newTask := &task.Task{
					Id:       "t1",
					Status:   "started",
					HostId:   "h1",
					BuildId:  "b1",
					Project:  "proj",
					Restarts: 1,
				}
				require.NoError(t, newTask.Insert(), "error inserting task")

				host := &host.Host{
					Id:          "h1",
					RunningTask: "t1",
					Provider:    evergreen.ProviderNameMock,
					Status:      evergreen.HostRunning,
				}
				So(host.Insert(), ShouldBeNil)
				cloud.GetMockProvider().Set(host.Id, cloud.MockInstance{
					Status: cloud.StatusRunning,
				})

				b := &build.Build{
					Id:      "b1",
					Tasks:   []build.TaskCache{{Id: "t1"}},
					Version: "v1",
				}
				So(b.Insert(), ShouldBeNil)

				v := &model.Version{
					Id: "v1",
				}
				So(v.Insert(), ShouldBeNil)

				// cleaning up the task should work
				So(cleanUpTimedOutTask(ctx, env, t.Name(), newTask), ShouldBeNil)

				// refresh the task - it should be reset
				newTask, err := task.FindOne(task.ById("t1"))
				So(err, ShouldBeNil)
				So(newTask.Status, ShouldEqual, evergreen.TaskUndispatched)
				So(newTask.Restarts, ShouldEqual, 2)

				Convey("an execution task should be cleaned up", func() {
					dt := &task.Task{
						Id:             "dt",
						Status:         evergreen.TaskStarted,
						HostId:         "h1",
						BuildId:        "b2",
						Project:        "proj",
						Restarts:       0,
						DisplayOnly:    true,
						ExecutionTasks: []string{"et1", "et2"},
					}
					et1 := &task.Task{
						Id:        "et1",
						Status:    evergreen.TaskStarted,
						HostId:    "h1",
						BuildId:   "b2",
						Project:   "proj",
						Restarts:  0,
						Requester: evergreen.PatchVersionRequester,
					}
					et2 := &task.Task{
						Id:     "et2",
						Status: evergreen.TaskStarted,
					}
					So(dt.Insert(), ShouldBeNil)
					So(et1.Insert(), ShouldBeNil)
					So(et2.Insert(), ShouldBeNil)
					b := &build.Build{
						Id:      "b2",
						Tasks:   []build.TaskCache{{Id: "dt"}},
						Version: "v1",
					}
					So(b.Insert(), ShouldBeNil)

					So(cleanUpTimedOutTask(ctx, env, t.Name(), et1), ShouldBeNil)
					et1, err := task.FindOneId(et1.Id)
					So(err, ShouldBeNil)
					So(et1.Status, ShouldEqual, evergreen.TaskFailed)
					dt, err = task.FindOneId(dt.Id)
					So(err, ShouldBeNil)
					So(dt.Status, ShouldEqual, evergreen.TaskFailed)
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
					Id:       "t1",
					Status:   "started",
					HostId:   "h1",
					BuildId:  "b1",
					Project:  "proj",
					Restarts: 1,
				}
				require.NoError(t, newTask.Insert(), "error inserting task")

				h := &host.Host{
					Id:          "h1",
					RunningTask: "t1",
					Provider:    evergreen.ProviderNameMock,
					Status:      evergreen.HostRunning,
				}
				So(h.Insert(), ShouldBeNil)

				build := &build.Build{
					Id:      "b1",
					Tasks:   []build.TaskCache{{Id: "t1"}},
					Version: "v1",
				}
				So(build.Insert(), ShouldBeNil)

				v := &model.Version{Id: "v1"}
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
					h, err = host.FindOne(host.ById("h1"))
					So(err, ShouldBeNil)
					So(h.RunningTask, ShouldEqual, "")

				})
				Convey("an externally terminated host should be marked terminated and its task should be reset", func() {
					cloud.GetMockProvider().Set(h.Id, cloud.MockInstance{
						Status: cloud.StatusTerminated,
					})

					So(cleanUpTimedOutTask(ctx, env, t.Name(), newTask), ShouldBeNil)

					var err error
					h, err = host.FindOneId("h1")
					So(err, ShouldBeNil)
					So(h.RunningTask, ShouldEqual, "")
					So(h.Status, ShouldEqual, evergreen.HostTerminated)
				})
			})
		})
	})
}
