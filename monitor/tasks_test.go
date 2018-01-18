package monitor

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func TestCleanupTask(t *testing.T) {

	testConfig := testutil.TestConfig()

	db.SetGlobalSessionProvider(testConfig.SessionFactory())

	Convey("When cleaning up a task", t, func() {

		// reset the db
		testutil.HandleTestingErr(db.ClearCollections(task.Collection, task.OldCollection, build.Collection),
			t, "error clearing tasks collection")
		testutil.HandleTestingErr(db.ClearCollections(host.Collection),
			t, "error clearing hosts collection")

		Convey("an error should be thrown if the passed-in projects slice"+
			" does not contain the task's project", func() {

			wrapper := doomedTaskWrapper{
				task: task.Task{
					Project: "proj",
				},
			}
			projects := map[string]model.Project{}
			err := cleanUpTask(wrapper, projects)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "could not find project")

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
			testutil.HandleTestingErr(db.ClearCollections(version.Collection),
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

				wrapper := doomedTaskWrapper{
					reason: HeartbeatTimeout,
					task:   *newTask,
				}

				projects := map[string]model.Project{
					"proj": {
						Identifier: "proj",
						Stepback:   false,
					},
				}

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

				v := &version.Version{
					Id: "v1",
				}
				So(v.Insert(), ShouldBeNil)

				// cleaning up the task should work
				So(cleanUpTask(wrapper, projects), ShouldBeNil)

				// refresh the task - it should be reset
				newTask, err := task.FindOne(task.ById("t1"))
				So(err, ShouldBeNil)
				So(newTask.Status, ShouldEqual, evergreen.TaskUndispatched)
				So(newTask.Restarts, ShouldEqual, 2)

				Convey("a display task should reset all of its execution tasks", func() {
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
						Id:    "b2",
						Tasks: []build.TaskCache{{Id: "dt"}},
					}
					So(b.Insert(), ShouldBeNil)
					wrapper := doomedTaskWrapper{
						reason: HeartbeatTimeout,
						task:   *et,
					}
					projects := map[string]model.Project{
						"proj": {
							Identifier: "proj",
							Stepback:   false,
						},
					}

					So(cleanUpTask(wrapper, projects), ShouldBeNil)
					dbTask, err := task.FindOne(task.ById(dt.Id))
					So(err, ShouldBeNil)
					So(dbTask.Status, ShouldEqual, evergreen.TaskUnstarted)
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

				wrapper := doomedTaskWrapper{
					reason: HeartbeatTimeout,
					task:   *newTask,
				}

				projects := map[string]model.Project{
					"proj": {
						Identifier: "proj",
						Stepback:   false,
					},
				}

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

				v := &version.Version{Id: "v1"}
				So(v.Insert(), ShouldBeNil)

				// cleaning up the task should work
				So(cleanUpTask(wrapper, projects), ShouldBeNil)

				// refresh the host, make sure its running task field has
				// been reset
				h, err := host.FindOne(host.ById("h1"))
				So(err, ShouldBeNil)
				So(h.RunningTask, ShouldEqual, "")

			})

		})

	})

}
