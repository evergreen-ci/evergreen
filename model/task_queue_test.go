package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

var (
	taskQueueTestConf = testutil.TestConfig()
)

func init() {
	db.SetGlobalSessionProvider(taskQueueTestConf.SessionFactory())
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
			tq, err := LoadTaskQueue(distroId)
			taskQueue := tq.(*TaskQueue)
			So(err, ShouldBeNil)
			So(taskQueue.Length(), ShouldEqual, 2)
			So(taskQueue.Queue[0].Id, ShouldEqual, taskIds[0])
			So(taskQueue.Queue[1].Id, ShouldEqual, taskIds[2])

		})
	})
}

func TestFindTask(t *testing.T) {
	assert := assert.New(t) // nolint

	q := &TaskQueue{
		Queue: []TaskQueueItem{
			{Id: "one", Group: "foo", Project: "a", Version: "b", BuildVariant: "a"},
			{Id: "two", Group: "bar", Project: "a", Version: "b", BuildVariant: "a"},
			{Id: "three", Project: "a", Version: "b", BuildVariant: "a"},
			{Id: "four", Project: "a", Version: "b", BuildVariant: "a"},
			{Id: "five", Group: "foo", Project: "aa", Version: "bb", BuildVariant: "a"},
			{Id: "six", Group: "bar", Project: "aa", Version: "bb", BuildVariant: "a"},
			{Id: "seven", Project: "aa", Version: "bb", BuildVariant: "a"},
			{Id: "eight", Project: "aa", Version: "bb", BuildVariant: "a"},
		},
	}

	// ensure that it's always nil if the group name isn't specified
	assert.Nil(q.FindTask(TaskSpec{}))
	assert.Nil(q.FindTask(TaskSpec{BuildVariant: "a"}))
	assert.Nil(q.FindTask(TaskSpec{ProjectID: "a"}))
	assert.Nil(q.FindTask(TaskSpec{Version: "b"}))
	assert.Nil(q.FindTask(TaskSpec{BuildVariant: "a", ProjectID: "a"}))
	assert.Nil(q.FindTask(TaskSpec{BuildVariant: "a", Version: "b"}))
	assert.Nil(q.FindTask(TaskSpec{ProjectID: "a", Version: "b"}))

	// ensure that we can get the task groups that we expect
	assert.Equal("five", q.FindTask(TaskSpec{Group: "foo", ProjectID: "aa", Version: "bb", BuildVariant: "a"}).Id)
	assert.Equal("one", q.FindTask(TaskSpec{Group: "foo", ProjectID: "a", Version: "b", BuildVariant: "a"}).Id)
	assert.Equal("six", q.FindTask(TaskSpec{Group: "bar", ProjectID: "aa", Version: "bb", BuildVariant: "a"}).Id)
	assert.Equal("two", q.FindTask(TaskSpec{Group: "bar", ProjectID: "a", Version: "b", BuildVariant: "a"}).Id)
}
