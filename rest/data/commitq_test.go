package data

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitq"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

type CommitQSuite struct {
	ctx Connector
	suite.Suite
}

func TestCommitQSuite(t *testing.T) {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())

	s := &CommitQSuite{
		ctx: &DBConnector{},
	}

	suite.Run(t, s)
}

func (s *CommitQSuite) SetupTest() {
	s.Require().NoError(db.Clear(commitq.Collection))
	s.Require().NoError(db.Clear(model.ProjectRefCollection))
	projRef := model.ProjectRef{Identifier: "mci"}
	s.Require().NoError(projRef.Insert())
}

func (s *CommitQSuite) TestEnqueue() {
	s.NoError(s.ctx.EnqueueItem("mci", "item1"))
}
