package queue

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
)

type mongoDriver struct {
	client     *mongo.Client
	name       string
	opts       MongoDBOptions
	instanceID string
	mu         sync.RWMutex
	canceler   context.CancelFunc
	dispatcher Dispatcher
}

// NewMongoDriver constructs a MongoDB backed queue driver
// implementation using the go.mongodb.org/mongo-driver as the
// database interface.
func newMongoDriver(name string, opts MongoDBOptions) (remoteQueueDriver, error) {
	host, _ := os.Hostname() // nolint

	if err := opts.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid mongo driver options")
	}

	return &mongoDriver{
		name:       name,
		opts:       opts,
		instanceID: fmt.Sprintf("%s.%s.%s", name, host, uuid.New()),
	}, nil
}

// openNewMongoDriver constructs and opens a new MongoDB driver instance
// using the specified session. It is equivalent to calling
// NewMongoDriver() and calling driver.Open().
func openNewMongoDriver(ctx context.Context, name string, opts MongoDBOptions, client *mongo.Client) (remoteQueueDriver, error) {
	d, err := newMongoDriver(name, opts)
	if err != nil {
		return nil, errors.Wrap(err, "could not create driver")
	}
	md, ok := d.(*mongoDriver)
	if !ok {
		return nil, errors.New("amboy programmer error: incorrect constructor")
	}

	if err := md.start(ctx, client); err != nil {
		return nil, errors.Wrap(err, "problem starting driver")
	}

	return d, nil
}

// newMongoGroupDriver is similar to the MongoDriver, except it
// prefixes job ids with a prefix and adds the group field to the
// documents in the database which makes it possible to manage
// distinct queues with a single MongoDB collection.
func newMongoGroupDriver(name string, opts MongoDBOptions, group string) (remoteQueueDriver, error) {
	host, _ := os.Hostname() // nolint

	if err := opts.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid mongo driver options")
	}
	opts.UseGroups = true
	opts.GroupName = group

	return &mongoDriver{
		name:       name,
		opts:       opts,
		instanceID: fmt.Sprintf("%s.%s.%s.%s", name, group, host, uuid.New()),
	}, nil
}

// OpenNewMongoGroupDriver constructs and opens a new MongoDB driver instance
// using the specified session. It is equivalent to calling
// NewMongoGroupDriver() and calling driver.Open().
func openNewMongoGroupDriver(ctx context.Context, name string, opts MongoDBOptions, group string, client *mongo.Client) (remoteQueueDriver, error) {
	d, err := newMongoGroupDriver(name, opts, group)
	if err != nil {
		return nil, errors.Wrap(err, "could not create driver")
	}
	md, ok := d.(*mongoDriver)
	if !ok {
		return nil, errors.New("amboy programmer error: incorrect constructor")
	}

	opts.UseGroups = true
	opts.GroupName = group

	if err := md.start(ctx, client); err != nil {
		return nil, errors.Wrap(err, "problem starting driver")
	}

	return d, nil
}

func (d *mongoDriver) ID() string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.instanceID
}

func (d *mongoDriver) Open(ctx context.Context) error {
	if d.canceler != nil {
		return nil
	}

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(d.opts.URI))
	if err != nil {
		return errors.Wrapf(err, "problem opening connection to mongodb at '%s", d.opts.URI)
	}

	return errors.Wrap(d.start(ctx, client), "problem starting driver")
}

func (d *mongoDriver) start(ctx context.Context, client *mongo.Client) error {
	dCtx, cancel := context.WithCancel(ctx)
	d.canceler = cancel

	d.mu.Lock()
	d.client = client
	d.mu.Unlock()

	startAt := time.Now()
	go func() {
		<-dCtx.Done()
		grip.Info(message.Fields{
			"message":  "closing session for mongodb driver",
			"id":       d.instanceID,
			"uptime":   time.Since(startAt),
			"span":     time.Since(startAt).String(),
			"service":  "amboy.queue.mdb",
			"is_group": d.opts.UseGroups,
			"group":    d.opts.GroupName,
		})
	}()

	if err := d.setupDB(ctx); err != nil {
		return errors.Wrap(err, "problem setting up database")
	}

	return nil
}

func (d *mongoDriver) getCollection() *mongo.Collection {
	db := d.client.Database(d.opts.DB)
	if d.opts.UseGroups {
		return db.Collection(addGroupSuffix(d.name))
	}

	return db.Collection(addJobsSuffix(d.name))
}

func (d *mongoDriver) setupDB(ctx context.Context) error {
	indexes := []mongo.IndexModel{}
	if !d.opts.SkipQueueIndexBuilds {
		indexes = append(indexes, d.queueIndexes()...)
	}
	if !d.opts.SkipReportingIndexBuilds {
		indexes = append(indexes, d.reportingIndexes()...)
	}

	if len(indexes) > 0 {
		_, err := d.getCollection().Indexes().CreateMany(ctx, indexes)
		return errors.Wrap(err, "problem building indexes")
	}

	return nil
}

