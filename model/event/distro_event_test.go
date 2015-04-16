package event

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
	"time"
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(mci.TestConfig()))
}

func TestLoggingDistroEvents(t *testing.T) {
	Convey("When logging distro events, ", t, func() {

		So(db.Clear(Collection), ShouldBeNil)

		Convey("logged events should be stored and queryable in sorted order", func() {
			distroId := "distro_id"
			userId := "user_id"

			// log some events, sleeping in between to make sure the times are different
			LogDistroAdded(distroId, userId, nil)
			time.Sleep(1 * time.Millisecond)
			LogDistroModified(distroId, userId, "update")
			time.Sleep(1 * time.Millisecond)
			LogDistroRemoved(distroId, userId, nil)
			time.Sleep(1 * time.Millisecond)

			// fetch all the events from the database, make sure they are
			// persisted correctly

			eventsForDistro, err := Find(DistroEventsInOrder(distroId))
			So(err, ShouldBeNil)

			event := eventsForDistro[0]
			So(event.EventType, ShouldEqual, EventDistroAdded)
			So(event.ResourceId, ShouldEqual, distroId)

			eventData, ok := event.Data.Data.(*DistroEventData)
			So(ok, ShouldBeTrue)
			So(eventData.ResourceType, ShouldEqual, ResourceTypeDistro)
			So(eventData.UserId, ShouldEqual, userId)
			So(eventData.Data, ShouldBeNil)

			event = eventsForDistro[1]
			So(event.EventType, ShouldEqual, EventDistroModified)
			So(event.ResourceId, ShouldEqual, distroId)

			eventData, ok = event.Data.Data.(*DistroEventData)
			So(ok, ShouldBeTrue)
			So(eventData.ResourceType, ShouldEqual, ResourceTypeDistro)
			So(eventData.UserId, ShouldEqual, userId)
			So(eventData.Data.(string), ShouldEqual, "update")

			event = eventsForDistro[2]
			So(event.EventType, ShouldEqual, EventDistroRemoved)
			So(event.ResourceId, ShouldEqual, distroId)

			eventData, ok = event.Data.Data.(*DistroEventData)
			So(ok, ShouldBeTrue)
			So(eventData.ResourceType, ShouldEqual, ResourceTypeDistro)
			So(eventData.UserId, ShouldEqual, userId)
			So(eventData.Data, ShouldBeNil)
		})
	})
}
