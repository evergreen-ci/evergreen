package model

import (
	"10gen.com/mci/db"
	"10gen.com/mci/model/distro"
	"10gen.com/mci/model/host"
	"10gen.com/mci/util"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestHostFindNextTask(t *testing.T) {

	Convey("With a host", t, func() {

		Convey("when finding the next task to be run on the host", func() {

			util.HandleTestingErr(db.ClearCollections(host.Collection,
				TasksCollection, TaskQueuesCollection), t,
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

				task := &Task{
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
