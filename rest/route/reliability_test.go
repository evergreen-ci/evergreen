package route

import (
        "context"
        "net/http"
        "testing"

        "github.com/evergreen-ci/evergreen"
        "github.com/evergreen-ci/evergreen/rest/data"
        "github.com/stretchr/testify/suite"
)

type TaskReliabilitySuite struct {
        suite.Suite
}

func configureTaskReliability(disabled bool) error {
        var err error
        flags := &evergreen.ServiceFlags{}
        err = flags.Get(evergreen.GetEnvironment())
        if err == nil {
                flags.TaskReliabilityDisabled = disabled
                err = flags.Set()
        }
        return err
}

func disableTaskReliability() error {
        return configureTaskReliability(true)
}

func enableTaskReliability() error {
        return configureTaskReliability(false)
}

func TestTaskReliabilitySuite(t *testing.T) {
        suite.Run(t, new(TaskReliabilitySuite))
}

func (s *TaskReliabilitySuite) SetupTest() {
        err := enableTaskReliability()
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

        err = disableTaskReliability()
        s.Require().NoError(err)

        handler := makeGetProjectTaskReliability(sc).(*taskReliabilityHandler)
        s.Require().NoError(err)

        resp := handler.Run(context.Background())

        s.NotNil(resp)
        s.Equal(http.StatusServiceUnavailable, resp.Status())
        s.Nil(resp.Pages())
}
