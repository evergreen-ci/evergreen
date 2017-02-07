package event

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(evergreen.TestConfig()))
}

func TestLoggingTaskEvents(t *testing.T) {
	Convey("Test task event logging", t, func() {

		testutil.HandleTestingErr(db.Clear(AllLogCollection), t,
			"Error clearing '%v' collection", AllLogCollection)

		Convey("All task events should be logged correctly", func() {
			taskId := "task_id"
			hostId := "host_id"

			LogTaskCreated(taskId)
			time.Sleep(1 * time.Millisecond)
			LogTaskDispatched(taskId, hostId)
			time.Sleep(1 * time.Millisecond)
			LogTaskUndispatched(taskId, hostId)
			time.Sleep(1 * time.Millisecond)
			LogTaskStarted(taskId)
			time.Sleep(1 * time.Millisecond)
			LogTaskFinished(taskId, hostId, evergreen.TaskSucceeded)

			eventsForTask, err := Find(AllLogCollection, TaskEventsInOrder(taskId))
			So(err, ShouldEqual, nil)

			event := eventsForTask[0]
			So(TaskCreated, ShouldEqual, event.EventType)
			So(taskId, ShouldEqual, event.ResourceId)

			eventData, ok := event.Data.Data.(*TaskEventData)
			So(ok, ShouldBeTrue)
			So(eventData.ResourceType, ShouldEqual, ResourceTypeTask)
			So(eventData.HostId, ShouldBeBlank)
			So(eventData.UserId, ShouldBeBlank)
			So(eventData.Status, ShouldBeBlank)
			So(eventData.Timestamp.IsZero(), ShouldBeTrue)

			event = eventsForTask[1]
			So(TaskDispatched, ShouldEqual, event.EventType)
			So(taskId, ShouldEqual, event.ResourceId)

			eventData, ok = event.Data.Data.(*TaskEventData)
			So(ok, ShouldBeTrue)
			So(eventData.ResourceType, ShouldEqual, ResourceTypeTask)
			So(eventData.HostId, ShouldEqual, hostId)
			So(eventData.UserId, ShouldBeBlank)
			So(eventData.Status, ShouldBeBlank)
			So(eventData.Timestamp.IsZero(), ShouldBeTrue)

			event = eventsForTask[2]
			So(TaskUndispatched, ShouldEqual, event.EventType)
			So(taskId, ShouldEqual, event.ResourceId)

			eventData, ok = event.Data.Data.(*TaskEventData)
			So(ok, ShouldBeTrue)
			So(eventData.ResourceType, ShouldEqual, ResourceTypeTask)
			So(eventData.HostId, ShouldEqual, hostId)
			So(eventData.UserId, ShouldBeBlank)
			So(eventData.Status, ShouldBeBlank)
			So(eventData.Timestamp.IsZero(), ShouldBeTrue)

			event = eventsForTask[3]
			So(TaskStarted, ShouldEqual, event.EventType)
			So(taskId, ShouldEqual, event.ResourceId)

			eventData, ok = event.Data.Data.(*TaskEventData)
			So(eventData.ResourceType, ShouldEqual, ResourceTypeTask)
			So(eventData.HostId, ShouldBeBlank)
			So(eventData.UserId, ShouldBeBlank)
			So(eventData.Status, ShouldBeBlank)
			So(eventData.Timestamp.IsZero(), ShouldBeTrue)

			event = eventsForTask[4]
			So(TaskFinished, ShouldEqual, event.EventType)
			So(taskId, ShouldEqual, event.ResourceId)

			eventData, ok = event.Data.Data.(*TaskEventData)
			So(ok, ShouldBeTrue)
			So(eventData.ResourceType, ShouldEqual, ResourceTypeTask)
			So(eventData.HostId, ShouldBeBlank)
			So(eventData.UserId, ShouldBeBlank)
			So(eventData.Status, ShouldEqual, evergreen.TaskSucceeded)
			So(eventData.Timestamp.IsZero(), ShouldBeTrue)
		})
	})
}
