package management

import (
	"context"
	"fmt"
	"time"

	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type dbQueueManager struct {
	client     *mongo.Client
	collection *mongo.Collection
	opts       DBQueueManagerOptions
}

// DBQueueManagerOptions describes the arguments to the operations to construct
// queue managers, and accommodates both group-backed queues and conventional
// queues.
type DBQueueManagerOptions struct {
	// Name is the prefix of the DB namespace to use.
	Name string
	// Group is the name of the queue group if managing a single group.
	Group string
	// SingleGroup indicates that the queue is managing a single queue group.
	SingleGroup bool
	// ByGroups indicates that the queue is managing multiple queues in a queue
	// group. Only a subset of operations are supported if ByGroups is
	// specified.
	ByGroups bool
	Options  queue.MongoDBOptions
}

func (o *DBQueueManagerOptions) hasGroups() bool { return o.SingleGroup || o.ByGroups }

func (o *DBQueueManagerOptions) collName() string {
	if o.hasGroups() {
		return addGroupSuffix(o.Name)
	}

	return addJobsSuffix(o.Name)
}

// Validate checks the state of the manager configuration, preventing logically
// invalid options.
func (o *DBQueueManagerOptions) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(o.SingleGroup && o.ByGroups, "cannot specify conflicting group options")
	catcher.NewWhen(o.Name == "", "must specify queue name")
	catcher.Wrap(o.Options.Validate(), "invalid mongo options")
	return catcher.Resolve()
}

// NewDBQueueManager produces a queue manager for (remote) queues that persist
// jobs in MongoDB. This implementation does not interact with the queue
// directly, and manages by interacting with the database directly.
func NewDBQueueManager(ctx context.Context, opts DBQueueManagerOptions) (Manager, error) {
	if err := opts.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid options")
	}

	client, err := mongo.NewClient(options.Client().ApplyURI(opts.Options.URI).SetConnectTimeout(time.Second))
	if err != nil {
		return nil, errors.Wrap(err, "problem constructing mongodb client")
	}

	if err = client.Connect(ctx); err != nil {
		return nil, errors.Wrap(err, "problem connecting to database")
	}

	db, err := MakeDBQueueManager(ctx, opts, client)
	if err != nil {
		return nil, errors.Wrap(err, "problem building reporting interface")
	}

	return db, nil
}

// MakeDBQueueManager make it possible to produce a queue manager with an
// existing database Connection. This operations runs the "ping" command and
// and will return an error if there is no session or no active server.
func MakeDBQueueManager(ctx context.Context, opts DBQueueManagerOptions, client *mongo.Client) (Manager, error) {
	if err := opts.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid options")
	}

	if client == nil {
		return nil, errors.New("cannot make a manager without a client")
	}

	if err := client.Ping(ctx, nil); err != nil {
		return nil, errors.Wrap(err, "could not establish a connection with the database")
	}

	db := &dbQueueManager{
		opts:       opts,
		client:     client,
		collection: client.Database(opts.Options.DB).Collection(opts.collName()),
	}

	return db, nil
}

func (db *dbQueueManager) aggregateCounters(ctx context.Context, stages ...bson.M) ([]JobCounters, error) {
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

func (db *dbQueueManager) aggregateRuntimes(ctx context.Context, stages ...bson.M) ([]JobRuntimes, error) {
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

func (db *dbQueueManager) aggregateErrors(ctx context.Context, stages ...bson.M) ([]JobErrorsForType, error) {
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

func (db *dbQueueManager) findJobs(ctx context.Context, match bson.M) ([]GroupedID, error) {
	group := bson.M{
		"_id":  nil,
		"jobs": bson.M{"$push": bson.M{"_id": "$_id", "group": "$group"}},
	}

	stages := []bson.M{
		{"$match": match},
		{"$group": group},
	}

	out := struct {
		Jobs []GroupedID `bson:"jobs"`
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

	return out.Jobs, nil
}

func (db *dbQueueManager) JobStatus(ctx context.Context, f StatusFilter) (*JobStatusReport, error) {
	if err := f.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid status filter")
	}

	match := bson.M{}

	group := bson.M{
		"_id":   "$type",
		"count": bson.M{"$sum": 1},
	}

	if db.opts.SingleGroup {
		match["group"] = db.opts.Group
	} else if db.opts.ByGroups {
		group["_id"] = bson.M{"type": "$type", "group": "$group"}
	}

	match = db.getStatusQuery(match, f)

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
		Filter: f,
		Stats:  counters,
	}, nil
}

func (db *dbQueueManager) RecentTiming(ctx context.Context, window time.Duration, f RuntimeFilter) (*JobRuntimeReport, error) {
	if window <= time.Second {
		return nil, errors.New("must specify windows greater than one second")
	}

	if err := f.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid runtime filter")
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
		Filter: f,
		Period: window,
		Stats:  runtimes,
	}, nil
}

// JobIDsByState returns job IDs filtered by job type and status filter. The
// returned job IDs are the internally-stored job IDs.
func (db *dbQueueManager) JobIDsByState(ctx context.Context, jobType string, f StatusFilter) (*JobReportIDs, error) {
	if err := f.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid status filter")
	}

	query := bson.M{
		"type": jobType,
	}

	query = db.getStatusQuery(query, f)

	if db.opts.SingleGroup {
		query["group"] = db.opts.Group
	}

	ids, err := db.findJobs(ctx, query)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &JobReportIDs{
		Filter:     f,
		Type:       jobType,
		GroupedIDs: ids,
	}, nil
}

