package shrub

type Task struct {
	Name               string           `json:"name" yaml:"name"`
	Dependencies       []TaskDependency `json:"depends_on,omitempty" yaml:"dependencies,omitempty"`
	Commands           CommandSequence  `json:"commands" yaml:"commands"`
	Tags               []string         `json:"tags,omitempty" yaml:"tags,omitempty"`
	PriorityOverride   int              `json:"priority,omitempty" yaml:"priority_override,omitempty"`
	ExecTimeoutSecs    int              `json:"exec_timeout_secs,omitempty" yaml:"exec_timeout_secs,omitempty"`
	IsPatchable        *bool            `json:"patchable,omitempty" yaml:"patchable,omitempty"`
	IsPatchOnly        *bool            `json:"patch_only,omitempty" yaml:"patch_only,omitempty"`
	IsAllowedForGitTag *bool            `json:"allow_for_git_tag,omitempty" yaml:"allow_for_git_tag,omitempty"`
	IsGitTagOnly       *bool            `json:"git_tag_only,omitempty" yaml:"git_tag_only,omitempty"`
	CanStepback        *bool            `json:"stepback,omitempty" yaml:"stepback,omitempty"`
	MustHaveResults    *bool            `json:"must_have_test_results,omitempty" yaml:"must_have_test_results,omitempty"`
}

type TaskDependency struct {
	Name    string `json:"name" yaml:"name"`
	Variant string `json:"variant" yaml:"variant"`
}

func (t *Task) Command(cmds ...Command) *Task {
	for _, c := range cmds {
		if err := c.Validate(); err != nil {
			panic(err)
		}

		t.Commands = append(t.Commands, c.Resolve())
	}

	return t
}

func (t *Task) AddCommand() *CommandDefinition {
	c := &CommandDefinition{}
	t.Commands = append(t.Commands, c)
	return c
}

func (t *Task) Dependency(dep ...TaskDependency) *Task {
	t.Dependencies = append(t.Dependencies, dep...)
	return t
}

func (t *Task) Function(fns ...string) *Task {
	for _, fn := range fns {
		t.Commands = append(t.Commands, &CommandDefinition{
			FunctionName: fn,
		})
	}

	return t
}

func (t *Task) Tag(tags ...string) *Task {
	t.Tags = append(t.Tags, tags...)
	return t
}

func (t *Task) FunctionWithVars(id string, vars map[string]string) *Task {
	t.Commands = append(t.Commands, &CommandDefinition{
		FunctionName: id,
		Vars:         vars,
	})

	return t
}

func (t *Task) Priority(pri int) *Task {
	t.PriorityOverride = pri
	return t
}

func (t *Task) ExecTimeout(s int) *Task {
	t.ExecTimeoutSecs = s
	return t
}

func (t *Task) Patchable(val bool) *Task {
	t.IsPatchable = &val
	return t
}

func (t *Task) PatchOnly(val bool) *Task {
	t.IsPatchOnly = &val
	return t
}

func (t *Task) AllowForGitTag(val bool) *Task {
	t.IsAllowedForGitTag = &val
	return t
}

func (t *Task) GitTagOnly(val bool) *Task {
	t.IsGitTagOnly = &val
	return t
}

func (t *Task) Stepback(val bool) *Task {
	t.CanStepback = &val
	return t
}

func (t *Task) MustHaveTestResults(val bool) *Task {
	t.MustHaveResults = &val
	return t
}

type TaskGroup struct {
	GroupName             string          `json:"name" yaml:"name"`
	MaxHosts              int             `json:"max_hosts,omitempty" yaml:"max_hosts,omitempty"`
	ShareProcesses        bool            `json:"share_processes,omitempty" yaml:"share_processes,omitemtpy"`
	SetupGroup            CommandSequence `json:"setup_group,omitempty" yaml:"setup_group,omitempty"`
	SetupGroupCanFailTask bool            `json:"setup_group_can_fail_task,omitempty" yaml:"setup_group_can_fail_task,omitempty"`
	SetupGroupTimeoutSecs int             `json:"setup_group_timeout_secs,omitempty" yaml:"setup_group_timeout_secs,omitempty"`
	SetupTask             CommandSequence `json:"setup_task,omitempty" yaml:"setup_task,omitempty"`
	Tasks                 []string        `json:"tasks" yaml:"tasks"`
	Tags                  []string        `json:"tags,omitempty" yaml:"tags,omitempty"`
	TeardownTask          CommandSequence `json:"teardown_task,omitempty" yaml:"teardown_task,omitempty"`
	TeardownGroup         CommandSequence `json:"teardown_group,omitempty" yaml:"teardown_group,omitempty"`
	Timeout               CommandSequence `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

func (g *TaskGroup) Name(id string) *TaskGroup {
	g.GroupName = id
	return g
}

func (g *TaskGroup) SetMaxHosts(num int) *TaskGroup {
	g.MaxHosts = num
	return g
}

func (g *TaskGroup) SetShareProcesses(val bool) *TaskGroup {
	g.ShareProcesses = val
	return g
}

func (g *TaskGroup) SetupGroupCommand(cmds ...Command) *TaskGroup {
	for _, c := range cmds {
		if err := c.Validate(); err != nil {
			panic(err)
		}
		g.SetupGroup = append(g.SetupGroup, c.Resolve())
	}
	return g
}

func (g *TaskGroup) SetSetupGroupCanFailTask(val bool) *TaskGroup {
	g.SetupGroupCanFailTask = val
	return g
}

func (g *TaskGroup) SetSetupGroupTimeoutSecs(timeoutSecs int) *TaskGroup {
	g.SetupGroupTimeoutSecs = timeoutSecs
	return g
}

func (g *TaskGroup) SetupTaskCommand(cmds ...Command) *TaskGroup {
	for _, c := range cmds {
		if err := c.Validate(); err != nil {
			panic(err)
		}
		g.SetupTask = append(g.SetupTask, c.Resolve())
	}
	return g
}

func (g *TaskGroup) Task(id ...string) *TaskGroup {
	g.Tasks = append(g.Tasks, id...)
	return g
}

func (g *TaskGroup) TeardownTaskCommand(cmds ...Command) *TaskGroup {
	for _, c := range cmds {
		if err := c.Validate(); err != nil {
			panic(err)
		}
		g.TeardownTask = append(g.TeardownTask, c.Resolve())
	}
	return g
}

func (g *TaskGroup) TeardownGroupCommand(cmds ...Command) *TaskGroup {
	for _, c := range cmds {
		if err := c.Validate(); err != nil {
			panic(err)
		}
		g.TeardownGroup = append(g.TeardownGroup, c.Resolve())
	}
	return g
}

func (g *TaskGroup) TimeoutCommand(cmds ...Command) *TaskGroup {
	for _, c := range cmds {
		if err := c.Validate(); err != nil {
			panic(err)
		}
		g.Timeout = append(g.Timeout, c.Resolve())
	}
	return g
}

func (g *TaskGroup) Tag(tags ...string) *TaskGroup {
	g.Tags = append(g.Tags, tags...)
	return g
}
