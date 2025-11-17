package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestBuildFromService(t *testing.T) {
	ctx := t.Context()

	settings := testutil.TestConfig()
	require.NoError(t, db.Clear(evergreen.ConfigCollection))
	settings.Ui.ParsleyUrl = "https://parsley.mongodb.org"
	require.NoError(t, evergreen.UpdateConfig(ctx, settings))

	env := evergreen.GetEnvironment()
	start := time.Now().Add(-10 * time.Minute)
	end := time.Now()

	for _, test := range []struct {
		name   string
		io     func() (any, *APITest)
		hasErr bool
	}{
		{
			name: "NoOptionals",
			io: func() (any, *APITest) {
				input := &testresult.TestResult{
					TaskID:        "task",
					Execution:     1,
					TestName:      "test_file",
					Status:        "test_status",
					LineNum:       15,
					TestStartTime: start,
					TestEndTime:   end,
				}

				output := &APITest{
					ID:        utility.ToStringPtr(input.TestName),
					Execution: input.Execution,
					Status:    utility.ToStringPtr(input.Status),
					TestFile:  utility.ToStringPtr(input.TestName),
					Logs: TestLogs{
						URL:        utility.ToStringPtr(input.GetLogURL(env, evergreen.LogViewerHTML)),
						URLRaw:     utility.ToStringPtr(input.GetLogURL(env, evergreen.LogViewerRaw)),
						URLParsley: utility.ToStringPtr(input.GetLogURL(env, evergreen.LogViewerParsley)),
						LineNum:    input.LineNum,
						TestName:   utility.ToStringPtr(input.GetLogTestName()),
					},
					StartTime: utility.ToTimePtr(start),
					EndTime:   utility.ToTimePtr(end),
					Duration:  input.Duration().Seconds(),
				}

				return input, output
			},
		},
		{
			name: "Optionals",
			io: func() (any, *APITest) {
				input := &testresult.TestResult{
					TaskID:          "task",
					Execution:       1,
					TestName:        "test_file",
					DisplayTestName: "display",
					GroupID:         "group",
					Status:          "test_status",
					LineNum:         15,
					TestStartTime:   start,
					TestEndTime:     end,
					LogInfo: &testresult.TestLogInfo{
						LineNum:       20,
						RenderingType: utility.ToStringPtr("resmoke"),
						Version:       1,
					},
				}

				output := &APITest{
					ID:        utility.ToStringPtr(input.TestName),
					Execution: input.Execution,
					TestFile:  utility.ToStringPtr(input.DisplayTestName),
					GroupID:   utility.ToStringPtr(input.GroupID),
					Status:    utility.ToStringPtr(input.Status),
					Logs: TestLogs{
						URL:           utility.ToStringPtr(input.GetLogURL(env, evergreen.LogViewerHTML)),
						URLRaw:        utility.ToStringPtr(input.GetLogURL(env, evergreen.LogViewerRaw)),
						URLParsley:    utility.ToStringPtr(input.GetLogURL(env, evergreen.LogViewerParsley)),
						LineNum:       int(input.LogInfo.LineNum),
						TestName:      utility.ToStringPtr(input.GetLogTestName()),
						RenderingType: input.LogInfo.RenderingType,
						Version:       input.LogInfo.Version,
					},
					StartTime: utility.ToTimePtr(start),
					EndTime:   utility.ToTimePtr(end),
					Duration:  input.Duration().Seconds(),
				}

				return input, output
			},
		},
		{
			name: "TaskID",
			io: func() (any, *APITest) {
				output := &APITest{
					TaskID: utility.ToStringPtr("task"),
				}

				return "task", output
			},
		},
		{
			name: "WithLogInfoLogName",
			io: func() (any, *APITest) {
				input := &testresult.TestResult{
					TaskID:        "task",
					Execution:     1,
					TestName:      "test_file",
					Status:        "test_status",
					LineNum:       15,
					TestStartTime: start,
					TestEndTime:   end,
					LogInfo: &testresult.TestLogInfo{
						LogName:       "log_name_from_loginfo",
						LineNum:       20,
						RenderingType: utility.ToStringPtr("resmoke"),
						Version:       1,
					},
				}

				output := &APITest{
					ID:        utility.ToStringPtr(input.TestName),
					Execution: input.Execution,
					Status:    utility.ToStringPtr(input.Status),
					TestFile:  utility.ToStringPtr(input.TestName),
					Logs: TestLogs{
						URL:           utility.ToStringPtr(input.GetLogURL(env, evergreen.LogViewerHTML)),
						URLRaw:        utility.ToStringPtr(input.GetLogURL(env, evergreen.LogViewerRaw)),
						URLParsley:    utility.ToStringPtr(input.GetLogURL(env, evergreen.LogViewerParsley)),
						LineNum:       int(input.LogInfo.LineNum),
						TestName:      utility.ToStringPtr(input.GetLogTestName()),
						RenderingType: input.LogInfo.RenderingType,
						Version:       input.LogInfo.Version,
					},
					StartTime: utility.ToTimePtr(start),
					EndTime:   utility.ToTimePtr(end),
					Duration:  input.Duration().Seconds(),
				}

				return input, output
			},
		},
		{
			name: "WithLogTestName",
			io: func() (any, *APITest) {
				input := &testresult.TestResult{
					TaskID:        "task",
					Execution:     1,
					TestName:      "test_file",
					LogTestName:   "log_test_name",
					Status:        "test_status",
					LineNum:       15,
					TestStartTime: start,
					TestEndTime:   end,
				}

				output := &APITest{
					ID:        utility.ToStringPtr(input.TestName),
					Execution: input.Execution,
					Status:    utility.ToStringPtr(input.Status),
					TestFile:  utility.ToStringPtr(input.TestName),
					Logs: TestLogs{
						URL:        utility.ToStringPtr(input.GetLogURL(env, evergreen.LogViewerHTML)),
						URLRaw:     utility.ToStringPtr(input.GetLogURL(env, evergreen.LogViewerRaw)),
						URLParsley: utility.ToStringPtr(input.GetLogURL(env, evergreen.LogViewerParsley)),
						LineNum:    input.LineNum,
						TestName:   utility.ToStringPtr(input.GetLogTestName()),
					},
					StartTime: utility.ToTimePtr(start),
					EndTime:   utility.ToTimePtr(end),
					Duration:  input.Duration().Seconds(),
				}

				return input, output
			},
		},
		{
			name: "InvalidType",
			io: func() (any, *APITest) {
				return &task.Task{}, &APITest{}
			},
			hasErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			at := &APITest{}
			input, output := test.io()
			err := at.BuildFromService(input)

			if test.hasErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, output, at)
			}
		})
	}
}
