package log

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/pkg/errors"
)

type logService interface { //nolint:unused
	Get(context.Context, GetOptions) (LogIterator, error)
	Append(context.Context, string, []LogLine) error
}

func getServiceImpl(env evergreen.Environment, serviceVersion int) (logService, error) { //nolint:unused
	return nil, errors.New("not implemented")
}
