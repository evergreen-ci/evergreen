package shrub

import "testing"

func TestTaskBuilder(t *testing.T) {
	cases := map[string]func(t *testing.T, task *Task){
		"DependencySetterNoop": func(t *testing.T, task *Task) {
			assert(t, len(task.Dependencies) == 0, "default value")
			t2 := task.Dependency()
			assert(t, task == t2, "chainable")
			assert(t, len(task.Dependencies) == 0, "not changed")
		},
		"DependencySetterOne": func(t *testing.T, task *Task) {
			t2 := task.Dependency(TaskDependency{Name: "foo"})
			assert(t, task == t2, "chainable")
			assert(t, len(task.Dependencies) == 1, "not changed")
			assert(t, task.Dependencies[0].Name == "foo")
		},
		"DependencySetterDuplicate": func(t *testing.T, task *Task) {
			t2 := task.Dependency(TaskDependency{Name: "foo"}).Dependency(
				TaskDependency{Name: "foo"},
				TaskDependency{Name: "bar"})
			assert(t, task == t2, "chainable")
			assert(t, len(task.Dependencies) == 3, "not changed")
			assert(t, task.Dependencies[0].Name == "foo")
			assert(t, task.Dependencies[1].Name == "foo")
			assert(t, task.Dependencies[2].Name == "bar")
		},
		"PrioritySetter": func(t *testing.T, task *Task) {
			assert(t, task.PriorityOverride == 0)
			t2 := task.Priority(42)
			assert(t, task == t2, "chainable")
			assert(t, task.PriorityOverride == 42)

		},
		"PriorityOverride": func(t *testing.T, task *Task) {
			task.Priority(9001)
			assert(t, task.PriorityOverride == 9001)

			task.Priority(0)
			assert(t, task.PriorityOverride == 0)
		},
		"AddCommand": func(t *testing.T, task *Task) {
			assert(t, len(task.Commands) == 0, "default value")
			cmd := task.AddCommand()
			require(t, len(task.Commands) == 1)
			assert(t, task.Commands[0] == cmd)
		},
		"CommandExtenderNoop": func(t *testing.T, task *Task) {
			t2 := task.Command()
			assert(t, task == t2, "chainable")
			require(t, len(task.Commands) == 0)
		},
		"CommandExtenderWithOneValidCommand": func(t *testing.T, task *Task) {
			t2 := task.Command(CmdExec{})
			assert(t, task == t2, "chainable")
			require(t, len(task.Commands) == 1)
		},
		"CommandExtenderWithOneMalformedCommand": func(t *testing.T, task *Task) {
			defer expect(t, "adding invalid command should panic")
			t2 := task.Command(CmdS3Put{})
			assert(t, task == t2, "chainable")
		},
		"CommandExtenderWithManyValidCommands": func(t *testing.T, task *Task) {
			t2 := task.Command(CmdExec{}, CmdExec{}).Command(CmdExec{})
			assert(t, task == t2, "chainable")
			require(t, len(task.Commands) == 3)
		},
		"AddZeroFunctions": func(t *testing.T, task *Task) {
			t2 := task.Function()
			assert(t, task == t2, "chainable")
			require(t, len(task.Commands) == 0)
		},
		"AddOneFunction": func(t *testing.T, task *Task) {
			t2 := task.Function("a")
			assert(t, task == t2, "chainable")
			require(t, len(task.Commands) == 1)
			assert(t, task.Commands[0].FunctionName == "a")
		},
		"AddMultipleFunctions": func(t *testing.T, task *Task) {
			t2 := task.Function("a").Function("b").Function("a", "b", "C")
			assert(t, task == t2, "chainable")
			require(t, len(task.Commands) == 5)
			assert(t, task.Commands[0].FunctionName == "a")
			assert(t, task.Commands[2].FunctionName == "a")
			assert(t, task.Commands[4].FunctionName == "C")
		},
		"AddOneFunctionWithEmptyVars": func(t *testing.T, task *Task) {
			t2 := task.FunctionWithVars("foo", nil)
			assert(t, task == t2, "chainable")
			require(t, len(task.Commands) == 1)
			assert(t, task.Commands[0].FunctionName == "foo")
			assert(t, task.Commands[0].Vars == nil)
		},
		"AddOneFunctionWithZeroVals": func(t *testing.T, task *Task) {
			t2 := task.FunctionWithVars("", nil)
			assert(t, task == t2, "chainable")
			require(t, len(task.Commands) == 1)
			assert(t, task.Commands[0].Vars == nil)
			assert(t, task.Commands[0].FunctionName == "")
		},
		"AddOneFunctionWithVars": func(t *testing.T, task *Task) {
			t2 := task.FunctionWithVars("foo", map[string]string{"a": "val"})
			assert(t, task == t2, "chainable")
			require(t, len(task.Commands) == 1)
			assert(t, task.Commands[0].FunctionName == "foo")
			require(t, task.Commands[0].Vars != nil)
			assert(t, task.Commands[0].Vars["a"] == "val")
		},
	}

	for name, test := range cases {
		task := &Task{}
		t.Run(name, func(t *testing.T) {
			test(t, task)
		})
	}
}

func TestTaskGroup(t *testing.T) {
	t.Run("NameSetter", func(t *testing.T) {
		g := &TaskGroup{}
		assert(t, g.GroupName == "")
		g.Name("hello")
		assert(t, g.GroupName == "hello")
	})
	t.Run("MaxHostsSetter", func(t *testing.T) {
		g := &TaskGroup{}
		assert(t, g.MaxHosts == 0)
		g.SetMaxHosts(1)
		assert(t, g.MaxHosts == 1)
		g.SetMaxHosts(1066)
		assert(t, g.MaxHosts == 1066)
	})
	t.Run("TaskAdder", func(t *testing.T) {
		g := &TaskGroup{}
		assert(t, len(g.Tasks) == 0, "default value")
		g.Task()
		assert(t, len(g.Tasks) == 0, "noop")
		g.Task("one")
		assert(t, len(g.Tasks) == 1, "first")
		g.Task("two")
		assert(t, len(g.Tasks) == 2, "no deduplicate")
		g.Task("two", "43")
		assert(t, len(g.Tasks) == 4, "multi add")
	})
}
