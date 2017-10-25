package units

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/mock"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/suite"
)

type StatUnitsSuite struct {
	sender *send.InternalSender
	env    *mock.Environment
	cancel context.CancelFunc
	suite.Suite
}

func TestStatUnitsSuite(t *testing.T) {
	suite.Run(t, new(StatUnitsSuite))
}

func (s *StatUnitsSuite) SetupTest() {
	s.sender = send.MakeInternalLogger()
	s.env = &mock.Environment{}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.Require().NoError(s.env.Configure(ctx, ""))
}

func (s *StatUnitsSuite) TestAmboyStatsCollector() {
	j := makeAmboyStatsCollector()
	j.env = nil
	s.False(j.Status().Completed)
	j.Run()
	s.True(j.Status().Completed)
	s.True(j.HasErrors())

	j = makeAmboyStatsCollector()
	s.False(j.Status().Completed)
	j.env = s.env
	j.logger = logging.MakeGrip(s.sender)
	s.False(s.sender.HasMessage())

	j.Run()
	s.False(s.sender.HasMessage())
	s.True(j.Status().Completed)

}
