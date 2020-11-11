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
)

const (
	Collection = "task_annotation"
	UISource   = "UI"
	APISource  = "API"
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
		return nil, errors.Wrap(err, "problem finding task annotations")
	}

	return annotations, err
}

func FindByID(id string) (*TaskAnnotation, error) {
	annotation, err := FindOne(ById(id))
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "problem finding task annotation")
	}

	return annotation, nil
}

func FindAnnotationsByTaskIds(ids []string) ([]TaskAnnotation, error) {
	return Find(ByTaskIds(ids))
}

// Insert writes the task_annotation to the database.
func (a *TaskAnnotation) Insert() error {
	return db.Insert(Collection, a)
}

// Update updates one task_annotation.
func (a *TaskAnnotation) Update() error {
	return db.UpdateId(Collection, a.Id, a)
}

// Remove removes one task_annotation.
func Remove(id string) error {
	return db.Remove(Collection, bson.M{IdKey: id})
}

// ById returns a query that contains an Id selector on the string, id.
func ById(id string) db.Q {
	return db.Query(bson.M{IdKey: id})
}

// ByTaskId returns a query for entries with the given Task Id
func ByTaskId(taskId string) db.Q {
	return db.Query(bson.M{TaskIdKey: taskId})
}

func ByTaskIds(ids []string) db.Q {
	q := bson.M{TaskIdKey: bson.M{"$in": ids}}
	return db.Query(q)
}

// ByIdAndExecution returns the query for a given Task Id and execution number
func ByIdAndExecution(id string, execution int) db.Q {
	q := bson.M{
		TaskIdKey:        id,
		TaskExecutionKey: execution,
	}
	return db.Query(q)
}
