package monitor

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestCleanupTask(t *testing.T) {

	testConfig := evergreen.TestConfig()

	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	Convey("When cleaning up a task", t, func() {

		// reset the db
		testutil.HandleTestingErr(db.ClearCollections(model.TasksCollection),
			t, "error clearing tasks collection")
		testutil.HandleTestingErr(db.ClearCollections(host.Collection),
			t, "error clearing hosts collection")

		Convey("an error should be thrown if the passed-in projects slice"+
			" does not contain the task's project", func() {

			wrapper := doomedTaskWrapper{
				task: model.Task{
					Project: "proj",
				},
			}
			projects := map[string]model.Project{}
			err := cleanUpTask(wrapper, projects)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "could not find project")

		})

		Convey("an error should be thrown if the task's host is marked with"+
			" the wrong running task id", func() {

			wrapper := doomedTaskWrapper{
				task: model.Task{
					Id:      "t1",
					HostId:  "h1",
					Project: "proj",
				},
			}
			projects := map[string]model.Project{
				"proj": model.Project{
					Identifier: "proj",
				},
			}
			host := &host.Host{
				Id:          "h1",
				RunningTask: "nott1",
			}
			So(host.Insert(), ShouldBeNil)

			err := cleanUpTask(wrapper, projects)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "but the host thinks it"+
				" is running task")

		})

		Convey("if the task's heartbeat timed out", func() {

			// reset the db
			testutil.HandleTestingErr(db.ClearCollections(model.TasksCollection),
				t, "error clearing tasks collection")
			testutil.HandleTestingErr(db.ClearCollections(host.Collection),
				t, "error clearing hosts collection")
			testutil.HandleTestingErr(db.ClearCollections(build.Collection),
				t, "error clearing builds collection")
			testutil.HandleTestingErr(db.ClearCollections(model.OldTasksCollection),
				t, "error clearing old tasks collection")
			testutil.HandleTestingErr(db.ClearCollections(version.Collection),
				t, "error clearing versions collection")

			Convey("the task should be reset", func() {

				task := &model.Task{
					Id:       "t1",
					Status:   "started",
					HostId:   "h1",
					BuildId:  "b1",
					Project:  "proj",
					Restarts: 1,
				}
				testutil.HandleTestingErr(task.Insert(), t, "error inserting task")

				wrapper := doomedTaskWrapper{
					reason: HeartbeatTimeout,
					task:   *task,
				}

				projects := map[string]model.Project{
					"proj": model.Project{
						Identifier: "proj",
						Stepback:   false,
					},
				}

				host := &host.Host{
					Id:          "h1",
					RunningTask: "t1",
				}
				So(host.Insert(), ShouldBeNil)

				build := &build.Build{
					Id:      "b1",
					Tasks:   []build.TaskCache{{Id: "t1"}},
					Version: "v1",
				}
				So(build.Insert(), ShouldBeNil)

				v := &version.Version{
					Id: "v1",
				}
				So(v.Insert(), ShouldBeNil)

				// cleaning up the task should work
				So(cleanUpTask(wrapper, projects), ShouldBeNil)

				// refresh the task - it should be reset
				task, err := model.FindTask("t1")
				So(err, ShouldBeNil)
				So(task.Status, ShouldEqual, evergreen.TaskUndispatched)
				So(task.Restarts, ShouldEqual, 2)

			})

			Convey("the running task field on the task's host should be"+
				" reset", func() {

				task := &model.Task{
					Id:       "t1",
					Status:   "started",
					HostId:   "h1",
					BuildId:  "b1",
					Project:  "proj",
					Restarts: 1,
				}
				testutil.HandleTestingErr(task.Insert(), t, "error inserting task")

				wrapper := doomedTaskWrapper{
					reason: HeartbeatTimeout,
					task:   *task,
				}

				projects := map[string]model.Project{
					"proj": model.Project{
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
