package route

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/taskoutput"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTaskOutputLogsBaseHandlerParse(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{
		Settings: user.UserSettings{
			Timezone: "UTC",
		},
	})
	env := evergreen.GetEnvironment()

	require.NoError(t, env.DB().Drop(ctx))
	defer func() {
		assert.NoError(t, env.DB().Drop(ctx))
	}()

	task0 := &task.Task{
		Id:        "task_0",
		OldTaskId: "task",
	}
	_, err := env.DB().Collection(task.OldCollection).InsertOne(ctx, task0)
	require.NoError(t, err)
	task1 := &task.Task{
		Id:        "task",
		Execution: 1,
	}
	_, err = env.DB().Collection(task.Collection).InsertOne(ctx, task1)
	require.NoError(t, err)

	for _, test := range []struct {
		name     string
		taskID   string
		urlQuery string
		expected *getTaskOutputLogsBaseHandler
		hasErr   bool
		errCode  int
	}{
		{
			name:     "InvalidExecution",
			taskID:   "task",
			urlQuery: "execution=NaN",
			hasErr:   true,
			errCode:  400,
		},
		{
			name:    "TaskDNE",
			taskID:  "DNE",
			hasErr:  true,
			errCode: 404,
		},
		{
			name:     "InvalidStart",
			taskID:   "task",
			urlQuery: "start=11-16-2023",
			hasErr:   true,
			errCode:  400,
		},
		{
			name:     "InvalidEnd",
			taskID:   "task",
			urlQuery: "end=11-16-2023",
			hasErr:   true,
			errCode:  400,
		},
		{
			name:     "InvalidLineLimit",
			taskID:   "task",
			urlQuery: "line_limit=NaN",
			hasErr:   true,
			errCode:  400,
		},
		{
			name:     "InvalidTailLimit",
			taskID:   "task",
			urlQuery: "tail_limit=NaN",
			hasErr:   true,
			errCode:  400,
		},
		{
			name:     "LineLimitAndTailLimitSet",
			taskID:   "task",
			urlQuery: "line_limit=100&tail_limit=100",
			hasErr:   true,
			errCode:  400,
		},
		{
			name:     "LineLimitAndPaginateSet",
			taskID:   "task",
			urlQuery: "line_limit=99&paginate=true",
			hasErr:   true,
			errCode:  400,
		},
		{
			name:     "TailLimitAndPaginateSet",
			taskID:   "task",
			urlQuery: "tail_limit=100&paginate=true",
			hasErr:   true,
			errCode:  400,
		},
		{
			name:     "PaginateTailLimitAndLineLimitSet",
			taskID:   "task",
			urlQuery: "line_limit=100&tail_limit=100&paginate=true",
			hasErr:   true,
			errCode:  400,
		},
		{
			name:     "DefaultParameters",
			taskID:   "task",
			expected: &getTaskOutputLogsBaseHandler{tsk: task1},
		},
		{
			name:     "ValidParametersWithLineLimit",
			taskID:   "task",
			urlQuery: "execution=0&start=2023-11-16T07:20:50.00Z&end=2023-11-16T08:20:50Z&line_limit=100&print_time=True&print_priority=True",
			expected: &getTaskOutputLogsBaseHandler{
				tsk:           task0,
				start:         utility.ToInt64Ptr(time.Date(2023, time.November, 16, 7, 20, 50, 0, time.UTC).UnixNano()),
				end:           utility.ToInt64Ptr(time.Date(2023, time.November, 16, 8, 20, 50, 0, time.UTC).UnixNano()),
				lineLimit:     100,
				printTime:     true,
				printPriority: true,
			},
		},
		{
			name:     "ValidParametersWithTailLimit",
			taskID:   "task",
			urlQuery: "execution=0&start=2023-11-16T07:20:50.00Z&end=2023-11-16T08:20:50Z&tail_limit=100&print_time=True&print_priority=True",
			expected: &getTaskOutputLogsBaseHandler{
				tsk:           task0,
				start:         utility.ToInt64Ptr(time.Date(2023, time.November, 16, 7, 20, 50, 0, time.UTC).UnixNano()),
				end:           utility.ToInt64Ptr(time.Date(2023, time.November, 16, 8, 20, 50, 0, time.UTC).UnixNano()),
				tailN:         100,
				printTime:     true,
				printPriority: true,
			},
		},
		{
			name:     "ValidParametersWithPaginate",
			taskID:   "task",
			urlQuery: "execution=0&start=2023-11-16T07:20:50.00Z&end=2023-11-16T08:20:50Z&paginate=true&print_time=True&print_priority=True",
			expected: &getTaskOutputLogsBaseHandler{
				tsk:           task0,
				start:         utility.ToInt64Ptr(time.Date(2023, time.November, 16, 7, 20, 50, 0, time.UTC).UnixNano()),
				end:           utility.ToInt64Ptr(time.Date(2023, time.November, 16, 8, 20, 50, 0, time.UTC).UnixNano()),
				printTime:     true,
				printPriority: true,
				paginate:      true,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			url, err := url.Parse(fmt.Sprintf("https://evergreen.mongodb.com/rest/v2/tasks/%s/build/task_logs?%s", test.taskID, test.urlQuery))
			require.NoError(t, err)
			req := &http.Request{Method: "GET"}
			req.URL = url
			req = gimlet.SetURLVars(req, map[string]string{"task_id": test.taskID})

			rh := &getTaskOutputLogsBaseHandler{}
			err = rh.parse(ctx, req)
			if test.hasErr {
				require.Error(t, err)
				errResp, ok := err.(gimlet.ErrorResponse)
				if test.errCode == 400 {
					// Should return a regular error and
					// let the wrapping gimlet handler func
					// set the default 400 HTTP error
					// status.
					assert.False(t, ok)
				} else {
					require.True(t, ok)
					assert.Equal(t, test.errCode, errResp.StatusCode)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.expected.tsk.Id, rh.tsk.Id)
				assert.Equal(t, test.expected.tsk.Execution, rh.tsk.Execution)
				assert.Equal(t, test.expected.start, rh.start)
				assert.Equal(t, test.expected.end, rh.end)
				assert.Equal(t, test.expected.lineLimit, rh.lineLimit)
				assert.Equal(t, test.expected.tailN, rh.tailN)
				assert.Equal(t, test.expected.printTime, rh.printTime)
				assert.Equal(t, test.expected.printPriority, rh.printPriority)
				assert.Equal(t, test.expected.paginate, rh.paginate)
				assert.Equal(t, 10*1024*1024, rh.softSizeLimit)
				assert.Equal(t, time.UTC, rh.timeZone)
			}
		})
	}
}

