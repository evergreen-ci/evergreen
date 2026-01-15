package service

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/log"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// the task's most recent log messages
const DefaultLogMessages = 100 // passed as a limit, so 0 means don't limit

func (uis *UIServer) taskLog(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	if projCtx.Task == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	tsk := projCtx.Task
	if execStr := gimlet.GetVars(r)["execution"]; execStr != "" {
		execution, err := strconv.Atoi(execStr)
		if err != nil {
			http.Error(w, "invalid execution", http.StatusBadRequest)
			return
		}

		if tsk.Execution != execution {
			tsk, err = task.FindOneIdAndExecution(r.Context(), tsk.Id, execution)
			if err != nil {
				uis.LoggedError(w, r, http.StatusInternalServerError, err)
				return
			}
			if tsk == nil {
				http.Error(w, fmt.Sprintf("task '%s' with execution '%d' not found", projCtx.Task.Id, execution), http.StatusNotFound)
				return
			}
		}
	}

	logType := r.FormValue("type")
	if logType == "EV" {
		loggedEvents, err := event.Find(r.Context(), event.MostRecentTaskEvents(projCtx.Task.Id, DefaultLogMessages))
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}

		gimlet.WriteJSON(w, loggedEvents)
		return
	}

	it, err := tsk.GetTaskLogs(r.Context(), task.TaskLogGetOptions{
		LogType: getTaskLogTypeMapping(logType),
		TailN:   DefaultLogMessages,
	})
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	lines, err := apimodels.ReadLogToSlice(it)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	gimlet.WriteJSON(w, struct {
		LogMessages []*apimodels.LogMessage
	}{LogMessages: lines})
}

func (uis *UIServer) taskLogRaw(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	if projCtx.Task == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	tsk := projCtx.Task
	if execStr := gimlet.GetVars(r)["execution"]; execStr != "" {
		execution, err := strconv.Atoi(execStr)
		if err != nil {
			http.Error(w, "invalid execution", http.StatusBadRequest)
			return
		}

		if tsk.Execution != execution {
			tsk, err = task.FindOneIdAndExecution(r.Context(), tsk.Id, execution)
			if err != nil {
				uis.LoggedError(w, r, http.StatusInternalServerError, err)
				return
			}
			if tsk == nil {
				http.Error(w, fmt.Sprintf("task '%s' with execution '%d' not found", projCtx.Task.Id, execution), http.StatusNotFound)
				return
			}
		}
	}

	logType := getTaskLogTypeMapping(r.FormValue("type"))
	it, err := tsk.GetTaskLogs(r.Context(), task.TaskLogGetOptions{LogType: logType})
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	if r.FormValue("text") == "true" || r.Header.Get("Content-Type") == "text/plain" {
		gimlet.WriteText(w, log.NewLogIteratorReader(it, log.LogIteratorReaderOptions{
			PrintTime:     true,
			TimeZone:      getUserTimeZone(MustHaveUser(r)),
			PrintPriority: r.FormValue("priority") == "true",
		}))
	} else {
		http.Redirect(w, r, fmt.Sprintf("%s/task/%s/html-log?execution=%d&origin=%s", uis.Settings.Ui.UIv2Url, tsk.Id, tsk.Execution, logType), http.StatusPermanentRedirect)
	}
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

func getTaskLogTypeMapping(prefix string) task.TaskLogType {
	switch prefix {
	case apimodels.AgentLogPrefix:
		return task.TaskLogTypeAgent
	case apimodels.SystemLogPrefix:
		return task.TaskLogTypeSystem
	case apimodels.TaskLogPrefix:
		return task.TaskLogTypeTask
	default:
		return task.TaskLogTypeAll
	}
}

func (uis *UIServer) taskFileRaw(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	if projCtx.Task == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	fileName := gimlet.GetVars(r)["file_name"]
	if fileName == "" {
		http.Error(w, "file name not specified", http.StatusBadRequest)
		return
	}
	executionNum := projCtx.Task.Execution
	var err error
	if execStr := gimlet.GetVars(r)["execution"]; execStr != "" {
		executionNum, err = strconv.Atoi(execStr)
		if err != nil {
			http.Error(w, "invalid execution", http.StatusBadRequest)
			return
		}
	}

	taskFiles, err := artifact.GetAllArtifacts(r.Context(), []artifact.TaskIDAndExecution{{TaskID: projCtx.Task.Id, Execution: executionNum}})
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrapf(err, "unable to find artifacts for task '%s'", projCtx.Task.Id))
		return
	}
	taskFiles, err = artifact.StripHiddenFiles(r.Context(), taskFiles, true)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrapf(err, "unable to strip hidden files for task '%s'", projCtx.Task.Id))
		return
	}
	var tFile *artifact.File
	for _, taskFile := range taskFiles {
		if taskFile.Name == fileName {
			tFile = &taskFile
			break
		}
	}
	if tFile == nil {
		uis.LoggedError(w, r, http.StatusNotFound, errors.New(fmt.Sprintf("file '%s' not found", fileName)))
		return
	}

	hasContentType := false
	for _, contentType := range uis.Settings.Ui.FileStreamingContentTypes {
		if strings.HasPrefix(tFile.ContentType, contentType) {
			hasContentType = true
			break
		}
	}
	if !hasContentType {
		uis.LoggedError(w, r, http.StatusBadRequest, errors.New(fmt.Sprintf("unsupported file content type '%s'", tFile.ContentType)))
		return
	}

	if err := validateArtifactURL(r.Context(), tFile.Link); err != nil {
		uis.LoggedError(w, r, http.StatusBadRequest, errors.Wrap(err, "artifact link not allowed"))
		return
	}

	response, err := http.Get(tFile.Link)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "downloading file"))
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		uis.LoggedError(w, r, response.StatusCode, errors.Errorf("failed to download file with status code: %d", response.StatusCode))
		return
	}

	// Create a buffer to stream the file in chunks.
	const bufferSize = 1024 * 1024 // 1MB
	buffer := make([]byte, bufferSize)

	w.Header().Set("Content-Type", tFile.ContentType)
	_, err = io.CopyBuffer(w, response.Body, buffer)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "writing to response"))
		return
	}

}