func (d *mongoDriver) queueIndexes() []mongo.IndexModel {
	primary := bsonx.Doc{}
	retrying := bsonx.Doc{}
	retryableJobIDAndAttempt := bsonx.Doc{}
	scopes := bsonx.Doc{}

	if d.opts.UseGroups {
		group := bsonx.Elem{Key: "group", Value: bsonx.Int32(1)}
		primary = append(primary, group)
		retryableJobIDAndAttempt = append(retryableJobIDAndAttempt, group)
		retrying = append(retrying, group)
		scopes = append(scopes, group)
	}

	primary = append(primary,
		bsonx.Elem{
			Key:   "status.completed",
			Value: bsonx.Int32(1),
		},
		bsonx.Elem{
			Key:   "status.in_prog",
			Value: bsonx.Int32(1),
		})

	if d.opts.Priority {
		primary = append(primary, bsonx.Elem{
			Key:   "priority",
			Value: bsonx.Int32(1),
		})
	}

	if d.opts.CheckWaitUntil {
		primary = append(primary, bsonx.Elem{
			Key:   "time_info.wait_until",
			Value: bsonx.Int32(1),
		})
	} else if d.opts.CheckDispatchBy {
		primary = append(primary, bsonx.Elem{
			Key:   "time_info.dispatch_by",
			Value: bsonx.Int32(1),
		})
	}

	retryableJobIDAndAttempt = append(retryableJobIDAndAttempt,
		bsonx.Elem{
			Key:   "retry_info.base_job_id",
			Value: bsonx.Int32(1),
		},
		bsonx.Elem{
			Key:   "retry_info.current_attempt",
			Value: bsonx.Int32(-1),
		},
	)
	retrying = append(retrying,
		bsonx.Elem{
			Key:   "status.completed",
			Value: bsonx.Int32(1),
		},
		bsonx.Elem{
			Key:   "retry_info.retryable",
			Value: bsonx.Int32(1),
		},
		bsonx.Elem{
			Key:   "retry_info.needs_retry",
			Value: bsonx.Int32(1),
		},
		bsonx.Elem{
			Key:   "status.mod_ts",
			Value: bsonx.Int32(-1),
		},
	)
	scopes = append(scopes, bsonx.Elem{
		Key:   "scopes",
		Value: bsonx.Int32(1),
	})

	indexes := []mongo.IndexModel{
		{Keys: primary},
		{Keys: retryableJobIDAndAttempt},
		{
			Keys: retrying,
			// We have to shorten the index name because the index name length
			// is limited to 127 bytes for MongoDB 4.0.
			// Source: https://docs.mongodb.com/manual/reference/limits/#Index-Name-Length
			// TODO: this only affects tests. Remove the custom index name once
			// CI tests have upgraded to MongoDB 4.2+.
			Options: options.Index().SetName("retrying_jobs"),
		},
		{
			Keys:    scopes,
			Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"scopes": bson.M{"$exists": true}}),
		},
	}

	if d.opts.TTL > 0 {
		ttl := int32(d.opts.TTL / time.Second)
		indexes = append(indexes, mongo.IndexModel{
			Keys: bsonx.Doc{
				{
					Key:   "time_info.created",
					Value: bsonx.Int32(1),
				},
			},
			Options: options.Index().SetExpireAfterSeconds(ttl),
		})
	}

	return indexes
}

func (d *mongoDriver) reportingIndexes() []mongo.IndexModel {
	indexes := []mongo.IndexModel{}
	completedInProgModTs := bsonx.Doc{}
	completedEnd := bsonx.Doc{}
	completedCreated := bsonx.Doc{}
	typeCompletedInProgModTs := bsonx.Doc{}
	typeCompletedEnd := bsonx.Doc{}

	if d.opts.UseGroups {
		group := bsonx.Elem{Key: "group", Value: bsonx.Int32(1)}
		completedInProgModTs = append(completedInProgModTs, group)
		completedEnd = append(completedEnd, group)
		completedCreated = append(completedCreated, group)
		typeCompletedInProgModTs = append(typeCompletedInProgModTs, group)
		typeCompletedEnd = append(typeCompletedEnd, group)
	}

	completedInProgModTs = append(completedInProgModTs,
		bsonx.Elem{
			Key:   "status.completed",
			Value: bsonx.Int32(1),
		},
		bsonx.Elem{
			Key:   "status.in_prog",
			Value: bsonx.Int32(1),
		},
		bsonx.Elem{
			Key:   "status.mod_ts",
			Value: bsonx.Int32(1),
		},
	)
	indexes = append(indexes, mongo.IndexModel{Keys: completedInProgModTs})

	completedEnd = append(completedEnd,
		bsonx.Elem{
			Key:   "status.completed",
			Value: bsonx.Int32(1),
		},
		bsonx.Elem{
			Key:   "time_info.end",
			Value: bsonx.Int32(1),
		},
	)
	indexes = append(indexes, mongo.IndexModel{Keys: completedEnd})

	completedCreated = append(completedCreated,
		bsonx.Elem{
			Key:   "status.completed",
			Value: bsonx.Int32(1),
		},
		bsonx.Elem{
			Key:   "time_info.created",
			Value: bsonx.Int32(1),
		},
	)
	indexes = append(indexes, mongo.IndexModel{Keys: completedCreated})

	typeCompletedInProgModTs = append(typeCompletedInProgModTs,
		bsonx.Elem{
			Key:   "type",
			Value: bsonx.Int32(1),
		},
		bsonx.Elem{
			Key:   "status.completed",
			Value: bsonx.Int32(1),
		},
		bsonx.Elem{
			Key:   "status.in_prog",
			Value: bsonx.Int32(1),
		},
		bsonx.Elem{
			Key:   "status.mod_ts",
			Value: bsonx.Int32(1),
		},
	)
	indexes = append(indexes, mongo.IndexModel{Keys: typeCompletedInProgModTs})

	typeCompletedEnd = append(typeCompletedEnd,
		bsonx.Elem{
			Key:   "type",
			Value: bsonx.Int32(1),
		},
		bsonx.Elem{
			Key:   "status.completed",
			Value: bsonx.Int32(1),
		},
		bsonx.Elem{
			Key:   "time_info.end",
			Value: bsonx.Int32(1),
		},
	)
	indexes = append(indexes, mongo.IndexModel{Keys: typeCompletedEnd})

	return indexes
}

