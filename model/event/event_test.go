package event

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

type eventSuite struct {
	suite.Suite
}

func TestEventSuite(t *testing.T) {
	suite.Run(t, &eventSuite{})
}

func (s *eventSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func (s *eventSuite) SetupTest() {
	s.NoError(db.ClearCollections(AllLogCollection))
}

func (s *eventSuite) TestTerribleUnmarshaller() {
	impls := []Data{
		&TaskEventData{ResourceType: ResourceTypeTask},
		&HostEventData{ResourceType: ResourceTypeHost},
		&DistroEventData{ResourceType: ResourceTypeDistro},
		&SchedulerEventData{ResourceType: ResourceTypeScheduler},
		&TaskSystemResourceData{ResourceType: EventTaskSystemInfo},
		&TaskProcessResourceData{ResourceType: EventTaskProcessInfo},
		&rawAdminEventData{
			ResourceType: ResourceTypeAdmin,
			Changes: rawConfigDataChange{
				// sad violin is sad. 0x0A is a BSON null
				Before: bson.Raw{
					Kind: 0x0A,
				},
				After: bson.Raw{
					Kind: 0x0A,
				},
			},
		},
	}

	logger := NewDBEventLogger(AllLogCollection)
	for _, t := range impls {
		event := Event{
			Timestamp: time.Now().Round(time.Millisecond).Truncate(time.Millisecond),
			Data:      DataWrapper{t},
		}

		s.NoError(logger.LogEvent(event))
		fetchedEvents, err := Find(AllLogCollection, db.Query(bson.M{}))
		s.NoError(err)
		s.Require().Len(fetchedEvents, 1)
		s.True(fetchedEvents[0].Data.IsValid())
		s.IsType(t, fetchedEvents[0].Data.Data)
		s.NoError(db.ClearCollections(AllLogCollection))
	}
}
