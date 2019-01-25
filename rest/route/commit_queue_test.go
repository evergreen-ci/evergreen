package route

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/stretchr/testify/suite"
)

type CommitQueueSuite struct {
	route *commitQueueGetHandler
	sc    *data.MockConnector

	suite.Suite
}

func TestCommitQueueSuite(t *testing.T) {
	suite.Run(t, new(CommitQueueSuite))
}

func (s *CommitQueueSuite) SetupTest() {
	s.sc = &data.MockConnector{
		MockCommitQueueConnector: data.MockCommitQueueConnector{},
	}
	route := makeGetCommitQueueItems(s.sc).(commitQueueGetHandler)
	s.route = &route
}

func (s *CommitQueueSuite) TestGetCommitQueue() {
	s.route.project = "evergreen-ci.evergreen.master"
	s.NoError(s.sc.GithubPREnqueueItem("evergreen-ci", "evergreen", 1))
	s.NoError(s.sc.GithubPREnqueueItem("evergreen-ci", "evergreen", 2))

	response := s.route.Run(context.Background())
	s.Equal(200, response.Status())
	s.Equal(model.APICommitQueue{
		ProjectID: model.ToAPIString("evergreen-ci.evergreen.master"),
		Queue:     []model.APIString{model.ToAPIString("1"), model.ToAPIString("2")},
	}, response.Data())
}
