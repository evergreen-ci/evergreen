package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
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
}

func TestGenerateSuite(t *testing.T) {
	suite.Run(t, new(GenerateSuite))
}

func (s *GenerateSuite) SetupTest() {
	s.Require().NoError(db.ClearCollections(task.Collection, build.Collection, VersionCollection, ParserProjectCollection, ProjectRefCollection))
	ref := ProjectRef{
		Id: "proj",
	}
	s.Require().NoError(ref.Insert())
	ref2 := ProjectRef{
		Id: "",
	}
	s.Require().NoError(ref2.Insert())
}

func (s *GenerateSuite) TestParseProjectFromJSON() {
	g, err := ParseProjectFromJSONString(sampleGenerateTasksYml)
	s.NotNil(g)
	s.Nil(err)

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

func (s *GenerateSuite) TestAddGeneratedProjectToConfig() {
	p := &Project{}
	ctx := context.Background()
	pp, err := LoadProjectInto(ctx, []byte(sampleProjYml), nil, "", p)
	s.NoError(err)
	cachedProject := cacheProjectData(p)
	g := sampleGeneratedProject
	newPP, err := g.addGeneratedProjectToConfig(pp, cachedProject)
	s.NoError(err)
	s.NotEmpty(newPP)
	s.Require().Len(newPP.Tasks, 6)
	s.Require().Len(newPP.BuildVariants, 3)
	s.Require().Len(newPP.Functions, 2)
	s.Equal(newPP.Tasks[0].Name, "say-hi")
	s.Equal(newPP.Tasks[1].Name, "say-bye")
	s.Equal(newPP.Tasks[2].Name, "a-depended-on-task")
	s.Equal(newPP.Tasks[3].Name, "task_that_has_dependencies")
	s.Equal(newPP.Tasks[4].Name, "new_task")
	s.Equal(newPP.Tasks[5].Name, "another_task")

	newPP2, err := g.addGeneratedProjectToConfig(&ParserProject{Functions: map[string]*YAMLCommandSet{}}, cachedProject)
	s.NoError(err)
	s.NotEmpty(newPP2)

	s.Equal(newPP.BuildVariants[0].Name, "a_variant")
	s.Require().Len(newPP.BuildVariants[0].DisplayTasks, 1)
	s.Equal(newPP.BuildVariants[0].DisplayTasks[0].Name, "my_display_task_old_variant")

	s.Equal(newPP.BuildVariants[1].Name, "new_buildvariant")
	s.Len(newPP.BuildVariants[1].DisplayTasks, 0)

	s.Equal(newPP.BuildVariants[2].Name, "another_variant")
	s.Require().Len(newPP.BuildVariants[2].DisplayTasks, 1)
	s.Equal(newPP.BuildVariants[2].DisplayTasks[0].Name, "my_display_task_new_variant")

	_, ok := newPP.Functions["a_function"]
	s.True(ok)
	_, ok = newPP.Functions["new_function"]
	s.True(ok)

	pp, err = LoadProjectInto(ctx, []byte(sampleProjYmlNoFunctions), nil, "", p)
	s.NoError(err)
	newPP, err = g.addGeneratedProjectToConfig(pp, cachedProject)
	s.NoError(err)
	s.NotNil(newPP)
	s.Require().Len(newPP.Tasks, 5)
	s.Require().Len(newPP.BuildVariants, 3)
	s.Len(newPP.Functions, 1)
	s.Equal(newPP.Tasks[0].Name, "say-hi")
	s.Equal(newPP.Tasks[1].Name, "say-bye")
	s.Equal(newPP.Tasks[3].Name, "new_task")

	newPP2, err = g.addGeneratedProjectToConfig(&ParserProject{Functions: map[string]*YAMLCommandSet{}}, cachedProject)
	s.NoError(err)
	s.NotEmpty(newPP2)
}

func (s *GenerateSuite) TestSaveNewBuildsAndTasks() {
	genTask := &task.Task{
		Id:       "task_that_called_generate_task",
		Project:  "proj",
		Version:  "version_that_called_generate_task",
		Priority: 10,
	}
	s.NoError(genTask.Insert())
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
	s.NoError(prevBatchTimeVersion.Insert())

	sampleBuild := build.Build{
		Id:           "sample_build",
		Project:      "proj",
		BuildVariant: "a_variant",
		Version:      "version_that_called_generate_task",
	}
	s.NoError(sampleBuild.Insert())
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
	}
	s.NoError(v.Insert())
	pp := &ParserProject{}
	err := util.UnmarshalYAMLWithFallback([]byte(sampleProjYml), &pp)
	s.NoError(err)
	pp.Id = "version_that_called_generate_task"
	s.NoError(pp.Insert())

	g := sampleGeneratedProject
	g.Task = genTask

	p, pp, err := FindAndTranslateProjectForVersion(v.Id, "proj")
	s.Require().NoError(err)
	p, pp, v, err = g.NewVersion(p, pp, v)
	s.Require().NoError(err)
	s.NoError(g.Save(context.Background(), p, pp, v))

	// verify we stopped saving versions
	v, err = VersionFindOneId(v.Id)
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

	pp, err = ParserProjectFindOneById(v.Id)
	s.NoError(err)
	s.Require().NotNil(pp)
	s.Equal(1, pp.ConfigUpdateNumber)
	builds, err := build.FindBuildsByVersions([]string{v.Id})
	s.NoError(err)
	tasks, err := task.FindAll(db.Query(bson.M{task.VersionKey: v.Id})) // with display
	s.NoError(err)
	s.Len(builds, 2)
	s.Len(tasks, 7)
	existingVariantTasks, err := task.Find(task.ByBuildId(sampleBuild.Id)) // without display
	s.NoError(err)
	s.Len(existingVariantTasks, 2)
	for _, existingTask := range existingVariantTasks {
		if existingTask.DisplayName == "say-bye" {
			s.False(existingTask.Activated)
		} else {
			s.True(existingTask.Activated)
		}
	}

	for _, task := range tasks {
		if task.DisplayOnly {
			s.EqualValues(0, task.Priority)
		} else {
			s.EqualValues(10, task.Priority,
				fmt.Sprintf("task '%s' for '%s' failed", task.DisplayName, task.BuildVariant))
		}
	}
}

