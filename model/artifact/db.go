package artifact

import (
	"10gen.com/mci/db"
	"10gen.com/mci/db/bsonutil"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
)

var (
	// BSON fields for artifact file structs
	TaskIdKey   = bsonutil.MustHaveTag(Entry{}, "TaskId")
	TaskNameKey = bsonutil.MustHaveTag(Entry{}, "TaskDisplayName")
	BuildIdKey  = bsonutil.MustHaveTag(Entry{}, "BuildId")
	FilesKey    = bsonutil.MustHaveTag(Entry{}, "Files")
	NameKey     = bsonutil.MustHaveTag(File{}, "Name")
	LinkKey     = bsonutil.MustHaveTag(File{}, "Link")
)

// === Queries ===

// ByTaskId returns a query for entries with the given Task Id
func ByTaskId(id string) db.Q {
	return db.Query(bson.D{{TaskIdKey, id}})
}

// ByBuildId returns all entries with the given Build Id, sorted by Task name
func ByBuildId(id string) db.Q {
	return db.Query(bson.D{{BuildIdKey, id}}).Sort([]string{TaskNameKey})
}

// === DB Logic ===

// Upsert updates the files entry in the db if an entry already exists,
// overwriting the existing file data. If no entry exists, one is created
func (e Entry) Upsert() error {
	for _, file := range e.Files {
		_, err := db.Upsert(
			Collection,
			bson.M{
				TaskIdKey:   e.TaskId,
				TaskNameKey: e.TaskDisplayName,
				BuildIdKey:  e.BuildId,
			},
			bson.M{
				"$addToSet": bson.M{
					FilesKey: file,
				},
			},
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// FindOne ets one Entry for the given query
func FindOne(query db.Q) (*Entry, error) {
	entry := &Entry{}
	err := db.FindOneQ(Collection, query, entry)
	if err == mgo.ErrNotFound {
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
