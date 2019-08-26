package reporting

import (
	"context"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type dbLegacyQueueStat struct {
	opts       queue.MongoDBOptions
	name       string
	session    *mgo.Session
	collection *mgo.Collection
}

// NewLegacyDBQueueState produces a queue Reporter for (remote) queues that persist
// jobs in MongoDB. This implementation does not interact with a queue
// directly, and reports by interacting with the database directly.
//
// This implementation creates a connection using the legacy MGO driver.
func NewLegacyDBQueueState(name string, opts queue.MongoDBOptions) (Reporter, error) {
	session, err := mgo.DialWithTimeout(opts.URI, time.Second)
	if err != nil {
		return nil, errors.Wrap(err, "problem connecting to mongodb")
	}

	db, err := MakeLegacyDBQueueState(name, opts, session)
	if err != nil {
		return nil, errors.Wrap(err, "problem building reporting interface")
	}

	return db, nil
}

// MakeLegacyDBQueueState make it possible to produce a queue reporter with
// an existing database Connection. This operations runs the "ping"
// command and will return an error if there is no session or no
// active server.
//
// This implementation uses a legacy MGO driver connection to the database.
func MakeLegacyDBQueueState(name string, opts queue.MongoDBOptions, session *mgo.Session) (Reporter, error) {
	if session == nil {
		return nil, errors.New("cannot make a reporter without a session")
	}

	var err error

	func() {
		// ping can never error, so we have to do something
		// crazy to catch the error and convert it to an err
		defer func() {
			if p := recover(); p != nil {
				err = recovery.HandlePanicWithError(p, err, "problem with connection")
			}
		}()
		err = session.Ping()
	}()

	if err != nil {
		return nil, errors.Wrap(err, "could not establish a connection with the database")
	}

	session.SetSocketTimeout(0)

	db := &dbLegacyQueueStat{
		name:       name,
		opts:       opts,
		session:    session,
		collection: session.DB(opts.DB).C(addJobsSuffix(name)),
	}

	return db, nil
}

func (db *dbLegacyQueueStat) aggregateCounters(stages ...bson.M) ([]JobCounters, error) {
	out := []JobCounters{}

	if err := db.collection.Pipe(stages).AllowDiskUse().All(&out); err != nil {
		return nil, errors.Wrap(err, "problem running aggregation")
	}

	return out, nil
}

func (db *dbLegacyQueueStat) aggregateRuntimes(stages ...bson.M) ([]JobRuntimes, error) {
	out := []JobRuntimes{}

	if err := db.collection.Pipe(stages).AllowDiskUse().All(&out); err != nil {
		return nil, errors.Wrap(err, "problem running aggregation")
	}

	return out, nil
}

func (db *dbLegacyQueueStat) findJobs(match bson.M) ([]string, error) {
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

	if err := db.collection.Pipe(stages).AllowDiskUse().All(&out); err != nil {
		return nil, errors.Wrap(err, "problem running aggregation")
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

func (db *dbLegacyQueueStat) aggregateErrors(stages ...bson.M) ([]JobErrorsForType, error) {
	out := []JobErrorsForType{}

	if err := db.collection.Pipe(stages).AllowDiskUse().All(&out); err != nil {
		return nil, errors.Wrap(err, "problem running aggregation")
	}

	return out, nil
}

func (db *dbLegacyQueueStat) JobStatus(ctx context.Context, f CounterFilter) (*JobStatusReport, error) {
	var err error

	if err = f.Validate(); err != nil {
		return nil, errors.WithStack(err)
	}

	var counters []JobCounters

	switch f {
	case InProgress:
		counters, err = db.aggregateCounters(
			bson.M{"$match": bson.M{
				"status.completed": false,
				"status.in_prog":   true,
			}},
			bson.M{"$group": bson.M{
				"_id":   "$type",
				"count": bson.M{"$sum": 1},
			}})
	case Pending:
		counters, err = db.aggregateCounters(
			bson.M{"$match": bson.M{
				"status.completed": false,
				"status.in_prog":   false,
			}},
			bson.M{"$group": bson.M{
				"_id":   "$type",
				"count": bson.M{"$sum": 1},
			}})
	case Stale:
		counters, err = db.aggregateCounters(
			bson.M{"$match": bson.M{
				"status.completed": false,
				"status.in_prog":   true,
				"status.mod_ts":    bson.M{"$gt": time.Now().Add(-amboy.LockTimeout)},
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

func (db *dbLegacyQueueStat) RecentTiming(ctx context.Context, window time.Duration, f RuntimeFilter) (*JobRuntimeReport, error) {
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
		runtimes, err = db.aggregateRuntimes(
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
		runtimes, err = db.aggregateRuntimes(
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
		runtimes, err = db.aggregateRuntimes(
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

func (db *dbLegacyQueueStat) JobIDsByState(ctx context.Context, jobType string, f CounterFilter) (*JobReportIDs, error) {
	var err error
	if err = f.Validate(); err != nil {
		return nil, errors.WithStack(err)
	}

	var ids []string

	switch f {
	case InProgress:
		ids, err = db.findJobs(bson.M{
			"type":             jobType,
			"status.completed": false,
			"status.in_prog":   true,
		})
	case Pending:
		ids, err = db.findJobs(bson.M{
			"type":             jobType,
			"status.completed": false,
			"status.in_prog":   false,
		})
	case Stale:
		ids, err = db.findJobs(bson.M{
			"type":             jobType,
			"status.completed": false,
			"status.in_prog":   true,
			"status.mod_ts":    bson.M{"$gt": time.Now().Add(-amboy.LockTimeout)},
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

func (db *dbLegacyQueueStat) RecentErrors(ctx context.Context, window time.Duration, f ErrorFilter) (*JobErrorsReport, error) {
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
		reports, err = db.aggregateErrors(
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
		reports, err = db.aggregateErrors(
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
		reports, err = db.aggregateErrors(
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

func (db *dbLegacyQueueStat) RecentJobErrors(ctx context.Context, jobType string, window time.Duration, f ErrorFilter) (*JobErrorsReport, error) {
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
		reports, err = db.aggregateErrors(
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
		reports, err = db.aggregateErrors(
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
		reports, err = db.aggregateErrors(
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
