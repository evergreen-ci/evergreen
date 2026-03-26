package model

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
)

var (
	bvBatchTime            = 60
	taskBatchTime          = 30
	sampleGeneratedProject = GeneratedProject{
		Tasks: []parserTask{
			{
				Name: "new_task",
				Commands: []PluginCommandConf{
					{
						Command: "shell.exec",
					},
				},
			},
			{
				Name: "another_task",
				Commands: []PluginCommandConf{
					{
						Command: "shell.exec",
					},
				},
				DependsOn: []parserDependency{
					{TaskSelector: taskSelector{
						Name:    "a-depended-on-task",
						Variant: &variantSelector{StringSelector: "*"},
					}},
				},
			},
		},
		BuildVariants: []parserBV{
			{
				Name:  "new_buildvariant",
				RunOn: []string{"arch"},
			},
			{
				Name: "a_variant",
				Tasks: parserBVTaskUnits{
					parserBVTaskUnit{
						Name:      "say-bye",
						BatchTime: &taskBatchTime,
					},
				},
				DisplayTasks: []displayTask{
					{
						Name:           "my_display_task_old_variant",
						ExecutionTasks: []string{"say-bye"},
					},
				},
			},
			{
				Name:      "another_variant",
				BatchTime: &bvBatchTime,
				Tasks: parserBVTaskUnits{
					parserBVTaskUnit{
						Name: "example_task_group",
					},
					parserBVTaskUnit{
						Name:      "say-bye",
						BatchTime: &taskBatchTime,
					},
				},
				RunOn: []string{"arch"},
				DisplayTasks: []displayTask{
					{
						Name:           "my_display_task_new_variant",
						ExecutionTasks: []string{"another_task"},
					},
				},
			},
		},
		Functions: map[string]*YAMLCommandSet{
			"new_function": {
				MultiCommand: []PluginCommandConf{},
			},
		},
		TaskGroups: []parserTaskGroup{
			{
				Name:     "example_task_group",
				MaxHosts: 1,
				Tasks: []string{
					"another_task",
				},
			},
		},
	}

	sampleGeneratedProjectWithAllMultiFields = GeneratedProject{
		BuildVariants: []parserBV{
			{
				Name: "honeydew",
				Tasks: []parserBVTaskUnit{
					{Name: "cat"},
					{Name: "doge"},
					{Name: "pika"},
				},
				DisplayTasks: []displayTask{},
			},
			{
				Name: "cantaloupe",
				Tasks: []parserBVTaskUnit{
					{Name: "quokka"},
				},
				DisplayTasks: []displayTask{
					{Name: "grouse"},
					{Name: "albatross"},
				},
			},
		},
		Tasks: []parserTask{
			{
				Name: "quokka",
				Commands: []PluginCommandConf{
					{Command: "shell.exec"},
					{Command: "shell.exec"},
				},
			},
			{
				Name: "pika",
				Commands: []PluginCommandConf{
					{Command: "shell.exec"},
				},
			},
		},
		TaskGroups: []parserTaskGroup{
			{
				Name:  "sea-bunny",
				Tasks: []string{"quokka", "pika"},
			},
			{
				Name:  "mola-mola",
				Tasks: []string{"quokka"},
			},
		},
		Functions: map[string]*YAMLCommandSet{
			"brownie": {MultiCommand: []PluginCommandConf{
				{Command: "shell.exec"},
				{Command: "shell.exec"},
			}},
			"cookie": {MultiCommand: []PluginCommandConf{
				{Command: "shell.exec"},
			}},
		},
	}

	smallGeneratedProject = GeneratedProject{
		BuildVariants: []parserBV{
			{
				Name: "my_build_variant",
				DisplayTasks: []displayTask{
					{
						Name:           "my_display_task",
						ExecutionTasks: []string{"my_display_task_gen"},
					},
				},
			},
		},
	}

	sampleGeneratedProjectAddToBVOnly = GeneratedProject{
		BuildVariants: []parserBV{
			{
				Name: "a_variant",
				Tasks: parserBVTaskUnits{
					parserBVTaskUnit{
						Name: "task_that_has_dependencies",
					},
				},
			},
		},
	}
	generatedProjectWithNewBuild = GeneratedProject{
		Tasks: []parserTask{
			{
				Name: "task_one",
				Commands: []PluginCommandConf{
					{
						Command: "shell.exec",
					},
				},
				DependsOn: []parserDependency{
					{TaskSelector: taskSelector{
						Name: "dependency",
					}},
				},
			},
			{
				Name: "dependency",

				Commands: []PluginCommandConf{
					{
						Command: "shell.exec",
					},
				},
				DependsOn: []parserDependency{
					{TaskSelector: taskSelector{
						Name:    "nested-dependency",
						Variant: &variantSelector{StringSelector: "*"},
					}},
				},
			},
		},
		BuildVariants: []parserBV{
			{
				Name: "another_variant",
				Tasks: parserBVTaskUnits{
					parserBVTaskUnit{
						Name: "task_one",
					},
					parserBVTaskUnit{
						Name:     "dependency",
						Activate: utility.FalsePtr(),
					},
				},
			},
		},
	}

	partiallyGeneratedProject = GeneratedProject{
		Tasks: []parserTask{
			{
				Name: "new_task",
				Commands: []PluginCommandConf{
					{
						Command: "shell.exec",
					},
				},
			},
			{
				Name: "another_new_task",
				Commands: []PluginCommandConf{
					{
						Command: "shell.exec",
					},
				},
			},
		},
		BuildVariants: []parserBV{
			{
				Name:  "new_variant",
				RunOn: []string{"arch"},
				Tasks: parserBVTaskUnits{
					parserBVTaskUnit{
						Name: "new_task",
					},
					parserBVTaskUnit{
						Name: "another_new_task",
					},
				},
			},
			{
				Name:  "another_new_variant",
				RunOn: []string{"arch"},
				Tasks: parserBVTaskUnits{
					parserBVTaskUnit{
						Name: "another_new_task",
					},
				},
			},
		},
	}

	projYmlTwoBVs = `
tasks:
  - name: nested-dependency
    command:
      - command: shell.exec
  - name: say-bye
    command:
      - command: shell.exec
buildvariants:
  - name: a_variant
    display_name: Variant Number One
    run_on:
    - "arch"
    tasks:
    - name: nested-dependency
  - name: another_variant
    display_name: Variant Number Two
    run_on:
    - "arch"
    tasks:
    - name: say-bye
`

	sampleProjYml = `
tasks:
  - name: say-hi
    commands:
      - command: shell.exec
  - name: say-bye
    commands:
      - command: shell.exec
  - name: a-depended-on-task
    command:
      - command: shell.exec
  - name: task_that_has_dependencies
    commands:
      - command: shell.exec
    depends_on:
      - name: "*"
        status: "*"

buildvariants:
  - name: a_variant
    display_name: Variant Number One
    run_on:
    - "arch"
    tasks:
    - name: say-hi
    - name: a-depended-on-task

functions:
  a_function:
    command: shell.exec
`
	sampleProjYmlTaskGroups = `
tasks:
  - name: say-hi
    commands:
      - command: shell.exec
  - name: say-bye
    commands:
      - command: shell.exec
  - name: say_something_else
  - name: task_that_has_dependencies
    commands:
      - command: shell.exec
    depends_on:
      - name: "*"

task_groups:
- name: my_task_group
  max_hosts: 1
  tasks:
  - say-hi
  - say-bye

buildvariants:
  - name: a_variant
    display_name: Variant Number One
    run_on:
    - "arch"
    tasks:
    - name: my_task_group
    - name: say_something_else

functions:
  a_function:
    command: shell.exec
`
	smallYml = `
tasks:
  - name: my_display_task_gen

buildvariants:
  - name: my_build_variant
    display_name: Variant Number One
    run_on:
    - "arch"
    tasks:
    - name: my_display_task_gen
`
	sampleProjYmlNoFunctions = `
tasks:
  - name: say-hi
    commands:
      - command: shell.exec
  - name: say-bye
    commands:
      - command: shell.exec
  - name: a-depended-on-task
    command:
      - command: shell.exec

buildvariants:
  - name: a_variant
    display_name: Variant Number One
    run_on:
    - "arch"
    tasks:
    - name: say-hi
    - name: a-depended-on-task
`

	sampleProjYAMLWithMultiFields = `
tasks:
- name: blueberry
  commands:
    - command: shell.exec
- name: strawberry
  commands:
    - command: shell.exec
- name: banana-is-a-berry
  commands:
    - command: shell.exec

buildvariants:
- name: rutabaga
  tasks:
    - name: blueberry
    - name: strawberry
- name: sweet-potato
  tasks:
    - name: strawberry
    - name: lotta-fruits

functions:
  purple:
    - command: shell.exec
    - command: shell.exec
  orange:
    - command: shell.exec

task_groups:
- name: i-am-a-fruitarian
  tasks:
    - blueberry
    - strawberry
- name: lotta-fruits
  tasks:
    - blueberry
    - banana-is-a-berry
`

	sampleGenerateTasksYml = `
{
    "functions": {
        "echo-hi": {
            "command": "shell.exec",
            "params": {
                "script": "echo hi"
            }
        },
        "echo-bye": [
            {
                "command": "shell.exec",
                "params": {
                    "script": "echo bye"
                }
            },
            {
                "command": "shell.exec",
                "params": {
                    "script": "echo bye again"
                }
            }
        ]
    },
    "tasks": [
        {
            "commands": [
                {
                    "command": "git.get_project",
                    "params": {
                        "directory": "src"
                    }
                },
                {
                    "func": "echo-hi"
                }
            ],
            "name": "test"
        }
    ],
    "buildvariants": [
        {
            "tasks": [
                {
                    "name": "test"
                }
            ],
            "display_name": "Ubuntu 16.04 Small",
            "run_on": [
                "ubuntu1604-test"
            ],
            "name": "first",
            "display_tasks": [
                {
                    "name": "display",
                    "execution_tasks": [
                        "test"
                    ]
                }
            ]
        },
        {
            "tasks": [
                {
                    "name": "test"
                }
            ],
            "display_name": "Ubuntu 16.04 Large",
            "run_on": [
                "ubuntu1604-build"
            ],
            "name": "second"
        }
    ],
    "task_groups": [
        {
            "name": "my_task_group",
            "max_hosts": 1,
            "tasks": [
              "test",
            ]
        }
    ]
}
`
)

type GenerateSuite struct {
	suite.Suite
	ctx            context.Context
	cancel         context.CancelFunc
	env            evergreen.Environment
	originalConfig *evergreen.Settings
}

func TestGenerateSuite(t *testing.T) {
	suite.Run(t, new(GenerateSuite))
}

func (s *GenerateSuite) SetupTest() {
	s.Require().NoError(db.ClearCollections(task.Collection, build.Collection, VersionCollection, ParserProjectCollection, ProjectRefCollection))
	ref := ProjectRef{
		Id: "proj",
	}
	s.Require().NoError(ref.Insert(s.ctx))
	ref2 := ProjectRef{
		Id: "",
	}
	s.Require().NoError(ref2.Insert(s.ctx))
	s.ctx, s.cancel = context.WithCancel(context.Background())
	env := &mock.Environment{}
	s.Require().NoError(env.Configure(s.ctx))
	s.env = env
	originalConfig, err := evergreen.GetConfig(s.ctx)
	s.Require().NoError(err)
	s.originalConfig = originalConfig
}

