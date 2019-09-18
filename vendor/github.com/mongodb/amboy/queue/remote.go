package queue

import (
	"context"

	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
)

// MongoDBQueueCreationOptions describes the options passed to the remote
// queue, that store jobs in a remote persistence layer to support
// distributed systems of workers.
type MongoDBQueueCreationOptions struct {
	Size    int
	Name    string
	Ordered bool
	MDB     MongoDBOptions
	Client  *mongo.Client
}

// NewMongoDBQueue builds a new queue that persists jobs to a MongoDB
// instance. These queues allow workers running in multiple processes
// to service shared workloads in multiple processes.
func NewMongoDBQueue(ctx context.Context, opts MongoDBQueueCreationOptions) (amboy.Queue, error) {
	if err := opts.Validate(); err != nil {
		return nil, errors.WithStack(err)
	}

	return opts.build(ctx)
}

// Validate ensure that the arguments defined are valid.
func (opts *MongoDBQueueCreationOptions) Validate() error {
	catcher := grip.NewBasicCatcher()

	catcher.NewWhen(opts.Name == "", "must specify a name")

	catcher.NewWhen(opts.Client == nil && (opts.MDB.URI == "" && opts.MDB.DB == ""),
		"must specify database options")

	return catcher.Resolve()
}

func (opts *MongoDBQueueCreationOptions) build(ctx context.Context) (amboy.Queue, error) {
	var driver remoteQueueDriver
	var err error

	if opts.Client == nil {
		if opts.MDB.UseGroups {
			driver = newMongoGroupDriver(opts.Name, opts.MDB, opts.MDB.GroupName)
		} else {
			driver = newMongoDriver(opts.Name, opts.MDB)
		}

		err = driver.Open(ctx)
	} else {
		if opts.MDB.UseGroups {
			driver, err = openNewMongoGroupDriver(ctx, opts.Name, opts.MDB, opts.MDB.GroupName, opts.Client)
		} else {
			driver, err = openNewMongoDriver(ctx, opts.Name, opts.MDB, opts.Client)
		}
	}

	if err != nil {
		return nil, errors.Wrap(err, "problem building driver")
	}

	var q remoteQueue
	if opts.Ordered {
		q = newSimpleRemoteOrdered(opts.Size)
	} else {
		q = newRemoteUnordered(opts.Size)
	}

	if err = q.SetDriver(driver); err != nil {
		return nil, errors.Wrap(err, "problem configuring queue")
	}

	return q, nil
}