func TestGetTaskLogsHandlerParse(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{
		Settings: user.UserSettings{
			Timezone: "UTC",
		},
	})
	env := evergreen.GetEnvironment()

	require.NoError(t, env.DB().Drop(ctx))
	defer func() {
		assert.NoError(t, env.DB().Drop(ctx))
	}()

	tsk := &task.Task{Id: "task"}
	_, err := env.DB().Collection(task.Collection).InsertOne(ctx, tsk)
	require.NoError(t, err)

	for _, test := range []struct {
		name     string
		urlQuery string
		expected *getTaskLogsHandler
		hasErr   bool
	}{
		{
			name:     "InvalidTaskLogType",
			urlQuery: "type=invalid",
			hasErr:   true,
		},
		{
			name: "DefaulParameters",
			expected: &getTaskLogsHandler{
				logType:                      taskoutput.TaskLogTypeAll,
				getTaskOutputLogsBaseHandler: getTaskOutputLogsBaseHandler{tsk: tsk},
			},
		},

		{
			name:     "ValidParameters",
			urlQuery: "type=agent_log&line_limit=100&print_time=true&print_priority=true",
			expected: &getTaskLogsHandler{
				logType: taskoutput.TaskLogTypeAgent,
				getTaskOutputLogsBaseHandler: getTaskOutputLogsBaseHandler{
					tsk:           tsk,
					lineLimit:     100,
					printTime:     true,
					printPriority: true,
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			url, err := url.Parse(fmt.Sprintf("https://evergreen.mongodb.com/rest/v2/tasks/task/build/task_logs?%s", test.urlQuery))
			require.NoError(t, err)
			req := &http.Request{Method: "GET"}
			req.URL = url
			req = gimlet.SetURLVars(req, map[string]string{"task_id": "task"})

			rh := &getTaskLogsHandler{}
			err = rh.Parse(ctx, req)
			if test.hasErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.expected.logType, rh.logType)
				assert.Equal(t, test.expected.tsk.Id, rh.tsk.Id)
				assert.Equal(t, test.expected.tsk.Execution, rh.tsk.Execution)
				assert.Equal(t, test.expected.start, rh.start)
				assert.Equal(t, test.expected.end, rh.end)
				assert.Equal(t, test.expected.lineLimit, rh.lineLimit)
				assert.Equal(t, test.expected.tailN, rh.tailN)
				assert.Equal(t, test.expected.printTime, rh.printTime)
				assert.Equal(t, test.expected.printPriority, rh.printPriority)
				assert.Equal(t, test.expected.paginate, rh.paginate)
				assert.Equal(t, 10*1024*1024, rh.softSizeLimit)
				assert.Equal(t, time.UTC, rh.timeZone)
			}
		})
	}
}