func (s *GenerateSuite) TearDownTest() {
	if s.originalConfig != nil {
		s.NoError(evergreen.UpdateConfig(s.ctx, s.originalConfig))
	}
}

func (s *GenerateSuite) TestParseProjectFromJSON() {
	g, err := ParseProjectFromJSONString(sampleGenerateTasksYml)
	s.NotNil(g)
	s.NoError(err)

	s.Len(g.Functions, 2)
	s.Contains(g.Functions, "echo-hi")
	s.Equal("shell.exec", g.Functions["echo-hi"].List()[0].Command)
	s.Equal("echo hi", g.Functions["echo-hi"].List()[0].Params["script"])
	s.Equal("echo bye", g.Functions["echo-bye"].List()[0].Params["script"])
	s.Equal("echo bye again", g.Functions["echo-bye"].List()[1].Params["script"])

	s.Len(g.Tasks, 1)
	s.Equal("git.get_project", g.Tasks[0].Commands[0].Command)

	s.Equal("src", g.Tasks[0].Commands[0].Params["directory"])
	s.Equal("echo-hi", g.Tasks[0].Commands[1].Function)
	s.Equal("test", g.Tasks[0].Name)

	s.Len(g.BuildVariants, 2)
	s.Equal("Ubuntu 16.04 Large", g.BuildVariants[1].DisplayName)
	s.Equal("second", g.BuildVariants[1].Name)
	s.Equal("test", g.BuildVariants[1].Tasks[0].Name)
	s.Equal("ubuntu1604-build", g.BuildVariants[1].RunOn[0])
	s.Equal("test", g.BuildVariants[0].DisplayTasks[0].ExecutionTasks[0])
	s.Len(g.BuildVariants[0].DisplayTasks, 1)
	s.Equal("display", g.BuildVariants[0].DisplayTasks[0].Name)

	s.Len(g.TaskGroups, 1)
	s.Equal("my_task_group", g.TaskGroups[0].Name)
	s.Equal(1, g.TaskGroups[0].MaxHosts)
	s.Equal("test", g.TaskGroups[0].Tasks[0])
}

func (s *GenerateSuite) TestValidateMaxVariants() {
	g := GeneratedProject{}
	s.NoError(g.validateMaxTasksAndVariants())
	for i := 0; i < maxGeneratedBuildVariants; i++ {
		g.BuildVariants = append(g.BuildVariants, parserBV{})
	}
	s.NoError(g.validateMaxTasksAndVariants())
	g.BuildVariants = append(g.BuildVariants, parserBV{})
	s.Error(g.validateMaxTasksAndVariants())
}

func (s *GenerateSuite) TestValidateMaxTasks() {
	g := GeneratedProject{}
	s.NoError(g.validateMaxTasksAndVariants())
	for i := 0; i < maxGeneratedTasks; i++ {
		g.Tasks = append(g.Tasks, parserTask{})
	}
	s.NoError(g.validateMaxTasksAndVariants())
	g.Tasks = append(g.Tasks, parserTask{})
	s.Error(g.validateMaxTasksAndVariants())
}

func (s *GenerateSuite) TestSaveWithMaxTasksPerVersion() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	settings := &evergreen.Settings{
		TaskLimits: evergreen.TaskLimitsConfig{
			MaxTasksPerVersion: 5,
		},
	}
	s.NoError(evergreen.UpdateConfig(ctx, settings))
	tasksThatExist := []task.Task{
		{
			Id:      "task_that_called_generate_task",
			Version: "version_that_called_generate_task",
			BuildId: "generate_build",
		},
		{
			Id:          "say-hi-task-id",
			Version:     "version_that_called_generate_task",
			BuildId:     "sample_build",
			DisplayName: "say-hi",
			Activated:   true,
		},
		{
			Id:          "say-bye-task-id",
			Version:     "version_that_called_generate_task",
			BuildId:     "sample_build",
			DisplayName: "say-bye",
		},
		{
			Id:          "say_something_else",
			Version:     "version_that_called_generate_task",
			BuildId:     "sample_build",
			DisplayName: "say_something_else",
		},
	}
	for _, t := range tasksThatExist {
		s.NoError(t.Insert(s.ctx))
	}
	sampleBuild := build.Build{
		Id:           "sample_build",
		BuildVariant: "a_variant",
		Version:      "version_that_called_generate_task",
	}
	v := &Version{
		Id:       "version_that_called_generate_task",
		BuildIds: []string{"sample_build"},
	}
	pp := &ParserProject{}
	err := util.UnmarshalYAMLWithFallback([]byte(sampleProjYmlTaskGroups), &pp)
	s.NoError(err)
	pp.Id = "version_that_called_generate_task"
	s.NoError(pp.Insert(s.ctx))
	s.NoError(sampleBuild.Insert(s.ctx))
	s.NoError(v.Insert(s.ctx))

	g := sampleGeneratedProjectAddToBVOnly
	g.Task = &tasksThatExist[0]
	p, pp, err := FindAndTranslateProjectForVersion(s.ctx, s.env.Settings(), v, false)
	s.Require().NoError(err)
	p, pp, v, err = g.NewVersion(context.Background(), p, pp, v)
	s.NoError(err)
	s.Error(g.Save(s.ctx, s.env.Settings(), p, pp, v))

	settings = &evergreen.Settings{
		TaskLimits: evergreen.TaskLimitsConfig{
			MaxTasksPerVersion: 10,
		},
	}
	s.NoError(evergreen.UpdateConfig(ctx, settings))

	s.NoError(g.Save(s.ctx, s.env.Settings(), p, pp, v))

	generatorTask, err := task.FindOneId(ctx, tasksThatExist[0].Id)
	s.NoError(err)
	s.Require().NotNil(generatorTask)
	s.Equal(4, generatorTask.NumGeneratedTasks)
}

func (s *GenerateSuite) TestValidateNoRedefine() {
	g := GeneratedProject{}
	s.NoError(g.validateNoRedefine(projectMaps{}))

	g.BuildVariants = []parserBV{{Name: "buildvariant_name", DisplayName: "I am a buildvariant"}}
	g.Tasks = []parserTask{{Name: "task_name"}}
	g.Functions = map[string]*YAMLCommandSet{"function_name": nil}
	s.NoError(g.validateNoRedefine(projectMaps{}))

	cachedProject := projectMaps{
		buildVariants: map[string]struct{}{
			"buildvariant_name": {},
		},
	}
	s.Error(g.validateNoRedefine(cachedProject))

	cachedProject = projectMaps{
		tasks: map[string]*ProjectTask{
			"task_name": {},
		},
	}
	s.Error(g.validateNoRedefine(cachedProject))

	cachedProject = projectMaps{
		functions: map[string]*YAMLCommandSet{
			"function_name": {},
		},
	}
	s.Error(g.validateNoRedefine(cachedProject))
}

func (s *GenerateSuite) TestValidateNoRecursiveGenerateTasks() {
	g := GeneratedProject{}
	cachedProject := projectMaps{}
	s.NoError(g.validateNoRecursiveGenerateTasks(cachedProject))

	cachedProject = projectMaps{}
	g = GeneratedProject{
		Tasks: []parserTask{
			{
				Commands: []PluginCommandConf{
					{
						Command: "generate.tasks",
					},
				},
			},
		},
	}
	s.Error(g.validateNoRecursiveGenerateTasks(cachedProject))

	cachedProject = projectMaps{}
	g = GeneratedProject{
		Functions: map[string]*YAMLCommandSet{
			"a_function": {
				MultiCommand: []PluginCommandConf{
					{
						Command: "generate.tasks",
					},
				},
			},
		},
	}
	s.Error(g.validateNoRecursiveGenerateTasks(cachedProject))

	cachedProject = projectMaps{
		tasks: map[string]*ProjectTask{
			"task_name": {
				Commands: []PluginCommandConf{
					{
						Command: "generate.tasks",
					},
				},
			},
		},
	}
	g = GeneratedProject{
		BuildVariants: []parserBV{
			{
				Tasks: []parserBVTaskUnit{
					{
						Name: "task_name",
					},
				},
			},
		},
	}
	s.Error(g.validateNoRecursiveGenerateTasks(cachedProject))

	cachedProject = projectMaps{
		tasks: map[string]*ProjectTask{
			"task_name": {
				Commands: []PluginCommandConf{
					{
						Function: "generate_function",
					},
				},
			},
		},
		functions: map[string]*YAMLCommandSet{
			"generate_function": {
				MultiCommand: []PluginCommandConf{
					{
						Command: "generate.tasks",
					},
				},
			},
		},
	}
	g = GeneratedProject{
		BuildVariants: []parserBV{
			{
				Tasks: []parserBVTaskUnit{
					{
						Name: "task_name",
					},
				},
			},
		},
	}
	s.Error(g.validateNoRecursiveGenerateTasks(cachedProject))
}

func (s *GenerateSuite) TestCacheProjectData() {
	var p Project
	_, err := LoadProjectInto(s.ctx, []byte(sampleProjYAMLWithMultiFields), nil, "", &p)
	s.Require().NoError(err)
	cached := cacheProjectData(&p)
	expectedBVs := map[string]bool{
		"rutabaga":     false,
		"sweet-potato": false,
	}
	for bvName := range cached.buildVariants {
		_, ok := expectedBVs[bvName]
		s.True(ok, "unexpected build variant '%s'", bvName)
		expectedBVs[bvName] = true
	}
	for bvName, found := range expectedBVs {
		s.True(found, "did not find expected build variant '%s'", bvName)
	}

	expectedTasks := map[string]bool{
		"blueberry":         false,
		"strawberry":        false,
		"banana-is-a-berry": false,
	}
	for taskName, tsk := range cached.tasks {
		_, ok := expectedTasks[taskName]
		s.True(ok, "unexpected build variant '%s'", taskName)
		s.Equal(taskName, tsk.Name, "task name key does not match the task it maps to")
		expectedTasks[taskName] = true
	}
	for taskName, found := range expectedTasks {
		s.True(found, "did not find expected task '%s'", taskName)
	}

	expectedFuncs := map[string]struct {
		numCmds int
		found   bool
	}{
		"purple": {numCmds: 2},
		"orange": {numCmds: 1},
	}
	for funcName, funcCmds := range cached.functions {
		expected, ok := expectedFuncs[funcName]
		s.True(ok, "unexpected function '%s'", funcName)
		s.Len(funcCmds.List(), expected.numCmds)

		expected.found = true
		expectedFuncs[funcName] = expected
	}
	for funcName, expected := range expectedFuncs {
		s.True(expected.found, "did not find expected function '%s'", funcName)
	}
}

