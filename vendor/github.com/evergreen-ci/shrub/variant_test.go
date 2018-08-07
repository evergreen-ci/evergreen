package shrub

import "testing"

func TestVariantBuilders(t *testing.T) {
	cases := map[string]func(*testing.T, *Variant){
		"NameSetter": func(t *testing.T, v *Variant) {
			assert(t, v.BuildName == "", "default value")
			v2 := v.Name("foo")
			assert(t, v.BuildName == "foo", "expected value")
			assert(t, v2 == v, "chainable")
		},
		"DisplayNameSetter": func(t *testing.T, v *Variant) {
			assert(t, v.BuildDisplayName == "", "default value")
			v2 := v.DisplayName("foo")
			assert(t, v.BuildDisplayName == "foo", "expected value")
			assert(t, v2 == v, "chainable")
		},
		"RunOnSetter": func(t *testing.T, v *Variant) {
			assert(t, len(v.DistroRunOn) == 0, "default value")
			v2 := v.RunOn("foo")

			require(t, len(v.DistroRunOn) == 1, "set")
			assert(t, v.DistroRunOn[0] == "foo", "expected value")
			assert(t, v2 == v, "chainable")
		},
		"RunOnMultipleTimes": func(t *testing.T, v *Variant) {
			v2 := v.RunOn("foo").RunOn("bar").RunOn("baz")

			require(t, len(v.DistroRunOn) == 1, "set")
			assert(t, v.DistroRunOn[0] == "baz", "expected value")
			assert(t, v2 == v, "chainable")
		},
		"TaskSpecSetterFirst": func(t *testing.T, v *Variant) {
			v2 := v.TaskSpec(TaskSpec{Name: "foo"})
			require(t, len(v.TaskSpecs) == 1, "set")
			assert(t, v.TaskSpecs[0].Name == "foo", "expected value")
			assert(t, v2 == v, "chainable")
		},
		"TaskSpecSetterSecond": func(t *testing.T, v *Variant) {
			v2 := v.TaskSpec(TaskSpec{Name: "first"}).TaskSpec(TaskSpec{Name: "foo"})
			require(t, len(v.TaskSpecs) == 2, "set")
			assert(t, v.TaskSpecs[0].Name == "first", "expected value")
			assert(t, v.TaskSpecs[1].Name == "foo", "expected value")
			assert(t, v2 == v, "chainable")
		},
		"SetExpansionSetter": func(t *testing.T, v *Variant) {
			v.Expanisons = map[string]interface{}{}
			assert(t, v.Expanisons != nil)
			v2 := v.SetExpansions(nil)
			assert(t, v2 == v, "chainable")
			assert(t, v.Expanisons == nil)
		},
		"SetExpansionOverride": func(t *testing.T, v *Variant) {
			v.Expanisons = map[string]interface{}{"b": "one"}
			assert(t, len(v.Expanisons) == 1)
			v2 := v.SetExpansions(map[string]interface{}{"a": "two"})
			assert(t, v2 == v, "chainable")
			assert(t, len(v.Expanisons) == 1)
			assert(t, v.Expanisons["a"] == "two")
		},
		"AddExpansionFirst": func(t *testing.T, v *Variant) {
			assert(t, v.Expanisons == nil)
			v2 := v.Expansion("one", 2)
			assert(t, v2 == v, "chainable")
			assert(t, len(v.Expanisons) == 1)
			assert(t, v.Expanisons["one"] == 2)
		},
		"AddExpansionSecond": func(t *testing.T, v *Variant) {
			v2 := v.Expansion("one", 2).Expansion("two", 42)
			assert(t, v2 == v, "chainable")
			assert(t, len(v.Expanisons) == 2)
			assert(t, v.Expanisons["two"] == 42)
		},
		"DisplayTaskNil": func(t *testing.T, v *Variant) {
			assert(t, len(v.DisplayTaskSpecs) == 0, "default value")
			v2 := v.DisplayTasks()
			assert(t, v2 == v, "chainable")
			assert(t, len(v.DisplayTaskSpecs) == 0, "length unchanged")
		},
		"DisplayTaskWithValues": func(t *testing.T, v *Variant) {
			v2 := v.DisplayTasks(DisplayTaskDefinition{Name: "one"},
				DisplayTaskDefinition{Name: "two"}).DisplayTasks(
				DisplayTaskDefinition{Name: "3"})

			assert(t, v2 == v, "chainable")
			assert(t, len(v.DisplayTaskSpecs) == 3, "length unchanged")
			assert(t, v.DisplayTaskSpecs[0].Name == "one")
			assert(t, v.DisplayTaskSpecs[1].Name == "two")
			assert(t, v.DisplayTaskSpecs[2].Name == "3")
		},
		"AddNoopTasks": func(t *testing.T, v *Variant) {
			assert(t, len(v.TaskSpecs) == 0, "default value")
			v2 := v.AddTasks("", "", "")
			assert(t, v2 == v, "chainable")
			assert(t, len(v.TaskSpecs) == 0, "no changes")
		},
		"AddSingleTask": func(t *testing.T, v *Variant) {
			assert(t, len(v.TaskSpecs) == 0, "default value")
			v2 := v.AddTasks("taskName")
			assert(t, v2 == v, "chainable")
			assert(t, len(v.TaskSpecs) == 1, "no changes")
			assert(t, v.TaskSpecs[0].Name == "taskName")
		},

		"AddSameTasks": func(t *testing.T, v *Variant) {
			assert(t, len(v.TaskSpecs) == 0, "default value")
			v2 := v.AddTasks("one", "one", "one")
			assert(t, v2 == v, "chainable")
			assert(t, len(v.TaskSpecs) == 3, "state impacted")
		},
		"AddDifferentTasks": func(t *testing.T, v *Variant) {
			assert(t, len(v.TaskSpecs) == 0, "default value")
			v2 := v.AddTasks("one", "two")
			assert(t, v2 == v, "chainable")
			assert(t, len(v.TaskSpecs) == 2, "state impacted")
		},
	}

	for name, test := range cases {
		v := &Variant{}
		t.Run(name, func(t *testing.T) {
			test(t, v)
		})
	}
}