func (d *mongoDriver) Close() {
	if d.canceler != nil {
		d.canceler()
	}
}

func buildCompoundID(n, id string) string { return fmt.Sprintf("%s.%s", n, id) }

func deconstructCompoundID(id, prefix string) string {
	return strings.TrimPrefix(id, prefix+".")
}

func (d *mongoDriver) getIDWithGroup(name string) string {
	if d.opts.UseGroups {
		name = buildCompoundID(d.opts.GroupName, name)
	}

	return name
}

func (d *mongoDriver) addMetadata(j *registry.JobInterchange) {
	d.addRetryToMetadata(j)
	d.addGroupToMetadata(j)
}

func (d *mongoDriver) removeMetadata(j *registry.JobInterchange) {
	d.removeGroupFromMetadata(j)
	d.removeRetryFromMetadata(j)
}

func (d *mongoDriver) addGroupToMetadata(j *registry.JobInterchange) {
	if !d.opts.UseGroups {
		return
	}

	j.Group = d.opts.GroupName
	j.Name = buildCompoundID(d.opts.GroupName, j.Name)
}

func (d *mongoDriver) removeGroupFromMetadata(j *registry.JobInterchange) {
	if !d.opts.UseGroups {
		return
	}

	j.Name = deconstructCompoundID(j.Name, d.opts.GroupName)
}

func (d *mongoDriver) addRetryToMetadata(j *registry.JobInterchange) {
	if !j.RetryInfo.Retryable {
		return
	}

	j.RetryInfo.BaseJobID = j.Name
	j.Name = buildCompoundID(retryAttemptPrefix(j.RetryInfo.CurrentAttempt), j.Name)
}

func (d *mongoDriver) removeRetryFromMetadata(j *registry.JobInterchange) {
	if !j.RetryInfo.Retryable {
		return
	}

	j.Name = deconstructCompoundID(j.Name, retryAttemptPrefix(j.RetryInfo.CurrentAttempt))
}

func (d *mongoDriver) modifyQueryForGroup(q bson.M) {
	if !d.opts.UseGroups {
		return
	}

	q["group"] = d.opts.GroupName
}

func (d *mongoDriver) Get(ctx context.Context, name string) (amboy.Job, error) {
	j := &registry.JobInterchange{}

	byRetryAttempt := bson.M{
		"retry_info.current_attempt": -1,
	}
	res := d.getCollection().FindOne(ctx, d.getIDQuery(name), options.FindOne().SetSort(byRetryAttempt))
	if err := res.Err(); err != nil {
		return nil, errors.Wrapf(err, "GET problem fetching '%s'", name)
	}

	if err := res.Decode(j); err != nil {
		return nil, errors.Wrapf(err, "GET problem decoding '%s'", name)
	}

	output, err := j.Resolve(d.opts.Format)
	if err != nil {
		return nil, errors.Wrapf(err,
			"GET problem converting '%s' to job object", name)
	}

	return output, nil
}

// getIDQuery matches a job ID against either a retryable job or a non-retryable
// job.
func (d *mongoDriver) getIDQuery(id string) bson.M {
	matchID := bson.M{"_id": d.getIDWithGroup(id)}

	matchRetryable := bson.M{"retry_info.base_job_id": id}
	d.modifyQueryForGroup(matchRetryable)

	return bson.M{
		"$or": []bson.M{
			matchID,
			matchRetryable,
		},
	}
}

// GetAttempt finds a retryable job matching the given job ID and execution
// attempt. This returns an error if no matching job is found.
func (d *mongoDriver) GetAttempt(ctx context.Context, id string, attempt int) (amboy.Job, error) {
	matchIDAndAttempt := bson.M{
		"retry_info.base_job_id":     id,
		"retry_info.current_attempt": attempt,
	}
	d.modifyQueryForGroup(matchIDAndAttempt)

	res := d.getCollection().FindOne(ctx, matchIDAndAttempt)
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, amboy.NewJobNotFoundError("no such job found")
		}
		return nil, errors.Wrap(err, "finding job attempt")
	}

	ji := &registry.JobInterchange{}
	if err := res.Decode(ji); err != nil {
		return nil, errors.Wrap(err, "decoding job")
	}

	j, err := ji.Resolve(d.opts.Format)
	if err != nil {
		return nil, errors.Wrap(err, "converting serialized job format to in-memory job")
	}

	return j, nil
}

// GetAllAttempts finds all execution attempts of a retryable job for a given
// job ID. This returns an error if no matching job is found. The returned jobs
// are sorted by increasing attempt number.
func (d *mongoDriver) GetAllAttempts(ctx context.Context, id string) ([]amboy.Job, error) {
	matchID := bson.M{
		"retry_info.base_job_id": id,
	}
	d.modifyQueryForGroup(matchID)

	sortAttempt := bson.M{"retry_info.current_attempt": -1}

	cursor, err := d.getCollection().Find(ctx, matchID, options.Find().SetSort(sortAttempt))
	if err != nil {
		return nil, errors.Wrap(err, "finding all attempts")
	}

	jobInts := []registry.JobInterchange{}
	if err := cursor.All(ctx, &jobInts); err != nil {
		return nil, errors.Wrap(err, "decoding job")
	}
	if len(jobInts) == 0 {
		return nil, amboy.NewJobNotFoundError("no such job found")
	}

	jobs := make([]amboy.Job, len(jobInts))
	for i, ji := range jobInts {
		j, err := ji.Resolve(d.opts.Format)
		if err != nil {
			return nil, errors.Wrap(err, "converting serialized job format to in-memory job")
		}
		jobs[len(jobs)-i-1] = j
	}

	return jobs, nil
}