func (s *GenerateSuite) TestSaveWithAlreadyGeneratedTasksAndVariants() {
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
	s.NoError(generatorTask.Insert())
	s.NoError(alreadyExistingTask.Insert())
	s.NoError(alreadyGeneratedVariant.Insert())

	v := &Version{
		Id:       "version_that_called_generate_task",
		BuildIds: []string{"new_variant"},
	}
	s.NoError(v.Insert())
	pp := &ParserProject{}
	err := util.UnmarshalYAMLWithFallback([]byte(sampleProjYmlTaskGroups), &pp)
	s.NoError(err)
	pp.Id = "version_that_called_generate_task"
	s.NoError(pp.Insert())
	// Setup parser project to be partially generated.
	p, pp, err := FindAndTranslateProjectForVersion(v.Id, "")
	s.NoError(err)

	g := partiallyGeneratedProject
	g.Task = generatorTask
	p, pp, v, err = g.NewVersion(p, pp, v)
	s.NoError(err)
	pp.UpdatedByGenerators = []string{generatorTask.Id}
	s.NoError(pp.TryUpsert())

	// Shouldn't error trying to add the same generated project.
	p, pp, v, err = g.NewVersion(p, pp, v)
	s.NoError(err)
	s.Len(pp.UpdatedByGenerators, 1) // Not modified again.

	s.NoError(g.Save(context.Background(), p, pp, v))

	tasks := []task.Task{}
	taskQuery := db.Query(bson.M{task.GeneratedByKey: "generator"}).Sort([]string{task.CreateTimeKey})
	err = db.FindAllQ(task.Collection, taskQuery, &tasks)
	s.NoError(err)
	s.Require().Len(tasks, 3)
	// New task is added both to previously generated variant, and new variant.
	s.Equal(tasks[0].DisplayName, "another_new_task")
	s.Equal(tasks[1].DisplayName, "another_new_task")
	s.Equal(tasks[2].DisplayName, "new_task")

	// New build is added.
	builds, err := build.FindBuildsByVersions([]string{v.Id})
	s.NoError(err)
	s.Require().Len(builds, 2)
	s.Equal(builds[0].BuildVariant, "new_variant")
	s.Equal(builds[1].BuildVariant, "another_new_variant")

}

