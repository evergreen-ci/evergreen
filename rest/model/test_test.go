package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestBuildFromService(t *testing.T) {
	start := time.Unix(12345, 6789)
	end := time.Unix(9876, 54321)

	for _, test := range []struct {
		name   string
		io     func() (interface{}, *APITest)
		hasErr bool
	}{
		{
			name: "EvergreenTestResultNoOptionals",
			io: func() (interface{}, *APITest) {
				input := &testresult.TestResult{
					TaskID:    "task",
					Execution: 1,
					TestFile:  "test_file",
					Status:    "test_status",
					LineNum:   15,
					ExitCode:  1,
					StartTime: utility.ToPythonTime(start),
					EndTime:   utility.ToPythonTime(end),
				}
				otr := task.ConvertToOld(input)

				output := &APITest{
					ID:        utility.ToStringPtr(input.ID.Hex()),
					Execution: input.Execution,
					Status:    utility.ToStringPtr(input.Status),
					TestFile:  utility.ToStringPtr(input.TestFile),
					Logs: TestLogs{
						URL:        utility.ToStringPtr(otr.GetLogURL(evergreen.LogViewerHTML)),
						URLRaw:     utility.ToStringPtr(otr.GetLogURL(evergreen.LogViewerRaw)),
						URLLobster: utility.ToStringPtr(otr.GetLogURL(evergreen.LogViewerLobster)),
						LineNum:    15,
					},
					ExitCode:  1,
					StartTime: utility.ToTimePtr(start),
					EndTime:   utility.ToTimePtr(end),
					Duration:  input.EndTime - input.StartTime,
				}

				return input, output
			},
		},
		{
			name: "EvergreenTestResultOptionals",
			io: func() (interface{}, *APITest) {
				input := &testresult.TestResult{
					TaskID:          "task",
					Execution:       1,
					TestFile:        "test_file",
					DisplayTestName: "display",
					GroupID:         "group",
					Status:          "test_status",
					LogID:           "id",
					LineNum:         15,
					ExitCode:        1,
					StartTime:       utility.ToPythonTime(start),
					EndTime:         utility.ToPythonTime(end),
				}
				otr := task.ConvertToOld(input)

				output := &APITest{
					ID:        utility.ToStringPtr(input.ID.Hex()),
					Execution: input.Execution,
					TestFile:  utility.ToStringPtr(input.DisplayTestName),
					GroupID:   utility.ToStringPtr(input.GroupID),
					Status:    utility.ToStringPtr(input.Status),
					Logs: TestLogs{
						URL:        utility.ToStringPtr(otr.GetLogURL(evergreen.LogViewerHTML)),
						URLRaw:     utility.ToStringPtr(otr.GetLogURL(evergreen.LogViewerRaw)),
						URLLobster: nil,
						LineNum:    15,
						LogID:      utility.ToStringPtr(input.LogID),
					},
					ExitCode:  1,
					StartTime: utility.ToTimePtr(start),
					EndTime:   utility.ToTimePtr(end),
					Duration:  input.EndTime - input.StartTime,
				}

				return input, output
			},
		},
		{
			name: "CedarTestResultNoOptionals",
			io: func() (interface{}, *APITest) {
				input := &apimodels.CedarTestResult{
					TaskID:    "task",
					Execution: 1,
					TestName:  "test",
					Status:    "status",
					LineNum:   15,
					Start:     start,
					End:       end,
				}
				otr := task.ConvertCedarTestResult(*input)

				output := &APITest{
					ID:        utility.ToStringPtr(input.TestName),
					Execution: input.Execution,
					TestFile:  utility.ToStringPtr(input.TestName),
					Status:    utility.ToStringPtr(input.Status),
					Logs: TestLogs{
						URL:        utility.ToStringPtr(otr.GetLogURL(evergreen.LogViewerHTML)),
						URLRaw:     utility.ToStringPtr(otr.GetLogURL(evergreen.LogViewerRaw)),
						URLLobster: utility.ToStringPtr(otr.GetLogURL(evergreen.LogViewerLobster)),
						LineNum:    15,
					},
					StartTime: utility.ToTimePtr(start),
					EndTime:   utility.ToTimePtr(end),
					Duration:  end.Sub(start).Seconds(),
				}

				return input, output
			},
		},
		{
			name: "CedarTestResultOptionals",
			io: func() (interface{}, *APITest) {
				input := &apimodels.CedarTestResult{
					TaskID:          "task",
					Execution:       1,
					TestName:        "test",
					DisplayTestName: "display",
					GroupID:         "group",
					Status:          "status",
					BaseStatus:      "base_status",
					LogTestName:     "log",
					LineNum:         15,
					Start:           start,
					End:             end,
				}
				otr := task.ConvertCedarTestResult(*input)

				output := &APITest{
					ID:         utility.ToStringPtr(input.TestName),
					Execution:  input.Execution,
					TestFile:   utility.ToStringPtr(input.DisplayTestName),
					GroupID:    utility.ToStringPtr(input.GroupID),
					Status:     utility.ToStringPtr(input.Status),
					BaseStatus: utility.ToStringPtr(input.BaseStatus),
					Logs: TestLogs{
						URL:        utility.ToStringPtr(otr.GetLogURL(evergreen.LogViewerHTML)),
						URLRaw:     utility.ToStringPtr(otr.GetLogURL(evergreen.LogViewerRaw)),
						URLLobster: utility.ToStringPtr(otr.GetLogURL(evergreen.LogViewerLobster)),
						LineNum:    15,
					},
					StartTime: utility.ToTimePtr(start),
					EndTime:   utility.ToTimePtr(end),
					Duration:  end.Sub(start).Seconds(),
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
				return &task.TestResult{}, &APITest{}
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
