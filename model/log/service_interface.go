package log

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/pkg/errors"
)

type logService interface {
	GetTaskLogs(context.Context, TaskOptions, GetOptions) (LogIterator, error)
}

func getServiceImpl(env evergreen.Environment, serviceVersion int) (logService, error) {
	return nil, errors.New("not implemented")
}
