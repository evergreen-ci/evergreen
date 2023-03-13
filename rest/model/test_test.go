package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestBuildFromService(t *testing.T) {
	env := evergreen.GetEnvironment()
	start := time.Now().Add(-10 * time.Minute)
	end := time.Now()

	for _, test := range []struct {
		name   string
		io     func() (interface{}, *APITest)
		hasErr bool
	}{
		{
			name: "NoOptionals",
			io: func() (interface{}, *APITest) {
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
						URLLobster: utility.ToStringPtr(input.GetLogURL(env, evergreen.LogViewerLobster)),
						URLParsley: utility.ToStringPtr(input.GetLogURL(env, evergreen.LogViewerParsley)),
						LineNum:    15,
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
			io: func() (interface{}, *APITest) {
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
				}

				output := &APITest{
					ID:        utility.ToStringPtr(input.TestName),
					Execution: input.Execution,
					TestFile:  utility.ToStringPtr(input.DisplayTestName),
					GroupID:   utility.ToStringPtr(input.GroupID),
					Status:    utility.ToStringPtr(input.Status),
					Logs: TestLogs{
						URL:        utility.ToStringPtr(input.GetLogURL(env, evergreen.LogViewerHTML)),
						URLRaw:     utility.ToStringPtr(input.GetLogURL(env, evergreen.LogViewerRaw)),
						URLLobster: utility.ToStringPtr(input.GetLogURL(env, evergreen.LogViewerLobster)),
						URLParsley: utility.ToStringPtr(input.GetLogURL(env, evergreen.LogViewerParsley)),
						LineNum:    15,
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
			io: func() (interface{}, *APITest) {
				output := &APITest{
					TaskID: utility.ToStringPtr("task"),
				}

				return "task", output
			},
		},
		{
			name: "InvalidType",
			io: func() (interface{}, *APITest) {
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
