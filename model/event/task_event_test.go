package event

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
	"time"
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(evergreen.TestConfig()))
}

func TestLoggingTaskEvents(t *testing.T) {
	Convey("Test task event logging", t, func() {

		util.HandleTestingErr(db.Clear(Collection), t,
			"Error clearing '%v' collection", Collection)

		Convey("All task events should be logged correctly", func() {
			taskId := "task_id"
			hostId := "host_id"

			LogTaskCreated(taskId)
			time.Sleep(1 * time.Millisecond)
			LogTaskDispatched(taskId, hostId)
			time.Sleep(1 * time.Millisecond)
			LogTaskStarted(taskId)
			time.Sleep(1 * time.Millisecond)
			LogTaskFinished(taskId, evergreen.TaskSucceeded)

			eventsForTask, err := Find(TaskEventsInOrder(taskId))
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
			So(TaskStarted, ShouldEqual, event.EventType)
			So(taskId, ShouldEqual, event.ResourceId)

			eventData, ok = event.Data.Data.(*TaskEventData)
			So(eventData.ResourceType, ShouldEqual, ResourceTypeTask)
			So(eventData.HostId, ShouldBeBlank)
			So(eventData.UserId, ShouldBeBlank)
			So(eventData.Status, ShouldBeBlank)
			So(eventData.Timestamp.IsZero(), ShouldBeTrue)

			event = eventsForTask[3]
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
