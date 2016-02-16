package model

import (
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"reflect"
	"testing"
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

func taskSelectorTaskEval(tse *taskSelectorEvaluator, bvts, expected []BuildVariantTask) {
	Convey(fmt.Sprintf(`tasks %v should evaluate properly`, bvts), func() {
		tasks, err := tse.EvaluateTasks(bvts)
		if expected != nil {
			So(err, ShouldBeNil)
		} else {
			So(err, ShouldNotBeNil)
		}
		So(len(tasks), ShouldEqual, len(expected))
		for _, e := range expected {
			exists := false
			for _, t := range tasks {
				if t.Name == e.Name && t.Priority == e.Priority && len(t.DependsOn) == len(e.DependsOn) {
					exists = true
				}
			}
			So(exists, ShouldBeTrue)
		}
	})
}

func taskSelectorDepEval(tse *taskSelectorEvaluator, deps, expected []TaskDependency) {
	Convey(fmt.Sprintf(`dependencies %v should evaluate properly`, deps), func() {
		tasks, err := tse.EvaluateDeps(deps)
		if expected != nil {
			So(err, ShouldBeNil)
		} else {
			So(err, ShouldNotBeNil)
		}
		So(len(tasks), ShouldEqual, len(expected))
		for _, e := range expected {
			exists := false
			for _, t := range tasks {
				if reflect.DeepEqual(t, e) {
					exists = true
				}
			}
			So(exists, ShouldBeTrue)
		}
	})
}

func TestTaskSelectorEvaluation(t *testing.T) {
	var tse *taskSelectorEvaluator

	Convey("With a colorful set of ProjectTasks", t, func() {
		taskDefs := []ProjectTask{
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
			tse = NewTaskSelectorEvaluator(taskDefs)

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

			Convey("should evaluate valid tasks pointers properly", func() {
				taskSelectorTaskEval(tse,
					[]BuildVariantTask{{Name: "white"}},
					[]BuildVariantTask{{Name: "white"}})
				taskSelectorTaskEval(tse,
					[]BuildVariantTask{{Name: "red", Priority: 500}, {Name: ".secondary"}},
					[]BuildVariantTask{{Name: "red", Priority: 500}, {Name: "orange"}, {Name: "purple"}, {Name: "green"}})
				taskSelectorTaskEval(tse,
					[]BuildVariantTask{
						{Name: "orange", Distros: []string{"d1"}},
						{Name: ".warm .secondary", Distros: []string{"d1"}}},
					[]BuildVariantTask{{Name: "orange", Distros: []string{"d1"}}})
				taskSelectorTaskEval(tse,
					[]BuildVariantTask{
						{Name: "orange", Distros: []string{"d1"}},
						{Name: "!.warm .secondary", Distros: []string{"d1"}}},
					[]BuildVariantTask{
						{Name: "orange", Distros: []string{"d1"}},
						{Name: "purple", Distros: []string{"d1"}},
						{Name: "green", Distros: []string{"d1"}}})
				taskSelectorTaskEval(tse,
					[]BuildVariantTask{{Name: "*"}},
					[]BuildVariantTask{
						{Name: "red"}, {Name: "blue"}, {Name: "yellow"},
						{Name: "orange"}, {Name: "purple"}, {Name: "green"},
						{Name: "brown"}, {Name: "white"}, {Name: "black"},
					})
				taskSelectorDepEval(tse,
					[]TaskDependency{
						{Name: "orange", Variant: "v1"},
						{Name: "!.warm .secondary", Variant: "v1"}},
					[]TaskDependency{
						{Name: "orange", Variant: "v1"},
						{Name: "purple", Variant: "v1"},
						{Name: "green", Variant: "v1"}})
				taskSelectorDepEval(tse,
					[]TaskDependency{
						{Name: "orange", Status: "*"},
						{Name: "!.warm .secondary", Status: "*"}},
					[]TaskDependency{
						{Name: "orange", Status: "*"},
						{Name: "purple", Status: "*"},
						{Name: "green", Status: "*"}})
				taskSelectorDepEval(tse,
					[]TaskDependency{{Name: "*"}},
					[]TaskDependency{{Name: "*"}})
			})
			Convey("should fail on  invalid tasks pointers like", func() {
				Convey("tasks and tags that do not exist", func() {
					_, err := tse.EvaluateTasks([]BuildVariantTask{{Name: "magenta"}})
					So(err, ShouldNotBeNil)
					_, err = tse.EvaluateDeps([]TaskDependency{{Name: "magenta"}})
					So(err, ShouldNotBeNil)
					_, err = tse.EvaluateTasks([]BuildVariantTask{{Name: "!magenta"}})
					So(err, ShouldNotBeNil)
					_, err = tse.EvaluateTasks([]BuildVariantTask{{Name: ".invisible"}})
					So(err, ShouldNotBeNil)
					_, err = tse.EvaluateTasks([]BuildVariantTask{{Name: "!.invisible"}})
					So(err, ShouldNotBeNil)
				})
				Convey("empty results", func() {
					_, err := tse.EvaluateTasks([]BuildVariantTask{{Name: ".warm .cool"}})
					So(err, ShouldNotBeNil)
					_, err = tse.EvaluateDeps([]TaskDependency{{Name: ".warm .cool"}})
					So(err, ShouldNotBeNil)
				})
				Convey("conflicting definitions", func() {
					_, err := tse.EvaluateTasks([]BuildVariantTask{
						{Name: "orange", Distros: []string{"d1"}},
						{Name: ".warm .secondary", Distros: []string{"d2"}}})
					So(err, ShouldNotBeNil)
					_, err = tse.EvaluateTasks([]BuildVariantTask{
						{Name: "orange", Distros: []string{"d1"}},
						{Name: ".warm .secondary"}})
					So(err, ShouldNotBeNil)
					_, err = tse.EvaluateDeps([]TaskDependency{
						{Name: "orange", Variant: "woop", Status: "*"},
						{Name: ".warm .secondary", Variant: "woop"}})
					So(err, ShouldNotBeNil)
				})
				Convey("bad selectors", func() {
					_, err := tse.EvaluateTasks([]BuildVariantTask{{Name: "ora!ge"}})
					So(err, ShouldNotBeNil)
					_, err = tse.EvaluateTasks([]BuildVariantTask{{Name: "purp.e"}})
					So(err, ShouldNotBeNil)
					_, err = tse.EvaluateTasks([]BuildVariantTask{{Name: ""}})
					So(err, ShouldNotBeNil)
				})
			})

		})
	})
}
