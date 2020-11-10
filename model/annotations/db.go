package annotations

import (
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

type AnnotationType int

const (
	APIAnnotationType AnnotationType = iota
	UserAnnotationType
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

func FindAPIAnnotationsByTaskIds(ids []string) ([]TaskAnnotation, error) {
	return Find(ByTaskIdsAndType(ids, APIAnnotationType))
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
				APIAnnotationKey: a,
				MetadataKey:      m,
			},
		},
	))
}

// Update updates one task_annotation.
func (annotation *TaskAnnotation) UpdateUserAnnotation(id string, execution int, a Annotation) error {
	return errors.WithStack(db.Update(
		Collection,
		bson.M{
			TaskIdKey:        id,
			TaskExecutionKey: execution,
		},
		bson.M{
			"$set": bson.M{
				UserAnnotationKey: a,
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

func ByTaskIdsAndType(ids []string, aType AnnotationType) db.Q {
	q := bson.M{TaskIdKey: bson.M{"$in": ids}}
	return queryWithType(q, aType)
}

// ByIdAndExecutionAndType returns the query for a given Task Id and
// execution number, populated according to the given type
func ByIdExecutionAndType(id string, execution int, aType AnnotationType) db.Q {
	q := bson.M{
		TaskIdKey:        id,
		TaskExecutionKey: execution,
	}
	return queryWithType(q, aType)
}

func queryWithType(q bson.M, aType AnnotationType) db.Q {
	switch aType {
	case APIAnnotationType:
		q[APIAnnotationKey] = bson.M{"$exists": true}
		return db.Query(q).WithoutFields(UserAnnotationKey)
	case UserAnnotationType:
		q[UserAnnotationKey] = bson.M{"$exists": true}
		return db.Query(q).WithoutFields(MetadataKey, APIAnnotationKey)
	}
	return db.Query(q)
}
