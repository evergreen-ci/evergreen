package event

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

type notificationEventSuite struct {
	suite.Suite
}

func TestNotificationsEventSuite(t *testing.T) {
	suite.Run(t, &notificationEventSuite{})
}

func (s *notificationEventSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func (s *notificationEventSuite) SetupTest() {
	s.NoError(db.ClearCollections(AllLogCollection))
}

func (s *notificationEventSuite) TestSubscriberUnmarshaller() {
}
