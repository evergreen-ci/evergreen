package migrations

import (
	"context"

	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/amboy/queue/driver"
	"github.com/mongodb/anser"
	"github.com/pkg/errors"
)

// Setup configures the migration environment, configuring the backing
// queue and a database session.
func Setup(ctx context.Context, mongodbURI string) (anser.Environment, error) {
	backend := driver.NewPriority()
	if err := backend.Open(ctx); err != nil {
		return nil, errors.WithStack(err)
	}
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)

	env := anser.GetEnvironment()
	env.RegisterCloser(func() error { cancel(); return nil })
	env.RegisterCloser(func() error { backend.Close(); return nil })

	q := queue.NewSimpleRemoteOrdered(4)
	if err := q.SetDriver(backend); err != nil {
		return nil, errors.WithStack(err)
	}

	if err := q.Start(ctx); err != nil {
		return nil, errors.WithStack(err)
	}

	if err := env.Setup(q, mongodbURI); err != nil {
		return nil, errors.WithStack(err)
	}

	return env, nil
}
