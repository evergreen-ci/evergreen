package annotations

import (
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
	TaskLinksKey       = bsonutil.MustHaveTag(TaskAnnotation{}, "TaskLinks")
)

const (
	Collection       = "task_annotations"
	UIRequester      = "ui"
	APIRequester     = "api"
	WebhookRequester = "webhook"
	MaxTaskLinks     = 1
)

// FindOne gets one TaskAnnotation for the given query.
func FindOne(query db.Q) (*TaskAnnotation, error) {
	annotation := &TaskAnnotation{}
	err := db.FindOneQ(Collection, query, annotation)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return annotation, err
}

// Find gets every TaskAnnotation matching the given query.
func Find(query db.Q) ([]TaskAnnotation, error) {
	annotations := []TaskAnnotation{}
	err := db.FindAllQ(Collection, query, &annotations)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "finding task annotations")
	}

	return annotations, err
}

func FindOneByTaskIdAndExecution(id string, execution int) (*TaskAnnotation, error) {
	return FindOne(db.Query(ByTaskIdAndExecution(id, execution)))
}

func FindByTaskIds(ids []string) ([]TaskAnnotation, error) {
	return Find(ByTaskIds(ids))
}

func FindByTaskId(id string) ([]TaskAnnotation, error) {
	return Find(db.Query(ByTaskId(id)))
}

// Upsert writes the task_annotation to the database.
func (a *TaskAnnotation) Upsert() error {
	set := bson.M{
		NoteKey:            a.Note,
		IssuesKey:          a.Issues,
		SuspectedIssuesKey: a.SuspectedIssues,
		CreatedIssuesKey:   a.CreatedIssues,
		TaskLinksKey:       a.TaskLinks,
	}
	if a.Metadata != nil {
		set[MetadataKey] = a.Metadata
	}
	_, err := db.Upsert(
		Collection,
		ByTaskIdAndExecution(a.TaskId, a.TaskExecution),
		bson.M{
			"$set": set,
		},
	)
	return err
}

// Update updates one task_annotation.
func (a *TaskAnnotation) Update() error {
	return db.UpdateId(Collection, a.Id, a)
}

// Remove removes one task_annotation.
func Remove(id string) error {
	return db.Remove(Collection, bson.M{IdKey: id})
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