func (db *dbQueueManager) getStatusQuery(q bson.M, f StatusFilter) bson.M {
	switch f {
	case InProgress:
		q["status.in_prog"] = true
		q["status.completed"] = false
	case Pending:
		q["status.in_prog"] = false
		q["status.completed"] = false
	case Stale:
		q["status.in_prog"] = true
		q["status.completed"] = false
		q["status.mod_ts"] = bson.M{"$lte": time.Now().Add(-db.opts.Options.LockTimeout)}
	case Completed:
		q["status.in_prog"] = false
		q["status.completed"] = true
	case Retrying:
		q["status.in_prog"] = false
		q["status.completed"] = true
		q["retry_info.retryable"] = true
		q["retry_info.needs_retry"] = true
	case All:
		// pass
	default:
	}

	return q
}

func (db *dbQueueManager) RecentErrors(ctx context.Context, window time.Duration, f ErrorFilter) (*JobErrorsReport, error) {
	if window <= time.Second {
		return nil, errors.New("must specify windows greater than one second")
	}

	if err := f.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid error filter")

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

func (db *dbQueueManager) RecentJobErrors(ctx context.Context, jobType string, window time.Duration, f ErrorFilter) (*JobErrorsReport, error) {
	if window <= time.Second {
		return nil, errors.New("must specify windows greater than one second")
	}

	if err := f.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid error filter")
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

func (*dbQueueManager) getUpdateStatement() bson.M {
	return bson.M{
		"$set": bson.M{
			"status.completed":       true,
			"retry_info.needs_retry": false,
		},
		// Increment the mod count by an arbitrary amount to prevent any queue
		// threads from operating on it anymore.
		"$inc":   bson.M{"status.mod_count": 3},
		"$unset": bson.M{"scopes": 1},
	}
}

func (db *dbQueueManager) completeJobs(ctx context.Context, query bson.M, f StatusFilter) error {
	if err := f.Validate(); err != nil {
		return errors.Wrapf(err, "invalid status filter")
	}

	if db.opts.Group != "" {
		query["group"] = db.opts.Group
	}

	query = db.getStatusQuery(query, f)

	res, err := db.collection.UpdateMany(ctx, query, db.getUpdateStatement())
	grip.Info(message.Fields{
		"op":         "mark-jobs-complete",
		"collection": db.collection.Name(),
		"filter":     f,
		"modified":   res.ModifiedCount,
	})
	return errors.Wrap(err, "problem marking jobs complete")
}

// CompleteJob marks a job complete by ID. The ID matches internally-stored job
// IDs rather than the logical job ID.
func (db *dbQueueManager) CompleteJob(ctx context.Context, id string) error {
	matchID := bson.M{}
	matchRetryableJobID := bson.M{"retry_info.base_job_id": id}
	if db.opts.Group != "" {
		matchID["_id"] = db.buildCompoundID(db.opts.Group, id)
		matchID["group"] = db.opts.Group
		matchRetryableJobID["group"] = db.opts.Group
	} else {
		matchID["_id"] = id
	}

	query := bson.M{
		"$or": []bson.M{matchID, matchRetryableJobID},
	}

	res, err := db.collection.UpdateMany(ctx, query, db.getUpdateStatement())
	if err != nil {
		return errors.Wrap(err, "marking job complete")
	}
	if res.MatchedCount == 0 {
		return errors.New("no job matched")
	}

	return nil
}

// CompleteJobs marks all jobs complete that match the status filter.
func (db *dbQueueManager) CompleteJobs(ctx context.Context, f StatusFilter) error {
	return db.completeJobs(ctx, bson.M{}, f)
}

// CompleteJobsByType marks all jobs complete that match the status filter and
// job type.
func (db *dbQueueManager) CompleteJobsByType(ctx context.Context, f StatusFilter, jobType string) error {
	return db.completeJobs(ctx, bson.M{"type": jobType}, f)
}

// CompleteJobByPattern marks all jobs complete that match the status filter and
// pattern. Patterns should be in Perl compatible regular expression syntax
// (https://docs.mongodb.com/manual/reference/operator/query/regex/index.html).
// Patterns match on internally-stored job IDs rather than logical job IDs.
func (db *dbQueueManager) CompleteJobsByPattern(ctx context.Context, f StatusFilter, pattern string) error {
	return db.completeJobs(ctx, bson.M{"_id": bson.M{"$regex": pattern}}, f)
}

func (*dbQueueManager) buildCompoundID(prefix, id string) string {
	return fmt.Sprintf("%s.%s", prefix, id)
}