func (d *mongoDriver) Put(ctx context.Context, j amboy.Job) error {
	ji, err := registry.MakeJobInterchange(j, d.opts.Format)
	if err != nil {
		return errors.Wrap(err, "problem converting job to interchange format")
	}

	if j.ShouldApplyScopesOnEnqueue() {
		ji.Scopes = j.Scopes()
	}

	d.addMetadata(ji)

	if _, err = d.getCollection().InsertOne(ctx, ji); err != nil {
		if d.isMongoDupScope(err) {
			return amboy.NewDuplicateJobScopeErrorf("job scopes '%s' conflict", j.Scopes())
		}
		if isMongoDupKey(err) {
			return amboy.NewDuplicateJobErrorf("job '%s' already exists", j.ID())
		}

		return errors.Wrapf(err, "problem saving new job %s", j.ID())
	}

	return nil
}

func (d *mongoDriver) getAtomicQuery(jobName string, modCount int) bson.M {
	d.mu.RLock()
	defer d.mu.RUnlock()
	owner := d.instanceID
	timeoutTs := time.Now().Add(-d.LockTimeout())

	return bson.M{
		"_id": jobName,
		"$or": []bson.M{
			// owner and modcount should match, which
			// means there's an active lock but we own it.
			//
			// The modcount is +1 in the case that we're
			// looking to update and update the modcount
			// (rather than just save, as in the Complete
			// case).
			{
				"status.owner":     owner,
				"status.mod_count": bson.M{"$in": []int{modCount, modCount - 1}},
				"status.mod_ts":    bson.M{"$gt": timeoutTs},
			},
			// modtime is older than the lock timeout,
			// regardless of what the other data is,
			{"status.mod_ts": bson.M{"$lte": timeoutTs}},
		},
	}
}

var errMongoNoDocumentsMatched = errors.New("no documents matched")

func isMongoDupKey(err error) bool {
	dupKeyErrs := getMongoDupKeyErrors(err)
	return dupKeyErrs.writeConcernError != nil || len(dupKeyErrs.writeErrors) != 0 || dupKeyErrs.commandError != nil
}

func (d *mongoDriver) isMongoDupScope(err error) bool {
	dupKeyErrs := getMongoDupKeyErrors(err)
	var index string
	if d.opts.UseGroups {
		index = " group_1_scopes_1 "
	} else {
		index = " scopes_1 "
	}
	if wce := dupKeyErrs.writeConcernError; wce != nil {
		if strings.Contains(wce.Message, index) {
			return true
		}
	}

	for _, werr := range dupKeyErrs.writeErrors {
		if strings.Contains(werr.Message, index) {
			return true
		}
	}

	if ce := dupKeyErrs.commandError; ce != nil {
		if strings.Contains(ce.Message, index) {
			return true
		}
	}

	return false
}

type mongoDupKeyErrors struct {
	writeConcernError *mongo.WriteConcernError
	writeErrors       []mongo.WriteError
	commandError      *mongo.CommandError
}

func getMongoDupKeyErrors(err error) mongoDupKeyErrors {
	var dupKeyErrs mongoDupKeyErrors

	if we, ok := errors.Cause(err).(mongo.WriteException); ok {
		dupKeyErrs.writeConcernError = getMongoDupKeyWriteConcernError(we)
		dupKeyErrs.writeErrors = getMongoDupKeyWriteErrors(we)
	}

	if ce, ok := errors.Cause(err).(mongo.CommandError); ok {
		dupKeyErrs.commandError = getMongoDupKeyCommandError(ce)
	}

	return dupKeyErrs
}

// TODO: this logic is a copy-paste and could potentially be replaced by
// upgrading the Go driver to a newer version:
// (https://github.com/mongodb/mongo-go-driver/blob/213fb80b373f70dba4f9f516dc4c718abe41c76b/mongo/errors.go#L87-L96)
func getMongoDupKeyWriteConcernError(err mongo.WriteException) *mongo.WriteConcernError {
	wce := err.WriteConcernError
	if wce == nil {
		return nil
	}

	switch wce.Code {
	case 11000, 11001, 12582:
		return wce
	case 16460:
		if strings.Contains(wce.Message, " E11000 ") {
			return wce
		}
		return nil
	default:
		return nil
	}
}

func getMongoDupKeyWriteErrors(err mongo.WriteException) []mongo.WriteError {
	if len(err.WriteErrors) == 0 {
		return nil
	}

	var werrs []mongo.WriteError
	for _, werr := range err.WriteErrors {
		if werr.Code == 11000 {
			werrs = append(werrs, werr)
		}
	}

	return werrs
}

func getMongoDupKeyCommandError(err mongo.CommandError) *mongo.CommandError {
	switch err.Code {
	case 11000, 11001:
		return &err
	case 16460:
		if strings.Contains(err.Message, " E11000 ") {
			return &err
		}
		return nil
	default:
		return nil
	}
}

func (d *mongoDriver) Save(ctx context.Context, j amboy.Job) error {
	ji, err := d.prepareInterchange(j)
	if err != nil {
		return errors.WithStack(err)
	}

	ji.Scopes = j.Scopes()

	return errors.WithStack(d.doUpdate(ctx, ji))
}