func (s *GenerateSuite) TestAddGeneratedProjectToConfig() {
	p := &Project{}
	pp, err := LoadProjectInto(s.ctx, []byte(sampleProjYml), nil, "", p)
	s.NoError(err)
	cachedProject := cacheProjectData(p)
	g := sampleGeneratedProject
	newPP, err := g.addGeneratedProjectToConfig(pp, cachedProject)
	s.NoError(err)
	s.NotEmpty(newPP)
	s.Require().Len(newPP.Tasks, 6)
	s.Require().Len(newPP.BuildVariants, 3)
	s.Require().Len(newPP.Functions, 2)
	s.Equal("say-hi", newPP.Tasks[0].Name)
	s.Equal("say-bye", newPP.Tasks[1].Name)
	s.Equal("a-depended-on-task", newPP.Tasks[2].Name)
	s.Equal("task_that_has_dependencies", newPP.Tasks[3].Name)
	s.Equal("new_task", newPP.Tasks[4].Name)
	s.Equal("another_task", newPP.Tasks[5].Name)

	newPP2, err := g.addGeneratedProjectToConfig(&ParserProject{Functions: map[string]*YAMLCommandSet{}}, cachedProject)
	s.NoError(err)
	s.NotEmpty(newPP2)

	s.Equal("a_variant", newPP.BuildVariants[0].Name)
	s.Require().Len(newPP.BuildVariants[0].DisplayTasks, 1)
	s.Equal("my_display_task_old_variant", newPP.BuildVariants[0].DisplayTasks[0].Name)

	s.Equal("new_buildvariant", newPP.BuildVariants[1].Name)
	s.Empty(newPP.BuildVariants[1].DisplayTasks)

	s.Equal("another_variant", newPP.BuildVariants[2].Name)
	s.Require().Len(newPP.BuildVariants[2].DisplayTasks, 1)
	s.Equal("my_display_task_new_variant", newPP.BuildVariants[2].DisplayTasks[0].Name)

	_, ok := newPP.Functions["a_function"]
	s.True(ok)
	_, ok = newPP.Functions["new_function"]
	s.True(ok)

	pp, err = LoadProjectInto(s.ctx, []byte(sampleProjYmlNoFunctions), nil, "", p)
	s.NoError(err)
	newPP, err = g.addGeneratedProjectToConfig(pp, cachedProject)
	s.NoError(err)
	s.NotNil(newPP)
	s.Require().Len(newPP.Tasks, 5)
	s.Require().Len(newPP.BuildVariants, 3)
	s.Len(newPP.Functions, 1)
	s.Equal("say-hi", newPP.Tasks[0].Name)
	s.Equal("say-bye", newPP.Tasks[1].Name)
	s.Equal("new_task", newPP.Tasks[3].Name)

	newPP2, err = g.addGeneratedProjectToConfig(&ParserProject{Functions: map[string]*YAMLCommandSet{}}, cachedProject)
	s.NoError(err)
	s.NotEmpty(newPP2)
}

func (s *GenerateSuite) TestSaveNewBuildsAndTasksWithBatchtime() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := &mock.Environment{}
	s.Require().NoError(env.Configure(ctx))

	genTask := &task.Task{
		Id:                   "task_that_called_generate_task",
		Project:              "proj",
		Version:              "version_that_called_generate_task",
		Priority:             10,
		BuildId:              "sample_build",
		Activated:            true,
		DisplayName:          "task_that_called_generate_task",
		IsEssentialToSucceed: true,
	}
	s.NoError(genTask.Insert(s.ctx))
	prevBatchTimeVersion := Version{
		Id:         "prev_version",
		Identifier: "proj",
		Requester:  evergreen.RepotrackerVersionRequester,
		BuildVariants: []VersionBuildStatus{
			{
				BuildId:      "prev_build",
				BuildVariant: "another_variant",
				ActivationStatus: ActivationStatus{
					Activated:  true,
					ActivateAt: time.Now(),
				},
				BatchTimeTasks: []BatchTimeTaskStatus{
					{
						TaskId:   "prev_task",
						TaskName: "say-bye",
						ActivationStatus: ActivationStatus{
							Activated:  true,
							ActivateAt: time.Now().Add(-time.Minute * 15), // 15 minutes ago
						},
					},
				},
			},
		},
	}
	s.NoError(prevBatchTimeVersion.Insert(s.ctx))

	sampleBuild := build.Build{
		Id:           "sample_build",
		Project:      "proj",
		BuildVariant: "a_variant",
		Version:      "version_that_called_generate_task",
	}
	s.NoError(sampleBuild.Insert(s.ctx))
	v := &Version{
		Id:         "version_that_called_generate_task",
		Identifier: "proj",
		BuildIds:   []string{"sample_build"},
		BuildVariants: []VersionBuildStatus{
			{
				BuildId:      "sample_build",
				BuildVariant: "a_variant",
			},
		},
		CreateTime: time.Now(),
		Requester:  evergreen.RepotrackerVersionRequester,
	}
	s.NoError(v.Insert(s.ctx))
	pp := &ParserProject{}
	err := util.UnmarshalYAMLWithFallback([]byte(sampleProjYml), &pp)
	s.NoError(err)
	pp.Id = "version_that_called_generate_task"
	s.NoError(pp.Insert(s.ctx))

	g := sampleGeneratedProject
	g.Task = genTask

	p, pp, err := FindAndTranslateProjectForVersion(ctx, env.Settings(), v, false)
	s.Require().NoError(err)
	p, pp, v, err = g.NewVersion(context.Background(), p, pp, v)
	s.Require().NoError(err)
	s.NoError(g.Save(s.ctx, s.env.Settings(), p, pp, v))

	// Should have created a pre-generated cached parser project
	preGeneratedPP, err := ParserProjectFindOneByID(ctx, s.env.Settings(), v.ProjectStorageMethod, preGeneratedParserProjectId(pp.Id))
	s.NoError(err)
	s.NotNil(preGeneratedPP)

	// verify we stopped saving versions
	v, err = VersionFindOneIdWithBuildVariants(s.ctx, v.Id)
	s.Equal(evergreen.ProjectStorageMethodDB, v.PreGenerationProjectStorageMethod)
	s.NoError(err)
	s.Require().NotNil(v)
	s.Require().Len(v.BuildVariants, 2)

	// batchtime task added to existing variant, despite no previous batchtime task
	s.Require().Len(v.BuildVariants[0].BatchTimeTasks, 1)
	s.InDelta(time.Now().Unix(), v.BuildVariants[0].BatchTimeTasks[0].ActivateAt.Unix(), 1)

	// new build added correctly
	s.False(v.BuildVariants[1].Activated)
	s.InDelta(time.Now().Add(60*time.Minute).Unix(), v.BuildVariants[1].ActivateAt.Unix(), 1)
	s.Require().Len(v.BuildVariants[1].BatchTimeTasks, 1)
	s.InDelta(time.Now().Add(15*time.Minute).Unix(), v.BuildVariants[1].BatchTimeTasks[0].ActivateAt.Unix(), 1)

	_, err = GetParserProjectStorage(s.ctx, s.env.Settings(), v.ProjectStorageMethod)
	s.Require().NoError(err)
	pp, err = ParserProjectFindOneByID(s.ctx, s.env.Settings(), v.ProjectStorageMethod, v.Id)
	s.NoError(err)
	s.Require().NotNil(pp)
	s.Len(pp.BuildVariants, 3)
	s.Len(pp.Tasks, 6)

	builds, err := build.FindBuildsByVersions(s.ctx, []string{v.Id})
	s.NoError(err)
	s.Len(builds, 2)
	for _, b := range builds {
		s.Equal(b.Id == sampleBuild.Id, b.HasUnfinishedEssentialTask, "existing build that has essential tasks added should be marked")
	}

	tasks, err := task.FindAll(ctx, db.Query(bson.M{task.VersionKey: v.Id})) // with display
	s.NoError(err)
	s.Len(tasks, 7)

	dbExistingBV, err := build.FindOneId(s.ctx, sampleBuild.Id)
	s.NoError(err)
	s.Require().NotZero(dbExistingBV)

	tasksInExistingBV, err := task.Find(ctx, task.ByBuildId(sampleBuild.Id)) // without display
	s.NoError(err)
	s.Len(tasksInExistingBV, 3)
	for _, tsk := range tasksInExistingBV {
		if tsk.DisplayName == "say-bye" {
			s.False(tsk.Activated)
			s.False(tsk.IsEssentialToSucceed)
		} else {
			s.True(tsk.Activated)
			s.True(tsk.IsEssentialToSucceed)
		}
	}

	for _, task := range tasks {
		if task.DisplayOnly {
			s.EqualValues(0, task.Priority)
		} else {
			s.EqualValues(10, task.Priority,
				"task '%s' for '%s' failed", task.DisplayName, task.BuildVariant)
		}
	}
}

func (s *GenerateSuite) TestSaveWithAlreadyGeneratedTasksAndVariants() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	generatorTask := &task.Task{
		Id:      "generator",
		BuildId: "generate_build",
		Version: "version_that_called_generate_task",
	}
	alreadyExistingTask := task.Task{
		Id:          "new_task",
		DisplayName: "new_task",
		BuildId:     "new_variant",
		Version:     "version_that_called_generate_task",
		GeneratedBy: "generator",
		CreateTime:  time.Now(),
	}
	alreadyGeneratedVariant := build.Build{
		Id:           "new_variant",
		BuildVariant: "new_variant",
		Version:      "version_that_called_generate_task",
		CreateTime:   time.Now(),
	}
	s.NoError(generatorTask.Insert(s.ctx))
	s.NoError(alreadyExistingTask.Insert(s.ctx))
	s.NoError(alreadyGeneratedVariant.Insert(s.ctx))

	v := &Version{
		Id:       "version_that_called_generate_task",
		BuildIds: []string{"new_variant"},
	}
	s.NoError(v.Insert(s.ctx))
	pp := &ParserProject{}
	err := util.UnmarshalYAMLWithFallback([]byte(sampleProjYmlTaskGroups), &pp)
	s.NoError(err)
	pp.Id = "version_that_called_generate_task"
	s.NoError(pp.Insert(s.ctx))
	// Setup parser project to be partially generated.
	p, pp, err := FindAndTranslateProjectForVersion(s.ctx, s.env.Settings(), v, false)
	s.NoError(err)

	g := partiallyGeneratedProject
	g.Task = generatorTask
	p, pp, v, err = g.NewVersion(context.Background(), p, pp, v)
	s.NoError(err)
	pp.UpdatedByGenerators = []string{generatorTask.Id}
	s.NoError(ParserProjectUpsertOne(ctx, s.env.Settings(), v.ProjectStorageMethod, pp))

	// Shouldn't error trying to add the same generated project.
	p, pp, v, err = g.NewVersion(context.Background(), p, pp, v)
	s.NoError(err)
	s.Len(pp.UpdatedByGenerators, 1) // Not modified again.

	s.NoError(g.Save(s.ctx, s.env.Settings(), p, pp, v))

	// Should not create a pre-generated cached parser project
	preGeneratedPP, err := ParserProjectFindOneByID(ctx, s.env.Settings(), v.ProjectStorageMethod, preGeneratedParserProjectId(pp.Id))
	s.NoError(err)
	s.Nil(preGeneratedPP)

	tasks := []task.Task{}
	taskQuery := db.Query(bson.M{task.GeneratedByKey: "generator"}).Sort([]string{task.CreateTimeKey})
	err = db.FindAllQ(s.ctx, task.Collection, taskQuery, &tasks)
	s.NoError(err)
	s.Require().Len(tasks, 3)
	// New task is added both to previously generated variant, and new variant.
	s.Equal("another_new_task", tasks[0].DisplayName)
	s.Equal("another_new_task", tasks[1].DisplayName)
	s.Equal("new_task", tasks[2].DisplayName)

	// New build is added.
	builds, err := build.FindBuildsByVersions(s.ctx, []string{v.Id})
	s.NoError(err)
	s.Require().Len(builds, 2)
	s.Equal("new_variant", builds[0].BuildVariant)
	s.Equal("another_new_variant", builds[1].BuildVariant)

}