func (s *GenerateSuite) TestSaveNewTasksWithDependencies() {
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
		s.NoError(t.Insert())
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
	s.NoError(pp.Insert())
	s.NoError(sampleBuild.Insert())
	s.NoError(v.Insert())

	g := sampleGeneratedProjectAddToBVOnly
	g.Task = &tasksThatExist[0]
	p, pp, err := FindAndTranslateProjectForVersion(v.Id, "")
	s.Require().NoError(err)
	p, pp, v, err = g.NewVersion(p, pp, v)
	s.NoError(err)
	s.NoError(g.Save(context.Background(), p, pp, v))

	v, err = VersionFindOneId(v.Id)
	s.NoError(err)
	s.Require().NotNil(v)

	pp, err = ParserProjectFindOneById(v.Id)
	s.NoError(err)
	s.Require().NotNil(pp)
	s.Equal(1, pp.ConfigUpdateNumber)

	tasks := []task.Task{}
	err = db.FindAllQ(task.Collection, db.Query(bson.M{}), &tasks)
	s.NoError(err)
	err = db.FindAllQ(task.Collection, db.Query(bson.M{"display_name": "task_that_has_dependencies"}), &tasks)
	s.NoError(err)
	s.Require().Len(tasks, 1)
	s.Require().Len(tasks[0].DependsOn, 3)
	expected := map[string]bool{"say-hi-task-id": false, "say-bye-task-id": false, "say_something_else": false}
	for _, dependency := range tasks[0].DependsOn {
		expected[dependency.TaskId] = true
	}
	for taskName, expect := range expected {
		s.True(expect, fmt.Sprintf("%s should be a dependency but wasn't", taskName))
	}
}

