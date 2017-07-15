package util

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestIsExpandable(t *testing.T) {

	Convey("With a passed in string...", t, func() {
		Convey("IsExpandable should return true if the string is expandable",
			func() {
				So(IsExpandable("${34}"), ShouldEqual, true)
				So(IsExpandable("${34}}"), ShouldEqual, true)
				So(IsExpandable("{${34}"), ShouldEqual, true)
				So(IsExpandable("{${34}}"), ShouldEqual, true)
				So(IsExpandable("${34"), ShouldEqual, false)
				So(IsExpandable("$34"), ShouldEqual, false)
				So(IsExpandable("{$34}}"), ShouldEqual, false)
				So(IsExpandable("34"), ShouldEqual, false)
			})
	})
}

func TestExpandValues(t *testing.T) {

	Convey("When expanding struct values", t, func() {

		expansions := NewExpansions(
			map[string]string{
				"exp1": "val1",
			},
		)

		Convey("if the input value is not a pointer to a struct, an error"+
			" should be returned", func() {
			So(ExpandValues("hello", expansions), ShouldNotBeNil)
			So(ExpandValues(struct{}{}, expansions), ShouldNotBeNil)
			So(ExpandValues([]string{"hi"}, expansions), ShouldNotBeNil)
		})

		Convey("if any non-string fields are tagged as expandable, an error"+
			" should be returned", func() {

			type s struct {
				FieldOne string `plugin:"expand"`
				FieldTwo int    `plugin:"expand"`
			}

			So(ExpandValues(&s{}, expansions), ShouldNotBeNil)

		})

		Convey("any fields of the input struct with the appropriate tag should"+
			" be expanded", func() {

			type s struct {
				FieldOne   string
				FieldTwo   string `plugin:"expand"`
				FieldThree string `plugin:"expand,hello"`
			}

			s1 := &s{
				FieldOne:   "hello ${exp1}",
				FieldTwo:   "hi ${exp1}",
				FieldThree: "yo ${exp2|yo}",
			}

			So(ExpandValues(s1, expansions), ShouldBeNil)

			// make sure the appropriate fields were expanded
			So(s1.FieldOne, ShouldEqual, "hello ${exp1}")
			So(s1.FieldTwo, ShouldEqual, "hi val1")
			So(s1.FieldThree, ShouldEqual, "yo yo")

		})

		Convey("any nested structs tagged as expandable should have their"+
			" fields expanded appropriately", func() {

			type inner struct {
				FieldOne string `plugin:"expand"`
			}

			type outer struct {
				FieldOne   string `plugin:"expand"`
				FieldTwo   inner  `plugin:"expand"`
				FieldThree inner
			}

			s := &outer{
				FieldOne: "hello ${exp1}",
				FieldTwo: inner{
					FieldOne: "hi ${exp1}",
				},
				FieldThree: inner{
					FieldOne: "yo ${exp1}",
				},
			}

			So(ExpandValues(s, expansions), ShouldBeNil)

			// make sure all fields, including nested ones, were expanded
			// correctly
			So(s.FieldOne, ShouldEqual, "hello val1")
			So(s.FieldTwo.FieldOne, ShouldEqual, "hi val1")
			So(s.FieldThree.FieldOne, ShouldEqual, "yo ${exp1}")

		})

		Convey("any nested maps tagged as expandable should have their"+
			" fields expanded appropriately", func() {

			type outer struct {
				FieldOne string            `plugin:"expand"`
				FieldTwo map[string]string `plugin:"expand"`
			}

			s := &outer{
				FieldOne: "hello ${exp1}",
				FieldTwo: map[string]string{
					"1":       "hi ${exp1}",
					"${exp1}": "yo ${exp1}",
				},
			}

			So(ExpandValues(s, expansions), ShouldBeNil)

			// make sure all fields, including nested maps, were expanded
			So(s.FieldOne, ShouldEqual, "hello val1")
			So(s.FieldTwo["1"], ShouldEqual, "hi val1")
			So(s.FieldTwo["val1"], ShouldEqual, "yo val1")
		})

		Convey("if the input value is a slice, expansion should work "+
			"for the fields within that slice", func() {

			type simpleStruct struct {
				StructFieldKeyOne string `plugin:"expand"`
				StructFieldKeyTwo string
			}
			type sliceStruct struct {
				SliceFieldKey []*simpleStruct `plugin:"expand"`
			}
			simpleStruct1 := simpleStruct{
				StructFieldKeyOne: "hello ${exp1}",
				StructFieldKeyTwo: "abc${expl}",
			}
			simpleSlice := make([]*simpleStruct, 0)
			simpleSlice = append(simpleSlice, &simpleStruct1)
			sliceStruct1 := sliceStruct{}
			sliceStruct1.SliceFieldKey = simpleSlice
			So(ExpandValues(&sliceStruct1, expansions), ShouldBeNil)

			// make sure the appropriate fields were expanded
			So(simpleStruct1.StructFieldKeyOne, ShouldEqual, "hello val1")
			So(simpleStruct1.StructFieldKeyTwo, ShouldEqual, "abc${expl}")

		})

		Convey("any nested structs/slices tagged as expandable should have "+
			"their fields expanded appropriately", func() {

			type innerStruct struct {
				FieldOne string `plugin:"expand"`
			}

			type innerSlice struct {
				FieldOne string `plugin:"expand"`
			}

			type middle struct {
				FieldOne   string `plugin:"expand"`
				FieldTwo   string
				FieldThree innerStruct
				FieldFour  []*innerSlice
			}

			type outer struct {
				FieldOne string    `plugin:"expand"`
				FieldTwo []*middle `plugin:"expand"`
			}

			innerStructObject := innerStruct{
				FieldOne: "hello ${exp1}",
			}

			innerSliceField := innerSlice{
				FieldOne: "hi ${exp1}",
			}

			innerSliceObject := make([]*innerSlice, 0)
			innerSliceObject = append(innerSliceObject, &innerSliceField)

			middleObjectOne := middle{
				FieldOne: "ab ${exp1}",
				FieldTwo: "abc ${exp1}",
			}

			middleObjectTwo := middle{
				FieldOne:   "abc ${exp1}",
				FieldTwo:   "abc ${exp1}",
				FieldThree: innerStructObject,
				FieldFour:  innerSliceObject,
			}

			middleObject := make([]*middle, 0)
			middleObject = append(middleObject, &middleObjectOne)
			middleObject = append(middleObject, &middleObjectTwo)

			s := &outer{
				FieldOne: "hello ${exp1}",
				FieldTwo: middleObject,
			}

			So(ExpandValues(s, expansions), ShouldBeNil)

			// make sure all fields, including nested ones, were expanded
			// correctly
			So(s.FieldOne, ShouldEqual, "hello val1")
			So(s.FieldTwo[0].FieldOne, ShouldEqual, "ab val1")
			So(s.FieldTwo[1].FieldOne, ShouldEqual, "abc val1")
			So(s.FieldTwo[0].FieldTwo, ShouldEqual, "abc ${exp1}")
			So(s.FieldTwo[1].FieldTwo, ShouldEqual, "abc ${exp1}")
			So(s.FieldTwo[1].FieldThree.FieldOne, ShouldEqual, "hello ${exp1}")
			So(s.FieldTwo[1].FieldFour[0].FieldOne, ShouldEqual, "hi ${exp1}")
		})
	})

	Convey("When expanding map values", t, func() {
		expansions := NewExpansions(
			map[string]string{
				"a": "A",
				"b": "B",
				"c": "C",
			},
		)

		Convey("a simple map expands properly", func() {
			testmap := map[string]string{
				"nope": "nothing",
				"${a}": "key",
				"val":  "${b}",
				"${c}": "${a}",
			}
			So(ExpandValues(&testmap, expansions), ShouldBeNil)
			So(testmap["nope"], ShouldEqual, "nothing")
			So(testmap["A"], ShouldEqual, "key")
			So(testmap["val"], ShouldEqual, "B")
			So(testmap["C"], ShouldEqual, "A")
		})

		Convey("a recursive map expands properly", func() {
			testmap := map[string]map[string]string{
				"${a}": {
					"deep": "${c}",
					"no":   "same",
				},
			}
			So(ExpandValues(&testmap, expansions), ShouldBeNil)
			So(len(testmap), ShouldEqual, 1)
			So(len(testmap["A"]), ShouldEqual, 2)
			So(testmap["A"]["no"], ShouldEqual, "same")
			So(testmap["A"]["deep"], ShouldEqual, "C")
		})
	})
}
