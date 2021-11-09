package shrub

import (
	"testing"
)

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
			require(t, task.PriorityOverride == 0, "default")
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
		"ExecTimeoutSetter": func(t *testing.T, task *Task) {
			require(t, task.ExecTimeoutSecs == 0, "default")
			task.ExecTimeout(50)
			assert(t, task.ExecTimeoutSecs == 50)

			task.ExecTimeout(0)
			assert(t, task.ExecTimeoutSecs == 0)
		},
		"PatchableSetter": func(t *testing.T, task *Task) {
			require(t, task.IsPatchable == nil, "default")
			task.Patchable(true)
			assert(t, task.IsPatchable != nil && *task.IsPatchable)

			task.Patchable(false)
			assert(t, task.IsPatchable != nil && !*task.IsPatchable)
		},
		"PatchOnlySetter": func(t *testing.T, task *Task) {
			require(t, task.IsPatchOnly == nil, "default")
			task.PatchOnly(true)
			assert(t, task.IsPatchOnly != nil && *task.IsPatchOnly)

			task.PatchOnly(false)
			assert(t, task.IsPatchOnly != nil && !*task.IsPatchOnly)
		},
		"AllowForGitTagSetter": func(t *testing.T, task *Task) {
			require(t, task.IsAllowedForGitTag == nil, "default")
			task.AllowForGitTag(true)
			assert(t, task.IsAllowedForGitTag != nil && *task.IsAllowedForGitTag)

			task.AllowForGitTag(false)
			assert(t, task.IsAllowedForGitTag != nil && !*task.IsAllowedForGitTag)
		},
		"GitTagOnlySetter": func(t *testing.T, task *Task) {
			require(t, task.IsGitTagOnly == nil, "default")
			task.GitTagOnly(true)
			assert(t, task.IsGitTagOnly != nil && *task.IsGitTagOnly)

			task.GitTagOnly(false)
			assert(t, task.IsGitTagOnly != nil && !*task.IsGitTagOnly)
		},
		"StepbackSetter": func(t *testing.T, task *Task) {
			require(t, task.CanStepback == nil, "default")
			task.Stepback(true)
			assert(t, task.CanStepback != nil && *task.CanStepback)

			task.Stepback(false)
			assert(t, task.CanStepback != nil && !*task.CanStepback)
		},
		"MustHaveTestResultsSetter": func(t *testing.T, task *Task) {
			require(t, task.MustHaveResults == nil, "default")
			task.MustHaveTestResults(true)
			assert(t, task.MustHaveResults != nil && *task.MustHaveResults)

			task.MustHaveTestResults(false)
			assert(t, task.MustHaveResults != nil && !*task.MustHaveResults)
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
		"TagAdder": func(t *testing.T, task *Task) {
			require(t, len(task.Tags) == 0, "default")
			task.Tag()
			assert(t, len(task.Tags) == 0, "noop")
			task.Tag("one")
			assert(t, len(task.Tags) == 1, "first")
			task.Tag("two")
			assert(t, len(task.Tags) == 2, "add again")
			task.Tag("two", "43")
			assert(t, len(task.Tags) == 4, "multi add without deduplicating")
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
	t.Run("ShareProcessesSetter", func(t *testing.T) {
		g := &TaskGroup{}
		require(t, !g.ShareProcesses)
		g.SetShareProcesses(true)
		assert(t, g.ShareProcesses, "set share processes")
		g.SetShareProcesses(false)
		assert(t, !g.ShareProcesses, "unset share processes")
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
	t.Run("SetupGroupAdder", func(t *testing.T) {
		g := &TaskGroup{}
		assert(t, len(g.SetupGroup) == 0, "default value")
		g.SetupGroupCommand()
		assert(t, len(g.SetupGroup) == 0, "noop")
		g.SetupGroupCommand(CmdExecShell{})
		assert(t, len(g.SetupGroup) == 1, "first command")
		g.SetupGroupCommand(CmdExec{})
		assert(t, len(g.SetupGroup) == 2, "no deduplicate")
		g.SetupGroupCommand(CmdExecShell{}, CmdExec{})
		assert(t, len(g.SetupGroup) == 4, "multi add")
		defer expect(t, "adding invalid command should panic")
		g.SetupGroupCommand(CmdS3Put{})
	})
	t.Run("SetupGroupCanFailTaskSetter", func(t *testing.T) {
		g := &TaskGroup{}
		require(t, !g.SetupGroupCanFailTask)
		g.SetSetupGroupCanFailTask(true)
		assert(t, g.SetupGroupCanFailTask, "setup group can fail tasks")
		g.SetSetupGroupCanFailTask(false)
		assert(t, !g.SetupGroupCanFailTask, "setup group cannot fail tasks")
	})
	t.Run("SetupGroupTimeoutSecsSetter", func(t *testing.T) {
		g := &TaskGroup{}
		require(t, !g.SetupGroupCanFailTask)
		g.SetSetupGroupTimeoutSecs(100)
		assert(t, g.SetupGroupTimeoutSecs == 100, "set setup group timeout secs")
		g.SetSetupGroupTimeoutSecs(0)
		assert(t, g.SetupGroupTimeoutSecs == 0, "unset setup group timeout secs")
	})
	t.Run("SetupTaskAdder", func(t *testing.T) {
		g := &TaskGroup{}
		assert(t, len(g.SetupTask) == 0, "default value")
		g.SetupTaskCommand()
		assert(t, len(g.SetupTask) == 0, "noop")
		g.SetupTaskCommand(CmdExecShell{})
		assert(t, len(g.SetupTask) == 1, "first command")
		g.SetupTaskCommand(CmdExec{})
		assert(t, len(g.SetupTask) == 2, "no deduplicate")
		g.SetupTaskCommand(CmdExecShell{}, CmdExec{})
		assert(t, len(g.SetupTask) == 4, "multi add")
		defer expect(t, "adding invalid command should panic")
		g.SetupTaskCommand(CmdS3Put{})
	})
	t.Run("TeardownTaskAdder", func(t *testing.T) {
		g := &TaskGroup{}
		assert(t, len(g.TeardownTask) == 0, "default value")
		g.TeardownTaskCommand()
		assert(t, len(g.TeardownTask) == 0, "noop")
		g.TeardownTaskCommand(CmdExecShell{})
		assert(t, len(g.TeardownTask) == 1, "first command")
		g.TeardownTaskCommand(CmdExec{})
		assert(t, len(g.TeardownTask) == 2, "no deduplicate")
		g.TeardownTaskCommand(CmdExecShell{}, CmdExec{})
		assert(t, len(g.TeardownTask) == 4, "multi add")
		defer expect(t, "adding invalid command should panic")
		g.TeardownTaskCommand(CmdS3Put{})
	})
	t.Run("TeardownGroupAdder", func(t *testing.T) {
		g := &TaskGroup{}
		assert(t, len(g.TeardownGroup) == 0, "default value")
		g.TeardownGroupCommand()
		assert(t, len(g.TeardownGroup) == 0, "noop")
		g.TeardownGroupCommand(CmdExecShell{})
		assert(t, len(g.TeardownGroup) == 1, "first command")
		g.TeardownGroupCommand(CmdExec{})
		assert(t, len(g.TeardownGroup) == 2, "no deduplicate")
		g.TeardownGroupCommand(CmdExecShell{}, CmdExec{})
		assert(t, len(g.TeardownGroup) == 4, "multi add")
		defer expect(t, "adding invalid command should panic")
		g.TeardownGroupCommand(CmdS3Put{})
	})
	t.Run("TimeoutAdder", func(t *testing.T) {
		g := &TaskGroup{}
		assert(t, len(g.Timeout) == 0, "default value")
		g.TimeoutCommand()
		assert(t, len(g.Timeout) == 0, "noop")
		g.TimeoutCommand(CmdExecShell{})
		assert(t, len(g.Timeout) == 1, "first command")
		g.TimeoutCommand(CmdExec{})
		assert(t, len(g.Timeout) == 2, "no deduplicate")
		g.TimeoutCommand(CmdExecShell{}, CmdExec{})
		assert(t, len(g.Timeout) == 4, "multi add")
		defer expect(t, "adding invalid command should panic")
		g.TimeoutCommand(CmdS3Put{})
	})
	t.Run("TagAdder", func(t *testing.T) {
		g := &TaskGroup{}
		require(t, len(g.Tags) == 0, "default value")
		g.Tag()
		assert(t, len(g.Tags) == 0, "noop")
		g.Tag("one")
		assert(t, len(g.Tags) == 1, "first")
		g.Tag("two")
		assert(t, len(g.Tags) == 2, "add again")
		g.Tag("two", "43")
		assert(t, len(g.Tags) == 4, "multi add without deduplicating")
	})
}
