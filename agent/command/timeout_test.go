package command

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimeoutUpdateParseParams(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T){
		"SetsTimeoutSecsFromIntParams": func(t *testing.T) {
			c := &timeout{}
			require.NoError(t, c.ParseParams(map[string]any{
				"timeout_secs":      100,
				"exec_timeout_secs": 200,
			}))
			conf := makeTimeoutTestTaskConfig()
			comm := client.NewMock("localhost")
			logger, err := comm.GetLoggerProducer(t.Context(), &conf.Task, nil)
			require.NoError(t, err)
			require.NoError(t, c.Execute(t.Context(), comm, logger, conf))
			assert.Equal(t, 100, conf.GetIdleTimeout())
			assert.Equal(t, 200, conf.GetExecTimeout())
		},
		"SetsTimeoutSecsFromStringParams": func(t *testing.T) {
			c := &timeout{}
			require.NoError(t, c.ParseParams(map[string]any{
				"timeout_secs":      "100",
				"exec_timeout_secs": "200",
			}))
			conf := makeTimeoutTestTaskConfig()
			comm := client.NewMock("localhost")
			logger, err := comm.GetLoggerProducer(t.Context(), &conf.Task, nil)
			require.NoError(t, err)
			require.NoError(t, c.Execute(t.Context(), comm, logger, conf))
			assert.Equal(t, 100, conf.GetIdleTimeout())
			assert.Equal(t, 200, conf.GetExecTimeout())
		},
		"FailsWithBothNonNumericStringParams": func(t *testing.T) {
			c := &timeout{}
			require.NoError(t, c.ParseParams(map[string]any{
				"timeout_secs":      "not-a-number",
				"exec_timeout_secs": "also-not-a-number",
			}))
			conf := makeTimeoutTestTaskConfig()
			comm := client.NewMock("localhost")
			logger, err := comm.GetLoggerProducer(t.Context(), &conf.Task, nil)
			require.NoError(t, err)
			assert.Error(t, c.Execute(t.Context(), comm, logger, conf))
		},
	} {
		t.Run(tName, tCase)
	}
}

func TestTimeoutUpdateExecute(t *testing.T) {
	highExecTimeoutSecs := int(evergreen.HighExecTimeoutThreshold.Seconds()) + 1

	for tName, tCase := range map[string]func(t *testing.T, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig){
		"SetsIdleTimeout": func(t *testing.T, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			c := &timeout{TimeoutSecs: 100}
			require.NoError(t, c.Execute(t.Context(), comm, logger, conf))
			assert.Equal(t, 100, conf.GetIdleTimeout())
			assert.Zero(t, conf.GetExecTimeout())
			assert.Zero(t, comm.ReportedHighExecTimeoutSecs)
		},
		"SetsExecTimeout": func(t *testing.T, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			c := &timeout{ExecTimeoutSecs: 100}
			require.NoError(t, c.Execute(t.Context(), comm, logger, conf))
			assert.Equal(t, 100, conf.GetExecTimeout())
			assert.Zero(t, conf.GetIdleTimeout())
			assert.Zero(t, comm.ReportedHighExecTimeoutSecs)
		},
		"SetsHighExecTimeoutAndReports": func(t *testing.T, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			c := &timeout{ExecTimeoutSecs: highExecTimeoutSecs}
			require.NoError(t, c.Execute(t.Context(), comm, logger, conf))
			assert.Equal(t, highExecTimeoutSecs, comm.ReportedHighExecTimeoutSecs)
			assert.Equal(t, highExecTimeoutSecs, conf.GetExecTimeout())
		},
		"CapsExecTimeoutAtMaxLimitAndReportsHighExecTimeout": func(t *testing.T, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			conf.MaxExecTimeoutSecs = 1e9
			c := &timeout{ExecTimeoutSecs: 1e10}
			require.NoError(t, c.Execute(t.Context(), comm, logger, conf))
			assert.Equal(t, c.ExecTimeoutSecs, comm.ReportedHighExecTimeoutSecs)
			assert.Equal(t, conf.MaxExecTimeoutSecs, conf.GetExecTimeout())
		},
		"HighExecTimeoutStillSetIfReportingFails": func(t *testing.T, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			comm.ReportHighExecTimeoutShouldFail = true
			c := &timeout{ExecTimeoutSecs: highExecTimeoutSecs}
			require.NoError(t, c.Execute(t.Context(), comm, logger, conf))
			assert.Equal(t, highExecTimeoutSecs, conf.GetExecTimeout())
		},
	} {
		t.Run(tName, func(t *testing.T) {
			conf := makeTimeoutTestTaskConfig()
			comm := client.NewMock("localhost")
			logger, err := comm.GetLoggerProducer(t.Context(), &conf.Task, nil)
			require.NoError(t, err)

			tCase(t, comm, logger, conf)
		})
	}
}

func makeTimeoutTestTaskConfig() *internal.TaskConfig {
	return &internal.TaskConfig{
		Task: task.Task{
			Id:     "task_id",
			Secret: "task_secret",
		},
		Expansions: util.Expansions{},
	}
}