func (d *mongoDriver) CompleteAndPut(ctx context.Context, toComplete amboy.Job, toPut amboy.Job) error {
	sess, err := d.client.StartSession()
	if err != nil {
		return errors.Wrap(err, "starting transaction session")
	}
	defer sess.EndSession(ctx)

	atomicCompleteAndPut := func(sessCtx mongo.SessionContext) (interface{}, error) {
		if err = d.Complete(sessCtx, toComplete); err != nil {
			return nil, errors.Wrap(err, "completing old job")
		}

		if err = d.Put(sessCtx, toPut); err != nil {
			return nil, errors.Wrap(err, "adding new job")
		}

		return nil, nil
	}

	if _, err = sess.WithTransaction(ctx, atomicCompleteAndPut); err != nil {
		return errors.Wrap(err, "atomic complete and put")
	}

	return nil
}

func (d *mongoDriver) Complete(ctx context.Context, j amboy.Job) error {
	ji, err := d.prepareInterchange(j)
	if err != nil {
		return errors.WithStack(err)
	}

	// It is safe to drop the scopes now in all cases except for one - if the
	// job still needs to retry and applies its scopes immediately to the retry
	// job, we cannot let go of the scopes yet because they will need to be
	// safely transferred to the retry job.
	if !ji.RetryInfo.ShouldRetry() || !ji.ApplyScopesOnEnqueue {
		ji.Scopes = nil
	}

	return errors.WithStack(d.doUpdate(ctx, ji))
}

func (d *mongoDriver) prepareInterchange(j amboy.Job) (*registry.JobInterchange, error) {
	stat := j.Status()
	stat.ErrorCount = len(stat.Errors)
	stat.ModificationTime = time.Now()
	j.SetStatus(stat)

	ji, err := registry.MakeJobInterchange(j, d.opts.Format)
	if err != nil {
		return nil, errors.Wrap(err, "problem converting job to interchange format")
	}
	return ji, nil
}

func (d *mongoDriver) doUpdate(ctx context.Context, ji *registry.JobInterchange) error {
	d.addMetadata(ji)

	query := d.getAtomicQuery(ji.Name, ji.Status.ModificationCount)
	res, err := d.getCollection().ReplaceOne(ctx, query, ji)
	if err != nil {
		if d.isMongoDupScope(err) {
			return amboy.NewDuplicateJobScopeErrorf("job scopes '%s' conflict", ji.Scopes)
		}
		return errors.Wrapf(err, "problem saving document %s: %+v", ji.Name, res)
	}

	if res.MatchedCount == 0 {
		return amboy.NewJobNotFoundErrorf("unmatched job [id=%s, matched=%d, modified=%d]", ji.Name, res.MatchedCount, res.ModifiedCount)
	}

	return nil
}

func (d *mongoDriver) Jobs(ctx context.Context) <-chan amboy.Job {
	output := make(chan amboy.Job)
	go func() {
		defer func() {
			if err := recovery.HandlePanicWithError(recover(), nil, "getting jobs"); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message":   "failed while getting jobs from the DB",
					"operation": "job iterator",
					"service":   "amboy.queue.mdb",
					"driver_id": d.ID(),
				}))
			}
			close(output)
		}()
		q := bson.M{}
		d.modifyQueryForGroup(q)

		iter, err := d.getCollection().Find(ctx, q, options.Find().SetSort(bson.M{"status.mod_ts": -1}))
		if err != nil {
			grip.Warning(message.WrapError(err, message.Fields{
				"id":        d.instanceID,
				"service":   "amboy.queue.mdb",
				"is_group":  d.opts.UseGroups,
				"group":     d.opts.GroupName,
				"operation": "job iterator",
				"message":   "problem with query",
			}))
			return
		}
		for iter.Next(ctx) {
			ji := &registry.JobInterchange{}
			if err = iter.Decode(ji); err != nil {
				grip.Warning(message.WrapError(err, message.Fields{
					"id":        d.instanceID,
					"service":   "amboy.queue.mdb",
					"is_group":  d.opts.UseGroups,
					"group":     d.opts.GroupName,
					"operation": "job iterator",
					"message":   "problem reading job from cursor",
				}))

				continue
			}

			var j amboy.Job
			j, err = ji.Resolve(d.opts.Format)
			if err != nil {
				grip.Warning(message.WrapError(err, message.Fields{
					"id":        d.instanceID,
					"service":   "amboy.queue.mdb",
					"operation": "job iterator",
					"is_group":  d.opts.UseGroups,
					"group":     d.opts.GroupName,
					"message":   "problem converting job obj",
				}))
				continue
			}

			select {
			case <-ctx.Done():
				return
			case output <- j:
			}
		}

		grip.Error(message.WrapError(iter.Err(), message.Fields{
			"id":        d.instanceID,
			"service":   "amboy.queue.mdb",
			"is_group":  d.opts.UseGroups,
			"group":     d.opts.GroupName,
			"operation": "job iterator",
			"message":   "database iterator error",
		}))
	}()

	return output
}