func (s *GenerateSuite) TestSaveNewTasksWithDependencies() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tasksThatExist := []task.Task{
		{
			Id:      "task_that_called_generate_task",
			Version: "version_that_called_generate_task",
			BuildId: "generate_build",
		},
		{
			Id:          "say-hi-task-id",
			Version:     "version_that_called_generate_task",
			BuildId:     "sample_build",
			DisplayName: "say-hi",
			Activated:   true,
		},
		{
			Id:          "say-bye-task-id",
			Version:     "version_that_called_generate_task",
			BuildId:     "sample_build",
			DisplayName: "say-bye",
		},
		{
			Id:          "say_something_else",
			Version:     "version_that_called_generate_task",
			BuildId:     "sample_build",
			DisplayName: "say_something_else",
		},
	}
	for _, t := range tasksThatExist {
		s.NoError(t.Insert(s.ctx))
	}

	sampleBuild := build.Build{
		Id:           "sample_build",
		BuildVariant: "a_variant",
		Version:      "version_that_called_generate_task",
	}
	v := &Version{
		Id:       "version_that_called_generate_task",
		BuildIds: []string{"sample_build"},
	}
	pp := &ParserProject{}
	err := util.UnmarshalYAMLWithFallback([]byte(sampleProjYmlTaskGroups), &pp)
	s.NoError(err)
	pp.Id = "version_that_called_generate_task"
	s.NoError(pp.Insert(s.ctx))
	s.NoError(sampleBuild.Insert(s.ctx))
	s.NoError(v.Insert(s.ctx))

	g := sampleGeneratedProjectAddToBVOnly
	g.Task = &tasksThatExist[0]
	p, pp, err := FindAndTranslateProjectForVersion(s.ctx, s.env.Settings(), v, false)
	s.Require().NoError(err)
	p, pp, v, err = g.NewVersion(context.Background(), p, pp, v)
	s.NoError(err)
	s.NoError(g.Save(s.ctx, s.env.Settings(), p, pp, v))

	v, err = VersionFindOneIdWithBuildVariants(s.ctx, v.Id)
	s.NoError(err)
	s.Require().NotNil(v)

	pp, err = ParserProjectFindOneByID(s.ctx, s.env.Settings(), v.ProjectStorageMethod, v.Id)
	s.NoError(err)
	s.Require().NotNil(pp)
	s.Require().Len(pp.BuildVariants, 1, "parser project should have same build variant")
	var taskWithDepsFound bool
	const expectedTask = "task_that_has_dependencies"
	for _, t := range pp.BuildVariants[0].Tasks {
		if t.Name == expectedTask {
			taskWithDepsFound = true
			break
		}
	}
	s.True(taskWithDepsFound, "task '%s' should have been added to build variant", expectedTask)

	tasks := []task.Task{}
	s.NoError(db.FindAllQ(s.ctx, task.Collection, db.Query(bson.M{task.DisplayNameKey: expectedTask}), &tasks))
	s.Require().Len(tasks, 1)
	s.Require().Len(tasks[0].DependsOn, 3)
	expected := map[string]bool{"say-hi-task-id": false, "say-bye-task-id": false, "say_something_else": false}
	for _, dependency := range tasks[0].DependsOn {
		expected[dependency.TaskId] = true
	}
	for taskID, expect := range expected {
		s.True(expect, "%s should be a dependency but wasn't", taskID)
	}

	generatorTask, err := task.FindOneId(ctx, tasksThatExist[0].Id)
	s.NoError(err)
	s.Require().NotNil(generatorTask)
	s.Equal(3, generatorTask.NumActivatedGeneratedTasks)
	s.Equal(4, generatorTask.NumGeneratedTasks)
}

func (s *GenerateSuite) TestSaveNewTasksWithDependenciesInNewBuilds() {
	generator := task.Task{
		Id:          "task_that_called_generate_task",
		DisplayName: "task_that_called_generate_task",
		Version:     "version_that_called_generate_task",
		BuildId:     "sample_build",
		Activated:   true,
	}
	s.NoError(generator.Insert(s.ctx))
	sampleBuild := build.Build{
		Id:           "sample_build",
		BuildVariant: "a_variant",
		Version:      "version_that_called_generate_task",
	}
	v := &Version{
		Id:            "version_that_called_generate_task",
		BuildIds:      []string{"sample_build"},
		BuildVariants: []VersionBuildStatus{},
	}
	pp := &ParserProject{}
	err := util.UnmarshalYAMLWithFallback([]byte(projYmlTwoBVs), &pp)
	s.NoError(err)
	pp.Id = "version_that_called_generate_task"
	s.NoError(pp.Insert(s.ctx))
	s.NoError(sampleBuild.Insert(s.ctx))
	s.NoError(v.Insert(s.ctx))

	g := generatedProjectWithNewBuild
	g.Task = &generator
	p, pp, err := FindAndTranslateProjectForVersion(s.ctx, s.env.Settings(), v, false)
	s.Require().NoError(err)
	p, pp, v, err = g.NewVersion(s.ctx, p, pp, v)
	s.NoError(err)
	s.NoError(g.Save(s.ctx, s.env.Settings(), p, pp, v))

	tasks := []task.Task{}
	s.NoError(db.FindAllQ(s.ctx, task.Collection, db.Query(bson.M{task.VersionKey: v.Id}), &tasks))
	s.Require().Len(tasks, 4)
	for _, t := range tasks {
		s.True(t.Activated)
	}
}

func (s *GenerateSuite) TestSaveNewTasksInExistingVariantUpdatesBuildStatus() {
	genTask := &task.Task{
		Id:          "t1",
		DisplayName: "generator_task",
		BuildId:     "b1",
		Version:     "v1",
	}
	s.NoError(genTask.Insert(s.ctx))

	finishedTask := task.Task{
		Id:           "t2",
		DisplayName:  "finished_task",
		BuildId:      "b2",
		BuildVariant: "other_bv",
		Version:      "v1",
	}
	s.NoError(finishedTask.Insert(s.ctx))

	existingGenBuild := build.Build{
		Id:           "b1",
		BuildVariant: "generator_bv",
		Version:      "v1",
		Activated:    true,
		Status:       evergreen.BuildStarted,
	}
	s.NoError(existingGenBuild.Insert(s.ctx))

	existingFinishedBuild := build.Build{
		Id:           "b2",
		BuildVariant: "other_bv",
		Version:      "v1",
		Activated:    true,
		Status:       evergreen.BuildSucceeded,
	}
	s.NoError(existingFinishedBuild.Insert(s.ctx))

	v := &Version{
		Id:       "v1",
		BuildIds: []string{"b1", "b2"},
	}
	s.NoError(v.Insert(s.ctx))

	parserProj := ParserProject{}
	initialConfig := `
tasks:
- name: generator_task
- name: finished_task

buildvariants:
- name: generator_bv
  run_on:
  - arch
  tasks:
  - name: generator_task
- name: other_bv
  run_on:
  - arch
  tasks:
  - name: finished_task
`
	s.NoError(util.UnmarshalYAMLWithFallback([]byte(initialConfig), &parserProj))
	parserProj.Id = "v1"
	s.NoError(parserProj.Insert(s.ctx))

	generateTasksJSON := `
{
	"tasks": [
		{
			"name": "new_task"
		}
	],
	"buildvariants": [
		{
			"name": "other_bv",
			"tasks": ["new_task"]
		}
	]
}
`

	g, err := ParseProjectFromJSONString(generateTasksJSON)
	s.Require().NoError(err)
	g.Task = genTask

	p, pp, err := FindAndTranslateProjectForVersion(s.ctx, s.env.Settings(), v, false)
	s.Require().NoError(err)
	p, pp, v, err = g.NewVersion(context.Background(), p, pp, v)
	s.NoError(err)
	s.NoError(g.Save(s.ctx, s.env.Settings(), p, pp, v))

	dbExistingGenBuild, err := build.FindOneId(s.ctx, existingGenBuild.Id)
	s.Require().NoError(err)
	s.Require().NotZero(dbExistingGenBuild)
	s.Equal(evergreen.BuildStarted, dbExistingGenBuild.Status, "status for build generating tasks should not change")

	dbExistingOtherBuild, err := build.FindOneId(s.ctx, existingFinishedBuild.Id)
	s.Require().NoError(err)
	s.Require().NotZero(dbExistingOtherBuild)
	s.Equal(evergreen.BuildStarted, dbExistingOtherBuild.Status, "status for build that previously had only finished tasks and now has new generated tasks to run should be running")
}

func (s *GenerateSuite) TestSaveNewTasksInNewVariantWithCrossVariantDependencyOnExistingUnscheduledTaskInExistingVariant() {
	// This tests generating a task that depends on a task in a different BV
	// that has already been defined but is not scheduled. It should scheduled
	// by the generate.tasks dependency.
	genTask := &task.Task{
		Id:      "generator",
		BuildId: "b1",
		Version: "v1",
	}
	s.NoError(genTask.Insert(s.ctx))

	existingGenBuild := build.Build{
		Id:           "b1",
		BuildVariant: "generator_bv",
		Version:      "v1",
		Activated:    true,
	}
	s.NoError(existingGenBuild.Insert(s.ctx))

	v := &Version{
		Id:       "v1",
		BuildIds: []string{"b1"},
	}
	s.NoError(v.Insert(s.ctx))

	parserProj := ParserProject{}
	initialConfig := `
tasks:
- name: defined_but_not_scheduled_task
- name: generator

buildvariants:
- name: defined_but_not_scheduled_bv
  run_on:
  - arch
  tasks:
  - name: defined_but_not_scheduled_task
- name: generator_bv
  run_on:
  - arch
  tasks:
  - name: generator
`
	s.NoError(util.UnmarshalYAMLWithFallback([]byte(initialConfig), &parserProj))
	parserProj.Id = "v1"
	s.NoError(parserProj.Insert(s.ctx))

	generateTasksJSON := `
{
	"tasks": [
		{
			"name": "generated_task_that_has_cross_variant_dependency",
			"depends_on": [
				{
					"name": "defined_but_not_scheduled_task",
					"variant": "defined_but_not_scheduled_bv"
				}
			]
		}
	],
	"buildvariants": [
		{
			"name": "generator_bv",
			"tasks": ["generated_task_that_has_cross_variant_dependency"]
		}
	]
}
`

	g, err := ParseProjectFromJSONString(generateTasksJSON)
	s.Require().NoError(err)
	g.Task = genTask

	p, pp, err := FindAndTranslateProjectForVersion(s.ctx, s.env.Settings(), v, false)
	s.Require().NoError(err)
	p, pp, v, err = g.NewVersion(context.Background(), p, pp, v)
	s.NoError(err)
	s.NoError(g.Save(s.ctx, s.env.Settings(), p, pp, v))

	alreadyDefinedTask := task.Task{}
	s.NoError(db.FindOneQ(s.ctx, task.Collection, db.Query(bson.M{task.DisplayNameKey: "defined_but_not_scheduled_task"}), &alreadyDefinedTask))
	s.True(alreadyDefinedTask.Activated, "dependency should be activated")

	taskWithDeps := task.Task{}
	s.NoError(db.FindOneQ(s.ctx, task.Collection, db.Query(bson.M{task.DisplayNameKey: "generated_task_that_has_cross_variant_dependency"}), &taskWithDeps))
	s.Require().Len(taskWithDeps.DependsOn, 1)
	s.Equal(alreadyDefinedTask.Id, taskWithDeps.DependsOn[0].TaskId, "generated task should depend on cross-variant dependency")
}

