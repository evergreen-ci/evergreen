package db

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.mongodb.org/mongo-driver/bson"
)

func TestQueryExecution(t *testing.T) {

	type insertableStruct struct {
		FieldOne   string `bson:"one"`
		FieldTwo   int    `bson:"two"`
		FieldThree string `bson:"three"`
	}

	collection := "test_query_collection"

	Convey("With a db and collection", t, func() {
		Convey("inserting a single item into the collection", func() {
			So(Clear(collection), ShouldBeNil)
			in := &insertableStruct{
				FieldOne: "1",
				FieldTwo: 1,
			}
			So(Insert(t.Context(), collection, in), ShouldBeNil)

			Convey("the item should be findable with a Query", func() {
				out := &insertableStruct{}
				query := Query(bson.M{"one": "1"})
				err := FindOneQContext(t.Context(), collection, query, out)
				So(err, ShouldBeNil)
				So(out, ShouldResemble, in)
			})
		})
		Convey("inserting a multiple items into the collection", func() {
			So(Clear(collection), ShouldBeNil)
			objs := []insertableStruct{
				{"X", 1, ""},
				{"X", 2, ""},
				{"X", 3, ""},
				{"X", 4, ""},
				{"X", 5, ""},
				{"X", 6, ""},
				{"X", 7, "COOL"},
			}
			for _, in := range objs {
				So(Insert(t.Context(), collection, in), ShouldBeNil)
			}

			BelowFive := Query(bson.M{"two": bson.M{"$lt": 5}})
			BelowFiveSorted := BelowFive.Sort([]string{"-two"})
			BelowFiveLimit := BelowFive.Limit(2).Skip(1)
			JustOneField := Query(bson.M{"two": 7}).Project(bson.D{{Key: "three", Value: 1}})

			Convey("BelowFive should return 4 documents", func() {
				out := []insertableStruct{}
				err := FindAllQ(t.Context(), collection, BelowFive, &out)
				So(err, ShouldBeNil)
				So(len(out), ShouldEqual, 4)
			})

			Convey("BelowFiveSorted should return 4 documents, sorted in reverse", func() {
				out := []insertableStruct{}
				err := FindAllQ(t.Context(), collection, BelowFiveSorted, &out)
				So(err, ShouldBeNil)
				So(len(out), ShouldEqual, 4)
				So(out[0].FieldTwo, ShouldEqual, 4)
				So(out[1].FieldTwo, ShouldEqual, 3)
				So(out[2].FieldTwo, ShouldEqual, 2)
				So(out[3].FieldTwo, ShouldEqual, 1)
			})

			Convey("BelowFiveLimit should return 2 documents", func() {
				out := []insertableStruct{}
				err := FindAllQ(t.Context(), collection, BelowFiveLimit, &out)
				So(err, ShouldBeNil)
				So(len(out), ShouldEqual, 2)
			})

			Convey("JustOneField should return 1 document", func() {
				out := []bson.M{}
				err := FindAllQ(t.Context(), collection, JustOneField, &out)
				So(err, ShouldBeNil)
				So(out[0]["three"], ShouldEqual, "COOL")
			})
		})
	})
}