func (d *mongoDriver) RetryableJobs(ctx context.Context, filter retryableJobFilter) <-chan amboy.Job {
	jobs := make(chan amboy.Job)

	go func() {
		defer func() {
			if err := recovery.HandlePanicWithError(recover(), nil, "getting retryable jobs"); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message":   "failed while getting retryable jobs from the DB",
					"operation": "retryable job iterator",
					"service":   "amboy.queue.mdb",
					"driver_id": d.ID(),
				}))
			}
			close(jobs)
		}()

		var q bson.M
		switch filter {
		case retryableJobAll:
			q = bson.M{"retry_info.retryable": true}
		case retryableJobAllRetrying:
			q = d.getRetryingQuery(bson.M{})
		case retryableJobActiveRetrying:
			q = d.getRetryingQuery(bson.M{})
			q["status.mod_ts"] = bson.M{"$gte": time.Now().Add(-d.LockTimeout())}
		case retryableJobStaleRetrying:
			q = d.getRetryingQuery(bson.M{})
			q["status.mod_ts"] = bson.M{"$lte": time.Now().Add(-d.LockTimeout())}
		default:
			return
		}
		d.modifyQueryForGroup(q)

		iter, err := d.getCollection().Find(ctx, q, options.Find().SetSort(bson.M{"status.mod_ts": -1}))
		if err != nil {
			grip.Warning(message.WrapError(err, message.Fields{
				"id":        d.instanceID,
				"service":   "amboy.queue.mdb",
				"is_group":  d.opts.UseGroups,
				"group":     d.opts.GroupName,
				"operation": "retryable job iterator",
				"message":   "problem with query",
			}))
			return
		}
		for iter.Next(ctx) {
			ji := &registry.JobInterchange{}
			if err = iter.Decode(ji); err != nil {
				grip.Warning(message.WrapError(err, message.Fields{
					"id":        d.instanceID,
					"service":   "amboy.queue.mdb",
					"is_group":  d.opts.UseGroups,
					"group":     d.opts.GroupName,
					"operation": "retryable job iterator",
					"message":   "problem reading job from cursor",
				}))
				continue
			}

			var j amboy.Job
			j, err = ji.Resolve(d.opts.Format)
			if err != nil {
				grip.Warning(message.WrapError(err, message.Fields{
					"id":        d.instanceID,
					"service":   "amboy.queue.mdb",
					"operation": "retryable job iterator",
					"is_group":  d.opts.UseGroups,
					"group":     d.opts.GroupName,
					"message":   "problem converting job object",
				}))
				continue
			}

			select {
			case <-ctx.Done():
				return
			case jobs <- j:
			}
		}

		grip.Error(message.WrapError(iter.Err(), message.Fields{
			"id":        d.instanceID,
			"service":   "amboy.queue.mdb",
			"is_group":  d.opts.UseGroups,
			"group":     d.opts.GroupName,
			"operation": "retryable job iterator",
			"message":   "database iterator error",
		}))
	}()

	return jobs
}

// JobInfo returns a channel that produces information about all jobs stored in
// the DB. Job information is returned in order of decreasing modification time.
func (d *mongoDriver) JobInfo(ctx context.Context) <-chan amboy.JobInfo {
	infos := make(chan amboy.JobInfo)
	go func() {
		defer close(infos)
		q := bson.M{}
		d.modifyQueryForGroup(q)

		iter, err := d.getCollection().Find(ctx,
			q,
			&options.FindOptions{
				Sort: bson.M{"status.mod_ts": -1},
				Projection: bson.M{
					"_id":        1,
					"status":     1,
					"retry_info": 1,
					"time_info":  1,
					"type":       1,
					"version":    1,
				},
			})
		if err != nil {
			grip.Warning(message.WrapError(err, message.Fields{
				"id":        d.instanceID,
				"service":   "amboy.queue.mdb",
				"operation": "job info iterator",
				"message":   "problem with query",
				"is_group":  d.opts.UseGroups,
				"group":     d.opts.GroupName,
			}))
			return
		}

		for iter.Next(ctx) {
			ji := &registry.JobInterchange{}
			if err := iter.Decode(ji); err != nil {
				grip.Warning(message.WrapError(err, message.Fields{
					"id":        d.instanceID,
					"service":   "amboy.queue.mdb",
					"operation": "job info iterator",
					"message":   "problem converting job obj",
					"is_group":  d.opts.UseGroups,
					"group":     d.opts.GroupName,
				}))
				continue
			}

			d.removeMetadata(ji)
			info := amboy.JobInfo{
				ID:     ji.Name,
				Status: ji.Status,
				Time:   ji.TimeInfo,
				Retry:  ji.RetryInfo,
				Type: amboy.JobType{
					Name:    ji.Type,
					Version: ji.Version,
				},
			}

			select {
			case <-ctx.Done():
				return
			case infos <- info:
			}

		}
	}()

	return infos
}

func (d *mongoDriver) getNextQuery() bson.M {
	lockTimeout := d.LockTimeout()
	now := time.Now()
	qd := bson.M{
		"$or": []bson.M{
			d.getPendingQuery(bson.M{}),
			d.getInProgQuery(
				bson.M{"status.mod_ts": bson.M{"$lte": now.Add(-lockTimeout)}},
			),
		},
	}

	d.modifyQueryForGroup(qd)

	timeLimits := bson.M{}
	if d.opts.CheckWaitUntil {
		timeLimits["time_info.wait_until"] = bson.M{"$lte": now}
	}
	if d.opts.CheckDispatchBy {
		timeLimits["$or"] = []bson.M{
			{"time_info.dispatch_by": bson.M{"$gt": now}},
			{"time_info.dispatch_by": time.Time{}},
		}
	}
	if len(timeLimits) > 0 {
		qd = bson.M{"$and": []bson.M{qd, timeLimits}}
	}
	return qd
}

