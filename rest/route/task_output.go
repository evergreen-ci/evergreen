package route

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/log"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/taskoutput"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

type getTaskOutputLogsBaseHandler struct {
	tsk           *task.Task
	start         *int64
	end           *int64
	lineLimit     int
	tailN         int
	printTime     bool
	printPriority bool
	paginate      bool
	softSizeLimit int
	timeZone      *time.Location

	env evergreen.Environment
	url string
}

func (h *getTaskOutputLogsBaseHandler) parse(ctx context.Context, r *http.Request) error {
	vals := r.URL.Query()

	var (
		execution *int
		err       error
	)
	if execString := vals.Get("execution"); execString != "" {
		exec, err := strconv.Atoi(execString)
		if err != nil {
			return errors.Wrap(err, "parsing execution")
		}

		execution = utility.ToIntPtr(exec)
	}
	h.tsk, err = task.FindByIdExecution(gimlet.GetVars(r)["task_id"], execution)
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "finding task").Error(),
		}
	}
	if h.tsk == nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    "task not found",
		}
	}

	if start := vals.Get("start"); start != "" {
		ts, err := time.Parse(time.RFC3339, start)
		if err != nil {
			return errors.Wrap(err, "parsing start time")
		}

		h.start = utility.ToInt64Ptr(ts.UnixNano())
	}
	if end := vals.Get("end"); end != "" {
		ts, err := time.Parse(time.RFC3339, end)
		if err != nil {
			return errors.Wrap(err, "parsing end time")
		}

		h.end = utility.ToInt64Ptr(ts.UnixNano())
	}

	if limit := vals.Get("line_limit"); limit != "" {
		h.lineLimit, err = strconv.Atoi(limit)
		if err != nil {
			return errors.Wrap(err, "parsing line limit")
		}
	}
	if tail := vals.Get("tail_limit"); tail != "" {
		h.tailN, err = strconv.Atoi(tail)
		if err != nil {
			return errors.Wrap(err, "parsing tail limit")
		}
	}

	h.printTime = strings.ToLower(vals.Get("print_time")) == "true"
	h.printPriority = strings.ToLower(vals.Get("print_priority")) == "true"
	h.paginate = strings.ToLower(vals.Get("paginate")) == "true"
	h.timeZone = getUserTimeZone(MustHaveUser(ctx))
	h.softSizeLimit = 10 * 1024 * 1024

	var count int
	if h.lineLimit > 0 {
		count++
	}
	if h.tailN > 0 {
		count++
	}
	if h.paginate {
		count++
	}
	if count > 1 {
		return errors.New("cannot set more than of: line limit, tail, paginate")
	}

	return nil
}

func (h *getTaskOutputLogsBaseHandler) createResponse(it log.LogIterator) gimlet.Responder {
	var resp gimlet.Responder
	opts := log.LogIteratorReaderOptions{
		PrintTime:     h.printTime,
		TimeZone:      h.timeZone,
		PrintPriority: h.printPriority,
	}
	if h.paginate {
		opts.SoftSizeLimit = h.softSizeLimit
		r := log.NewLogIteratorReader(it, opts)

		data, err := io.ReadAll(r)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "reading paginated log lines"))
		}
		resp = gimlet.NewTextResponse(data)

		pages := &gimlet.ResponsePages{
			Prev: &gimlet.Page{
				BaseURL:         h.url,
				KeyQueryParam:   "start",
				LimitQueryParam: "limit",
				Key:             time.Unix(0, utility.FromInt64Ptr(h.start)).In(h.timeZone).Format(time.RFC3339),
				Relation:        "prev",
			},
		}
		if next := r.NextTimestamp(); next != nil {
			pages.Next = &gimlet.Page{
				BaseURL:         h.url,
				KeyQueryParam:   "start",
				LimitQueryParam: "limit",
				Key:             time.Unix(0, utility.FromInt64Ptr(next)).In(h.timeZone).Format(time.RFC3339),
				Relation:        "next",
			}
		}

		if err := resp.SetPages(pages); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "setting response pages"))
		}
	} else {
		resp = gimlet.NewTextResponse(log.NewLogIteratorReader(it, opts))
	}

	return resp
}

// GET /tasks/{task_id}/build/task_logs
type getTaskLogsHandler struct {
	logType taskoutput.TaskLogType

	getTaskOutputLogsBaseHandler
}

func makeGetTaskLogs(env evergreen.Environment, url string) *getTaskLogsHandler {
	return &getTaskLogsHandler{
		getTaskOutputLogsBaseHandler: getTaskOutputLogsBaseHandler{env: env, url: url},
	}
}

func (h *getTaskLogsHandler) Factory() gimlet.RouteHandler {
	return &getTaskLogsHandler{
		getTaskOutputLogsBaseHandler: getTaskOutputLogsBaseHandler{env: h.env, url: h.url},
	}
}

func (h *getTaskLogsHandler) Parse(ctx context.Context, r *http.Request) error {
	if h.logType = taskoutput.TaskLogType(r.URL.Query().Get("type")); h.logType == "" {
		h.logType = taskoutput.TaskLogTypeAll
	} else if err := h.logType.Validate(false); err != nil {
		return err
	}

	if err := h.parse(ctx, r); err != nil {
		return err
	}

	return nil
}

func (h *getTaskLogsHandler) Run(ctx context.Context) gimlet.Responder {
	it, err := h.tsk.GetTaskLogs(ctx, h.env, taskoutput.TaskLogGetOptions{
		LogType:   h.logType,
		Start:     h.start,
		End:       h.end,
		LineLimit: h.lineLimit,
		TailN:     h.tailN,
	})
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting task logs"))
	}

	return h.createResponse(it)
}

// GET /tasks/{task_id}/build/test_logs/{path}
type getTestLogsHandler struct {
	logPaths []string

	getTaskOutputLogsBaseHandler
}

func makeGetTestLogs(env evergreen.Environment, url string) *getTestLogsHandler {
	return &getTestLogsHandler{
		getTaskOutputLogsBaseHandler: getTaskOutputLogsBaseHandler{env: env, url: url},
	}
}

func (h *getTestLogsHandler) Factory() gimlet.RouteHandler {
	return &getTestLogsHandler{
		getTaskOutputLogsBaseHandler: getTaskOutputLogsBaseHandler{env: h.env, url: h.url},
	}
}

func (h *getTestLogsHandler) Parse(ctx context.Context, r *http.Request) error {
	h.logPaths = append([]string{gimlet.GetVars(r)["path"]}, r.URL.Query()["logs_to_merge"]...)

	if err := h.parse(ctx, r); err != nil {
		return err
	}

	return nil
}

func (h *getTestLogsHandler) Run(ctx context.Context) gimlet.Responder {
	it, err := h.tsk.GetTestLogs(ctx, h.env, taskoutput.TestLogGetOptions{
		LogPaths:  h.logPaths,
		Start:     h.start,
		End:       h.end,
		LineLimit: h.lineLimit,
		TailN:     h.tailN,
	})
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting task logs"))
	}

	return h.createResponse(it)
}

// getUserTimeZone returns the time zone specified by the user settings.
// Defaults to `America/New_York`.
func getUserTimeZone(u *user.DBUser) *time.Location {
	tz := u.Settings.Timezone
	if tz == "" {
		tz = "America/New_York"
	}

	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.UTC
	}

	return loc
}
