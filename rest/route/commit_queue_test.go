package route

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/stretchr/testify/suite"
)

type CommitQueueSuite struct {
	sc *data.MockConnector

	suite.Suite
}

func TestCommitQueueSuite(t *testing.T) {
	suite.Run(t, new(CommitQueueSuite))
}

func (s *CommitQueueSuite) SetupTest() {
	s.sc = &data.MockConnector{
		MockCommitQueueConnector: data.MockCommitQueueConnector{},
	}
}

func (s *CommitQueueSuite) TestGetCommitQueue() {
	route := makeGetCommitQueueItems(s.sc).(commitQueueGetHandler)
	route.project = "evergreen-ci.evergreen.master"
	s.NoError(s.sc.GithubPREnqueueItem("evergreen-ci", "evergreen", 1))
	s.NoError(s.sc.GithubPREnqueueItem("evergreen-ci", "evergreen", 2))

	response := route.Run(context.Background())
	s.Equal(200, response.Status())
	s.Equal(&model.APICommitQueue{
		ProjectID: model.ToAPIString("evergreen-ci.evergreen.master"),
		Queue:     []model.APIString{model.ToAPIString("1"), model.ToAPIString("2")},
	}, response.Data())
}

func (s *CommitQueueSuite) TestDeleteItem() {
	route := makeDeleteCommitQueueItems(s.sc).(commitQueueDeleteItemHandler)
	s.NoError(s.sc.GithubPREnqueueItem("evergreen-ci", "evergreen", 1))
	s.NoError(s.sc.GithubPREnqueueItem("evergreen-ci", "evergreen", 2))
	route.project = "evergreen-ci.evergreen.master"
	route.item = "1"

	response := route.Run(context.Background())
	s.Equal(204, response.Status())
}
