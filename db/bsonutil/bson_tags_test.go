package bsonutil

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTag(t *testing.T) {

	Convey("When checking for bson tags", t, func() {

		Convey("fetching the bson tag for a missing struct field should return"+
			" an error", func() {

			type s struct {
			}

			_, err := Tag(s{}, "FieldOne")
			So(err, ShouldNotBeNil)

		})

		Convey("fetching the bson tag for a struct field without the tag"+
			" should return the empty string, and no error", func() {

			type s struct {
				FieldOne string
			}

			tagVal, err := Tag(s{}, "FieldOne")
			So(err, ShouldBeNil)
			So(tagVal, ShouldEqual, "")

		})

		Convey("fetching the bson tag for a struct field with a specified tag"+
			" should return the tag value", func() {

			type s struct {
				FieldOne string `bson:"tag1"`
				FieldTwo string `bson:"tag2"`
			}

			tagVal, err := Tag(s{}, "FieldOne")
			So(err, ShouldBeNil)
			So(tagVal, ShouldEqual, "tag1")
			tagVal, err = Tag(s{}, "FieldTwo")
			So(err, ShouldBeNil)
			So(tagVal, ShouldEqual, "tag2")

		})

		Convey("if there are extra modifiers such as omitempty, they should be"+
			" ignored", func() {

			type s struct {
				FieldOne string `bson:"tag1,omitempty"`
			}

			tagVal, err := Tag(s{}, "FieldOne")
			So(err, ShouldBeNil)
			So(tagVal, ShouldEqual, "tag1")
		})

	})
}
