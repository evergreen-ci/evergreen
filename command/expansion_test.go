package command

import (
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestExpansions(t *testing.T) {

	Convey("With a set of expansions", t, func() {

		Convey("the expansions object should contain the values it is"+
			" initialized with", func() {

			expansions := NewExpansions(map[string]string{
				"key1": "val1",
				"key2": "val2",
			})
			So((*expansions)["key1"], ShouldEqual, "val1")
			So((*expansions)["key2"], ShouldEqual, "val2")

		})

		Convey("updating the expansions with a map should update all of the"+
			" specified values", func() {

			expansions := NewExpansions(map[string]string{
				"key1": "val1",
				"key2": "val2",
			})
			expansions.Update(map[string]string{
				"key2": "newval1",
				"key3": "newval2",
			})

			So((*expansions)["key1"], ShouldEqual, "val1")
			So((*expansions)["key2"], ShouldEqual, "newval1")
			So((*expansions)["key3"], ShouldEqual, "newval2")

		})

		Convey("fetching an expansion should return the appropriate value,"+
			" or the empty string if the value is not present", func() {

			expansions := NewExpansions(map[string]string{
				"key1": "val1",
				"key2": "val2",
			})
			So(expansions.Get("key1"), ShouldEqual, "val1")
			So(expansions.Get("key2"), ShouldEqual, "val2")
			So(expansions.Get("key3"), ShouldEqual, "")

		})

		Convey("checking for a key's existence should return whether it is"+
			" specified as an expansion", func() {

			expansions := NewExpansions(map[string]string{
				"key1": "val1",
				"key2": "val2",
			})
			So(expansions.Exists("key1"), ShouldBeTrue)
			So(expansions.Exists("key2"), ShouldBeTrue)
			So(expansions.Exists("key3"), ShouldBeFalse)

		})

		Convey("updating from a yaml file should get keys copied over", func() {
			expansions := NewExpansions(map[string]string{
				"key1": "val1",
				"key2": "val2",
			})
			err := expansions.UpdateFromYaml("testdata/expansions.yml")
			So(err, ShouldBeNil)
			So(expansions.Get("marge"), ShouldEqual, "1")
			So(expansions.Get("lisa"), ShouldEqual, "2")
			So(expansions.Get("bart"), ShouldEqual, "3")
			So(expansions.Get("key1"), ShouldEqual, "blah")
		})

	})
}

func TestExpandString(t *testing.T) {

	Convey("When applying expansions to a command string", t, func() {

		expansions := NewExpansions(map[string]string{
			"key1": "val1",
			"key2": "val2",
		})

		Convey("if the string contains no variables to be expanded, it should"+
			" be returned unchanged", func() {

			toExpand := "hello hello hellohello"
			expanded := "hello hello hellohello"

			exp, err := expansions.ExpandString("")
			So(err, ShouldBeNil)
			So(exp, ShouldEqual, "")

			exp, err = expansions.ExpandString(toExpand)
			So(err, ShouldBeNil)
			So(exp, ShouldEqual, expanded)

		})

		Convey("if the string contains variables to be expanded", func() {

			Convey("if the variables occur as keys in the expansions map",
				func() {

					Convey("they should be replaced with the appropriate"+
						" values from the map", func() {

						toExpand := "hello hello ${key1} hello${key2}hello" +
							"${key1}"
						expanded := "hello hello val1 helloval2helloval1"

						exp, err := expansions.ExpandString(toExpand)
						So(err, ShouldBeNil)
						So(exp, ShouldEqual, expanded)

					})

				})

			Convey("if the variables do not occur as keys in the expansions"+
				" map", func() {

				Convey("any variables with default values provided should be"+
					" replaced with their default value", func() {

					toExpand := "hello ${key1|blah}${key3|blech}hello " +
						"${key4|}hello"
					expanded := "hello val1blechhello hello"

					exp, err := expansions.ExpandString(toExpand)
					So(err, ShouldBeNil)
					So(exp, ShouldEqual, expanded)

				})

				Convey("any variables without default values provided should"+
					" be replaced with the empty string", func() {

					toExpand := "hello ${key1|blah}${key3}hello ${key4} " +
						"${key5|blech}hello"
					expanded := "hello val1hello  blechhello"

					exp, err := expansions.ExpandString(toExpand)
					So(err, ShouldBeNil)
					So(exp, ShouldEqual, expanded)
				})

			})

		})

		Convey("badly formed command strings should cause an error", func() {

			badStr1 := "hello ${key1|blah}${key3}hello${ ${key4} " +
				"${key5|blech}hello"
			badStr2 := "hello ${hellohello"

			_, err := expansions.ExpandString(badStr1)
			So(err, ShouldNotBeNil)

			_, err = expansions.ExpandString(badStr2)
			So(err, ShouldNotBeNil)
		})

	})
}
