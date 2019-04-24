package reporting

import (
	"context"
	"time"

	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type dbQueueStat struct {
	opts       queue.MongoDBOptions
	name       string
	client     *mongo.Client
	collection *mongo.Collection
}

// NewDBQueueState produces a queue Reporter for (remote) queues that persist
// jobs in MongoDB. This implementation does not interact with a queue
// directly, and reports by interacting with the database directly.
func NewDBQueueState(ctx context.Context, name string, opts queue.MongoDBOptions) (Reporter, error) {
	client, err := mongo.NewClient(options.Client().ApplyURI(opts.URI).SetConnectTimeout(time.Second))
	if err != nil {
		return nil, errors.Wrap(err, "problem constructing mongodb client")
	}

	connctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := client.Connect(connctx); err != nil {
		return nil, errors.Wrap(err, "problem connecting to database")
	}

	db, err := MakeDBQueueState(ctx, name, opts, client)
	if err != nil {
		return nil, errors.Wrap(err, "problem building reporting interface")
	}

	return db, nil
}

// MakeDBQueueState make it possible to produce a queue reporter with
// an existing database Connection. This operations runs the "ping"
// command and will return an error if there is no session or no
// active server.
func MakeDBQueueState(ctx context.Context, name string, opts queue.MongoDBOptions, client *mongo.Client) (Reporter, error) {
	if client == nil {
		return nil, errors.New("cannot make a reporter without a client")
	}

	if err := client.Ping(ctx, nil); err != nil {
		return nil, errors.Wrap(err, "could not establish a connection with the database")
	}

	db := &dbQueueStat{
		name:       name,
		opts:       opts,
		client:     client,
		collection: client.Database(opts.DB).Collection(addJobsSuffix(name)),
	}

	return db, nil
}

func (db *dbQueueStat) aggregateCounters(ctx context.Context, stages ...bson.M) ([]JobCounters, error) {
	cursor, err := db.collection.Aggregate(ctx, stages, options.Aggregate().SetAllowDiskUse(true))
	if err != nil {
		return nil, errors.Wrap(err, "problem running aggregation")
	}

	catcher := grip.NewBasicCatcher()
	out := []JobCounters{}
	for cursor.Next(ctx) {
		val := JobCounters{}
		err = cursor.Decode(&val)
		if err != nil {
			catcher.Add(err)
			continue
		}
		out = append(out, val)
	}
	catcher.Add(cursor.Err())
	if catcher.HasErrors() {
		return nil, errors.Wrap(catcher.Resolve(), "problem running job counters aggregation")
	}

	return out, nil
}

func (db *dbQueueStat) aggregateRuntimes(ctx context.Context, stages ...bson.M) ([]JobRuntimes, error) {
	cursor, err := db.collection.Aggregate(ctx, stages, options.Aggregate().SetAllowDiskUse(true))
	if err != nil {
		return nil, errors.Wrap(err, "problem running aggregation")
	}

	catcher := grip.NewBasicCatcher()
	out := []JobRuntimes{}
	for cursor.Next(ctx) {
		val := JobRuntimes{}
		err = cursor.Decode(&val)
		if err != nil {
			catcher.Add(err)
			continue
		}
		out = append(out, val)
	}
	catcher.Add(cursor.Err())
	if catcher.HasErrors() {
		return nil, errors.Wrap(catcher.Resolve(), "problem running job runtimes aggregation")
	}

	return out, nil
}

func (db *dbQueueStat) aggregateErrors(ctx context.Context, stages ...bson.M) ([]JobErrorsForType, error) {
	cursor, err := db.collection.Aggregate(ctx, stages, options.Aggregate().SetAllowDiskUse(true))
	if err != nil {
		return nil, errors.Wrap(err, "problem running aggregation")
	}

	catcher := grip.NewBasicCatcher()
	out := []JobErrorsForType{}
	for cursor.Next(ctx) {
		val := JobErrorsForType{}
		err = cursor.Decode(&val)
		if err != nil {
			catcher.Add(err)
			continue
		}
		out = append(out, val)
	}
	catcher.Add(cursor.Err())
	if catcher.HasErrors() {
		return nil, errors.Wrap(catcher.Resolve(), "problem running job counters aggregation")
	}

	return out, nil
}

func (db *dbQueueStat) findJobs(ctx context.Context, match bson.M) ([]string, error) {
	stages := []bson.M{
		{"$match": match},
		{"$group": bson.M{
			"_id":  nil,
			"jobs": bson.M{"$push": "$_id"},
		}},
	}

	out := []struct {
		Jobs []string `bson:"jobs"`
	}{}

	cursor, err := db.collection.Aggregate(ctx, stages, options.Aggregate().SetAllowDiskUse(true))
	if err != nil {
		return nil, errors.Wrap(err, "problem running query")
	}

	if cursor.Next(ctx) {
		if err := cursor.Decode(&out); err != nil {
			return nil, errors.Wrap(err, "problem decoding result")
		}
	}

	switch len(out) {
	case 0:
		return []string{}, nil
	case 1:
		return out[0].Jobs, nil
	default:
		return nil, errors.Errorf("job results malformed with %d results", len(out))
	}
}

func (db *dbQueueStat) JobStatus(ctx context.Context, f CounterFilter) (*JobStatusReport, error) {
	var err error

	if err = f.Validate(); err != nil {
		return nil, errors.WithStack(err)
	}

	var counters []JobCounters

	switch f {
	case InProgress:
		counters, err = db.aggregateCounters(ctx,
			bson.M{"$match": bson.M{
				"status.completed": false,
				"status.in_prog":   true,
			}},
			bson.M{"$group": bson.M{
				"_id":   "$type",
				"count": bson.M{"$sum": 1},
			}})
	case Pending:
		counters, err = db.aggregateCounters(ctx,
			bson.M{"$match": bson.M{
				"status.completed": false,
				"status.in_prog":   false,
			}},
			bson.M{"$group": bson.M{
				"_id":   "$type",
				"count": bson.M{"$sum": 1},
			}})
	case Stale:
		counters, err = db.aggregateCounters(ctx,
			bson.M{"$match": bson.M{
				"status.completed": false,
				"status.in_prog":   true,
				"status.mod_ts":    bson.M{"$gt": time.Now().Add(-queue.LockTimeout)},
			}},
			bson.M{"$group": bson.M{
				"_id":   "$type",
				"count": bson.M{"$sum": 1},
			}})
	default:
		return nil, errors.New("invalid job status filter")
	}

	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &JobStatusReport{
		Filter: string(f),
		Stats:  counters,
	}, nil
}

func (db *dbQueueStat) RecentTiming(ctx context.Context, window time.Duration, f RuntimeFilter) (*JobRuntimeReport, error) {
	var err error

	if err = f.Validate(); err != nil {
		return nil, errors.WithStack(err)
	}

	if window <= time.Second {
		return nil, errors.New("must specify windows greater than one second")
	}

	var runtimes []JobRuntimes

	switch f {
	case Duration:
		runtimes, err = db.aggregateRuntimes(ctx,
			bson.M{"$match": bson.M{
				"status.completed": true,
				"time_info.end":    bson.M{"$gt": time.Now().Add(-window)},
			}},
			bson.M{"$group": bson.M{
				"_id": "$type",
				"duration": bson.M{"$avg": bson.M{
					"$multiply": []interface{}{bson.M{
						"$subtract": []string{"$time_info.end", "$time_info.start"}},
						1000000, // convert to nanoseconds
					},
				}},
			}})
	case Latency:
		now := time.Now()
		runtimes, err = db.aggregateRuntimes(ctx,
			bson.M{"$match": bson.M{
				"status.completed":  false,
				"time_info.created": bson.M{"$gt": now.Add(-window)},
			}},
			bson.M{"$group": bson.M{
				"_id": "$type",
				"duration": bson.M{"$avg": bson.M{
					"$multiply": []interface{}{bson.M{
						"$subtract": []interface{}{now, "$time_info.created"}},
						1000000, // convert to nanoseconds
					},
				}},
			}})
	case Running:
		now := time.Now()
		runtimes, err = db.aggregateRuntimes(ctx,
			bson.M{"$match": bson.M{
				"status.completed": false,
				"status.in_prog":   true,
			}},
			bson.M{"$project": bson.M{
				"duration": bson.M{
					"$subtract": []interface{}{now, "$time_info.created"}},
			}},
		)
	default:
		return nil, errors.New("invalid job runtime filter")
	}

	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &JobRuntimeReport{
		Filter: string(f),
		Period: window,
		Stats:  runtimes,
	}, nil
}

func (db *dbQueueStat) JobIDsByState(ctx context.Context, jobType string, f CounterFilter) (*JobReportIDs, error) {
	var err error
	if err = f.Validate(); err != nil {
		return nil, errors.WithStack(err)
	}

	var ids []string

	switch f {
	case InProgress:
		ids, err = db.findJobs(ctx, bson.M{
			"type":             jobType,
			"status.completed": false,
			"status.in_prog":   true,
		})
	case Pending:
		ids, err = db.findJobs(ctx, bson.M{
			"type":             jobType,
			"status.completed": false,
			"status.in_prog":   false,
		})
	case Stale:
		ids, err = db.findJobs(ctx, bson.M{
			"type":             jobType,
			"status.completed": false,
			"status.in_prog":   true,
			"status.mod_ts":    bson.M{"$gt": time.Now().Add(-queue.LockTimeout)},
		})

	default:
		return nil, errors.New("invalid job status filter")
	}

	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &JobReportIDs{
		Filter: string(f),
		Type:   jobType,
		IDs:    ids,
	}, nil
}

func (db *dbQueueStat) RecentErrors(ctx context.Context, window time.Duration, f ErrorFilter) (*JobErrorsReport, error) {
	var err error
	if err = f.Validate(); err != nil {
		return nil, errors.WithStack(err)

	}
	if window <= time.Second {
		return nil, errors.New("must specify windows greater than one second")
	}

	var reports []JobErrorsForType

	now := time.Now()

	switch f {
	case UniqueErrors:
		reports, err = db.aggregateErrors(ctx,
			bson.M{"$match": bson.M{
				"status.completed": true,
				"status.err_count": bson.M{"$gt": 0},
				"time_info.end":    bson.M{"$gt": now.Add(-window)},
			}},
			bson.M{"$group": bson.M{
				"_id":     "$type",
				"count":   bson.M{"$sum": 1},
				"total":   bson.M{"$sum": "$status.err_count"},
				"average": bson.M{"$avg": "$status.err_count"},
				"errors":  bson.M{"$push": "$status.errors"},
			}})
	case AllErrors:
		reports, err = db.aggregateErrors(ctx,
			bson.M{"$match": bson.M{
				"status.completed": true,
				"status.err_count": bson.M{"$gt": 0},
				"time_info.end":    bson.M{"$gt": now.Add(-window)},
			}},
			bson.M{"$group": bson.M{
				"_id":     "$type",
				"count":   bson.M{"$sum": 1},
				"total":   bson.M{"$sum": "$status.err_count"},
				"average": bson.M{"$avg": "$status.err_count"},
				"errors":  bson.M{"$push": "$statys.errors"},
			}},
			bson.M{"$unwind": "$status.errors"},
			bson.M{"$group": bson.M{
				"_id":     "$type",
				"count":   bson.M{"$first": "$count"},
				"total":   bson.M{"$first": "$total"},
				"average": bson.M{"$first": "$average"},
				"errors":  bson.M{"$addToSet": "$errors"},
			}})
	case StatsOnly:
		reports, err = db.aggregateErrors(ctx,
			bson.M{"$match": bson.M{
				"status.completed": true,
				"status.err_count": bson.M{"$gt": 0},
				"time_info.end":    bson.M{"$gt": now.Add(-window)},
			}},
			bson.M{"$group": bson.M{
				"_id":     "$type",
				"count":   bson.M{"$sum": 1},
				"total":   bson.M{"$sum": "$status.err_count"},
				"average": bson.M{"$avg": "$status.err_count"},
			}})
	default:
		return nil, errors.New("operation is not supported")
	}

	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &JobErrorsReport{
		Period:         window,
		FilteredByType: false,
		Data:           reports,
	}, nil
}

func (db *dbQueueStat) RecentJobErrors(ctx context.Context, jobType string, window time.Duration, f ErrorFilter) (*JobErrorsReport, error) {
	var err error

	if err = f.Validate(); err != nil {
		return nil, errors.WithStack(err)
	}

	if window <= time.Second {
		return nil, errors.New("must specify windows greater than one second")
	}

	var reports []JobErrorsForType

	now := time.Now()

	switch f {
	case UniqueErrors:
		reports, err = db.aggregateErrors(ctx,
			bson.M{"$match": bson.M{
				"type":             jobType,
				"status.completed": true,
				"status.err_count": bson.M{"$gt": 0},
				"time_info.end":    bson.M{"$gt": now.Add(-window)},
			}},
			bson.M{"$group": bson.M{
				"_id":     nil,
				"count":   bson.M{"$sum": 1},
				"total":   bson.M{"$sum": "$status.err_count"},
				"average": bson.M{"$avg": "$status.err_count"},
				"errors":  bson.M{"$push": "$statys.errors"},
			}})
	case AllErrors:
		reports, err = db.aggregateErrors(ctx,
			bson.M{"$match": bson.M{
				"type":             jobType,
				"status.completed": true,
				"status.err_count": bson.M{"$gt": 0},
				"time_info.end":    bson.M{"$gt": now.Add(-window)},
			}},
			bson.M{"$group": bson.M{
				"_id":     nil,
				"count":   bson.M{"$sum": 1},
				"total":   bson.M{"$sum": "$status.err_count"},
				"average": bson.M{"$avg": "$status.err_count"},
				"errors":  bson.M{"$push": "$statys.errors"},
			}},
			bson.M{"$unwind": "$status.errors"},
			bson.M{"$group": bson.M{
				"_id":     nil,
				"count":   bson.M{"$first": "$count"},
				"total":   bson.M{"$first": "$total"},
				"average": bson.M{"$first": "$average"},
				"errors":  bson.M{"$addToSet": "$errors"},
			}})
	case StatsOnly:
		reports, err = db.aggregateErrors(ctx,
			bson.M{"$match": bson.M{
				"type":             jobType,
				"status.completed": true,
				"status.err_count": bson.M{"$gt": 0},
				"time_info.end":    bson.M{"$gt": now.Add(-window)},
			}},
			bson.M{"$group": bson.M{
				"_id":     nil,
				"count":   bson.M{"$sum": 1},
				"total":   bson.M{"$sum": "$status.err_count"},
				"average": bson.M{"$avg": "$status.err_count"},
			}})
	default:
		return nil, errors.New("operation is not supported")

	}

	if err != nil {
		return nil, errors.WithStack(err)
	}

	for idx := range reports {
		reports[idx].ID = jobType
	}
	return &JobErrorsReport{
		Period:         window,
		FilteredByType: true,
		Data:           reports,
	}, nil
}
