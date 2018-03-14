package triggers

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

type triggersSuite struct {
	suite.Suite
}

func TestTriggers(t *testing.T) {
	suite.Run(t, &triggersSuite{})
}

func (s *triggersSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func (s *triggersSuite) TestProcess() {

}
