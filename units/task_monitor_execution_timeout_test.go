package units

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func TestCleanupTask(t *testing.T) {
	Convey("When cleaning up a task", t, func() {
		// reset the db
		testutil.HandleTestingErr(db.ClearCollections(task.Collection, task.OldCollection, build.Collection),
			t, "error clearing tasks collection")
		testutil.HandleTestingErr(db.ClearCollections(host.Collection),
			t, "error clearing hosts collection")

		Convey("an error should be thrown if the passed-in projects slice"+
			" does not contain the task's project", func() {

			err := cleanUpTimedOutTask(&task.Task{
				Project: "proj",
			})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "not found")

		})

		Convey("if the task's heartbeat timed out", func() {

			// reset the db
			testutil.HandleTestingErr(db.ClearCollections(task.Collection),
				t, "error clearing tasks collection")
			testutil.HandleTestingErr(db.ClearCollections(host.Collection),
				t, "error clearing hosts collection")
			testutil.HandleTestingErr(db.ClearCollections(build.Collection),
				t, "error clearing builds collection")
			testutil.HandleTestingErr(db.ClearCollections(task.OldCollection),
				t, "error clearing old tasks collection")
			testutil.HandleTestingErr(db.ClearCollections(model.VersionCollection),
				t, "error clearing versions collection")

			Convey("the task should be reset", func() {

				newTask := &task.Task{
					Id:       "t1",
					Status:   "started",
					HostId:   "h1",
					BuildId:  "b1",
					Project:  "proj",
					Restarts: 1,
				}
				testutil.HandleTestingErr(newTask.Insert(), t, "error inserting task")

				host := &host.Host{
					Id:          "h1",
					RunningTask: "t1",
				}
				So(host.Insert(), ShouldBeNil)

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
				So(cleanUpTimedOutTask(newTask), ShouldBeNil)

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
						ExecutionTasks: []string{"et"},
					}
					et := &task.Task{
						Id:       "et",
						Status:   evergreen.TaskStarted,
						HostId:   "h1",
						BuildId:  "b2",
						Project:  "proj",
						Restarts: 0,
					}
					So(dt.Insert(), ShouldBeNil)
					So(et.Insert(), ShouldBeNil)
					b := &build.Build{
						Id:      "b2",
						Tasks:   []build.TaskCache{{Id: "dt"}},
						Version: "v1",
					}
					So(b.Insert(), ShouldBeNil)

					So(cleanUpTimedOutTask(et), ShouldBeNil)
					et, err := task.FindOneId(et.Id)
					So(err, ShouldBeNil)
					So(et.Status, ShouldEqual, evergreen.TaskFailed)
					dt, err = task.FindOneId(dt.Id)
					So(err, ShouldBeNil)
					So(dt.Status, ShouldEqual, evergreen.TaskSystemUnresponse)
				})
			})

			Convey("the running task field on the task's host should be"+
				" reset", func() {

				newTask := &task.Task{
					Id:       "t1",
					Status:   "started",
					HostId:   "h1",
					BuildId:  "b1",
					Project:  "proj",
					Restarts: 1,
				}
				testutil.HandleTestingErr(newTask.Insert(), t, "error inserting task")

				h := &host.Host{
					Id:          "h1",
					RunningTask: "t1",
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

				// cleaning up the task should work
				So(cleanUpTimedOutTask(newTask), ShouldBeNil)

				// refresh the host, make sure its running task field has
				// been reset
				h, err := host.FindOne(host.ById("h1"))
				So(err, ShouldBeNil)
				So(h.RunningTask, ShouldEqual, "")

			})
		})
	})
}
