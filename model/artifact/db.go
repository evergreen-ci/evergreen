package artifact

import (
	"context"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"go.mongodb.org/mongo-driver/bson"
)

var (
	// BSON fields for artifact file structs
	TaskIdKey      = bsonutil.MustHaveTag(Entry{}, "TaskId")
	TaskNameKey    = bsonutil.MustHaveTag(Entry{}, "TaskDisplayName")
	BuildIdKey     = bsonutil.MustHaveTag(Entry{}, "BuildId")
	FilesKey       = bsonutil.MustHaveTag(Entry{}, "Files")
	ExecutionKey   = bsonutil.MustHaveTag(Entry{}, "Execution")
	CreateTimeKey  = bsonutil.MustHaveTag(Entry{}, "CreateTime")
	NameKey        = bsonutil.MustHaveTag(File{}, "Name")
	LinkKey        = bsonutil.MustHaveTag(File{}, "Link")
	ContentTypeKey = bsonutil.MustHaveTag(File{}, "ContentType")
	AWSSecretKey   = bsonutil.MustHaveTag(File{}, "AWSSecret")
	FileKeyKey     = bsonutil.MustHaveTag(File{}, "FileKey")
)

type TaskIDAndExecution struct {
	TaskID    string
	Execution int
}

// === Queries ===

// ByTaskId returns a query for entries with the given Task Id
func ByTaskId(id string) db.Q {
	return db.Query(bson.M{TaskIdKey: id})
}

// ByTaskIdAndExecution returns a query for entries with the given Task Id and
// execution number
func ByTaskIdAndExecution(id string, execution int) db.Q {
	return db.Query(bson.M{
		TaskIdKey:    id,
		ExecutionKey: execution,
	})
}

// ByTaskIdWithoutExecution returns a query for entries with the given Task Id
// that do not have an execution number associated with them
func ByTaskIdWithoutExecution(id string) db.Q {
	return db.Query(bson.M{
		TaskIdKey: id,
		ExecutionKey: bson.M{
			"$exists": false,
		},
	})
}

func ByTaskIdsAndExecutions(tasks []TaskIDAndExecution) db.Q {
	orClause := []bson.M{}
	for _, t := range tasks {
		orClause = append(orClause, bson.M{
			TaskIdKey:    t.TaskID,
			ExecutionKey: t.Execution,
		})
	}
	return db.Query(bson.M{
		"$or": orClause,
	})
}

func ByTaskIds(taskIds []string) db.Q {
	return db.Query(bson.M{
		TaskIdKey: bson.M{
			"$in": taskIds,
		},
	})
}

// ByBuildId returns all entries with the given Build Id, sorted by Task name
func ByBuildId(id string) db.Q {
	return db.Query(bson.M{BuildIdKey: id}).Sort([]string{TaskNameKey})
}

// === DB Logic ===

// Upsert updates the files entry in the db if an entry already exists,
// overwriting the existing file data. If no entry exists, one is created
func (e Entry) Upsert(ctx context.Context) error {
	_, err := db.Upsert(
		ctx,
		Collection,
		bson.M{
			TaskIdKey:    e.TaskId,
			TaskNameKey:  e.TaskDisplayName,
			BuildIdKey:   e.BuildId,
			ExecutionKey: e.Execution,
		},
		bson.M{
			"$addToSet": bson.M{
				FilesKey: bson.M{
					"$each": e.Files,
				},
			},
			"$setOnInsert": bson.M{
				ExecutionKey: e.Execution,
			},
		},
	)
	return err
}

// FindOne gets one Entry for the given query
func FindOne(ctx context.Context, query db.Q) (*Entry, error) {
	entry := &Entry{}
	err := db.FindOneQContext(ctx, Collection, query, entry)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return entry, err
}

// FindAll gets every Entry for the given query
func FindAll(ctx context.Context, query db.Q) ([]Entry, error) {
	entries := []Entry{}
	err := db.FindAllQ(ctx, Collection, query, &entries)
	return entries, err
}

// UpdateFileLink updates the link for a single artifact file matching task ID, execution,
// file name, and current link. Returns db.ErrNotFound if no file matched.
func UpdateFileLink(ctx context.Context, taskID string, execution int, fileName, currentLink, newLink string) error {
	filter := bson.M{
		TaskIdKey:    taskID,
		ExecutionKey: execution,
		FilesKey: bson.M{"$elemMatch": bson.M{
			NameKey: fileName,
			LinkKey: currentLink,
		}},
	}
	update := bson.M{"$set": bson.M{
		bsonutil.GetDottedKeyName(FilesKey, "$", LinkKey): newLink,
	}}
	return db.Update(ctx, Collection, filter, update)
}

// UpdateFileKey updates the S3 file key for a single artifact file matching task ID, execution,
// file name, and current file key. This is used for rotating S3 signed artifacts to point to
// a different object in the same bucket.
func UpdateFileKey(ctx context.Context, taskID string, execution int, fileName, currentFileKey, newFileKey string) error {
	filter := bson.M{
		TaskIdKey:    taskID,
		ExecutionKey: execution,
		FilesKey: bson.M{"$elemMatch": bson.M{
			NameKey:    fileName,
			FileKeyKey: currentFileKey,
		}},
	}
	update := bson.M{"$set": bson.M{
		bsonutil.GetDottedKeyName(FilesKey, "$", FileKeyKey): newFileKey,
	}}
	return db.Update(ctx, Collection, filter, update)
}
