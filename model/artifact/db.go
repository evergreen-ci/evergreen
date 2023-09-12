package artifact

import (
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
	AwsSecretKey   = bsonutil.MustHaveTag(File{}, "AwsSecret")
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

func BySecret(secret string) db.Q {
	return db.Query(bson.M{
		FilesKey: bson.M{
			"$elemMatch": bson.M{
				AwsSecretKey: secret,
			},
		},
	})
}

// === DB Logic ===

// Upsert updates the files entry in the db if an entry already exists,
// overwriting the existing file data. If no entry exists, one is created
func (e Entry) Upsert() error {
	_, err := db.Upsert(
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

func (e Entry) Update() error {
	update := bson.M{
		TaskIdKey:   e.TaskId,
		TaskNameKey: e.TaskDisplayName,
		BuildIdKey:  e.BuildId,
	}
	if e.Execution == 0 {
		update["$or"] = []bson.M{
			bson.M{ExecutionKey: bson.M{"$exists": false}},
			bson.M{ExecutionKey: 0},
		}
	} else {
		update[ExecutionKey] = e.Execution
	}

	err := db.Update(
		Collection,
		update,
		bson.M{
			"$set": bson.M{
				FilesKey: e.Files,
			},
		},
	)

	return err
}

// FindOne gets one Entry for the given query
func FindOne(query db.Q) (*Entry, error) {
	entry := &Entry{}
	err := db.FindOneQ(Collection, query, entry)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return entry, err
}

// FindAll gets every Entry for the given query
func FindAll(query db.Q) ([]Entry, error) {
	entries := []Entry{}
	err := db.FindAllQ(Collection, query, &entries)
	return entries, err
}
