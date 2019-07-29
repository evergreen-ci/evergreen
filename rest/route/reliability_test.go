package route

import (
        "context"
        "net/http"
        "net/url"
        "testing"

        "github.com/evergreen-ci/evergreen"
        "github.com/evergreen-ci/evergreen/rest/data"
        "github.com/stretchr/testify/suite"
        "gopkg.in/mgo.v2/bson"
)

type TaskReliabilitySuite struct {
        suite.Suite
}

func configureTaskReliability(disabled bool) error {
        flags := &evergreen.ServiceFlags{}
        env := evergreen.GetEnvironment()
        ctx, cancel := env.Context()
        defer cancel()
        _, err := env.DB().Collection(evergreen.ConfigCollection).UpdateOne(ctx,
                bson.M{"_id": flags.SectionId()},
                bson.M{"$set": bson.M{"task_reliability_disabled": disabled}})
        return err
}

func disableTaskReliability() {
        configureTaskReliability(true)
}

func enableTaskReliability() {
        configureTaskReliability(false)
}

func TestTaskReliabilitySuite(t *testing.T) {
        suite.Run(t, new(TaskReliabilitySuite))
}
func (s *TaskReliabilitySuite) SetupTest() {
        enableTaskReliability()
}

func (s *TaskReliabilitySuite) TestParseTaskReliabilityFilter() {
        values := url.Values{}
        handler := taskReliabilityHandler{}

        err := handler.parseTaskReliabilityFilter(values)
        s.Require().NoError(err)
}

func (s *TaskReliabilitySuite) TestRunTestHandler() {
        var err error
        sc := &data.MockConnector{
                MockStatsConnector: data.MockStatsConnector{},
                URL:                "https://example.net/test",
        }
        handler := makeGetProjectTaskReliability(sc).(*taskReliabilityHandler)
        s.Require().NoError(err)

        resp := handler.Run(context.Background())

        s.NotNil(resp)
        s.Equal(http.StatusOK, resp.Status())
        s.Nil(resp.Pages())
}

func (s *TaskReliabilitySuite) TestDisabledRunTestHandler() {
        var err error
        sc := &data.MockConnector{
                MockStatsConnector: data.MockStatsConnector{},
                URL:                "https://example.net/test",
        }

        disableTaskReliability()

        handler := makeGetProjectTaskReliability(sc).(*taskReliabilityHandler)
        s.Require().NoError(err)

        resp := handler.Run(context.Background())

        s.NotNil(resp)
        s.Equal(http.StatusServiceUnavailable, resp.Status())
        s.Nil(resp.Pages())
}