func TestGetTestLogsHandlerParse(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{
		Settings: user.UserSettings{
			Timezone: "UTC",
		},
	})
	env := evergreen.GetEnvironment()

	require.NoError(t, env.DB().Drop(ctx))
	defer func() {
		assert.NoError(t, env.DB().Drop(ctx))
	}()

	tsk := &task.Task{Id: "task"}
	_, err := env.DB().Collection(task.Collection).InsertOne(ctx, tsk)
	require.NoError(t, err)

	for _, test := range []struct {
		name     string
		urlQuery string
		expected *getTestLogsHandler
	}{
		{
			name: "DefaultParameters",
			expected: &getTestLogsHandler{
				logPaths:                     []string{"test0.log"},
				getTaskOutputLogsBaseHandler: getTaskOutputLogsBaseHandler{tsk: tsk},
			},
		},
		{
			name:     "ValidParameters",
			urlQuery: "logs_to_merge=test1.log&logs_to_merge=test2.log&type=agent_log&line_limit=100&print_time=true&print_priority=true",
			expected: &getTestLogsHandler{
				logPaths: []string{"test0.log", "test1.log", "test2.log"},
				getTaskOutputLogsBaseHandler: getTaskOutputLogsBaseHandler{
					tsk:           tsk,
					lineLimit:     100,
					printTime:     true,
					printPriority: true,
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			url, err := url.Parse(fmt.Sprintf("https://evergreen.mongodb.com/rest/v2/tasks/task/build/test_logs/test0.log?%s", test.urlQuery))
			require.NoError(t, err)
			req := &http.Request{Method: "GET"}
			req.URL = url
			req = gimlet.SetURLVars(req, map[string]string{
				"task_id": "task",
				"path":    "test0.log",
			})

			rh := &getTestLogsHandler{}
			require.NoError(t, rh.Parse(ctx, req))
			assert.Equal(t, test.expected.logPaths, rh.logPaths)
			assert.Equal(t, test.expected.tsk.Id, rh.tsk.Id)
			assert.Equal(t, test.expected.tsk.Execution, rh.tsk.Execution)
			assert.Equal(t, test.expected.start, rh.start)
			assert.Equal(t, test.expected.end, rh.end)
			assert.Equal(t, test.expected.lineLimit, rh.lineLimit)
			assert.Equal(t, test.expected.tailN, rh.tailN)
			assert.Equal(t, test.expected.printTime, rh.printTime)
			assert.Equal(t, test.expected.printPriority, rh.printPriority)
			assert.Equal(t, test.expected.paginate, rh.paginate)
			assert.Equal(t, 10*1024*1024, rh.softSizeLimit)
			assert.Equal(t, time.UTC, rh.timeZone)
		})
	}
}
