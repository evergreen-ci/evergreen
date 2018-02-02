package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNoteStorage(t *testing.T) {
	Convey("With a test note to save", t, func() {
		So(db.Clear(NotesCollection), ShouldBeNil)
		n := Note{
			TaskId:       "t1",
			UnixNanoTime: time.Now().UnixNano(),
			Content:      "test note",
		}
		Convey("saving the note should work without error", func() {
			So(n.Upsert(), ShouldBeNil)

			Convey("the note should be retrievable", func() {
				n2, err := NoteForTask("t1")
				So(err, ShouldBeNil)
				So(n2, ShouldNotBeNil)
				So(*n2, ShouldResemble, n)
			})
			Convey("saving the note again should overwrite the existing note", func() {
				n3 := n
				n3.Content = "new content"
				So(n3.Upsert(), ShouldBeNil)
				n4, err := NoteForTask("t1")
				So(err, ShouldBeNil)
				So(n4, ShouldNotBeNil)
				So(n4.TaskId, ShouldEqual, "t1")
				So(n4.Content, ShouldEqual, n3.Content)
			})
		})
	})
}
