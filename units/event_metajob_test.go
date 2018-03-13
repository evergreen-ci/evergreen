package units

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

type eventMetaJobSuite struct {
	suite.Suite
}

func TestEventMetaJob(t *testing.T) {
	suite.Run(t, &eventMetaJobSuite{})
}

func (s *eventMetaJobSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func (s *eventMetaJobSuite) SetupTest() {
	s.NoError(db.ClearCollections(event.AllLogCollection))

	events := []event.EventLogEntry{
		{
			ResourceType: event.ResourceTypeHost,
			Data: &event.HostEventData{
				ResourceType: event.ResourceTypeHost,
			},
		},
		{
			ProcessedAt:  time.Now(),
			ResourceType: event.ResourceTypeHost,
			Data: &event.HostEventData{
				ResourceType: event.ResourceTypeHost,
			},
		},
	}

	logger := event.NewDBEventLogger(event.AllLogCollection)
	for i := range events {
		s.NoError(logger.LogEvent(&events[i]))
	}
}

func (s *eventMetaJobSuite) TestJob() {
}