func (s *GenerateSuite) TestSaveNewTasksInNewVariantWithCrossVariantDependencyOnNewTaskInNewVariant() {
	// This tests generating a task that depends on a task in a different BV,
	// which is also created by generate.tasks.
	genTask := &task.Task{
		Id:      "generator",
		BuildId: "b1",
		Version: "v1",
	}
	s.NoError(genTask.Insert(s.ctx))

	existingGenBuild := build.Build{
		Id:           "b1",
		BuildVariant: "generator_bv",
		Version:      "v1",
		Activated:    true,
	}
	s.NoError(existingGenBuild.Insert(s.ctx))

	v := &Version{
		Id:       "v1",
		BuildIds: []string{"b1"},
	}
	s.NoError(v.Insert(s.ctx))

	parserProj := ParserProject{}
	initialConfig := `
tasks:
- name: generator

buildvariants:
- name: generator_bv
  run_on:
  - arch
  tasks:
  - name: generator
`
	s.NoError(util.UnmarshalYAMLWithFallback([]byte(initialConfig), &parserProj))
	parserProj.Id = "v1"
	s.NoError(parserProj.Insert(s.ctx))

	generateTasksJSON := `
{
	"tasks": [
		{
			"name": "generated_task_that_has_cross_variant_dependency",
			"depends_on": [
				{
					"name": "generated_task",
					"variant": "generated_bv"
				}
			]
		},
		{
			"name": "generated_task",
		}
	],
	"buildvariants": [
		{
			"name": "generator_bv",
			"tasks": ["generated_task_that_has_cross_variant_dependency"]
		},
		{
			"name": "generated_bv",
			"tasks": ["generated_task"],
			"run_on": ["arch"]
		}
	]
}
`

	g, err := ParseProjectFromJSONString(generateTasksJSON)
	s.Require().NoError(err)
	g.Task = genTask

	p, pp, err := FindAndTranslateProjectForVersion(s.ctx, s.env.Settings(), v, false)
	s.Require().NoError(err)
	p, pp, v, err = g.NewVersion(context.Background(), p, pp, v)
	s.NoError(err)
	s.NoError(g.Save(s.ctx, s.env.Settings(), p, pp, v))

	depTask := task.Task{}
	s.NoError(db.FindOneQ(s.ctx, task.Collection, db.Query(bson.M{task.DisplayNameKey: "generated_task"}), &depTask))
	s.True(depTask.Activated, "dependency should be activated")

	taskWithDeps := task.Task{}
	s.NoError(db.FindOneQ(s.ctx, task.Collection, db.Query(bson.M{task.DisplayNameKey: "generated_task_that_has_cross_variant_dependency"}), &taskWithDeps))
	s.Require().Len(taskWithDeps.DependsOn, 1)
	s.Equal(depTask.Id, taskWithDeps.DependsOn[0].TaskId, "generated task should depend on cross-variant dependency")
}

func (s *GenerateSuite) TestSaveNewTasksInNewVariantWithCrossVariantDependencyOnNewTaskInExistingVariant() {
	// This tests generating a task in a new BV that depends on a new task in a
	// different BV. The other BV already exists, and the new dependency is
	// appended to it.
	genTask := &task.Task{
		Id:      "generator",
		BuildId: "b1",
		Version: "v1",
	}
	s.NoError(genTask.Insert(s.ctx))
	existingTask := &task.Task{
		Id:      "existing_task",
		BuildId: "b2",
		Version: "v1",
	}
	s.NoError(existingTask.Insert(s.ctx))

	existingGenBuild := build.Build{
		Id:           "b1",
		BuildVariant: "generator_bv",
		Version:      "v1",
		Activated:    true,
	}
	s.NoError(existingGenBuild.Insert(s.ctx))
	existingBuild := build.Build{
		Id:           "b2",
		BuildVariant: "existing_bv",
		Version:      "v1",
		Activated:    true,
	}
	s.NoError(existingBuild.Insert(s.ctx))
	v := &Version{
		Id:       "v1",
		BuildIds: []string{"b1", "b2"},
	}
	s.NoError(v.Insert(s.ctx))
	parserProj := ParserProject{}
	initialConfig := `
tasks:
- name: generator
- name: existing_task

buildvariants:
- name: generator_bv
  run_on:
  - arch
  tasks:
  - name: generator
- name: existing_bv
  run_on:
  - arch
  tasks:
  - name: existing_task

`
	s.NoError(util.UnmarshalYAMLWithFallback([]byte(initialConfig), &parserProj))
	parserProj.Id = "v1"
	s.NoError(parserProj.Insert(s.ctx))

	generateTasksJSON := `
{
	"tasks": [
		{
			"name": "generated_task_that_has_cross_variant_dependency",
			"depends_on": [
				{
					"name": "generated_task",
					"variant": "existing_bv"
				}
			]
		},
		{
			"name": "generated_task",
		}
	],
	"buildvariants": [
		{
			"name": "generator_bv",
			"tasks": ["generated_task_that_has_cross_variant_dependency"]
		},
		{
			"name": "existing_bv",
			"tasks": ["generated_task"]
		}
	]
}
`

	g, err := ParseProjectFromJSONString(generateTasksJSON)
	s.Require().NoError(err)
	g.Task = genTask

	p, pp, err := FindAndTranslateProjectForVersion(s.ctx, s.env.Settings(), v, false)
	s.Require().NoError(err)
	p, pp, v, err = g.NewVersion(context.Background(), p, pp, v)
	s.NoError(err)
	s.NoError(g.Save(s.ctx, s.env.Settings(), p, pp, v))

	depTask := task.Task{}
	s.NoError(db.FindOneQ(s.ctx, task.Collection, db.Query(bson.M{task.DisplayNameKey: "generated_task"}), &depTask))
	s.True(depTask.Activated, "dependency should be activated")

	taskWithDeps := task.Task{}
	s.NoError(db.FindOneQ(s.ctx, task.Collection, db.Query(bson.M{task.DisplayNameKey: "generated_task_that_has_cross_variant_dependency"}), &taskWithDeps))
	s.Require().Len(taskWithDeps.DependsOn, 1)
	s.Equal(depTask.Id, taskWithDeps.DependsOn[0].TaskId, "generated task should depend on cross-variant dependency")
}

func (s *GenerateSuite) TestSaveNewTasksInExistingVariantWithCrossVariantDependencyOnNewTaskInExistingBuild() {
	// This tests generating a task in an existing BV that depends on a new task
	// in a different BV. The other BV already exists, and the new task is
	// appended to it.
	genTask := &task.Task{
		Id:      "generator",
		BuildId: "b1",
		Version: "v1",
	}
	s.NoError(genTask.Insert(s.ctx))
	existingTask := &task.Task{
		Id:      "existing_task",
		BuildId: "b2",
		Version: "v1",
	}
	s.NoError(existingTask.Insert(s.ctx))

	existingGenBuild := build.Build{
		Id:           "b1",
		BuildVariant: "generator_bv",
		Version:      "v1",
		Activated:    true,
	}
	s.NoError(existingGenBuild.Insert(s.ctx))
	existingDepBuild := build.Build{
		Id:           "b2",
		BuildVariant: "existing_bv",
		Version:      "v1",
		Activated:    true,
	}
	s.NoError(existingDepBuild.Insert(s.ctx))
	v := &Version{
		Id:       "v1",
		BuildIds: []string{"b1", "b2"},
	}
	s.NoError(v.Insert(s.ctx))
	parserProj := ParserProject{}
	initialConfig := `
tasks:
- name: generator
- name: existing_task

buildvariants:
- name: generator_bv
  run_on:
  - arch
  tasks:
  - name: generator
- name: existing_bv
  run_on:
  - arch
  tasks:
  - name: existing_task
`
	s.NoError(util.UnmarshalYAMLWithFallback([]byte(initialConfig), &parserProj))
	parserProj.Id = "v1"
	s.NoError(parserProj.Insert(s.ctx))

	generateTasksJSON := `
{
	"tasks": [
		{
			"name": "generated_task_that_has_cross_variant_dependency",
			"depends_on": [
				{
					"name": "generated_task",
					"variant": "existing_bv"
				}
			]
		},
		{
			"name": "generated_task",
		}
	],
	"buildvariants": [
		{
			"name": "generator_bv",
			"tasks": ["generated_task_that_has_cross_variant_dependency"]
		},
		{
			"name": "existing_bv",
			"tasks": ["generated_task"],
		}
	]
}
`

	g, err := ParseProjectFromJSONString(generateTasksJSON)
	s.Require().NoError(err)
	g.Task = genTask

	p, pp, err := FindAndTranslateProjectForVersion(s.ctx, s.env.Settings(), v, false)
	s.Require().NoError(err)
	p, pp, v, err = g.NewVersion(context.Background(), p, pp, v)
	s.NoError(err)
	s.NoError(g.Save(s.ctx, s.env.Settings(), p, pp, v))

	depTask := task.Task{}
	s.NoError(db.FindOneQ(s.ctx, task.Collection, db.Query(bson.M{task.DisplayNameKey: "generated_task"}), &depTask))
	s.True(depTask.Activated, "dependency should be activated")

	taskWithDeps := task.Task{}
	s.NoError(db.FindOneQ(s.ctx, task.Collection, db.Query(bson.M{task.DisplayNameKey: "generated_task_that_has_cross_variant_dependency"}), &taskWithDeps))
	s.Require().Len(taskWithDeps.DependsOn, 1)
	s.Equal(depTask.Id, taskWithDeps.DependsOn[0].TaskId, "generated task should depend on cross-variant dependency")
}