func (uis *UIServer) testLog(w http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	vals := r.URL.Query()

	taskID := vars["task_id"]
	execution, err := strconv.Atoi(vars["task_execution"])
	if err != nil {
		http.Error(w, "invalid execution", http.StatusBadRequest)
		return
	}
	tsk, err := task.FindOneIdAndExecution(r.Context(), taskID, execution)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	if tsk == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	testName := vars["test_name"]
	if testName == "" {
		testName = vals.Get("test_name")
	}
	it, err := tsk.GetTestLogs(r.Context(), task.TestLogGetOptions{LogPaths: []string{testName}})
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	if vals.Get("text") == "true" || r.Header.Get("Content-Type") == "text/plain" {
		gimlet.WriteText(w, log.NewLogIteratorReader(it, log.LogIteratorReaderOptions{
			PrintTime:     true,
			PrintPriority: r.FormValue("priority") == "true",
			TimeZone:      getUserTimeZone(MustHaveUser(r)),
		}))
	} else {
		http.Redirect(w, r, fmt.Sprintf("%s/task/%s/test-html-log?execution=%d&testName=%s", uis.Settings.Ui.UIv2Url, tsk.Id, tsk.Execution, testName), http.StatusPermanentRedirect)
	}
}

var blockedCIDRs = []*net.IPNet{
	// AWS metadata endpoints
	mustParseCIDR("169.254.0.0/16"),
	mustParseCIDR("fe80::/10"),
	// Loopback endpoints
	mustParseCIDR("127.0.0.0/8"),
	mustParseCIDR("::1/128"),
}

func mustParseCIDR(c string) *net.IPNet {
	_, n, err := net.ParseCIDR(c)
	if err != nil {
		panic(err)
	}
	return n
}

func isBlockedIP(ip net.IP) bool {
	for _, n := range blockedCIDRs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// validateArtifactURL validates that a URL is safe to fetch
func validateArtifactURL(ctx context.Context, raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return errors.Wrap(err, "invalid URL")
	}

	// Only allow HTTP and HTTPS
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.Errorf("unsupported scheme %s", u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return errors.New("missing host")
	}

	// Block literal IP addresses entirely
	if ip := net.ParseIP(host); ip != nil {
		return errors.New("literal IP hosts are not allowed")
	}

	// Resolve hostname and check all returned IPs
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return errors.Wrap(err, "DNS resolution failed")
	}

	for _, ip := range ips {
		if isBlockedIP(ip) {
			return errors.Errorf("host %s resolves to blocked address %s", host, ip.String())
		}
	}
	return nil
}
