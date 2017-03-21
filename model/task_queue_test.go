package model

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip"
	. "github.com/smartystreets/goconvey/convey"
)

var (
	_                 fmt.Stringer = nil
	taskQueueTestConf              = testutil.TestConfig()
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(taskQueueTestConf))
	grip.SetSender(testutil.SetupTestSender("/tmp/task_queue_test.log"))
}

func TestDequeueTask(t *testing.T) {

	var taskIds []string
	var distroId string
	var taskQueue *TaskQueue

	Convey("When attempting to pull a task from a task queue", t, func() {

		taskIds = []string{"t1", "t2", "t3"}
		distroId = "d1"
		taskQueue = &TaskQueue{
			Distro: distroId,
			Queue:  []TaskQueueItem{},
		}

		So(db.Clear(TaskQueuesCollection), ShouldBeNil)

		Convey("if the task queue is empty, an error should be thrown", func() {
			So(taskQueue.Save(), ShouldBeNil)
			So(taskQueue.DequeueTask(taskIds[0]), ShouldNotBeNil)
		})

		Convey("if the task is not present in the queue, an error should be"+
			" thrown", func() {
			taskQueue.Queue = append(taskQueue.Queue,
				TaskQueueItem{Id: taskIds[1]})
			So(taskQueue.Save(), ShouldBeNil)
			So(taskQueue.DequeueTask(taskIds[0]), ShouldNotBeNil)
		})

		Convey("if the task is present in the queue, it should be removed"+
			" from the in-memory and db versions of the queue", func() {
			taskQueue.Queue = []TaskQueueItem{
				{Id: taskIds[0]},
				{Id: taskIds[1]},
				{Id: taskIds[2]},
			}
			So(taskQueue.Save(), ShouldBeNil)
			So(taskQueue.DequeueTask(taskIds[1]), ShouldBeNil)

			// make sure the queue was updated in memory
			So(taskQueue.Length(), ShouldEqual, 2)
			So(taskQueue.Queue[0].Id, ShouldEqual, taskIds[0])
			So(taskQueue.Queue[1].Id, ShouldEqual, taskIds[2])

			// make sure the db representation was updated
			taskQueue, err := FindTaskQueueForDistro(distroId)
			So(err, ShouldBeNil)
			So(taskQueue.Length(), ShouldEqual, 2)
			So(taskQueue.Queue[0].Id, ShouldEqual, taskIds[0])
			So(taskQueue.Queue[1].Id, ShouldEqual, taskIds[2])

		})

	})
}