func (s *GenerateSuite) TestSaveNewTaskWithExistingExecutionTask() {
	taskThatExists := task.Task{
		Id:           "task_that_called_generate_task",
		Version:      "version_that_called_generate_task",
		BuildVariant: "my_build_variant",
	}
	taskDisplayGen := task.Task{
		Id:           "_my_build_variant_my_display_task_gen__01_01_01_00_00_00",
		DisplayName:  "my_display_task_gen",
		Version:      "version_that_called_generate_task",
		BuildVariant: "my_build_variant",
	}
	sampleBuild := build.Build{
		Id:           "sample_build",
		BuildVariant: "my_build_variant",
		Version:      "version_that_called_generate_task",
	}
	v := &Version{
		Id:       "version_that_called_generate_task",
		BuildIds: []string{"sample_build"},
	}
	pp := &ParserProject{}
	err := util.UnmarshalYAMLWithFallback([]byte(smallYml), &pp)
	s.NoError(err)
	pp.Id = "version_that_called_generate_task"
	s.NoError(pp.Insert(s.ctx))
	s.NoError(taskThatExists.Insert(s.ctx))
	s.NoError(taskDisplayGen.Insert(s.ctx))
	s.NoError(sampleBuild.Insert(s.ctx))
	s.NoError(v.Insert(s.ctx))

	g := smallGeneratedProject
	g.Task = &taskThatExists
	p, pp, err := FindAndTranslateProjectForVersion(s.ctx, s.env.Settings(), v, false)
	s.Require().NoError(err)
	p, pp, v, err = g.NewVersion(context.Background(), p, pp, v)
	s.Require().NoError(err)
	s.NoError(g.Save(s.ctx, s.env.Settings(), p, pp, v))

	v, err = VersionFindOneIdWithBuildVariants(s.ctx, v.Id)
	s.NoError(err)
	s.Require().NotNil(v)

	pp, err = ParserProjectFindOneByID(s.ctx, s.env.Settings(), v.ProjectStorageMethod, v.Id)
	s.NoError(err)
	s.Require().NotNil(pp)
	s.Require().Len(pp.BuildVariants, 1, "parser project should have same build variant")
	const expectedDisplayTask = "my_display_task"
	var dtFound bool
	for _, dt := range pp.BuildVariants[0].DisplayTasks {
		if dt.Name == expectedDisplayTask {
			dtFound = true
			break
		}
	}
	s.True(dtFound, "display task '%s' should have been added", expectedDisplayTask)

	tasks := []task.Task{}
	s.NoError(db.FindAllQ(s.ctx, task.Collection, db.Query(bson.M{}), &tasks))
	s.NoError(db.FindAllQ(s.ctx, task.Collection, db.Query(bson.M{task.DisplayNameKey: "my_display_task_gen"}), &tasks))
	s.Len(tasks, 1)
	s.NoError(db.FindAllQ(s.ctx, task.Collection, db.Query(bson.M{task.DisplayNameKey: "my_display_task"}), &tasks))
	s.Len(tasks, 1)
	s.Len(tasks[0].ExecutionTasks, 1)
}

func (s *GenerateSuite) TestMergeGeneratedProjects() {
	projects := []GeneratedProject{sampleGeneratedProjectWithAllMultiFields}
	merged, err := MergeGeneratedProjects(context.Background(), projects)
	s.Require().NoError(err)

	expectedBVs := map[string]struct {
		numTasks        int
		numDisplayTasks int
		found           bool
	}{
		"honeydew": {numTasks: 3},
		"cantaloupe": {
			numTasks:        1,
			numDisplayTasks: 2,
		},
	}
	for _, bv := range merged.BuildVariants {
		expected, ok := expectedBVs[bv.Name]
		s.True(ok, "unexpected build variant '%s'", bv.Name)
		s.Len(bv.Tasks, expected.numTasks, "unexpected number of tasks for build variant '%s'", bv.Name)
		s.Len(bv.DisplayTasks, expected.numDisplayTasks, "unexpected number of display tasks for build variant '%s'", bv.DisplayName)

		expected.found = true
		expectedBVs[bv.Name] = expected
	}
	for bvName, expected := range expectedBVs {
		s.True(expected.found, "did not find expected build variant '%s'", bvName)
	}

	expectedTasks := map[string]struct {
		numCmds int
		found   bool
	}{
		"quokka": {numCmds: 2},
		"pika":   {numCmds: 1},
	}
	for _, tsk := range merged.Tasks {
		expected, ok := expectedTasks[tsk.Name]
		s.True(ok, "unexpected task '%s'", tsk.Name)
		s.Len(tsk.Commands, expected.numCmds, "unexpected number of commands for task '%s'", tsk.Name)

		expected.found = true
		expectedTasks[tsk.Name] = expected
	}
	for taskName, expected := range expectedTasks {
		s.True(expected.found, "did not find expected task '%s'", taskName)
	}

	expectedFuncs := map[string]struct {
		numCmds int
		found   bool
	}{
		"brownie": {numCmds: 2},
		"cookie":  {numCmds: 1},
	}
	for funcName, funcCmds := range merged.Functions {
		expected, ok := expectedFuncs[funcName]
		s.True(ok, "unexpected function '%s'", funcName)
		s.Len(funcCmds.List(), expected.numCmds, "unexpected number of commands for function '%s'", funcName)

		expected.found = true
		expectedFuncs[funcName] = expected
	}
	for funcName, expected := range expectedFuncs {
		s.True(expected.found, "did not find expected function '%s'", funcName)
	}

	expectedTaskGroups := map[string]struct {
		numTasks int
		found    bool
	}{
		"sea-bunny": {numTasks: 2},
		"mola-mola": {numTasks: 1},
	}
	for _, tg := range merged.TaskGroups {
		expected, ok := expectedTaskGroups[tg.Name]
		s.True(ok, "unexpected task group '%s'", tg.Name)
		s.Len(tg.Tasks, expected.numTasks, "unexpected number of tasks for task group '%s'", tg.Name)

		expected.found = true
		expectedTaskGroups[tg.Name] = expected
	}
	for tgName, expected := range expectedTaskGroups {
		s.True(expected.found, "did not find expected task group '%s'", tgName)
	}
}

func (s *GenerateSuite) TestMergeGeneratedProjectsWithNoTasks() {
	projects := []GeneratedProject{smallGeneratedProject}
	merged, err := MergeGeneratedProjects(context.Background(), projects)
	s.Require().NoError(err)
	s.Require().NotNil(merged)
	s.Require().Len(merged.BuildVariants, 1)
	s.Len(merged.BuildVariants[0].DisplayTasks, 1)
}

func TestSimulateNewDependencyGraph(t *testing.T) {
	defer func() {
		assert.NoError(t, db.Clear(task.Collection))
	}()
	require.NoError(t, db.Clear(task.Collection))

	v := &Version{Id: "v0"}
	generatorTask := task.Task{Id: "mci_bv0_generator__01_01_01_00_00_00", Version: v.Id, BuildVariant: "bv0", DisplayName: "generator"}
	require.NoError(t, generatorTask.Insert(t.Context()))

	t.Run("CreatesCycle", func(t *testing.T) {
		// Generator generates: [ A (depends on B) , B (depends on) C), C (depends on A) ]
		// the graph:
		//   +-----+            +-----+
		//   |  A  |            |  C  |
		//   +-----+ <--------- +-----+
		//            \         ^
		//             v       /
		//              +-----+
		//              |  B  |
		//              +-----+

		project := &Project{
			BuildVariants: []BuildVariant{
				{Name: "bv0", Tasks: []BuildVariantTaskUnit{
					{Name: "A", Variant: "bv0", DependsOn: []TaskUnitDependency{{Name: "B", Variant: "bv0"}}},
					{Name: "B", Variant: "bv0", DependsOn: []TaskUnitDependency{{Name: "C", Variant: "bv0"}}},
					{Name: "C", Variant: "bv0", DependsOn: []TaskUnitDependency{{Name: "A", Variant: "bv0"}}},
					{Name: "generator", Variant: "bv0"},
				}},
			},
			Tasks: []ProjectTask{
				{Name: "A"},
				{Name: "B"},
				{Name: "C"},
				{Name: "generator"},
			},
		}

		g := GeneratedProject{
			Task: &generatorTask,
			BuildVariants: []parserBV{
				{
					Name: "bv0",
					Tasks: []parserBVTaskUnit{
						{Name: "A"},
						{Name: "B"},
						{Name: "C"},
					},
				},
			},
		}
		assert.Error(t, g.CheckForCycles(context.Background(), v, project, &ProjectRef{Identifier: "mci"}))
	})

	t.Run("CreatesLoop", func(t *testing.T) {
		// Generator generates: [ A (depends on B), B (depends on A) ]
		// the graph:
		//   +-----+ ---------> +-----+
		//   |  A  |            |  B  |
		//   +-----+ <--------- +-----+

		project := &Project{
			BuildVariants: []BuildVariant{
				{Name: "bv0", Tasks: []BuildVariantTaskUnit{
					{Name: "A", Variant: "bv0", DependsOn: []TaskUnitDependency{{Name: "B", Variant: "bv0"}}},
					{Name: "B", Variant: "bv0", DependsOn: []TaskUnitDependency{{Name: "A", Variant: "bv0"}}},
					{Name: "generator", Variant: "bv0"},
				}},
			},
			Tasks: []ProjectTask{
				{Name: "A"},
				{Name: "B"},
				{Name: "generator"},
			},
		}

		g := GeneratedProject{
			Task: &generatorTask,
			BuildVariants: []parserBV{
				{
					Name: "bv0",
					Tasks: []parserBVTaskUnit{
						{Name: "A"},
						{Name: "B"},
					},
				},
			},
		}

		assert.Error(t, g.CheckForCycles(context.Background(), v, project, &ProjectRef{Identifier: "mci"}))
	})
	t.Run("Dependent on Generator", func(t *testing.T) {
		// Generator generates: [ A (depends on generator) ]
		//   +-----+            +-------------+
		//   |  A  | ---------> |  generator  |
		//   +-----+            +-------------+
		// This should not error. Because A will only be able to run after the generator, it is okay for it to be dependent on it.

		project := &Project{
			BuildVariants: []BuildVariant{
				{Name: "bv0", Tasks: []BuildVariantTaskUnit{
					{Name: "A", Variant: "bv0", DependsOn: []TaskUnitDependency{{Name: "generator", Variant: "bv0"}}},
					{Name: "generator", Variant: "bv0"},
				}},
			},
			Tasks: []ProjectTask{
				{Name: "A"},
				{Name: "generator"},
			},
		}

		g := GeneratedProject{
			Task: &generatorTask,
			BuildVariants: []parserBV{
				{
					Name: "bv0",
					Tasks: []parserBVTaskUnit{
						{Name: "A"},
					},
				},
			},
		}
		assert.NoError(t, g.CheckForCycles(context.Background(), v, project, &ProjectRef{Identifier: "mci"}))
	})

	t.Run("Loop with generator", func(t *testing.T) {
		// Generator generates: [ generated (depends on) --> A (which depends on the generator) ]
		//   +-----------+            +-----+
		//   | generator |            |  A  |
		//   +-----------+ <--------- +-----+
		//                 \         ^
		//                  v       /
		//                +-----------+
		//                | generated |
		//                +-----------+

		// This should not error because the arrow from generator to generated is not a dependency. As a dependency
		// graph it would look like this, because generated can't run before the generator (and is therefore
		// inherently "dependent" on the generator). It will execute in the following order with no problem:
		// generator ->  A -> generated.
		//   +-----------+            +-----+
		//   | generator |            |  A  |
		//   +-----------+ <--------- +-----+
		//                  ^        ^
		//                   \      /
		//                 +-----------+
		//                 | generated |
		//                 +-----------+

		project := &Project{
			BuildVariants: []BuildVariant{
				{Name: "bv0", Tasks: []BuildVariantTaskUnit{
					{Name: "A", Variant: "bv0", DependsOn: []TaskUnitDependency{{Name: "generator", Variant: "bv0"}}},
					{Name: "generated", Variant: "bv0", DependsOn: []TaskUnitDependency{{Name: "A", Variant: "bv0"}}},
					{Name: "generator", Variant: "bv0"},
				}},
			},
			Tasks: []ProjectTask{
				{Name: "A"},
				{Name: "B"},
				{Name: "C"},
				{Name: "generator"},
			},
		}

		g := GeneratedProject{
			Task: &generatorTask,
			BuildVariants: []parserBV{
				{
					Name: "bv0",
					Tasks: []parserBVTaskUnit{
						{Name: "generated"},
					},
				},
			},
		}
		assert.NoError(t, g.CheckForCycles(context.Background(), v, project, &ProjectRef{Identifier: "mci"}))
	})

	t.Run("NoCycles", func(t *testing.T) {
		project := &Project{
			BuildVariants: []BuildVariant{
				{
					Name: "bv0",
					Tasks: []BuildVariantTaskUnit{
						{Name: "generated", Variant: "bv0"},
						{Name: "dependedOn", Variant: "bv0", DependsOn: []TaskUnitDependency{{Name: "generator", Variant: "bv0"}}},
						{Name: "generator", Variant: "bv0"},
					},
				},
			},
			Tasks: []ProjectTask{
				{Name: "generated"},
				{Name: "dependedOn"},
				{Name: "generator"},
			},
		}

		g := GeneratedProject{
			Task: &generatorTask,
			BuildVariants: []parserBV{
				{
					Name: "bv0",
					Tasks: []parserBVTaskUnit{
						{Name: "generated"},
					},
				},
			},
		}
		assert.NoError(t, g.CheckForCycles(context.Background(), v, project, &ProjectRef{Identifier: "mci"}))
	})

	t.Run("InactiveBuild", func(t *testing.T) {
		project := &Project{
			BuildVariants: []BuildVariant{
				{
					Name: "bv0",
					Tasks: []BuildVariantTaskUnit{
						{Name: "generated", Variant: "bv0", DependsOn: []TaskUnitDependency{{Name: "dependedOn", Variant: "bv0"}}},
						{Name: "dependedOn", Variant: "bv0", DependsOn: []TaskUnitDependency{{Name: "generator", Variant: "bv0"}}},
						{Name: "generator"},
					},
				},
			},
			Tasks: []ProjectTask{
				{Name: "generated"},
				{Name: "dependedOn"},
				{Name: "generator"},
			},
		}

		g := GeneratedProject{
			Task: &generatorTask,
			BuildVariants: []parserBV{
				{
					Name: "bv0",
					Tasks: []parserBVTaskUnit{
						{Name: "generated"},
					},
					Activate: utility.FalsePtr(),
				},
			},
		}
		assert.NoError(t, g.CheckForCycles(context.Background(), v, project, &ProjectRef{Identifier: "mci"}))
	})
}

