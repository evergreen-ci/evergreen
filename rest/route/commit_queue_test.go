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
	s.NoError(s.sc.EnqueueItem("evergreen-ci", "evergreen", "master", "1"))
	s.NoError(s.sc.EnqueueItem("evergreen-ci", "evergreen", "master", "2"))

	response := route.Run(context.Background())
	s.Equal(200, response.Status())
	s.Equal(&model.APICommitQueue{
		ProjectID: model.ToAPIString("evergreen-ci.evergreen.master"),
		Queue:     []model.APIString{model.ToAPIString("1"), model.ToAPIString("2")},
	}, response.Data())
}

func (s *CommitQueueSuite) TestDeleteItem() {
	route := makeDeleteCommitQueueItems(s.sc).(commitQueueDeleteItemHandler)
	s.NoError(s.sc.EnqueueItem("evergreen-ci", "evergreen", "master", "1"))
	s.NoError(s.sc.EnqueueItem("evergreen-ci", "evergreen", "master", "2"))
	route.project = "evergreen-ci.evergreen.master"

	// Valid delete
	route.item = "1"
	response := route.Run(context.Background())
	s.Equal(204, response.Status())

	// Already deleted
	response = route.Run(context.Background())
	s.Equal(404, response.Status())

	// Invalid project
	route.project = "not_here"
	route.item = "2"
	response = route.Run(context.Background())
	s.Equal(404, response.Status())
}

func (s *CommitQueueSuite) TestClearAll() {
	route := makeClearCommitQueuesHandler(s.sc).(*commitQueueClearAllHandler)
	s.NoError(s.sc.EnqueueItem("evergreen-ci", "evergreen", "master", "12"))
	s.NoError(s.sc.EnqueueItem("evergreen-ci", "evergreen", "master", "23"))
	s.NoError(s.sc.EnqueueItem("evergreen-ci", "logkeeper", "master", "45"))
	s.NoError(s.sc.EnqueueItem("evergreen-ci", "logkeeper", "master", "56"))

	response := route.Run(context.Background())
	s.Equal(200, response.Status())
	s.Equal(struct {
		clearedCount int `json:"cleared_count"`
	}{2}, response.Data())

	response = route.Run(context.Background())
	s.Equal(200, response.Status())
	s.Equal(struct {
		clearedCount int `json:"cleared_count"`
	}{0}, response.Data())
}
