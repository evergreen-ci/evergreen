package migrations

import (
	"context"

	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/anser"
	"github.com/pkg/errors"
)

// Setup configures the migration environment, configuring the backing
// queue and a database session.
func Setup(ctx context.Context, mongodbURI string) (anser.Environment, error) {
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)

	env := anser.GetEnvironment()
	env.RegisterCloser(func() error { cancel(); return nil })

	// This should be switched back to an ordered queue (EVG-2318)
	q := queue.NewLocalUnordered(8)
	if err := q.Start(ctx); err != nil {
		return nil, errors.WithStack(err)
	}

	if err := env.Setup(q, mongodbURI); err != nil {
		return nil, errors.WithStack(err)
	}

	return env, nil
}