func TestFilterInactiveTasks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.ClearCollections(build.Collection))
	}()

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, g GeneratedProject, v *Version){
		"DoesNotFilterActiveNewBuild": func(ctx context.Context, t *testing.T, g GeneratedProject, v *Version) {
			tasks, err := g.filterInactiveTasks(ctx, TVPairSet{{
				TaskName: g.BuildVariants[0].Tasks[0].Name,
				Variant:  g.BuildVariants[0].Name,
			}}, v, &Project{})

			assert.NoError(t, err)
			assert.Len(t, tasks, 1)
		},
		"DoesNotFilterExplicitlyActiveNewBuild": func(ctx context.Context, t *testing.T, g GeneratedProject, v *Version) {
			g.BuildVariants[0].Activate = utility.TruePtr()

			tasks, err := g.filterInactiveTasks(ctx, TVPairSet{{
				TaskName: g.BuildVariants[0].Tasks[0].Name,
				Variant:  g.BuildVariants[0].Name,
			}}, v, &Project{})

			assert.NoError(t, err)
			assert.Len(t, tasks, 1)
		},
		"FiltersInactiveNewBuild": func(ctx context.Context, t *testing.T, g GeneratedProject, v *Version) {
			g.BuildVariants[0].Activate = utility.FalsePtr()

			tasks, err := g.filterInactiveTasks(ctx, TVPairSet{{
				TaskName: g.BuildVariants[0].Tasks[0].Name,
				Variant:  g.BuildVariants[0].Name,
			}}, v, &Project{})

			assert.NoError(t, err)
			assert.Empty(t, tasks)
		},
		"FiltersNewBuildsWithBatchTime": func(ctx context.Context, t *testing.T, g GeneratedProject, v *Version) {
			g.BuildVariants[0].BatchTime = utility.ToIntPtr(10)

			tasks, err := g.filterInactiveTasks(ctx, TVPairSet{{
				TaskName: g.BuildVariants[0].Tasks[0].Name,
				Variant:  g.BuildVariants[0].Name,
			}}, v, &Project{})

			assert.NoError(t, err)
			assert.Empty(t, tasks)
		},
		"DoesNotFilterNewBuildWithBatchTimeForPatch": func(ctx context.Context, t *testing.T, g GeneratedProject, v *Version) {
			v.Requester = evergreen.PatchVersionRequester
			g.BuildVariants[0].BatchTime = utility.ToIntPtr(10)

			tasks, err := g.filterInactiveTasks(ctx, TVPairSet{{
				TaskName: g.BuildVariants[0].Tasks[0].Name,
				Variant:  g.BuildVariants[0].Name,
			}}, v, &Project{})

			assert.NoError(t, err)
			assert.Len(t, tasks, 1)
		},
		"FiltersExplicitlyActiveNewBuildWithBatchTime": func(ctx context.Context, t *testing.T, g GeneratedProject, v *Version) {
			// When activate and batchtime are both defined on the build
			// variant, batchtime takes priority.
			g.BuildVariants[0].Activate = utility.TruePtr()
			g.BuildVariants[0].BatchTime = utility.ToIntPtr(10)

			tasks, err := g.filterInactiveTasks(ctx, TVPairSet{{
				TaskName: g.BuildVariants[0].Tasks[0].Name,
				Variant:  g.BuildVariants[0].Name,
			}}, v, &Project{})

			assert.NoError(t, err)
			assert.Empty(t, tasks)
		},
		"FiltersExplicitlyActiveNewBuildWithBatchTimeAndActivateAtDifferentLevels": func(ctx context.Context, t *testing.T, g GeneratedProject, v *Version) {
			// TODO (EVG-20904): When activate and batchtime are defined at
			// different levels and furthermore, activate is defined at a more
			// specific level, the variant-level batchtime still takes priority
			// and the task gets filtered out and will not activate until later.
			// This is unintuitive because the behavior of static YAML vs.
			// generate.tasks differ in prioritization of the fields. For
			// example, if this same configuration is done in the static YAML,
			// the task will activate immediately.
			g.BuildVariants[0].Tasks[0].Activate = utility.TruePtr()
			g.BuildVariants[0].BatchTime = utility.ToIntPtr(10)

			tasks, err := g.filterInactiveTasks(ctx, TVPairSet{{
				TaskName: g.BuildVariants[0].Tasks[0].Name,
				Variant:  g.BuildVariants[0].Name,
			}}, v, &Project{})

			assert.NoError(t, err)
			assert.Empty(t, tasks)
		},
		"DoesNotFilterActiveExistingBuild": func(ctx context.Context, t *testing.T, g GeneratedProject, v *Version) {
			b := &build.Build{
				BuildVariant: g.BuildVariants[0].Name,
				Version:      v.Id,
			}
			assert.NoError(t, b.Insert(t.Context()))

			p := &Project{
				BuildVariants: []BuildVariant{
					{Name: g.BuildVariants[0].Name},
				},
			}

			tasks, err := g.filterInactiveTasks(ctx, TVPairSet{{
				TaskName: g.BuildVariants[0].Tasks[0].Name,
				Variant:  g.BuildVariants[0].Name,
			}}, v, p)

			assert.NoError(t, err)
			assert.Len(t, tasks, 1)
		},
		"DoesNotFilterExplicitlyActiveExistingBuild": func(ctx context.Context, t *testing.T, g GeneratedProject, v *Version) {
			b := &build.Build{
				BuildVariant: g.BuildVariants[0].Name,
				Version:      v.Id,
			}
			assert.NoError(t, b.Insert(t.Context()))

			p := &Project{
				BuildVariants: []BuildVariant{
					{
						Name:     g.BuildVariants[0].Name,
						Activate: utility.TruePtr(),
					},
				},
			}

			tasks, err := g.filterInactiveTasks(ctx, TVPairSet{{
				TaskName: g.BuildVariants[0].Tasks[0].Name,
				Variant:  g.BuildVariants[0].Name,
			}}, v, p)

			assert.NoError(t, err)
			assert.Len(t, tasks, 1)
		},
		"FiltersInactiveExistingBuild": func(ctx context.Context, t *testing.T, g GeneratedProject, v *Version) {
			b := &build.Build{
				BuildVariant: g.BuildVariants[0].Name,
				Version:      v.Id,
			}
			assert.NoError(t, b.Insert(t.Context()))

			p := &Project{
				BuildVariants: []BuildVariant{
					{
						Name:     g.BuildVariants[0].Name,
						Activate: utility.FalsePtr(),
					},
				},
			}

			tasks, err := g.filterInactiveTasks(ctx, TVPairSet{{
				TaskName: g.BuildVariants[0].Tasks[0].Name,
				Variant:  g.BuildVariants[0].Name,
			}}, v, p)

			assert.NoError(t, err)
			assert.Empty(t, tasks)
		},
		"DoesNotFilterExplicitlyActiveTask": func(ctx context.Context, t *testing.T, g GeneratedProject, v *Version) {
			g.BuildVariants[0].Tasks[0].Activate = utility.TruePtr()

			tasks, err := g.filterInactiveTasks(ctx, TVPairSet{{
				TaskName: g.BuildVariants[0].Tasks[0].Name,
				Variant:  g.BuildVariants[0].Name,
			}}, v, &Project{})

			assert.NoError(t, err)
			assert.Len(t, tasks, 1)
		},
		"FiltersInactiveTask": func(ctx context.Context, t *testing.T, g GeneratedProject, v *Version) {
			g.BuildVariants[0].Tasks[0].Activate = utility.FalsePtr()

			tasks, err := g.filterInactiveTasks(ctx, TVPairSet{{
				TaskName: g.BuildVariants[0].Tasks[0].Name,
				Variant:  g.BuildVariants[0].Name,
			}}, v, &Project{})

			assert.NoError(t, err)
			assert.Empty(t, tasks)
		},
		"FiltersNonstepbackTasks": func(ctx context.Context, t *testing.T, g GeneratedProject, v *Version) {
			g.BuildVariants[0].Tasks[0].Activate = utility.FalsePtr()
			g.BuildVariants[0].Tasks = append(g.BuildVariants[0].Tasks,
				parserBVTaskUnit{Name: "generated-2", Activate: utility.TruePtr()},
				parserBVTaskUnit{Name: "generated-3", Activate: utility.FalsePtr()}, // background task.
				parserBVTaskUnit{Name: "generated-4", Activate: utility.FalsePtr()}, // task to stepback.
			)
			g.Task.ActivatedBy = evergreen.StepbackTaskActivator
			g.Task.GeneratedTasksToActivate = map[string][]string{g.BuildVariants[0].Name: {g.BuildVariants[0].Tasks[3].Name}}

			tasks, err := g.filterInactiveTasks(ctx, TVPairSet{
				{TaskName: g.BuildVariants[0].Tasks[0].Name, Variant: g.BuildVariants[0].Name},
				{TaskName: g.BuildVariants[0].Tasks[1].Name, Variant: g.BuildVariants[0].Name},
				{TaskName: g.BuildVariants[0].Tasks[2].Name, Variant: g.BuildVariants[0].Name},
				{TaskName: g.BuildVariants[0].Tasks[3].Name, Variant: g.BuildVariants[0].Name},
			}, v, &Project{})
			require.NoError(t, err)
			assert.Len(t, tasks, 2)

			foundAlwaysActive := false
			foundStepbackTask := false
			for _, task := range tasks {
				assert.NotEqual(t, g.BuildVariants[0].Tasks[0], task.TaskName)
				assert.NotEqual(t, g.BuildVariants[0].Tasks[2], task.TaskName)

				if task.TaskName == g.BuildVariants[0].Tasks[1].Name {
					foundAlwaysActive = true
				} else if task.TaskName == g.BuildVariants[0].Tasks[3].Name {
					foundStepbackTask = true
				}
			}
			assert.True(t, foundAlwaysActive, "always active task should not be filtered")
			assert.True(t, foundStepbackTask, "stepback task should not be filtered")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithCancel(ctx)
			defer tcancel()

			require.NoError(t, db.ClearCollections(build.Collection))

			g := GeneratedProject{
				BuildVariants: []parserBV{
					{
						Name: "bv0",
						Tasks: []parserBVTaskUnit{
							{Name: "generated"},
						},
					},
				},
				Task: &task.Task{},
			}
			v := &Version{Id: "version_id", Requester: evergreen.RepotrackerVersionRequester}

			tCase(tctx, t, g, v)
		})
	}
}

