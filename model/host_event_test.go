package model

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
	"time"
)

var (
	hostEventTestConfig = mci.TestConfig()
)

func init() {
	db.SetGlobalSessionProvider(
		db.SessionFactoryFromConfig(hostEventTestConfig))
}

func TestLoggingHostEvents(t *testing.T) {
	Convey("When logging host events", t, func() {

		So(db.Clear(EventLogCollection), ShouldBeNil)

		Convey("all events logged should be persisted to the database, and"+
			" fetching them in order should sort by the time they were"+
			" logged", func() {

			hostId := "host_id"
			hostname := "hostname"
			taskId := "task_id"
			taskPid := "12345"

			// log some events, sleeping in between to make sure the times are different
			LogHostCreatedEvent(hostId)
			time.Sleep(1 * time.Millisecond)
			LogHostStatusChangedEvent(hostId, mci.HostRunning, mci.HostTerminated)
			time.Sleep(1 * time.Millisecond)
			LogHostDNSNameSetEvent(hostId, hostname)
			time.Sleep(1 * time.Millisecond)
			LogHostProvisionedEvent(hostId)
			time.Sleep(1 * time.Millisecond)
			LogHostRunningTaskSetEvent(hostId, taskId)
			time.Sleep(1 * time.Millisecond)
			LogHostRunningTaskClearedEvent(hostId, taskId)
			time.Sleep(1 * time.Millisecond)
			LogHostTaskPidSetEvent(hostId, taskPid)
			time.Sleep(1 * time.Millisecond)

			// fetch all the events from the database, make sure they are
			// persisted correctly

			eventsForHost, err := FindAllHostEventsInOrder(hostId)
			So(err, ShouldBeNil)

			event := eventsForHost[0]
			So(event.EventType, ShouldEqual, EventHostCreated)
			So(event.ResourceId, ShouldEqual, hostId)

			eventData, ok := event.Data.EventData.(*HostEventData)
			So(ok, ShouldBeTrue)
			So(eventData.ResourceType, ShouldEqual, ResourceTypeHost)
			So(eventData.OldStatus, ShouldBeBlank)
			So(eventData.NewStatus, ShouldBeBlank)
			So(eventData.SetupLog, ShouldBeBlank)
			So(eventData.Hostname, ShouldBeBlank)
			So(eventData.TaskId, ShouldBeBlank)
			So(eventData.TaskPid, ShouldBeBlank)

			event = eventsForHost[1]
			So(event.EventType, ShouldEqual, EventHostStatusChanged)
			So(event.ResourceId, ShouldEqual, hostId)

			eventData, ok = event.Data.EventData.(*HostEventData)
			So(ok, ShouldBeTrue)
			So(eventData.ResourceType, ShouldEqual, ResourceTypeHost)
			So(eventData.OldStatus, ShouldEqual, mci.HostRunning)
			So(eventData.NewStatus, ShouldEqual, mci.HostTerminated)
			So(eventData.SetupLog, ShouldBeBlank)
			So(eventData.Hostname, ShouldBeBlank)
			So(eventData.TaskId, ShouldBeBlank)
			So(eventData.TaskPid, ShouldBeBlank)

			event = eventsForHost[2]
			So(event.EventType, ShouldEqual, EventHostDNSNameSet)
			So(event.ResourceId, ShouldEqual, hostId)

			eventData, ok = event.Data.EventData.(*HostEventData)
			So(ok, ShouldBeTrue)
			So(eventData.ResourceType, ShouldEqual, ResourceTypeHost)
			So(eventData.OldStatus, ShouldBeBlank)
			So(eventData.NewStatus, ShouldBeBlank)
			So(eventData.SetupLog, ShouldBeBlank)
			So(eventData.Hostname, ShouldEqual, hostname)
			So(eventData.TaskId, ShouldBeBlank)
			So(eventData.TaskPid, ShouldBeBlank)

			event = eventsForHost[3]
			So(event.EventType, ShouldEqual, EventHostProvisioned)
			So(event.ResourceId, ShouldEqual, hostId)

			eventData, ok = event.Data.EventData.(*HostEventData)
			So(ok, ShouldBeTrue)
			So(eventData.ResourceType, ShouldEqual, ResourceTypeHost)
			So(eventData.OldStatus, ShouldBeBlank)
			So(eventData.NewStatus, ShouldBeBlank)
			So(eventData.SetupLog, ShouldBeBlank)
			So(eventData.Hostname, ShouldBeBlank)
			So(eventData.TaskId, ShouldBeBlank)
			So(eventData.TaskPid, ShouldBeBlank)

			event = eventsForHost[4]
			So(event.EventType, ShouldEqual, EventHostRunningTaskSet)
			So(event.ResourceId, ShouldEqual, hostId)

			eventData, ok = event.Data.EventData.(*HostEventData)
			So(ok, ShouldBeTrue)
			So(eventData.ResourceType, ShouldEqual, ResourceTypeHost)
			So(eventData.OldStatus, ShouldBeBlank)
			So(eventData.NewStatus, ShouldBeBlank)
			So(eventData.SetupLog, ShouldBeBlank)
			So(eventData.Hostname, ShouldBeBlank)
			So(eventData.TaskId, ShouldEqual, taskId)
			So(eventData.TaskPid, ShouldBeBlank)

			event = eventsForHost[5]
			So(event.EventType, ShouldEqual, EventHostRunningTaskCleared)
			So(event.ResourceId, ShouldEqual, hostId)

			eventData, ok = event.Data.EventData.(*HostEventData)
			So(ok, ShouldBeTrue)
			So(eventData.ResourceType, ShouldEqual, ResourceTypeHost)
			So(eventData.OldStatus, ShouldBeBlank)
			So(eventData.NewStatus, ShouldBeBlank)
			So(eventData.SetupLog, ShouldBeBlank)
			So(eventData.Hostname, ShouldBeBlank)
			So(eventData.TaskId, ShouldEqual, taskId)
			So(eventData.TaskPid, ShouldBeBlank)

			event = eventsForHost[6]
			So(event.EventType, ShouldEqual, EventHostTaskPidSet)
			So(event.ResourceId, ShouldEqual, hostId)

			eventData, ok = event.Data.EventData.(*HostEventData)
			So(ok, ShouldBeTrue)
			So(eventData.ResourceType, ShouldEqual, ResourceTypeHost)
			So(eventData.OldStatus, ShouldBeBlank)
			So(eventData.NewStatus, ShouldBeBlank)
			So(eventData.SetupLog, ShouldBeBlank)
			So(eventData.Hostname, ShouldBeBlank)
			So(eventData.TaskId, ShouldBeBlank)
			So(eventData.TaskPid, ShouldEqual, taskPid)
		})
	})
}
