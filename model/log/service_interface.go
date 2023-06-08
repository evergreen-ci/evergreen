package log

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/pkg/errors"
)

type logService interface { //nolint:unused
	GetTaskLogPrefix(TaskOptions, TaskLogType) (string, error)
	GetTaskLogs(context.Context, TaskOptions, GetOptions) (LogReader, error)
}

func getServiceImpl(env evergreen.Environment, serviceVersion int) (logService, error) { //nolint:unused
	return nil, errors.New("not implemented")
}
