package annotations

import (
	"context"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

var (
	// bson fields for the TaskAnnotation struct
	IdKey              = bsonutil.MustHaveTag(TaskAnnotation{}, "Id")
	TaskIdKey          = bsonutil.MustHaveTag(TaskAnnotation{}, "TaskId")
	TaskExecutionKey   = bsonutil.MustHaveTag(TaskAnnotation{}, "TaskExecution")
	MetadataKey        = bsonutil.MustHaveTag(TaskAnnotation{}, "Metadata")
	NoteKey            = bsonutil.MustHaveTag(TaskAnnotation{}, "Note")
	IssuesKey          = bsonutil.MustHaveTag(TaskAnnotation{}, "Issues")
	SuspectedIssuesKey = bsonutil.MustHaveTag(TaskAnnotation{}, "SuspectedIssues")
	CreatedIssuesKey   = bsonutil.MustHaveTag(TaskAnnotation{}, "CreatedIssues")
	IssueLinkIssueKey  = bsonutil.MustHaveTag(IssueLink{}, "IssueKey")
	MetadataLinksKey   = bsonutil.MustHaveTag(TaskAnnotation{}, "MetadataLinks")
)

const (
	Collection            = "task_annotations"
	UIRequester           = "ui"
	APIRequester          = "api"
	WebhookRequester      = "webhook"
	MaxMetadataLinks      = 1
	MaxMetadataTextLength = 40
)

// FindOne gets one TaskAnnotation for the given query.
func FindOne(ctx context.Context, query db.Q) (*TaskAnnotation, error) {
	annotation := &TaskAnnotation{}
	err := db.FindOneQContext(ctx, Collection, query, annotation)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return annotation, err
}

// Find gets every TaskAnnotation matching the given query.
func Find(ctx context.Context, query db.Q) ([]TaskAnnotation, error) {
	annotations := []TaskAnnotation{}
	err := db.FindAllQ(ctx, Collection, query, &annotations)
	if err != nil {
		return nil, errors.Wrap(err, "finding task annotations")
	}

	return annotations, err
}

func FindOneByTaskIdAndExecution(ctx context.Context, id string, execution int) (*TaskAnnotation, error) {
	return FindOne(ctx, db.Query(ByTaskIdAndExecution(id, execution)))
}

func FindByTaskIds(ctx context.Context, ids []string) ([]TaskAnnotation, error) {
	return Find(ctx, ByTaskIds(ids))
}

func FindByTaskId(ctx context.Context, id string) ([]TaskAnnotation, error) {
	return Find(ctx, db.Query(ByTaskId(id)))
}

// Upsert writes the task_annotation to the database.
func (a *TaskAnnotation) Upsert(ctx context.Context) error {
	set := bson.M{
		NoteKey:            a.Note,
		IssuesKey:          a.Issues,
		SuspectedIssuesKey: a.SuspectedIssues,
		CreatedIssuesKey:   a.CreatedIssues,
		MetadataLinksKey:   a.MetadataLinks,
	}
	if a.Metadata != nil {
		set[MetadataKey] = a.Metadata
	}
	_, err := db.Upsert(
		ctx,
		Collection,
		ByTaskIdAndExecution(a.TaskId, a.TaskExecution),
		bson.M{
			"$set": set,
		},
	)
	return err
}

// ByTaskId returns the query for a given Task Id
func ByTaskId(id string) bson.M {
	return bson.M{TaskIdKey: id}
}

func ByTaskIds(ids []string) db.Q {
	q := bson.M{TaskIdKey: bson.M{"$in": ids}}
	return db.Query(q)
}

// ByTaskIdAndExecution returns the query for a given Task Id and execution number
func ByTaskIdAndExecution(id string, execution int) bson.M {
	q := bson.M{
		TaskIdKey:        id,
		TaskExecutionKey: execution,
	}
	return q
}
