package reporting

import (
	"context"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type dbQueueStat struct {
	client     *mongo.Client
	collection *mongo.Collection
	opts       DBQueueReporterOptions
}

// DBQueueReporterOptions describes the arguments to the operations to
// construct queue reporters, and accommodates both group-backed
// queues and conventional queues.
type DBQueueReporterOptions struct {
	Name        string
	Group       string
	SingleGroup bool
	ByGroups    bool
	Options     queue.MongoDBOptions
}

func (o *DBQueueReporterOptions) hasGroups() bool { return o.SingleGroup || o.ByGroups }

func (o *DBQueueReporterOptions) collName() string {
	if o.hasGroups() {
		return addGroupSuffix(o.Name)
	}

	return addJobsSuffix(o.Name)
}

// Validate checks the state of the reporter configuration, preventing
// logically invalid options.
func (o *DBQueueReporterOptions) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(o.SingleGroup && o.ByGroups, "cannot specify conflicting group options")
	catcher.NewWhen(o.Name == "", "must specify queue name")
	return catcher.Resolve()
}

// NewDBQueueState produces a queue Reporter for (remote) queues that persist
// jobs in MongoDB. This implementation does not interact with a queue
// directly, and reports by interacting with the database directly.
func NewDBQueueState(ctx context.Context, opts DBQueueReporterOptions) (Reporter, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	client, err := mongo.NewClient(options.Client().ApplyURI(opts.Options.URI).SetConnectTimeout(time.Second))
	if err != nil {
		return nil, errors.Wrap(err, "problem constructing mongodb client")
	}

	if err = client.Connect(ctx); err != nil {
		return nil, errors.Wrap(err, "problem connecting to database")
	}

	db, err := MakeDBQueueState(ctx, opts, client)
	if err != nil {
		return nil, errors.Wrap(err, "problem building reporting interface")
	}

	return db, nil
}

