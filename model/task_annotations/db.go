package task_annotations

import (
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

var (
	// bson fields for the TaskAnnotation struct
	IdKey             = bsonutil.MustHaveTag(TaskAnnotation{}, "Id")
	TaskIdKey         = bsonutil.MustHaveTag(TaskAnnotation{}, "TaskId")
	TaskExecutionKey  = bsonutil.MustHaveTag(TaskAnnotation{}, "TaskExecution")
	APIAnnotationKey  = bsonutil.MustHaveTag(TaskAnnotation{}, "APIAnnotation")
	UserAnnotationKey = bsonutil.MustHaveTag(TaskAnnotation{}, "UserAnnotation")
	MetadataKey       = bsonutil.MustHaveTag(TaskAnnotation{}, "Metadata")
)

const Collection = "task_annotation"

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
		return nil, errors.Wrap(err, "problem finding task annotation")
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

// Insert writes the task_annotation to the database.
func (annotation *TaskAnnotation) Insert() error {
	return db.Insert(Collection, annotation)
}

// Update updates one task_annotation.
func (annotation *TaskAnnotation) Update() error {
	return db.UpdateId(Collection, annotation.Id, annotation)
}

// Update updates one task_annotation.
func (annotation *TaskAnnotation) UpdateAPIAnnotation(id string, execution int, a Annotation, m *birch.Document) error {
	return errors.WithStack(db.Update(
		Collection,
		bson.M{
			TaskIdKey:        id,
			TaskExecutionKey: execution,
		},
		bson.M{
			"$set": bson.M{
				"api_annotation": a,
				"metadata":       m,
			},
		},
	))
}

// Update updates one task_annotation.
func (annotation *TaskAnnotation) UpdateUserAnnotation(id string, execution int, a Annotation, author string) error {
	var userAnnotation = a
	a.Source.Author = author
	a.Source.Time = time.Now()

	return errors.WithStack(db.Update(
		Collection,
		bson.M{
			TaskIdKey:        id,
			TaskExecutionKey: execution,
		},
		bson.M{
			"$set": bson.M{
				"api_annotation": userAnnotation,
			},
		},
	))
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

// ByTaskIdAndExecution returns a query for the entry with the given Task Id and
// execution number
func ByTaskIdAndExecution(id string, execution int) db.Q {
	return db.Query(bson.M{
		TaskIdKey:        id,
		TaskExecutionKey: execution,
	})
}

// FindAPIAnnotation returns the apiAnnotation for a given Task Id and
// execution number
func FindAPIAnnotation(id string, execution int) db.Q {
	return db.Query(bson.M{
		TaskIdKey:        id,
		TaskExecutionKey: execution,
	}).WithoutFields(UserAnnotationKey)
}

// FindUserAnnotatiion returns the userAnnotation for a given Task Id and
// execution number
func FindUserAnnotatiion(id string, execution int) db.Q {
	return db.Query(bson.M{
		TaskIdKey:        id,
		TaskExecutionKey: execution,
	}).WithoutFields(MetadataKey, APIAnnotationKey)
}
