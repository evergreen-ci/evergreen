package model

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// helper for comparing a selector string with its expected output
func selectorShouldParse(s string, expected Selector) {
	Convey(fmt.Sprintf(`selector string "%v" should parse correctly`, s), func() {
		So(ParseSelector(s), ShouldResemble, expected)
	})
}

func TestBasicSelector(t *testing.T) {
	Convey("With a set of test selection strings", t, func() {

		Convey("single selectors should parse", func() {
			selectorShouldParse("myTask", Selector{{name: "myTask"}})
			selectorShouldParse("!myTask", Selector{{name: "myTask", negated: true}})
			selectorShouldParse(".myTag", Selector{{name: "myTag", tagged: true}})
			selectorShouldParse("!.myTag", Selector{{name: "myTag", tagged: true, negated: true}})
			selectorShouldParse("*", Selector{{name: "*"}})
		})

		Convey("multi-selectors should parse", func() {
			selectorShouldParse(".tag1 .tag2", Selector{
				{name: "tag1", tagged: true},
				{name: "tag2", tagged: true},
			})
			selectorShouldParse(".tag1 !.tag2", Selector{
				{name: "tag1", tagged: true},
				{name: "tag2", tagged: true, negated: true},
			})
			selectorShouldParse("!.tag1 .tag2", Selector{
				{name: "tag1", tagged: true, negated: true},
				{name: "tag2", tagged: true},
			})
			selectorShouldParse(".mytag !mytask", Selector{
				{name: "mytag", tagged: true},
				{name: "mytask", negated: true},
			})
			selectorShouldParse(".tag1 .tag2 .tag3 !.tag4", Selector{
				{name: "tag1", tagged: true},
				{name: "tag2", tagged: true},
				{name: "tag3", tagged: true},
				{name: "tag4", tagged: true, negated: true},
			})

			Convey("selectors with unusual whitespace should parse", func() {
				selectorShouldParse("    .myTag   ", Selector{{name: "myTag", tagged: true}})
				selectorShouldParse(".mytag\t\t!mytask", Selector{
					{name: "mytag", tagged: true},
					{name: "mytask", negated: true},
				})
				selectorShouldParse("\r\n.mytag\r\n!mytask\n", Selector{
					{name: "mytag", tagged: true},
					{name: "mytask", negated: true},
				})
			})
		})
	})
}

func taskSelectorShouldEval(tse *taskSelectorEvaluator, s string, expected []string) {
	Convey(fmt.Sprintf(`selector "%v" should evaluate to %v`, s, expected), func() {
		names, err := tse.evalSelector(ParseSelector(s))
		if expected != nil {
			So(err, ShouldBeNil)
		} else {
			So(err, ShouldNotBeNil)
		}
		So(len(names), ShouldEqual, len(expected))
		for _, e := range expected {
			So(names, ShouldContain, e)
		}
	})
}

func TestTaskSelectorEvaluation(t *testing.T) {
	var tse *taskSelectorEvaluator

	Convey("With a colorful set of ProjectTasks", t, func() {
		taskDefs := []parserTask{
			{Name: "red", Tags: []string{"primary", "warm"}},
			{Name: "orange", Tags: []string{"secondary", "warm"}},
			{Name: "yellow", Tags: []string{"primary", "warm"}},
			{Name: "green", Tags: []string{"secondary", "cool"}},
			{Name: "blue", Tags: []string{"primary", "cool"}},
			{Name: "purple", Tags: []string{"secondary", "cool"}},
			{Name: "brown", Tags: []string{"tertiary"}},
			{Name: "black", Tags: []string{"special"}},
			{Name: "white", Tags: []string{"special"}},
		}

		Convey("a taskSelectorEvaluator", func() {
			tse = NewParserTaskSelectorEvaluator(taskDefs)

			Convey("should evaluate single name selectors properly", func() {
				taskSelectorShouldEval(tse, "red", []string{"red"})
				taskSelectorShouldEval(tse, "white", []string{"white"})
			})

			Convey("should evaluate single tag selectors properly", func() {
				taskSelectorShouldEval(tse, ".warm", []string{"red", "orange", "yellow"})
				taskSelectorShouldEval(tse, ".cool", []string{"blue", "green", "purple"})
				taskSelectorShouldEval(tse, ".special", []string{"white", "black"})
				taskSelectorShouldEval(tse, ".primary", []string{"red", "blue", "yellow"})
			})

			Convey("should evaluate multi-tag selectors properly", func() {
				taskSelectorShouldEval(tse, ".warm .cool", nil)
				taskSelectorShouldEval(tse, ".cool .primary", []string{"blue"})
				taskSelectorShouldEval(tse, ".warm .secondary", []string{"orange"})
			})

			Convey("should evaluate selectors with negation properly", func() {
				taskSelectorShouldEval(tse, "!.special",
					[]string{"red", "orange", "yellow", "green", "blue", "purple", "brown"})
				taskSelectorShouldEval(tse, ".warm !yellow", []string{"red", "orange"})
				taskSelectorShouldEval(tse, "!.primary !.secondary", []string{"black", "white", "brown"})
			})

			Convey("should evaluate special selectors", func() {
				taskSelectorShouldEval(tse, "*",
					[]string{"red", "orange", "yellow", "green", "blue", "purple", "brown", "black", "white"})
			})

			Convey("should fail on bad selectors like", func() {

				Convey("empty selectors", func() {
					_, err := tse.evalSelector(Selector{})
					So(err, ShouldNotBeNil)
				})

				Convey("names that don't exist", func() {
					_, err := tse.evalSelector(ParseSelector("salmon"))
					So(err, ShouldNotBeNil)
					_, err = tse.evalSelector(ParseSelector("!azure"))
					So(err, ShouldNotBeNil)
				})

				Convey("tags that don't exist", func() {
					_, err := tse.evalSelector(ParseSelector(".fall"))
					So(err, ShouldNotBeNil)
					_, err = tse.evalSelector(ParseSelector("!.spring"))
					So(err, ShouldNotBeNil)
				})

				Convey("using . and ! with *", func() {
					_, err := tse.evalSelector(ParseSelector(".*"))
					So(err, ShouldNotBeNil)
					_, err = tse.evalSelector(ParseSelector("!*"))
					So(err, ShouldNotBeNil)
				})

				Convey("illegal names", func() {
					_, err := tse.evalSelector(ParseSelector("!!purple"))
					So(err, ShouldNotBeNil)
					_, err = tse.evalSelector(ParseSelector(".!purple"))
					So(err, ShouldNotBeNil)
					_, err = tse.evalSelector(ParseSelector("..purple"))
					So(err, ShouldNotBeNil)
				})
			})
		})
	})
}