func (s *GenerateSuite) TestSaveNewTasksWithCrossVariantDependencies() {
	t1 := &task.Task{
		Id:      "generator",
		BuildId: "b1",
		Version: "v1",
	}
	s.NoError(t1.Insert())

	existingBuild := build.Build{
		Id:           "b1",
		BuildVariant: "a_variant",
		Version:      "v1",
	}
	v := &Version{
		Id:       "v1",
		BuildIds: []string{"b1"},
	}
	parserProj := ParserProject{}
	config := `tasks:
- name: say_something
- name: generator

buildvariants:
- name: a_variant
  run_on:
  - "arch"
  tasks:
  - name: say_something
  - name: generator
`
	err := util.UnmarshalYAMLWithFallback([]byte(config), &parserProj)
	s.NoError(err)
	parserProj.Id = "v1"
	s.NoError(parserProj.Insert())
	s.NoError(existingBuild.Insert())
	s.NoError(v.Insert())

	g := GeneratedProject{
		Task: t1,
		Tasks: []parserTask{
			{
				Name: "task_that_has_dependencies",
				DependsOn: []parserDependency{
					{
						TaskSelector: taskSelector{
							Name: "say_something",
							Variant: &variantSelector{
								StringSelector: "a_variant",
							},
						},
					},
				},
			},
		},
		BuildVariants: []parserBV{
			{
				Name: "a_new_variant",
				Tasks: parserBVTaskUnits{
					parserBVTaskUnit{
						Name: "task_that_has_dependencies",
					},
				},
				RunOn: []string{"arch"},
			},
		},
	}

	p, pp, err := FindAndTranslateProjectForVersion(v.Id, "")
	s.Require().NoError(err)
	p, pp, v, err = g.NewVersion(p, pp, v)
	s.NoError(err)
	s.NoError(g.Save(context.Background(), p, pp, v))

	// the depended-on task is created in the existing variant
	saySomething := task.Task{}
	err = db.FindOneQ(task.Collection, db.Query(bson.M{"display_name": "say_something"}), &saySomething)
	s.NoError(err)

	// the dependent task depends on the depended-on task
	taskWithDeps := task.Task{}
	err = db.FindOneQ(task.Collection, db.Query(bson.M{"display_name": "task_that_has_dependencies"}), &taskWithDeps)
	s.NoError(err)
	s.Require().Len(taskWithDeps.DependsOn, 1)
	s.Equal(taskWithDeps.DependsOn[0].TaskId, saySomething.Id)
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
	s.NoError(pp.Insert())
	s.NoError(taskThatExists.Insert())
	s.NoError(taskDisplayGen.Insert())
	s.NoError(sampleBuild.Insert())
	s.NoError(v.Insert())

	g := smallGeneratedProject
	g.Task = &taskThatExists
	p, pp, err := FindAndTranslateProjectForVersion(v.Id, "")
	s.Require().NoError(err)
	p, pp, v, err = g.NewVersion(p, pp, v)
	s.Require().NoError(err)
	s.NoError(g.Save(context.Background(), p, pp, v))

	v, err = VersionFindOneId(v.Id)
	s.NoError(err)
	s.Require().NotNil(v)

	pp, err = ParserProjectFindOneById(v.Id)
	s.NoError(err)
	s.Require().NotNil(pp)
	s.Equal(1, pp.ConfigUpdateNumber)

	tasks := []task.Task{}
	s.NoError(db.FindAllQ(task.Collection, db.Query(bson.M{}), &tasks))
	s.NoError(db.FindAllQ(task.Collection, db.Query(bson.M{"display_name": "my_display_task_gen"}), &tasks))
	s.Len(tasks, 1)
	s.NoError(db.FindAllQ(task.Collection, db.Query(bson.M{"display_name": "my_display_task"}), &tasks))
	s.Len(tasks, 1)
	s.Len(tasks[0].ExecutionTasks, 1)
}