func TestAddDependencies(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.Clear(task.Collection))

	existingTasks := []task.Task{
		{Id: "t1", DependsOn: []task.Dependency{{TaskId: "generator", Status: evergreen.TaskSucceeded}}},
		{Id: "t2", DependsOn: []task.Dependency{{TaskId: "generator", Status: task.AllStatuses}}},
	}
	for _, task := range existingTasks {
		assert.NoError(t, task.Insert(t.Context()))
	}

	g := GeneratedProject{Task: &task.Task{Id: "generator"}}
	assert.NoError(t, g.addDependencies(context.Background(), []string{"t3"}))

	t1, err := task.FindOneId(ctx, "t1")
	assert.NoError(t, err)
	assert.Len(t, t1.DependsOn, 2)
	for _, dep := range t1.DependsOn {
		assert.Equal(t, evergreen.TaskSucceeded, dep.Status)
	}

	t2, err := task.FindOneId(ctx, "t2")
	assert.NoError(t, err)
	assert.Len(t, t2.DependsOn, 2)
	for _, dep := range t2.DependsOn {
		assert.Equal(t, task.AllStatuses, dep.Status)
	}
}

func TestTaskGroupActivation(t *testing.T) {
	// Test that activate: false on a task group reference properly
	// expands to all tasks within the group.
	for tName, tCase := range map[string]func(t *testing.T){
		"ExpandsTaskGroupWithActivateFalse": func(t *testing.T) {
			g := GeneratedProject{
				Task: &task.Task{},
				BuildVariants: []parserBV{
					{
						Name: "variant1",
						Tasks: []parserBVTaskUnit{
							{
								Name:     "my-task-group",
								Activate: utility.FalsePtr(),
							},
						},
					},
				},
				Tasks: []parserTask{
					{Name: "task1"},
					{Name: "task2"},
					{Name: "task3"},
				},
				TaskGroups: []parserTaskGroup{
					{
						Name:  "my-task-group",
						Tasks: []string{"task1", "task2", "task3"},
					},
				},
			}

			activationInfo := g.findTasksAndVariantsWithSpecificActivations(evergreen.RepotrackerVersionRequester)

			// Verify that all individual tasks in the group are marked as having specific activation
			require.Contains(t, activationInfo.activationTasks, "variant1")
			tasksWithSpecificActivation := activationInfo.activationTasks["variant1"]
			assert.ElementsMatch(t, []string{"task1", "task2", "task3"}, tasksWithSpecificActivation,
				"all tasks in the group should be marked with specific activation")

			// Verify individual task activation checks work correctly
			assert.True(t, activationInfo.taskHasSpecificActivation("variant1", "task1"))
			assert.True(t, activationInfo.taskHasSpecificActivation("variant1", "task2"))
			assert.True(t, activationInfo.taskHasSpecificActivation("variant1", "task3"))
		},
		"ExpandsTaskGroupWithBatchTime": func(t *testing.T) {
			batchTime := 60
			g := GeneratedProject{
				Task: &task.Task{},
				BuildVariants: []parserBV{
					{
						Name: "variant1",
						Tasks: []parserBVTaskUnit{
							{
								Name:      "my-task-group",
								BatchTime: &batchTime,
							},
						},
					},
				},
				Tasks: []parserTask{
					{Name: "task1"},
					{Name: "task2"},
				},
				TaskGroups: []parserTaskGroup{
					{
						Name:  "my-task-group",
						Tasks: []string{"task1", "task2"},
					},
				},
			}

			activationInfo := g.findTasksAndVariantsWithSpecificActivations(evergreen.RepotrackerVersionRequester)

			// Verify that all individual tasks in the group are marked as having specific activation
			require.Contains(t, activationInfo.activationTasks, "variant1")
			tasksWithSpecificActivation := activationInfo.activationTasks["variant1"]
			assert.ElementsMatch(t, []string{"task1", "task2"}, tasksWithSpecificActivation,
				"all tasks in the group should be marked with specific activation")
		},
		"ExpandsTaskGroupInStepback": func(t *testing.T) {
			g := GeneratedProject{
				Task: &task.Task{
					ActivatedBy: evergreen.StepbackTaskActivator,
					GeneratedTasksToActivate: map[string][]string{
						"variant1": {"task2"}, // Only task2 should be activated via stepback
					},
				},
				BuildVariants: []parserBV{
					{
						Name: "variant1",
						Tasks: []parserBVTaskUnit{
							{
								Name:     "my-task-group",
								Activate: utility.FalsePtr(),
							},
						},
					},
				},
				Tasks: []parserTask{
					{Name: "task1"},
					{Name: "task2"},
					{Name: "task3"},
				},
				TaskGroups: []parserTaskGroup{
					{
						Name:  "my-task-group",
						Tasks: []string{"task1", "task2", "task3"},
					},
				},
			}

			activationInfo := g.findTasksAndVariantsWithSpecificActivations(evergreen.RepotrackerVersionRequester)

			// Verify stepback info contains all tasks from the group
			require.Contains(t, activationInfo.stepbackTasks, "variant1")
			stepbackTasks := activationInfo.stepbackTasks["variant1"]
			require.Len(t, stepbackTasks, 3, "all tasks in the group should have stepback info")

			// Verify only task2 should be activated (it's in GeneratedTasksToActivate)
			for _, st := range stepbackTasks {
				if st.task == "task2" {
					assert.True(t, st.activate, "task2 should be activated via stepback")
				} else {
					assert.False(t, st.activate, "task1 and task3 should not be activated")
				}
			}
		},
		"ExpandsTaskGroupInStepbackByGroupName": func(t *testing.T) {
			// Test that when stepback stores the task group name (not individual task names),
			// all tasks in the group are properly activated.
			g := GeneratedProject{
				Task: &task.Task{
					ActivatedBy: evergreen.StepbackTaskActivator,
					GeneratedTasksToActivate: map[string][]string{
						"variant1": {"my-task-group"}, // Stepback stores the task GROUP name
					},
				},
				BuildVariants: []parserBV{
					{
						Name: "variant1",
						Tasks: []parserBVTaskUnit{
							{
								Name:     "my-task-group",
								Activate: utility.FalsePtr(),
							},
						},
					},
				},
				Tasks: []parserTask{
					{Name: "task1"},
					{Name: "task2"},
					{Name: "task3"},
				},
				TaskGroups: []parserTaskGroup{
					{
						Name:  "my-task-group",
						Tasks: []string{"task1", "task2", "task3"},
					},
				},
			}

			activationInfo := g.findTasksAndVariantsWithSpecificActivations(evergreen.RepotrackerVersionRequester)

			// Verify stepback info contains all tasks from the group
			require.Contains(t, activationInfo.stepbackTasks, "variant1")
			stepbackTasks := activationInfo.stepbackTasks["variant1"]
			require.Len(t, stepbackTasks, 3, "all tasks in the group should have stepback info")

			// All tasks should be activated because the task group name is in GeneratedTasksToActivate
			for _, st := range stepbackTasks {
				assert.True(t, st.activate, "all tasks in the group should be activated when task group name is in stepback list")
			}
		},
		"DoesNotExpandRegularTask": func(t *testing.T) {
			g := GeneratedProject{
				Task: &task.Task{},
				BuildVariants: []parserBV{
					{
						Name: "variant1",
						Tasks: []parserBVTaskUnit{
							{
								Name:     "regular-task",
								Activate: utility.FalsePtr(),
							},
						},
					},
				},
				Tasks: []parserTask{
					{Name: "regular-task"},
				},
				TaskGroups: []parserTaskGroup{},
			}

			activationInfo := g.findTasksAndVariantsWithSpecificActivations(evergreen.RepotrackerVersionRequester)

			// Verify the regular task (not a group) is handled correctly
			require.Contains(t, activationInfo.activationTasks, "variant1")
			tasksWithSpecificActivation := activationInfo.activationTasks["variant1"]
			assert.Equal(t, []string{"regular-task"}, tasksWithSpecificActivation,
				"regular task should be in the list as-is, not expanded")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tCase(t)
		})
	}
}
