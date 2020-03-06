package graphql

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/rest/route"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/timber/fetcher"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// GetGroupedFiles returns the files of a Task inside a GroupedFile struct
func GetGroupedFiles(ctx context.Context, name string, taskID string, execution int) (*GroupedFiles, error) {
	taskFiles, err := artifact.GetAllArtifacts([]artifact.TaskIDAndExecution{{TaskID: taskID, Execution: execution}})
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}
	hasUser := gimlet.GetUser(ctx) != nil
	strippedFiles, err := artifact.StripHiddenFiles(taskFiles, hasUser)
	if err != nil {
		return nil, err
	}

	apiFileList := []*restModel.APIFile{}
	for _, file := range strippedFiles {
		apiFile := restModel.APIFile{}
		err := apiFile.BuildFromService(file)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("error stripping hidden files"))
		}
		apiFileList = append(apiFileList, &apiFile)
	}
	return &GroupedFiles{TaskName: &name, Files: apiFileList}, nil
}

func SetScheduled(ctx context.Context, sc data.Connector, taskID string, isActive bool) (*restModel.APITask, error) {
	usr := route.MustHaveUser(ctx)
	if err := model.SetActiveState(taskID, usr.Username(), isActive); err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	task, err := task.FindOneId(taskID)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}
	if task == nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}
	apiTask := restModel.APITask{}
	err = apiTask.BuildFromService(task)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	err = apiTask.BuildFromService(sc.GetURL())
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	return &apiTask, nil
}

// GetFormattedDate returns a time.Time type in the format "Dec 13, 2020, 11:58:04 pm"
func GetFormattedDate(t *time.Time, timezone string) (*string, error) {
	if t == nil {
		return nil, nil
	}

	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, err
	}

	timeInUserTimezone := t.In(loc)
	newTime := fmt.Sprintf("%s %d, %d, %s", timeInUserTimezone.Month(), timeInUserTimezone.Day(), timeInUserTimezone.Year(), timeInUserTimezone.Format(time.Kitchen))

	return &newTime, nil
}

// IsURL returns true if str is a url with scheme and domain name
func IsURL(str string) bool {
	u, err := url.ParseRequestURI(str)
	return err == nil && u.Scheme != "" && u.Host != ""
}

// BaseTaskStatuses represents the format {buildVariant: {displayName: status}} for base task statuses
type BaseTaskStatuses map[string]map[string]string

// GetBaseTaskStatusesFromPatchID gets the status of each base build associated with a task
func GetBaseTaskStatusesFromPatchID(r *queryResolver, patchID string) (BaseTaskStatuses, error) {
	version, err := r.sc.FindVersionById(patchID)
	if err != nil {
		return nil, fmt.Errorf("Error getting version %s: %s", patchID, err.Error())
	}
	if version == nil {
		return nil, fmt.Errorf("No version found for ID %s", patchID)
	}
	baseVersion, err := model.VersionFindOne(model.VersionBaseVersionFromPatch(version.Identifier, version.Revision))
	if err != nil {
		return nil, fmt.Errorf("Error getting base version from version %s: %s", version.Id, err.Error())
	}
	if baseVersion == nil {
		return nil, fmt.Errorf("No base version found from version %s", version.Id)
	}
	baseTasks, err := task.FindTasksFromVersions([]string{baseVersion.Id})
	if err != nil {
		return nil, fmt.Errorf("Error getting tasks from version %s: %s", baseVersion.Id, err.Error())
	}
	if baseTasks == nil {
		return nil, fmt.Errorf("No tasks found for version %s", baseVersion.Id)
	}

	baseTaskStatusesByDisplayNameByVariant := make(map[string]map[string]string)
	for _, task := range baseTasks {
		if _, ok := baseTaskStatusesByDisplayNameByVariant[task.BuildVariant]; !ok {
			baseTaskStatusesByDisplayNameByVariant[task.BuildVariant] = map[string]string{}
		}
		baseTaskStatusesByDisplayNameByVariant[task.BuildVariant][task.DisplayName] = task.Status
	}
	return baseTaskStatusesByDisplayNameByVariant, nil
}

func readBuildloggerToSlice(ctx context.Context, taskID string, r io.ReadCloser) []apimodels.LogMessage {
	lines := []apimodels.LogMessage{}
	lineChan := make(chan apimodels.LogMessage, 1024)
	go readBuildloggerToChan(ctx, taskID, r, lineChan)

	for {
		line, more := <-lineChan
		if !more {
			break
		}

		lines = append(lines, line)
	}

	return lines
}

func readBuildloggerToChan(ctx context.Context, taskID string, r io.ReadCloser, lines chan<- apimodels.LogMessage) {
	var (
		line string
		err  error
	)

	defer close(lines)
	if r == nil {
		return
	}

	reader := bufio.NewReader(r)
	for err == nil {
		line, err = reader.ReadString('\n')
		if err != nil && err != io.EOF {
			grip.Warning(message.WrapError(err, message.Fields{
				"task_id": taskID,
				"message": "problem reading buildlogger log lines",
			}))
			return
		}

		severity := string(level.Info)
		if strings.HasPrefix(line, "[P: ") {
			severity = strings.TrimSpace(line[3:6])
			if err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"task_id": taskID,
					"message": "problem reading buildlogger log line severity",
				}))
				err = nil
			}
		}

		select {
		case <-ctx.Done():
			grip.Error(message.WrapError(ctx.Err(), message.Fields{
				"task_id": taskID,
				"message": "context error while reading buildlogger log lines",
			}))
		case lines <- apimodels.LogMessage{
			Message:  strings.TrimSuffix(line, "\n"),
			Severity: severity,
		}:
		}
	}
}

func GetBuildloggerLogs(ctx context.Context, buildloggerBaseURL, taskId, logType string, tail, execution int) (io.ReadCloser, error) {
	usr := gimlet.GetUser(ctx)
	fmt.Println(usr.GetAPIKey())
	taskId = "evergreen_lint_lint_service_d7550a92b28636350af9d375b0df7e341477751d_20_03_06_13_55_56"
	execution = 0
	buildloggerBaseURL = "cedar.mongodb.com"
	opts := fetcher.GetOptions{
		BaseURL:       fmt.Sprintf("https://%s", buildloggerBaseURL),
		UserKey:       "1dc6f93f6cde1ea0f4a41b90f36a25af",
		UserName:      "arjun.patel",
		TaskID:        taskId,
		Execution:     execution,
		PrintTime:     true,
		PrintPriority: true,
		Tail:          tail,
	}
	switch logType {
	case apimodels.TaskLogPrefix:
		opts.ProcessName = evergreen.LogTypeTask
	case apimodels.SystemLogPrefix:
		opts.ProcessName = evergreen.LogTypeSystem
	case apimodels.AgentLogPrefix:
		opts.ProcessName = evergreen.LogTypeAgent
	}

	logReader, err := fetcher.Logs(ctx, opts)
	return logReader, errors.Wrapf(err, "failed to get logs for '%s' from buildlogger, using evergreen logger", taskId)
}
