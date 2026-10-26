package db

import (
	"io"
	"strings"
	"testing"
	"time"

	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	_ "github.com/evergreen-ci/evergreen/testutil"
	adb "github.com/mongodb/anser/db"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func TestDBUtils(t *testing.T) {

	type insertableStruct struct {
		FieldOne   string `bson:"field_one"`
		FieldTwo   int    `bson:"field_two"`
		FieldThree string `bson:"field_three"`
	}

	collection := "test_collection"

	Convey("With a db and collection", t, func() {

		So(Clear(collection), ShouldBeNil)

		Convey("creating a collection should be idempotent", func() {
			So(CreateCollections(collection), ShouldBeNil)
			So(CreateCollections(collection), ShouldBeNil)
		})

		Convey("inserting an item into the collection should update the"+
			" database accordingly", func() {

			in := &insertableStruct{
				FieldOne: "1",
				FieldTwo: 1,
			}

			So(Insert(t.Context(), collection, in), ShouldBeNil)

			out := &insertableStruct{}
			err := FindOneQ(t.Context(), collection, Query(bson.M{}), out)
			So(err, ShouldBeNil)
			So(out, ShouldResemble, in)

		})

		Convey("clearing a collection should remove all items from the"+
			" collection", func() {

			in := &insertableStruct{
				FieldOne: "1",
				FieldTwo: 1,
			}

			inTwo := &insertableStruct{
				FieldOne: "2",
				FieldTwo: 2,
			}

			// insert, make sure both were inserted
			So(Insert(t.Context(), collection, in), ShouldBeNil)
			So(Insert(t.Context(), collection, inTwo), ShouldBeNil)
			count, err := Count(t.Context(), collection, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 2)

			// clear and validate the collection is empty
			So(Clear(collection), ShouldBeNil)
			count, err = Count(t.Context(), collection, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 0)

		})

		Convey("after writing a gridfs file, reading it back should match the contents", func() {

			So(Clear("testfiles.chunks"), ShouldBeNil)
			So(Clear("testfiles.files"), ShouldBeNil)
			id := mgobson.NewObjectId().Hex()
			So(WriteGridFile(t.Context(), "testfiles", id, strings.NewReader(id)), ShouldBeNil)
			file, err := GetGridFile(t.Context(), "testfiles", id)
			So(err, ShouldBeNil)
			raw, err := io.ReadAll(file)
			So(err, ShouldBeNil)
			So(string(raw), ShouldEqual, id)
		})

		Convey("removing an item from a collection should remove it and leave"+
			" the rest of the collection untouched", func() {

			in := &insertableStruct{
				FieldOne: "1",
				FieldTwo: 1,
			}

			inTwo := &insertableStruct{
				FieldOne: "2",
				FieldTwo: 2,
			}

			// insert, make sure both were inserted
			So(Insert(t.Context(), collection, in), ShouldBeNil)
			So(Insert(t.Context(), collection, inTwo), ShouldBeNil)
			count, err := Count(t.Context(), collection, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 2)

			// remove just the first
			So(Remove(t.Context(), collection, bson.M{"field_one": "1"}),
				ShouldBeNil)
			count, err = Count(t.Context(), collection, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 1)

			out := &insertableStruct{}
			err = FindOneQ(t.Context(), collection, Query(bson.M{}), out)
			So(err, ShouldBeNil)
			So(out, ShouldResemble, inTwo)

		})

		Convey("removing multiple items from a collection should only remove"+
			" the matching ones", func() {

			in := &insertableStruct{
				FieldOne: "1",
				FieldTwo: 1,
			}

			inTwo := &insertableStruct{
				FieldOne: "2",
				FieldTwo: 2,
			}

			inThree := &insertableStruct{
				FieldOne: "1",
				FieldTwo: 2,
			}

			// insert, make sure all were inserted
			So(Insert(t.Context(), collection, in), ShouldBeNil)
			So(Insert(t.Context(), collection, inTwo), ShouldBeNil)
			So(Insert(t.Context(), collection, inThree), ShouldBeNil)
			count, err := Count(t.Context(), collection, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 3)

			// remove just the first
			So(RemoveAll(t.Context(), collection, bson.M{"field_one": "1"}),
				ShouldBeNil)
			count, err = Count(t.Context(), collection, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 1)

			out := &insertableStruct{}
			err = FindOneQ(t.Context(), collection, Query(bson.M{}), out)
			So(err, ShouldBeNil)
			So(out, ShouldResemble, inTwo)
		})

		Convey("finding all matching items should use the correct filter, and"+
			" should respect the projection, sort, skip, and limit passed"+
			" in", func() {

			in := &insertableStruct{
				FieldOne:   "1",
				FieldTwo:   1,
				FieldThree: "x",
			}

			inTwo := &insertableStruct{
				FieldOne:   "2",
				FieldTwo:   1,
				FieldThree: "y",
			}

			inThree := &insertableStruct{
				FieldOne:   "3",
				FieldTwo:   1,
				FieldThree: "z",
			}

			inFour := &insertableStruct{
				FieldOne:   "4",
				FieldTwo:   2,
				FieldThree: "z",
			}

			// insert, make sure all were inserted
			So(Insert(t.Context(), collection, in), ShouldBeNil)
			So(Insert(t.Context(), collection, inTwo), ShouldBeNil)
			So(Insert(t.Context(), collection, inThree), ShouldBeNil)
			So(Insert(t.Context(), collection, inFour), ShouldBeNil)
			count, err := Count(t.Context(), collection, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 4)

			// run a find that should only match the first three, should not
			// project field_three, should sort backwards on field_one, skip
			// one and limit to one (meaning only the second struct should be
			// returned)
			out := []insertableStruct{}
			err = FindAllQ(t.Context(), collection, Query(bson.M{"field_two": 1}).
				Project(bson.M{"field_three": 0}).
				Sort([]string{"-field_one"}).
				Limit(1).
				Skip(1),
				&out,
			)
			So(err, ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			So(out[0].FieldOne, ShouldEqual, "2")
			So(out[0].FieldThree, ShouldEqual, "") // not projected

		})

		Convey("updating one item in a collection should apply the correct"+
			" update", func() {

			in := &insertableStruct{
				FieldOne: "1",
				FieldTwo: 1,
			}

			inTwo := &insertableStruct{
				FieldOne: "2",
				FieldTwo: 2,
			}

			// insert, make sure both were inserted
			So(Insert(t.Context(), collection, in), ShouldBeNil)
			So(Insert(t.Context(), collection, inTwo), ShouldBeNil)
			count, err := Count(t.Context(), collection, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 2)

			// update the second
			err = Update(
				t.Context(),
				collection,
				bson.M{
					"field_one": "2",
				},
				bson.M{
					"$set": bson.M{
						"field_two": 3,
					},
				},
			)
			So(err, ShouldBeNil)

			out := &insertableStruct{}
			err = FindOneQ(t.Context(), collection, Query(bson.M{"field_one": "2"}), out)
			So(err, ShouldBeNil)
			So(out.FieldTwo, ShouldEqual, 3)

		})

		Convey("updating multiple items in a collection should update all of"+
			" the matched ones, and no others", func() {

			in := &insertableStruct{
				FieldOne: "1",
				FieldTwo: 1,
			}

			inTwo := &insertableStruct{
				FieldOne: "2",
				FieldTwo: 2,
			}

			inThree := &insertableStruct{
				FieldOne: "1",
				FieldTwo: 2,
			}

			// insert, make sure all were inserted
			So(Insert(t.Context(), collection, in), ShouldBeNil)
			So(Insert(t.Context(), collection, inTwo), ShouldBeNil)
			So(Insert(t.Context(), collection, inThree), ShouldBeNil)
			count, err := Count(t.Context(), collection, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 3)

			// update the first and third
			_, err = UpdateAll(
				t.Context(),
				collection,
				bson.M{
					"field_one": "1",
				},
				bson.M{
					"$set": bson.M{
						"field_two": 3,
					},
				},
			)
			So(err, ShouldBeNil)

			out := []insertableStruct{}
			err = FindAllQ(t.Context(), collection, Query(bson.M{"field_two": 3}), &out)
			So(err, ShouldBeNil)
			So(len(out), ShouldEqual, 2)

		})

		Convey("when upserting an item into the collection", func() {

			Convey("if the item does not exist, it should be inserted", func() {

				in := &insertableStruct{
					FieldOne: "1",
					FieldTwo: 1,
				}

				_, err := Upsert(t.Context(),
					collection,
					bson.M{
						"field_one": in.FieldOne,
					},
					bson.M{
						"$set": bson.M{
							"field_two": in.FieldTwo,
						},
					},
				)
				So(err, ShouldBeNil)

				out := &insertableStruct{}
				err = FindOneQ(t.Context(), collection, Query(bson.M{}), out)
				So(err, ShouldBeNil)
				So(out, ShouldResemble, in)

			})

			Convey("if the item already exists, it should be updated", func() {

				in := &insertableStruct{
					FieldOne: "1",
					FieldTwo: 1,
				}

				So(Insert(t.Context(), collection, in), ShouldBeNil)
				in.FieldTwo = 2

				_, err := Upsert(t.Context(),
					collection,
					bson.M{
						"field_one": in.FieldOne,
					},
					bson.M{
						"$set": bson.M{
							"field_two": in.FieldTwo,
						},
					},
				)
				So(err, ShouldBeNil)

				out := &insertableStruct{}
				err = FindOneQ(t.Context(), collection, Query(bson.M{}), out)
				So(err, ShouldBeNil)
				So(out, ShouldResemble, in)
			})

		})

		Convey("finding and modifying in a collection should run the specified"+
			" find and modify", func() {

			in := &insertableStruct{
				FieldOne: "1",
				FieldTwo: 1,
			}

			So(Insert(t.Context(), collection, in), ShouldBeNil)
			in.FieldTwo = 2

			change := adb.Change{
				Update: bson.M{
					"$set": bson.M{
						"field_two": in.FieldTwo,
					},
				},
				ReturnNew: true,
			}

			out := &insertableStruct{}
			cInfo, err := FindAndModify(t.Context(),
				collection,
				bson.M{
					"field_one": in.FieldOne,
				},
				nil,
				change,
				out,
			)
			So(err, ShouldBeNil)
			So(cInfo.Updated, ShouldEqual, 1)

		})

		Convey("a simple aggregation command should run successfully", func() {

			in := &insertableStruct{
				FieldOne: "1",
				FieldTwo: 1,
			}
			inTwo := &insertableStruct{
				FieldOne: "2",
				FieldTwo: 2,
			}
			inThree := &insertableStruct{
				FieldOne: "2",
				FieldTwo: 3,
			}
			So(Insert(t.Context(), collection, in), ShouldBeNil)
			So(Insert(t.Context(), collection, inTwo), ShouldBeNil)
			So(Insert(t.Context(), collection, inThree), ShouldBeNil)

			testPipeline := []bson.M{
				{"$group": bson.M{
					"_id":   "$field_one",
					"total": bson.M{"$sum": "$field_two"}}},
				{"$sort": bson.M{"total": -1}},
			}

			output := []bson.M{}
			err := Aggregate(t.Context(), collection, testPipeline, &output)
			So(err, ShouldBeNil)
			So(len(output), ShouldEqual, 2)
			So(output[0]["total"], ShouldEqual, 5)
			So(output[0]["_id"], ShouldEqual, "2")
			So(output[1]["total"], ShouldEqual, 1)
			So(output[1]["_id"], ShouldEqual, "1")

			Convey("and should be able to marshal results to a struct", func() {
				type ResultStruct struct {
					Id       string `bson:"_id"`
					TotalSum int    `bson:"total"`
				}
				output := []ResultStruct{}
				err := Aggregate(t.Context(), collection, testPipeline, &output)
				So(err, ShouldBeNil)
				So(len(output), ShouldEqual, 2)
				So(output[0], ShouldResemble, ResultStruct{"2", 5})
				So(output[1], ShouldResemble, ResultStruct{"1", 1})
			})
		})
	})
}

func TestClearGridFSCollections(t *testing.T) {
	assert := assert.New(t)

	assert.NoError(WriteGridFile(t.Context(), "testfiles", "test.txt", strings.NewReader("lorem ipsum")))

	reader, err := GetGridFile(t.Context(), "testfiles", "test.txt")
	assert.NoError(err)
	defer reader.Close()

	var bytes []byte
	bytes, err = io.ReadAll(reader)
	assert.NoError(err)
	assert.Equal("lorem ipsum", string(bytes))

	assert.NoError(ClearGridCollections("testfiles"))

	reader, err = GetGridFile(t.Context(), "testfiles", "test.txt")
	assert.Error(err)
	assert.Nil(reader)
}

func TestWithLastUpdatedAt(t *testing.T) {
	type doc struct {
		ID            string    `bson:"_id"`
		Field         string    `bson:"field"`
		LastUpdatedAt time.Time `bson:"_last_updated_at,omitempty"`
	}

	t.Run("UntrackedCollectionShouldBeUnchanged", func(t *testing.T) {
		in := &doc{ID: "id", Field: "value"}
		assert.Same(t, in, withLastUpdatedAt("untracked", in))
	})
	t.Run("TrackedCollectionShouldAddField", func(t *testing.T) {
		before := time.Now().Add(-time.Second)
		out, ok := withLastUpdatedAt("tasks", &doc{ID: "id", Field: "value"}).(bson.Raw)
		require.True(t, ok)
		assert.Equal(t, "id", out.Lookup("_id").StringValue())
		assert.Equal(t, "value", out.Lookup("field").StringValue())
		assert.True(t, out.Lookup(lastUpdatedAtField).Time().After(before))
	})
	t.Run("ExistingFieldShouldBeKept", func(t *testing.T) {
		in := &doc{ID: "id", LastUpdatedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}
		assert.Same(t, in, withLastUpdatedAt("tasks", in))
	})
}

func TestWithLastUpdatedAtUpdate(t *testing.T) {
	currentDate := bson.M{lastUpdatedAtField: true}

	t.Run("UntrackedCollectionShouldBeUnchanged", func(t *testing.T) {
		update := bson.M{"$set": bson.M{"a": 1}}
		assert.Equal(t, update, withLastUpdatedAtUpdate("untracked", update))
	})
	t.Run("OperatorBSONMShouldAddCurrentDate", func(t *testing.T) {
		out := withLastUpdatedAtUpdate("tasks", bson.M{"$set": bson.M{"a": 1}})
		assert.Equal(t, bson.M{"$set": bson.M{"a": 1}, "$currentDate": currentDate}, out)
	})
	t.Run("OperatorBSONDShouldAddCurrentDate", func(t *testing.T) {
		out := withLastUpdatedAtUpdate("tasks", bson.D{{Key: "$set", Value: bson.M{"a": 1}}})
		assert.Equal(t, bson.D{{Key: "$set", Value: bson.M{"a": 1}}, {Key: "$currentDate", Value: currentDate}}, out)
	})
	t.Run("ExistingCurrentDateShouldBeUnchanged", func(t *testing.T) {
		update := bson.M{"$currentDate": bson.M{"other": true}}
		assert.Equal(t, update, withLastUpdatedAtUpdate("tasks", update))
	})
	t.Run("PipelineShouldAppendSetStage", func(t *testing.T) {
		out, ok := withLastUpdatedAtUpdate("tasks", []bson.M{{"$set": bson.M{"a": 1}}}).([]bson.M)
		require.True(t, ok)
		require.Len(t, out, 2)
		assert.Equal(t, bson.M{"$set": bson.M{lastUpdatedAtField: "$$NOW"}}, out[1])
	})
	t.Run("EmptyUpdateShouldBeUnchanged", func(t *testing.T) {
		for name, update := range map[string]any{
			"EmptyBSONM":       bson.M{},
			"EmptyBSOND":       bson.D{},
			"EmptySetBSONM":    bson.M{"$set": bson.M{}},
			"EmptySetBSOND":    bson.D{{Key: "$set", Value: bson.D{}}},
			"EmptySetAndUnset": bson.M{"$set": bson.M{}, "$unset": bson.M{}},
			"EmptyPipeline":    []bson.M{},
		} {
			t.Run(name, func(t *testing.T) {
				assert.Equal(t, update, withLastUpdatedAtUpdate("tasks", update))
			})
		}
	})
	t.Run("PartiallyEmptyUpdateShouldAddCurrentDate", func(t *testing.T) {
		out := withLastUpdatedAtUpdate("tasks", bson.M{"$set": bson.M{}, "$unset": bson.M{"a": 1}})
		assert.Equal(t, bson.M{"$set": bson.M{}, "$unset": bson.M{"a": 1}, "$currentDate": currentDate}, out)
	})
}