func TestSimulateNewDependencyGraph(t *testing.T) {
	defer func() {
		assert.NoError(t, db.Clear(task.Collection))
	}()
	require.NoError(t, db.Clear(task.Collection))

	v := &Version{Id: "v0"}
	generatorTask := task.Task{Id: "mci_bv0_generator__01_01_01_00_00_00", Version: v.Id, BuildVariant: "bv0", DisplayName: "generator"}
	require.NoError(t, generatorTask.Insert())

	t.Run("CreatesCycle", func(t *testing.T) {
		project := &Project{
			BuildVariants: []BuildVariant{
				{Name: "bv0", Tasks: []BuildVariantTaskUnit{
					{Name: "generated", DependsOn: []TaskUnitDependency{{Name: "dependedOn", Variant: "bv0"}}},
					{Name: "dependedOn", DependsOn: []TaskUnitDependency{{Name: "generator", Variant: "bv0"}}},
					{Name: "generator"},
				}},
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
		assert.Error(t, g.CheckForCycles(v, project, &ProjectRef{Identifier: "mci"}))
	})

	t.Run("CreatesLoop", func(t *testing.T) {
		project := &Project{
			BuildVariants: []BuildVariant{
				{Name: "bv0", Tasks: []BuildVariantTaskUnit{
					{Name: "generated", DependsOn: []TaskUnitDependency{{Name: "generator", Variant: "bv0"}}},
					{Name: "generator"},
				}},
			},
			Tasks: []ProjectTask{
				{Name: "generated"},
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

		assert.Error(t, g.CheckForCycles(v, project, &ProjectRef{Identifier: "mci"}))
	})

	t.Run("NoCycles", func(t *testing.T) {
		project := &Project{
			BuildVariants: []BuildVariant{
				{
					Name: "bv0",
					Tasks: []BuildVariantTaskUnit{
						{Name: "generated"},
						{Name: "dependedOn", DependsOn: []TaskUnitDependency{{Name: "generator", Variant: "bv0"}}},
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
				},
			},
		}
		assert.NoError(t, g.CheckForCycles(v, project, &ProjectRef{Identifier: "mci"}))
	})

	t.Run("InactiveBuild", func(t *testing.T) {
		project := &Project{
			BuildVariants: []BuildVariant{
				{
					Name: "bv0",
					Tasks: []BuildVariantTaskUnit{
						{Name: "generated", DependsOn: []TaskUnitDependency{{Name: "dependedOn", Variant: "bv0"}}},
						{Name: "dependedOn", DependsOn: []TaskUnitDependency{{Name: "generator", Variant: "bv0"}}},
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
		assert.NoError(t, g.CheckForCycles(v, project, &ProjectRef{Identifier: "mci"}))
	})
}

func TestFilterInactiveTasks(t *testing.T) {
	v := &Version{Requester: evergreen.PatchVersionRequester}

	t.Run("ActiveNonExistentBuild", func(t *testing.T) {
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

		tasks, err := g.filterInactiveTasks(TVPairSet{{TaskName: "generated", Variant: "bv0"}}, v, &Project{})
		assert.NoError(t, err)
		assert.Len(t, tasks, 1)
	})

	t.Run("InactiveNonExistentBuild", func(t *testing.T) {
		g := GeneratedProject{
			BuildVariants: []parserBV{
				{
					Name: "bv0",
					Tasks: []parserBVTaskUnit{
						{Name: "generated"},
					},
					Activate: utility.FalsePtr(),
				},
			},
			Task: &task.Task{},
		}

		tasks, err := g.filterInactiveTasks(TVPairSet{{TaskName: "generated", Variant: "bv0"}}, v, &Project{})
		assert.NoError(t, err)
		assert.Empty(t, tasks)
	})

	t.Run("ActiveExistingBuild", func(t *testing.T) {
		defer func() {
			assert.NoError(t, db.Clear(build.Collection))
		}()
		assert.NoError(t, db.Clear(build.Collection))
		assert.NoError(t, (&build.Build{DisplayName: "bv0"}).Insert())

		g := GeneratedProject{
			BuildVariants: []parserBV{
				{
					Name: "bv0",
					Tasks: []parserBVTaskUnit{
						{
							Name: "generated",
						},
					},
				},
			},
			Task: &task.Task{},
		}

		p := &Project{
			BuildVariants: []BuildVariant{
				{Name: "bv0"},
			},
		}

		tasks, err := g.filterInactiveTasks(TVPairSet{{TaskName: "generated", Variant: "bv0"}}, v, p)
		assert.NoError(t, err)
		assert.Len(t, tasks, 1)
	})

	t.Run("InactiveExistingBuild", func(t *testing.T) {
		defer func() {
			assert.NoError(t, db.Clear(build.Collection))
		}()
		assert.NoError(t, db.Clear(build.Collection))
		assert.NoError(t, (&build.Build{BuildVariant: "bv0"}).Insert())

		g := GeneratedProject{
			BuildVariants: []parserBV{
				{
					Name: "bv0",
					Tasks: []parserBVTaskUnit{
						{
							Name: "generated",
						},
					},
				},
			},
			Task: &task.Task{},
		}

		p := &Project{
			BuildVariants: []BuildVariant{
				{Name: "bv0", Activate: utility.FalsePtr()},
			},
		}

		tasks, err := g.filterInactiveTasks(TVPairSet{{TaskName: "generated", Variant: "bv0"}}, v, p)
		assert.NoError(t, err)
		assert.Empty(t, tasks)
	})

	t.Run("InactiveTask", func(t *testing.T) {
		g := GeneratedProject{
			BuildVariants: []parserBV{
				{
					Name: "bv0",
					Tasks: []parserBVTaskUnit{
						{
							Name:     "generated",
							Activate: utility.FalsePtr(),
						},
					},
				},
			},
			Task: &task.Task{},
		}

		tasks, err := g.filterInactiveTasks(TVPairSet{{TaskName: "generated", Variant: "bv0"}}, v, &Project{})
		assert.NoError(t, err)
		assert.Empty(t, tasks)
	})
}

func (s *GenerateSuite) TestMergeGeneratedProjectsWithNoTasks() {
	projects := []GeneratedProject{smallGeneratedProject}
	merged, err := MergeGeneratedProjects(projects)
	s.Require().NoError(err)
	s.Require().NotNil(merged)
	s.Require().Len(merged.BuildVariants, 1)
	s.Len(merged.BuildVariants[0].DisplayTasks, 1)
}

func TestUpdateParserProject(t *testing.T) {
	alreadyUpdatedTestName := "TaskAlreadyUpdatedParserProject"
	withZeroTestName := "WithZero"
	taskId := "tId"
	for testName, setupTest := range map[string]func(t *testing.T, v *Version, pp *ParserProject){
		"noParserProject": func(t *testing.T, v *Version, pp *ParserProject) {
			assert.NoError(t, v.Insert())
		},
		"ParserProjectMoreRecent": func(t *testing.T, v *Version, pp *ParserProject) {
			pp.ConfigUpdateNumber = 5
			assert.NoError(t, v.Insert())
			assert.NoError(t, pp.Insert())
		},
		"ConfigMostRecent": func(t *testing.T, v *Version, pp *ParserProject) {
			pp.ConfigUpdateNumber = 1
			assert.NoError(t, v.Insert())
			assert.NoError(t, pp.Insert())
		},
		withZeroTestName: func(t *testing.T, v *Version, pp *ParserProject) {
			assert.NoError(t, v.Insert())
			assert.NoError(t, pp.Insert())
		},
		alreadyUpdatedTestName: func(t *testing.T, v *Version, pp *ParserProject) {
			pp.UpdatedByGenerators = []string{taskId}
			pp.ConfigUpdateNumber = 1
			assert.NoError(t, v.Insert())
			assert.NoError(t, pp.Insert())
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(VersionCollection, ParserProjectCollection))
			v := &Version{Id: "my-version"}
			pp := &ParserProject{Id: "my-version"}
			setupTest(t, v, pp)
			assert.NoError(t, updateParserProject(v, pp, taskId))
			v, err := VersionFindOneId(v.Id)
			assert.NoError(t, err)
			require.NotNil(t, v)
			pp, err = ParserProjectFindOneById(v.Id)
			assert.NoError(t, err)
			require.NotNil(t, pp)
			assert.Len(t, pp.UpdatedByGenerators, 1)
			assert.Contains(t, pp.UpdatedByGenerators, taskId)
			if testName == withZeroTestName {
				assert.Equal(t, 1, pp.ConfigUpdateNumber)
				return
			} else if testName == alreadyUpdatedTestName {
				// Not changed because we've already updated.
				assert.Equal(t, 1, pp.ConfigUpdateNumber)
				return
			}
		})
	}
}

func TestAddDependencies(t *testing.T) {
	require.NoError(t, db.Clear(task.Collection))

	existingTasks := []task.Task{
		{Id: "t1", DependsOn: []task.Dependency{{TaskId: "generator", Status: evergreen.TaskSucceeded}}},
		{Id: "t2", DependsOn: []task.Dependency{{TaskId: "generator", Status: task.AllStatuses}}},
	}
	for _, task := range existingTasks {
		assert.NoError(t, task.Insert())
	}

	g := GeneratedProject{Task: &task.Task{Id: "generator"}}
	assert.NoError(t, g.addDependencies([]string{"t3"}))

	t1, err := task.FindOneId("t1")
	assert.NoError(t, err)
	assert.Len(t, t1.DependsOn, 2)
	for _, dep := range t1.DependsOn {
		assert.Equal(t, evergreen.TaskSucceeded, dep.Status)
	}

	t2, err := task.FindOneId("t2")
	assert.NoError(t, err)
	assert.Len(t, t2.DependsOn, 2)
	for _, dep := range t2.DependsOn {
		assert.Equal(t, task.AllStatuses, dep.Status)
	}
}
