package event

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

const ResourceTypeMockEvent = "mock_event"

func TestEventDrivenNotification(t *testing.T) {
	suite.Run(t, &subscriptionsSuite{})
}

type mockEvent struct {
	RType     string `bson:"r_type" json:"resource_type"`
	Something int    `bson:"something" json:"something"`
}

func (e *mockEvent) IsValid() bool {
	return e.RType == ResourceTypeMockEvent
}

type ednSuite struct {
	suite.Suite
}

func (s *ednSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())

	eventRegistry[ResourceTypeMockEvent] = func() Data {
		return &mockEvent{
			RType: ResourceTypeMockEvent,
		}
	}
}

func (s *ednSuite) TearDownSuite() {
	delete(eventRegistry, ResourceTypeMockEvent)
}

func (s *ednSuite) TearDownTest() {
	s.NoError(db.Clear(AllLogCollection))
}

func (s *ednSuite) TestThings() {
	// an event is logged
	logger := NewDBEventLogger(AllLogCollection)

	data := NewEventFromType(ResourceTypeMockEvent).(*mockEvent)
	data.Something = 5

	logger.LogEvent(EventLogEntry{
		Timestamp:  time.Now(),
		ResourceId: "something",
		EventType:  "mock",
		Data:       data,
	})
}
