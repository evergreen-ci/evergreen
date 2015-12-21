package model

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestHostFindNextTask(t *testing.T) {

	Convey("With a host", t, func() {

		Convey("when finding the next task to be run on the host", func() {

			testutil.HandleTestingErr(db.ClearCollections(host.Collection,
				task.Collection, TaskQueuesCollection), t,
				"Error clearing test collections")

			h := &host.Host{
				Id:     "hostId",
				Distro: distro.Distro{},
			}
			So(h.Insert(), ShouldBeNil)

			Convey("if there is no task queue for the host's distro, no task"+
				" should be returned", func() {

				nextTask, err := NextTaskForHost(h)
				So(err, ShouldBeNil)
				So(nextTask, ShouldBeNil)

			})

			Convey("if the task queue is empty, no task should be"+
				" returned", func() {

				tQueue := &TaskQueue{
					Distro: h.Distro.Id,
				}
				So(tQueue.Save(), ShouldBeNil)

				nextTask, err := NextTaskForHost(h)
				So(err, ShouldBeNil)
				So(nextTask, ShouldBeNil)

			})

			Convey("if the task queue is not empty, the corresponding task"+
				" object from the database should be returned", func() {

				tQueue := &TaskQueue{
					Distro: h.Distro.Id,
					Queue: []TaskQueueItem{
						TaskQueueItem{
							Id: "taskOne",
						},
					},
				}
				So(tQueue.Save(), ShouldBeNil)

				task := &task.Task{
					Id: "taskOne",
				}
				So(task.Insert(), ShouldBeNil)

				nextTask, err := NextTaskForHost(h)
				So(err, ShouldBeNil)
				So(nextTask.Id, ShouldEqual, task.Id)

			})

		})

	})
}
