package model

import (
	"10gen.com/mci/db"
	"10gen.com/mci/util"
	. "github.com/smartystreets/goconvey/convey"
	"labix.org/v2/mgo/bson"
	"testing"
)

func reset(t *testing.T) {
	util.HandleTestingErr(
		db.Clear(ArtifactFilesCollection),
		t, "Error clearing collection")
}

func TestArtifactFileEntryUpsert(t *testing.T) {
	Convey("With an artifact file entry", t, func() {
		reset(t)

		testEntry := ArtifactFileEntry{
			TaskId:          "task1",
			TaskDisplayName: "Task One",
			BuildId:         "build1",
			Files: []ArtifactFile{
				{"cat_pix", "http://placekitten.com/800/600"},
				{"fast_download", "https://fastdl.mongodb.org"},
			},
		}

		Convey("upsert should succeed", func() {
			So(testEntry.Upsert(), ShouldBeNil)

			Convey("so all fields should be present in the db", func() {
				entryFromDb, err := FindOneArtifactFileEntryByTask("task1")
				So(err, ShouldBeNil)
				So(entryFromDb.TaskId, ShouldEqual, "task1")
				So(entryFromDb.TaskDisplayName, ShouldEqual, "Task One")
				So(entryFromDb.BuildId, ShouldEqual, "build1")
				So(len(entryFromDb.Files), ShouldEqual, 2)
				So(entryFromDb.Files[0].Name, ShouldEqual, "cat_pix")
				So(entryFromDb.Files[0].Link, ShouldEqual, "http://placekitten.com/800/600")
				So(entryFromDb.Files[1].Name, ShouldEqual, "fast_download")
				So(entryFromDb.Files[1].Link, ShouldEqual, "https://fastdl.mongodb.org")
			})

			Convey("and with a following update", func() {
				// reusing test entry but overwriting files field --
				// consider this as an additional update from the agent
				testEntry.Files = []ArtifactFile{
					{"cat_pix", "http://placekitten.com/300/400"},
					{"the_value_of_four", "4"},
				}
				So(testEntry.Upsert(), ShouldBeNil)
				count, err := db.Count(ArtifactFilesCollection, bson.M{})
				So(err, ShouldBeNil)
				So(count, ShouldEqual, 1)

				Convey("all updated fields should change,", func() {
					entryFromDb, err := FindOneArtifactFileEntryByTask("task1")
					So(err, ShouldBeNil)
					So(len(entryFromDb.Files), ShouldEqual, 4)
					So(entryFromDb.Files[0].Name, ShouldEqual, "cat_pix")
					So(entryFromDb.Files[0].Link, ShouldEqual, "http://placekitten.com/800/600")
					So(entryFromDb.Files[1].Name, ShouldEqual, "fast_download")
					So(entryFromDb.Files[1].Link, ShouldEqual, "https://fastdl.mongodb.org")
					So(entryFromDb.Files[2].Name, ShouldEqual, "cat_pix")
					So(entryFromDb.Files[2].Link, ShouldEqual, "http://placekitten.com/300/400")
					So(entryFromDb.Files[3].Name, ShouldEqual, "the_value_of_four")
					So(entryFromDb.Files[3].Link, ShouldEqual, "4")

					Convey("but non-updated fields should remain unchanged in db", func() {
						So(entryFromDb.TaskId, ShouldEqual, "task1")
						So(entryFromDb.TaskDisplayName, ShouldEqual, "Task One")
						So(entryFromDb.BuildId, ShouldEqual, "build1")
					})
				})
			})
		})
	})
}
