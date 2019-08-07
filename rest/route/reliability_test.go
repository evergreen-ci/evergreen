package route

import (
        "context"
        "net/http"
        "testing"

        "github.com/evergreen-ci/evergreen"
        "github.com/evergreen-ci/evergreen/rest/data"
        "github.com/stretchr/testify/assert"
)

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

func setupTest(t *testing.T) error {
        return enableTaskReliability()
}

func TestRunTestHandler(t *testing.T) {
        assert := assert.New(t)
        err := setupTest(t)
        assert.NoError(err)

        sc := &data.MockConnector{
                MockStatsConnector: data.MockStatsConnector{},
                URL:                "https://example.net/test",
        }
        handler := makeGetProjectTaskReliability(sc).(*taskReliabilityHandler)
        assert.NoError(err)

        resp := handler.Run(context.Background())

        assert.NotNil(resp)
        assert.Equal(http.StatusOK, resp.Status())
        assert.Nil(resp.Pages())
}

func TestDisabledRunTestHandler(t *testing.T) {
        assert := assert.New(t)
        err := setupTest(t)
        assert.NoError(err)
        sc := &data.MockConnector{
                MockStatsConnector: data.MockStatsConnector{},
                URL:                "https://example.net/test",
        }

        err = disableTaskReliability()
        assert.NoError(err)

        handler := makeGetProjectTaskReliability(sc).(*taskReliabilityHandler)

        resp := handler.Run(context.Background())

        assert.NotNil(resp)
        assert.Equal(http.StatusServiceUnavailable, resp.Status())
        assert.Nil(resp.Pages())
}
