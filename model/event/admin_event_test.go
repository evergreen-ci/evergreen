package event

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

var (
	testConfig = testutil.TestConfig()
)

type AdminEventSuite struct {
	suite.Suite
	u *user.DBUser
}

func TestAdminEventSuite(t *testing.T) {
	s := new(AdminEventSuite)
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
	s.u = &user.DBUser{Id: "user"}
	suite.Run(t, s)
}

func (s *AdminEventSuite) SetupTest() {
	err := db.Clear(AllLogCollection)
	s.Require().NoError(err)
}
