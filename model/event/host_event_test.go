package event

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	. "github.com/smartystreets/goconvey/convey"
)

func TestLoggingHostEvents(t *testing.T) {
	Convey("When logging host events", t, func() {

		So(db.Clear(LegacyEventLogCollection), ShouldBeNil)

		Convey("all events logged should be persisted to the database, and"+
			" fetching them in order should sort by the time they were"+
			" logged", func() {

			hostId := "host_id"
			hostTag := "host_tag"
			hostname := "hostname"
			taskId := "task_id"

			// log some events, sleeping in between to make sure the times are different
			LogHostCreated(hostId)
			time.Sleep(1 * time.Millisecond)
			LogHostStatusChanged(hostTag, evergreen.HostRunning, evergreen.HostTerminated, "user", "myLogs")
			time.Sleep(1 * time.Millisecond)
			LogHostDNSNameSet(hostId, hostname)
			time.Sleep(1 * time.Millisecond)
			LogHostProvisioned(hostTag)
			time.Sleep(1 * time.Millisecond)
			LogHostRunningTaskSet(hostId, taskId, 0)
			time.Sleep(1 * time.Millisecond)
			LogHostRunningTaskCleared(hostId, taskId, 0)
			time.Sleep(1 * time.Millisecond)

			// fetch all the events from the database, make sure they are
			// persisted correctly

			eventsForHost, err := Find(LegacyEventLogCollection, MostRecentHostEvents(hostId, hostTag, 50))
			So(err, ShouldBeNil)

			So(eventsForHost, ShouldHaveLength, 6)
			event := eventsForHost[5]
			So(event.EventType, ShouldEqual, EventHostCreated)

			eventData, ok := event.Data.(*HostEventData)
			So(ok, ShouldBeTrue)
			So(eventData.OldStatus, ShouldBeBlank)
			So(eventData.NewStatus, ShouldBeBlank)
			So(eventData.Logs, ShouldBeBlank)
			So(eventData.Hostname, ShouldBeBlank)
			So(eventData.TaskId, ShouldBeBlank)
			So(eventData.TaskPid, ShouldBeBlank)

			event = eventsForHost[4]
			So(event.EventType, ShouldEqual, EventHostStatusChanged)

			eventData, ok = event.Data.(*HostEventData)
			So(ok, ShouldBeTrue)
			So(eventData.OldStatus, ShouldEqual, evergreen.HostRunning)
			So(eventData.NewStatus, ShouldEqual, evergreen.HostTerminated)
			So(eventData.Logs, ShouldEqual, "myLogs")
			So(eventData.Hostname, ShouldBeBlank)
			So(eventData.TaskId, ShouldBeBlank)
			So(eventData.TaskPid, ShouldBeBlank)

			event = eventsForHost[3]
			So(event.EventType, ShouldEqual, EventHostDNSNameSet)

			eventData, ok = event.Data.(*HostEventData)
			So(ok, ShouldBeTrue)
			So(eventData.OldStatus, ShouldBeBlank)
			So(eventData.NewStatus, ShouldBeBlank)
			So(eventData.Logs, ShouldBeBlank)
			So(eventData.Hostname, ShouldEqual, hostname)
			So(eventData.TaskId, ShouldBeBlank)
			So(eventData.TaskPid, ShouldBeBlank)

			event = eventsForHost[2]
			So(event.EventType, ShouldEqual, EventHostProvisioned)

			eventData, ok = event.Data.(*HostEventData)
			So(ok, ShouldBeTrue)
			So(eventData.OldStatus, ShouldBeBlank)
			So(eventData.NewStatus, ShouldBeBlank)
			So(eventData.Logs, ShouldBeBlank)
			So(eventData.Hostname, ShouldBeBlank)
			So(eventData.TaskId, ShouldBeBlank)
			So(eventData.TaskPid, ShouldBeBlank)

			event = eventsForHost[1]
			So(event.EventType, ShouldEqual, EventHostRunningTaskSet)

			eventData, ok = event.Data.(*HostEventData)
			So(ok, ShouldBeTrue)
			So(eventData.OldStatus, ShouldBeBlank)
			So(eventData.NewStatus, ShouldBeBlank)
			So(eventData.Logs, ShouldBeBlank)
			So(eventData.Hostname, ShouldBeBlank)
			So(eventData.TaskId, ShouldEqual, taskId)
			So(eventData.TaskPid, ShouldBeBlank)

			event = eventsForHost[0]
			So(event.EventType, ShouldEqual, EventHostRunningTaskCleared)

			eventData, ok = event.Data.(*HostEventData)
			So(ok, ShouldBeTrue)
			So(eventData.OldStatus, ShouldBeBlank)
			So(eventData.NewStatus, ShouldBeBlank)
			So(eventData.Logs, ShouldBeBlank)
			So(eventData.Hostname, ShouldBeBlank)
			So(eventData.TaskId, ShouldEqual, taskId)
			So(eventData.TaskPid, ShouldBeBlank)

			// test logging multiple executions of the same task
			err = UpdateHostTaskExecutions(hostId, taskId, 0)
			So(err, ShouldBeNil)

			eventsForHost, err = Find(LegacyEventLogCollection, MostRecentHostEvents(hostId, "", 50))
			So(err, ShouldBeNil)
			So(len(eventsForHost), ShouldBeGreaterThan, 0)
			for _, event = range eventsForHost {
				So(event.ResourceType, ShouldEqual, ResourceTypeHost)
				eventData = event.Data.(*HostEventData)
				if eventData.TaskId != "" {
					So(eventData.Execution, ShouldEqual, "0")
				} else {
					So(eventData.Execution, ShouldEqual, "")
				}
			}
		})
	})
}
