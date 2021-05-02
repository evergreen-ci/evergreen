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

func (m *dbQueueManager) aggregateCounters(ctx context.Context, stages ...bson.M) ([]JobTypeCount, error) {
	cursor, err := m.collection.Aggregate(ctx, stages, options.Aggregate().SetAllowDiskUse(true))
	if err != nil {
		return nil, errors.Wrap(err, "problem running aggregation")
	}

	catcher := grip.NewBasicCatcher()
	var out []JobTypeCount
	for cursor.Next(ctx) {
		res := struct {
			Type  string `bson:"_id"`
			Group string `bson:"group,omitempty"`
			Count int    `bson:"count"`
		}{}
		err = cursor.Decode(&res)
		if err != nil {
			catcher.Add(err)
			continue
		}
		out = append(out, JobTypeCount{
			Type:  res.Type,
			Group: res.Group,
			Count: res.Count,
		})
	}
	catcher.Add(cursor.Err())
	if catcher.HasErrors() {
		return nil, errors.Wrap(catcher.Resolve(), "problem running job counters aggregation")
	}

	return out, nil
}

func (m *dbQueueManager) findJobIDs(ctx context.Context, match bson.M) ([]GroupedID, error) {
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

	cursor, err := m.collection.Aggregate(ctx, stages, options.Aggregate().SetAllowDiskUse(true))
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

func (m *dbQueueManager) JobStatus(ctx context.Context, f StatusFilter) ([]JobTypeCount, error) {
	if err := f.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid status filter")
	}

	match := bson.M{}

	group := bson.M{
		"_id":   "$type",
		"count": bson.M{"$sum": 1},
	}

	if m.opts.SingleGroup {
		match["group"] = m.opts.Group
	} else if m.opts.ByGroups {
		group["_id"] = bson.M{"type": "$type", "group": "$group"}
	}

	match = m.getStatusQuery(match, f)

	stages := []bson.M{
		{"$match": match},
		{"$group": group},
	}

	if m.opts.ByGroups {
		stages = append(stages, bson.M{"$project": bson.M{
			"_id":   "$_id.type",
			"count": "$count",
			"group": "$_id.group",
		}})
	}

	counters, err := m.aggregateCounters(ctx, stages...)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return counters, nil
}

// JobIDsByState returns job IDs filtered by job type and status filter. The
// returned job IDs are the internally-stored job IDs.
func (m *dbQueueManager) JobIDsByState(ctx context.Context, jobType string, f StatusFilter) ([]GroupedID, error) {
	if err := f.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid status filter")
	}

	query := bson.M{
		"type": jobType,
	}

	query = m.getStatusQuery(query, f)

	if m.opts.SingleGroup {
		query["group"] = m.opts.Group
	}

	groupedIDs, err := m.findJobIDs(ctx, query)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return groupedIDs, nil
}

func (m *dbQueueManager) getStatusQuery(q bson.M, f StatusFilter) bson.M {
	switch f {
	case Pending:
		q = m.getPendingQuery(q)
	case InProgress:
		q = m.getInProgQuery(q)
	case Stale:
		q = m.getStaleQuery(m.getInProgQuery(q))
	case Completed:
		q = m.getCompletedQuery(q)
	case Retrying:
		q = m.getRetryingQuery(q)
	case StaleRetrying:
		q = m.getStaleQuery(m.getRetryingQuery(q))
	case All:
		// pass
	default:
	}

	return q
}
func (*dbQueueManager) getPendingQuery(q bson.M) bson.M {
	q["status.in_prog"] = false
	q["status.completed"] = false
	return q
}

func (*dbQueueManager) getInProgQuery(q bson.M) bson.M {
	q["status.in_prog"] = true
	q["status.completed"] = false
	return q
}

func (*dbQueueManager) getCompletedQuery(q bson.M) bson.M {
	q["status.in_prog"] = false
	q["status.completed"] = true
	return q
}

func (m *dbQueueManager) getRetryingQuery(q bson.M) bson.M {
	q = m.getCompletedQuery(q)
	q["retry_info.retryable"] = true
	q["retry_info.needs_retry"] = true
	return q
}

func (m *dbQueueManager) getStaleQuery(q bson.M) bson.M {
	q["status.mod_ts"] = bson.M{"$lte": time.Now().Add(-m.opts.Options.LockTimeout)}
	return q
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

func (m *dbQueueManager) completeJobs(ctx context.Context, query bson.M, f StatusFilter) error {
	if err := f.Validate(); err != nil {
		return errors.Wrapf(err, "invalid status filter")
	}

	if m.opts.Group != "" {
		query["group"] = m.opts.Group
	}

	query = m.getStatusQuery(query, f)

	res, err := m.collection.UpdateMany(ctx, query, m.getUpdateStatement())
	grip.Info(message.Fields{
		"op":         "mark-jobs-complete",
		"collection": m.collection.Name(),
		"filter":     f,
		"modified":   res.ModifiedCount,
	})
	return errors.Wrap(err, "problem marking jobs complete")
}

// CompleteJob marks a job complete by ID. The ID matches internally-stored job
// IDs rather than the logical job ID.
func (m *dbQueueManager) CompleteJob(ctx context.Context, id string) error {
	matchID := bson.M{}
	matchRetryableJobID := bson.M{"retry_info.base_job_id": id}
	if m.opts.Group != "" {
		matchID["_id"] = m.buildCompoundID(m.opts.Group, id)
		matchID["group"] = m.opts.Group
		matchRetryableJobID["group"] = m.opts.Group
	} else {
		matchID["_id"] = id
	}

	query := bson.M{
		"$or": []bson.M{matchID, matchRetryableJobID},
	}

	res, err := m.collection.UpdateMany(ctx, query, m.getUpdateStatement())
	if err != nil {
		return errors.Wrap(err, "marking job complete")
	}
	if res.MatchedCount == 0 {
		return errors.New("no job matched")
	}

	return nil
}

// CompleteJobs marks all jobs complete that match the status filter.
func (m *dbQueueManager) CompleteJobs(ctx context.Context, f StatusFilter) error {
	return m.completeJobs(ctx, bson.M{}, f)
}

// CompleteJobsByType marks all jobs complete that match the status filter and
// job type.
func (m *dbQueueManager) CompleteJobsByType(ctx context.Context, f StatusFilter, jobType string) error {
	return m.completeJobs(ctx, bson.M{"type": jobType}, f)
}

// CompleteJobByPattern marks all jobs complete that match the status filter and
// pattern. Patterns should be in Perl compatible regular expression syntax
// (https://docs.mongodb.com/manual/reference/operator/query/regex/index.html).
// Patterns match on internally-stored job IDs rather than logical job IDs.
func (m *dbQueueManager) CompleteJobsByPattern(ctx context.Context, f StatusFilter, pattern string) error {
	return m.completeJobs(ctx, bson.M{"_id": bson.M{"$regex": pattern}}, f)
}

func (*dbQueueManager) buildCompoundID(prefix, id string) string {
	return fmt.Sprintf("%s.%s", prefix, id)
}