// MakeDBQueueState make it possible to produce a queue reporter with
// an existing database Connection. This operations runs the "ping"
// command and will return an error if there is no session or no
// active server.
func MakeDBQueueState(ctx context.Context, opts DBQueueReporterOptions, client *mongo.Client) (Reporter, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	if client == nil {
		return nil, errors.New("cannot make a reporter without a client")
	}

	if err := client.Ping(ctx, nil); err != nil {
		return nil, errors.Wrap(err, "could not establish a connection with the database")
	}

	db := &dbQueueStat{
		opts:       opts,
		client:     client,
		collection: client.Database(opts.Options.DB).Collection(opts.collName()),
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
	group := bson.M{
		"_id":  nil,
		"jobs": bson.M{"$push": "$_id"},
	}

	if db.opts.ByGroups {
		group["_id"] = "$group"
	}

	stages := []bson.M{
		{"$match": match},
		{"$group": group},
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
	if err := f.Validate(); err != nil {
		return nil, errors.WithStack(err)
	}

	match := bson.M{
		"status.completed": false,
	}

	group := bson.M{
		"_id":   "$type",
		"count": bson.M{"$sum": 1},
	}

	if db.opts.SingleGroup {
		match["group"] = db.opts.Group
	} else if db.opts.ByGroups {
		group["_id"] = bson.M{"type": "$type", "group": "$group"}
	}

	switch f {
	case InProgress:
		match["status.in_prog"] = true
	case Pending:
		match["status.in_prog"] = false
	case Stale:
		match["status.in_prog"] = true
		match["status.mod_ts"] = bson.M{"$gt": time.Now().Add(-amboy.LockTimeout)}
	default:
		return nil, errors.New("invalid job status filter")
	}

	stages := []bson.M{
		{"$match": match},
		{"$group": group},
	}

	if db.opts.ByGroups {
		stages = append(stages, bson.M{"$project": bson.M{
			"_id":   "$_id.type",
			"count": "$count",
			"group": "$_id.group",
		}})
	}

	counters, err := db.aggregateCounters(ctx, stages...)

	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &JobStatusReport{
		Filter: string(f),
		Stats:  counters,
	}, nil
}

func (db *dbQueueStat) RecentTiming(ctx context.Context, window time.Duration, f RuntimeFilter) (*JobRuntimeReport, error) {
	if window <= time.Second {
		return nil, errors.New("must specify windows greater than one second")
	}

	if err := f.Validate(); err != nil {
		return nil, errors.WithStack(err)
	}

	var match bson.M
	var group bson.M

	groupOp := "$group"
	switch f {
	case Duration:
		match = bson.M{
			"status.completed": true,
			"time_info.end":    bson.M{"$gt": time.Now().Add(-window)},
		}
		group = bson.M{
			"_id": "$type",
			"duration": bson.M{"$avg": bson.M{
				"$multiply": []interface{}{bson.M{
					"$subtract": []string{"$time_info.end", "$time_info.start"}},
					1000000, // convert to nanoseconds
				},
			}},
		}
	case Latency:
		now := time.Now()
		match = bson.M{
			"status.completed":  false,
			"time_info.created": bson.M{"$gt": now.Add(-window)},
		}
		group = bson.M{
			"_id": "$type",
			"duration": bson.M{"$avg": bson.M{
				"$multiply": []interface{}{bson.M{
					"$subtract": []interface{}{now, "$time_info.created"}},
					1000000, // convert to nanoseconds
				}},
			},
		}
	case Running:
		now := time.Now()
		groupOp = "$project"
		match = bson.M{
			"status.completed": false,
			"status.in_prog":   true,
		}
		group = bson.M{
			"_id": "$_id",
			"duration": bson.M{
				"$subtract": []interface{}{now, "$time_info.created"},
			},
		}
	default:
		return nil, errors.New("invalid job runtime filter")
	}

	if db.opts.SingleGroup {
		match["group"] = db.opts.Group
	}

	stages := []bson.M{
		{"$match": match},
		{groupOp: group},
	}

	if db.opts.ByGroups {
		stages = append(stages, bson.M{"$project": bson.M{
			"_id":      "$_id.type",
			"group":    "$_id.group",
			"duration": "$duration",
		}})
	}

	runtimes, err := db.aggregateRuntimes(ctx, stages...)
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
	if err := f.Validate(); err != nil {
		return nil, errors.WithStack(err)
	}

	query := bson.M{
		"type":             jobType,
		"status.completed": false,
	}

	switch f {
	case InProgress:
		query["status.in_prog"] = true
	case Pending:
		query["status.in_prog"] = false
	case Stale:
		query["status.in_prog"] = true
		query["status.mod_ts"] = bson.M{"$gt": time.Now().Add(-amboy.LockTimeout)}
	default:
		return nil, errors.New("invalid job status filter")
	}

	if db.opts.SingleGroup {
		query["group"] = db.opts.Group
	}

	ids, err := db.findJobs(ctx, query)
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
	if window <= time.Second {
		return nil, errors.New("must specify windows greater than one second")
	}

	if err := f.Validate(); err != nil {
		return nil, errors.WithStack(err)

	}
	now := time.Now()

	match := bson.M{
		"status.completed": true,
		"status.err_count": bson.M{"$gt": 0},
		"time_info.end":    bson.M{"$gt": now.Add(-window)},
	}

	group := bson.M{
		"_id":     "$type",
		"count":   bson.M{"$sum": 1},
		"total":   bson.M{"$sum": "$status.err_count"},
		"average": bson.M{"$avg": "$status.err_count"},
		"errors":  bson.M{"$push": "$status.errors"},
	}

	if db.opts.SingleGroup {
		match["group"] = db.opts.Group
	}
	if db.opts.ByGroups {
		group["_id"] = bson.M{"type": "$type", "group": "$group"}
	}

	stages := []bson.M{
		{"$match": match},
	}

	switch f {
	case UniqueErrors:
		stages = append(stages, bson.M{"$group": group})
	case AllErrors:
		stages = append(stages,
			bson.M{"$group": group},
			bson.M{"$unwind": "$status.errors"},
			bson.M{"$group": bson.M{
				"_id":     group["_id"],
				"count":   bson.M{"$first": "$count"},
				"total":   bson.M{"$first": "$total"},
				"average": bson.M{"$first": "$average"},
				"errors":  bson.M{"$addToSet": "$errors"},
			}})
	case StatsOnly:
		delete(group, "errors")
		stages = append(stages, bson.M{"$group": group})
	default:
		return nil, errors.New("operation is not supported")
	}

	if db.opts.ByGroups {
		prj := bson.M{
			"_id":     "$_id.type",
			"group":   "$_id.group",
			"count":   "$count",
			"total":   "$total",
			"average": "$average",
		}
		if f != StatsOnly {
			prj["errors"] = "$errors"
		}

		stages = append(stages, bson.M{"$project": prj})
	}

	reports, err := db.aggregateErrors(ctx, stages...)
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
	if window <= time.Second {
		return nil, errors.New("must specify windows greater than one second")
	}

	if err := f.Validate(); err != nil {
		return nil, errors.WithStack(err)
	}

	now := time.Now()

	match := bson.M{
		"type":             jobType,
		"status.completed": true,
		"status.err_count": bson.M{"$gt": 0},
		"time_info.end":    bson.M{"$gt": now.Add(-window)},
	}

	group := bson.M{
		"_id":     nil,
		"count":   bson.M{"$sum": 1},
		"total":   bson.M{"$sum": "$status.err_count"},
		"average": bson.M{"$avg": "$status.err_count"},
		"errors":  bson.M{"$push": "$statys.errors"},
	}

	if db.opts.SingleGroup {
		match["group"] = db.opts.Group
	}

	if db.opts.ByGroups {
		group["_id"] = bson.M{"type": "$type", "group": "$group"}
	}

	stages := []bson.M{
		{"$match": match},
	}

	switch f {
	case UniqueErrors:
		stages = append(stages, bson.M{"$group": group})
	case AllErrors:
		stages = append(stages,
			bson.M{"$group": group},
			bson.M{"$unwind": "$status.errors"},
			bson.M{"$group": bson.M{
				"_id":     group["_id"],
				"count":   bson.M{"$first": "$count"},
				"total":   bson.M{"$first": "$total"},
				"average": bson.M{"$first": "$average"},
				"errors":  bson.M{"$addToSet": "$errors"},
			}})
	case StatsOnly:
		delete(group, "errors")
		stages = append(stages, bson.M{"$group": group})
	default:
		return nil, errors.New("operation is not supported")

	}

	prj := bson.M{
		"_id":     "$_id.type",
		"count":   "$count",
		"total":   "$total",
		"average": "$average",
	}

	if db.opts.ByGroups {
		prj["group"] = "$_id.group"

		if f != StatsOnly {
			prj["errors"] = "$errors"
		}
	}

	stages = append(stages, bson.M{"$project": prj})

	reports, err := db.aggregateErrors(ctx, stages...)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &JobErrorsReport{
		Period:         window,
		FilteredByType: true,
		Data:           reports,
	}, nil
}
