package model

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"go.mongodb.org/mongo-driver/bson"
)

const NotesCollection = "build_baron_notes"

// Note contains arbitrary information entered by an Evergreen user, scope to a task.
type Note struct {
	TaskId       string `bson:"_id" json:"-"`
	UnixNanoTime int64  `bson:"time" json:"time"`
	Content      string `bson:"content" json:"content"`
}

// Note DB Logic

var NoteTaskIdKey = bsonutil.MustHaveTag(Note{}, "TaskId")

// Upsert overwrites an existing note.
func (n *Note) Upsert() error {
	_, err := db.Upsert(
		NotesCollection,
		bson.M{NoteTaskIdKey: n.TaskId},
		n,
	)
	return err
}

// NoteForTask returns the note for the given task Id, if it exists.
func NoteForTask(taskId string) (*Note, error) {
	n := &Note{}
	err := db.FindOneQ(
		NotesCollection,
		db.Query(bson.M{NoteTaskIdKey: taskId}),
		n,
	)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return n, err
}