func (d *mongoDriver) Next(ctx context.Context) amboy.Job {
	var (
		qd             bson.M
		job            amboy.Job
		misses         int
		dispatchMisses int
		dispatchSkips  int
	)

	startAt := time.Now()
	defer func() {
		grip.WarningWhen(
			time.Since(startAt) > time.Second,
			message.Fields{
				"duration_secs": time.Since(startAt).Seconds(),
				"service":       "amboy.queue.mdb",
				"operation":     "next job",
				"attempts":      dispatchMisses,
				"skips":         dispatchSkips,
				"misses":        misses,
				"dispatched":    job != nil,
				"message":       "slow job dispatching operation",
				"id":            d.instanceID,
				"is_group":      d.opts.UseGroups,
				"group":         d.opts.GroupName,
			})
	}()

	opts := options.Find()
	if d.opts.Priority {
		opts.SetSort(bson.M{"priority": -1})
	}

	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			misses++
			qd = d.getNextQuery()
			iter, err := d.getCollection().Find(ctx, qd, opts)
			if err != nil {
				grip.Warning(message.WrapError(err, message.Fields{
					"id":            d.instanceID,
					"service":       "amboy.queue.mdb",
					"operation":     "retrieving next job",
					"message":       "problem generating query",
					"is_group":      d.opts.UseGroups,
					"group":         d.opts.GroupName,
					"duration_secs": time.Since(startAt).Seconds(),
				}))
				return nil
			}

			job, dispatchInfo := d.tryDispatchJob(ctx, iter, startAt)
			dispatchMisses += dispatchInfo.misses
			dispatchSkips += dispatchInfo.skips
			if err = iter.Err(); err != nil {
				if job != nil {
					d.dispatcher.Release(ctx, job)
				}
				grip.Warning(message.WrapError(err, message.Fields{
					"id":            d.instanceID,
					"service":       "amboy.queue.mdb",
					"message":       "problem reported by iterator",
					"operation":     "retrieving next job",
					"is_group":      d.opts.UseGroups,
					"group":         d.opts.GroupName,
					"duration_secs": time.Since(startAt).Seconds(),
				}))
				return nil
			}

			if err = iter.Close(ctx); err != nil {
				if job != nil {
					d.dispatcher.Release(ctx, job)
				}
				grip.Warning(message.WrapError(err, message.Fields{
					"id":        d.instanceID,
					"service":   "amboy.queue.mdb",
					"message":   "problem closing iterator",
					"operation": "retrieving next job",
					"is_group":  d.opts.UseGroups,
					"group":     d.opts.GroupName,
				}))
				return nil
			}

			if job != nil {
				return job
			}

			timer.Reset(time.Duration(misses * rand.Intn(int(d.opts.WaitInterval))))
		}
	}
}

// dispatchAttemptInfo contains aggregate statistics on an attempt to dispatch
// jobs.
type dispatchAttemptInfo struct {
	// skips is the number of times it encountered a job that was not yet ready
	// to dispatch.
	skips int
	// misses is the number of times the dispatcher attempted to dispatch a job
	// but failed.
	misses int
}

// tryDispatchJob takes an iterator over the candidate Amboy jobs and attempts
// to dispatch one of them. If it succeeds, it returns the successfully
// dispatched job.
func (d *mongoDriver) tryDispatchJob(ctx context.Context, iter *mongo.Cursor, startAt time.Time) (amboy.Job, dispatchAttemptInfo) {
	var dispatchInfo dispatchAttemptInfo
	for iter.Next(ctx) {
		ji := &registry.JobInterchange{}
		if err := iter.Decode(ji); err != nil {
			grip.Warning(message.WrapError(err, message.Fields{
				"id":            d.instanceID,
				"service":       "amboy.queue.mdb",
				"operation":     "converting next job",
				"message":       "problem reading document from cursor",
				"is_group":      d.opts.UseGroups,
				"group":         d.opts.GroupName,
				"duration_secs": time.Since(startAt).Seconds(),
			}))
			// try for the next thing in the iterator if we can
			continue
		}

		j, err := ji.Resolve(d.opts.Format)
		if err != nil {
			grip.Warning(message.WrapError(err, message.Fields{
				"id":            d.instanceID,
				"service":       "amboy.queue.mdb",
				"operation":     "converting document",
				"message":       "problem converting job from intermediate form",
				"is_group":      d.opts.UseGroups,
				"group":         d.opts.GroupName,
				"duration_secs": time.Since(startAt).Seconds(),
			}))
			// try for the next thing in the iterator if we can
			continue
		}

		if j.TimeInfo().IsStale() {
			res, err := d.getCollection().DeleteOne(ctx, bson.M{"_id": ji.Name})
			msg := message.Fields{
				"id":            d.instanceID,
				"service":       "amboy.queue.mdb",
				"num_deleted":   res.DeletedCount,
				"message":       "found stale job",
				"operation":     "job staleness check",
				"job_id":        j.ID(),
				"job_type":      j.Type().Name,
				"is_group":      d.opts.UseGroups,
				"group":         d.opts.GroupName,
				"duration_secs": time.Since(startAt).Seconds(),
			}
			grip.Warning(message.WrapError(err, msg))
			grip.NoticeWhen(err == nil, msg)
			continue
		}

		lockTimeout := d.LockTimeout()
		if !isDispatchable(j.Status(), lockTimeout) {
			dispatchInfo.skips++
			continue
		} else if d.isOtherJobHoldingScopes(ctx, j) {
			dispatchInfo.skips++
			continue
		}

		if err = d.dispatcher.Dispatch(ctx, j); err != nil {
			dispatchInfo.misses++
			grip.DebugWhen(isDispatchable(j.Status(), lockTimeout) && !d.isMongoDupScope(err),
				message.WrapError(err, message.Fields{
					"id":            d.instanceID,
					"service":       "amboy.queue.mdb",
					"operation":     "dispatch job",
					"job_id":        j.ID(),
					"job_type":      j.Type().Name,
					"scopes":        j.Scopes(),
					"stat":          j.Status(),
					"is_group":      d.opts.UseGroups,
					"group":         d.opts.GroupName,
					"dup_key":       isMongoDupKey(err),
					"duration_secs": time.Since(startAt).Seconds(),
				}),
			)
			continue
		}

		return j, dispatchInfo
	}

	return nil, dispatchInfo
}

// isOtherJobHoldingScopes checks whether or not a different job is already
// holding the scopes required for this job.
func (d *mongoDriver) isOtherJobHoldingScopes(ctx context.Context, j amboy.Job) bool {
	if len(j.Scopes()) == 0 {
		return false
	}

	query := bson.M{
		"scopes": bson.M{"$in": j.Scopes()},
		"$not":   d.getIDQuery(j.ID()),
	}
	d.modifyQueryForGroup(query)
	num, err := d.getCollection().CountDocuments(ctx, query)
	if err != nil {
		return false
	}
	return num > 0
}

func (d *mongoDriver) Stats(ctx context.Context) amboy.QueueStats {
	coll := d.getCollection()

	statusFilter := bson.M{
		"$or": []bson.M{
			d.getPendingQuery(bson.M{}),
			d.getInProgQuery(bson.M{}),
			d.getRetryingQuery(bson.M{}),
		},
	}
	d.modifyQueryForGroup(statusFilter)
	matchStatus := bson.M{"$match": statusFilter}
	groupStatuses := bson.M{
		"$group": bson.M{
			"_id": bson.M{
				"completed":   "$status.completed",
				"in_prog":     "$status.in_prog",
				"needs_retry": "$retry_info.needs_retry",
			},
			"count": bson.M{"$sum": 1},
		},
	}
	pipeline := []bson.M{matchStatus, groupStatuses}

	c, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"id":         d.instanceID,
			"service":    "amboy.queue.mdb",
			"collection": coll.Name(),
			"is_group":   d.opts.UseGroups,
			"group":      d.opts.GroupName,
			"operation":  "queue stats",
			"message":    "could not count documents by status",
		}))
		return amboy.QueueStats{}
	}
	statusGroups := []struct {
		ID struct {
			Completed  bool `bson:"completed"`
			InProg     bool `bson:"in_prog"`
			NeedsRetry bool `bson:"needs_retry"`
		} `bson:"_id"`
		Count int `bson:"count"`
	}{}
	if err := c.All(ctx, &statusGroups); err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"id":         d.instanceID,
			"service":    "amboy.queue.mdb",
			"collection": coll.Name(),
			"is_group":   d.opts.UseGroups,
			"group":      d.opts.GroupName,
			"operation":  "queue stats",
			"message":    "failed to decode counts by status",
		}))
		return amboy.QueueStats{}
	}

	var pending, inProg, retrying int
	for _, group := range statusGroups {
		if !group.ID.InProg && !group.ID.Completed {
			pending += group.Count
		}
		if group.ID.InProg {
			inProg += group.Count
		}
		if group.ID.Completed && group.ID.NeedsRetry {
			retrying += group.Count
		}
	}

	// The aggregation cannot also count all the documents in the collection
	// without a collection scan, so query it separately. Because completed is
	// calculated between two non-atomic queries, the statistics could be
	// inconsistent (i.e. you could have an incorrect count of completed jobs).

	var total int64
	if d.opts.UseGroups {
		total, err = coll.CountDocuments(ctx, bson.M{"group": d.opts.GroupName})
	} else {
		total, err = coll.EstimatedDocumentCount(ctx)
	}
	grip.Warning(message.WrapError(err, message.Fields{
		"id":         d.instanceID,
		"service":    "amboy.queue.mdb",
		"collection": coll.Name(),
		"operation":  "queue stats",
		"is_group":   d.opts.UseGroups,
		"group":      d.opts.GroupName,
		"message":    "problem counting total jobs",
	}))

	completed := int(total) - pending - inProg

	return amboy.QueueStats{
		Total:     int(total),
		Pending:   pending,
		Running:   inProg,
		Completed: completed,
		Retrying:  retrying,
	}
}

// getPendingQuery modifies the query to find jobs that have not started yet.
func (d *mongoDriver) getPendingQuery(q bson.M) bson.M {
	q["status.completed"] = false
	q["status.in_prog"] = false
	return q
}

// getInProgQuery modifies the query to find jobs that are in progress.
func (d *mongoDriver) getInProgQuery(q bson.M) bson.M {
	q["status.completed"] = false
	q["status.in_prog"] = true
	return q
}

// getCompletedQuery modifies the query to find jobs that are completed.
func (d *mongoDriver) getCompletedQuery(q bson.M) bson.M {
	q["status.completed"] = true
	return q
}

// getRetryingQuery modifies the query to find jobs that are retrying.
func (d *mongoDriver) getRetryingQuery(q bson.M) bson.M {
	q = d.getCompletedQuery(q)
	q["retry_info.retryable"] = true
	q["retry_info.needs_retry"] = true
	return q
}

func (d *mongoDriver) LockTimeout() time.Duration {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.opts.LockTimeout
}

func (d *mongoDriver) Dispatcher() Dispatcher {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.dispatcher
}

func (d *mongoDriver) SetDispatcher(disp Dispatcher) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dispatcher = disp
}
