package model

import (
	"context"
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
)

var (
	sampleGeneratedProject = GeneratedProject{
		Tasks: []parserTask{
			parserTask{
				Name: "new_task",
				Commands: []PluginCommandConf{
					PluginCommandConf{
						Command: "shell.exec",
					},
				},
			},
			parserTask{
				Name: "another_task",
				Commands: []PluginCommandConf{
					PluginCommandConf{
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
			parserBV{
				Name: "new_buildvariant",
			},
			parserBV{
				Name: "a_variant",
				Tasks: parserBVTaskUnits{
					parserBVTaskUnit{
						Name: "say-bye",
					},
				},
				DisplayTasks: []displayTask{
					displayTask{
						Name:           "my_display_task_old_variant",
						ExecutionTasks: []string{"say-bye"},
					},
				},
			},
			parserBV{
				Name: "another_variant",
				Tasks: parserBVTaskUnits{
					parserBVTaskUnit{
						Name: "example_task_group",
					},
				},
				DisplayTasks: []displayTask{
					displayTask{
						Name:           "my_display_task_new_variant",
						ExecutionTasks: []string{"another_task"},
					},
				},
			},
		},
		Functions: map[string]*YAMLCommandSet{
			"new_function": &YAMLCommandSet{
				MultiCommand: []PluginCommandConf{},
			},
		},
		TaskGroups: []parserTaskGroup{
			parserTaskGroup{
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
			parserBV{
				Name: "my_build_variant",
				DisplayTasks: []displayTask{
					displayTask{
						Name:           "my_display_task",
						ExecutionTasks: []string{"my_display_task_gen"},
					},
				},
			},
		},
	}

	sampleGeneratedProjectAddToBVOnly = GeneratedProject{
		BuildVariants: []parserBV{
			parserBV{
				Name: "a_variant",
				Tasks: parserBVTaskUnits{
					parserBVTaskUnit{
						Name: "task_that_has_dependencies",
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
	s.Require().NoError(db.ClearCollections(task.Collection, build.Collection, VersionCollection))
}

func (s *GenerateSuite) TestParseProjectFromJSON() {
	g, err := ParseProjectFromJSON([]byte(sampleGenerateTasksYml))
	s.NotNil(g)
	s.Nil(err)

	s.Len(g.Functions, 2)
	s.Contains(g.Functions, "echo-hi")
	s.Equal("shell.exec", g.Functions["echo-hi"].List()[0].Command)

	s.Require().NoError(g.Functions["echo-hi"].List()[0].resolveParams())
	s.Equal("echo hi", g.Functions["echo-hi"].List()[0].Params["script"])

	s.Require().NoError(g.Functions["echo-bye"].List()[0].resolveParams())
	s.Equal("echo bye", g.Functions["echo-bye"].List()[0].Params["script"])

	s.Require().NoError(g.Functions["echo-bye"].List()[1].resolveParams())
	s.Equal("echo bye again", g.Functions["echo-bye"].List()[1].Params["script"])

	s.Len(g.Tasks, 1)
	s.Equal("git.get_project", g.Tasks[0].Commands[0].Command)

	s.Require().NoError(g.Tasks[0].Commands[0].resolveParams())
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
	c := grip.NewBasicCatcher()
	g.validateMaxTasksAndVariants(c)
	s.NoError(c.Resolve())
	for i := 0; i < maxGeneratedBuildVariants; i++ {
		g.BuildVariants = append(g.BuildVariants, parserBV{})
	}
	g.validateMaxTasksAndVariants(c)
	s.NoError(c.Resolve())
	g.BuildVariants = append(g.BuildVariants, parserBV{})
	g.validateMaxTasksAndVariants(c)
	s.Error(c.Resolve())
}

func (s *GenerateSuite) TestValidateMaxTasks() {
	g := GeneratedProject{}
	c := grip.NewBasicCatcher()
	g.validateMaxTasksAndVariants(c)
	s.NoError(c.Resolve())
	for i := 0; i < maxGeneratedTasks; i++ {
		g.Tasks = append(g.Tasks, parserTask{})
	}
	g.validateMaxTasksAndVariants(c)
	s.NoError(c.Resolve())
	g.Tasks = append(g.Tasks, parserTask{})
	g.validateMaxTasksAndVariants(c)
	s.Error(c.Resolve())
}

func (s *GenerateSuite) TestValidateNoRedefine() {
	g := GeneratedProject{}
	c := grip.NewBasicCatcher()
	g.validateNoRedefine(projectMaps{}, c)
	s.NoError(c.Resolve())

	g.BuildVariants = []parserBV{parserBV{Name: "buildvariant_name", DisplayName: "I am a buildvariant"}}
	g.Tasks = []parserTask{parserTask{Name: "task_name"}}
	g.Functions = map[string]*YAMLCommandSet{"function_name": nil}
	g.validateNoRedefine(projectMaps{}, c)
	s.NoError(c.Resolve())

	cachedProject := projectMaps{
		buildVariants: map[string]struct{}{
			"buildvariant_name": struct{}{},
		},
	}
	g.validateNoRedefine(cachedProject, c)
	s.Error(c.Resolve())

	c = grip.NewBasicCatcher()
	cachedProject = projectMaps{
		tasks: map[string]*ProjectTask{
			"task_name": &ProjectTask{},
		},
	}
	g.validateNoRedefine(cachedProject, c)
	s.Error(c.Resolve())

	c = grip.NewBasicCatcher()
	cachedProject = projectMaps{
		functions: map[string]*YAMLCommandSet{
			"function_name": &YAMLCommandSet{},
		},
	}
	g.validateNoRedefine(cachedProject, c)
	s.Error(c.Resolve())

}

func (s *GenerateSuite) TestValidateNoRecursiveGenerateTasks() {
	g := GeneratedProject{}
	c := grip.NewBasicCatcher()
	cachedProject := projectMaps{}
	g.validateNoRecursiveGenerateTasks(cachedProject, c)
	s.NoError(c.Resolve())

	c = grip.NewBasicCatcher()
	cachedProject = projectMaps{}
	g = GeneratedProject{
		Tasks: []parserTask{
			parserTask{
				Commands: []PluginCommandConf{
					PluginCommandConf{
						Command: "generate.tasks",
					},
				},
			},
		},
	}
	g.validateNoRecursiveGenerateTasks(cachedProject, c)
	s.Error(c.Resolve())

	c = grip.NewBasicCatcher()
	cachedProject = projectMaps{}
	g = GeneratedProject{
		Functions: map[string]*YAMLCommandSet{
			"a_function": &YAMLCommandSet{
				MultiCommand: []PluginCommandConf{
					PluginCommandConf{
						Command: "generate.tasks",
					},
				},
			},
		},
	}
	g.validateNoRecursiveGenerateTasks(cachedProject, c)
	s.Error(c.Resolve())

	c = grip.NewBasicCatcher()
	cachedProject = projectMaps{
		tasks: map[string]*ProjectTask{
			"task_name": &ProjectTask{
				Commands: []PluginCommandConf{
					PluginCommandConf{
						Command: "generate.tasks",
					},
				},
			},
		},
	}
	g = GeneratedProject{
		BuildVariants: []parserBV{
			parserBV{
				Tasks: []parserBVTaskUnit{
					parserBVTaskUnit{
						Name: "task_name",
					},
				},
			},
		},
	}
	g.validateNoRecursiveGenerateTasks(cachedProject, c)
	s.Error(c.Resolve())

	c = grip.NewBasicCatcher()
	cachedProject = projectMaps{
		tasks: map[string]*ProjectTask{
			"task_name": &ProjectTask{
				Commands: []PluginCommandConf{
					PluginCommandConf{
						Function: "generate_function",
					},
				},
			},
		},
		functions: map[string]*YAMLCommandSet{
			"generate_function": &YAMLCommandSet{
				MultiCommand: []PluginCommandConf{
					PluginCommandConf{
						Command: "generate.tasks",
					},
				},
			},
		},
	}
	g = GeneratedProject{
		BuildVariants: []parserBV{
			parserBV{
				Tasks: []parserBVTaskUnit{
					parserBVTaskUnit{
						Name: "task_name",
					},
				},
			},
		},
	}
	g.validateNoRecursiveGenerateTasks(cachedProject, c)
	s.Error(c.Resolve())
}

func (s *GenerateSuite) TestAddGeneratedProjectToConfig() {
	p := &Project{}
	err := LoadProjectInto([]byte(sampleProjYml), "", p)
	s.NoError(err)
	cachedProject := cacheProjectData(p)
	g := sampleGeneratedProject
	config, err := g.addGeneratedProjectToConfig(sampleProjYml, cachedProject)
	s.NoError(err)
	s.Contains(config, "say-hi")
	s.Contains(config, "new_task")
	s.Contains(config, "a_variant")
	s.Contains(config, "new_buildvariant")
	s.Contains(config, "a_function")
	s.Contains(config, "new_function")
	s.Contains(config, "say-bye")
	s.Contains(config, "my_display_task_new_variant")
	s.Contains(config, "my_display_task_old_variant")

	config, err = g.addGeneratedProjectToConfig(sampleProjYmlNoFunctions, cachedProject)
	s.NoError(err)
	s.Contains(config, "say-hi")
	s.Contains(config, "new_task")
	s.Contains(config, "a_variant")
	s.Contains(config, "new_buildvariant")
	s.Contains(config, "say-bye")
	s.Contains(config, "my_display_task_new_variant")
	s.Contains(config, "my_display_task_old_variant")
}

func (s *GenerateSuite) TestSaveNewBuildsAndTasks() {
	genTask := task.Task{
		Id:       "task_that_called_generate_task",
		Version:  "version_that_called_generate_task",
		Priority: 10,
	}
	s.NoError(genTask.Insert())

	sampleBuild := build.Build{
		Id:           "sample_build",
		BuildVariant: "a_variant",
	}
	v := &Version{
		Id:                 "version_that_called_generate_task",
		BuildIds:           []string{"sample_build"},
		Config:             sampleProjYml,
		ConfigUpdateNumber: 4,
	}
	s.NoError(sampleBuild.Insert())
	s.NoError(v.Insert())

	g := sampleGeneratedProject
	g.TaskID = "task_that_called_generate_task"
	p, v, t, pm, err := g.NewVersion()
	s.NoError(err)
	s.NoError(g.Save(context.Background(), p, v, t, pm))
	s.Equal(5, v.ConfigUpdateNumber)
	builds, err := build.Find(db.Query(bson.M{}))
	s.NoError(err)
	tasks := []task.Task{}
	err = db.FindAllQ(task.Collection, db.Q{}, &tasks)
	s.NoError(err)
	s.Len(builds, 3)
	s.Len(tasks, 6)

	for _, task := range tasks {
		if task.DisplayOnly {
			s.EqualValues(0, task.Priority)
		} else {
			s.EqualValues(10, task.Priority)
		}
	}
}

func (s *GenerateSuite) TestSaveNewTasksWithDependencies() {
	tasksThatExist := []task.Task{
		{
			Id:      "task_that_called_generate_task",
			Version: "version_that_called_generate_task",
			BuildId: "generate_build",
		},
		{
			Id:      "say-hi-task-id",
			Version: "version_that_called_generate_task",
			BuildId: "sample_build",
		},
		{
			Id:      "say-bye-task-id",
			Version: "version_that_called_generate_task",
			BuildId: "sample_build",
		},
		{
			Id:      "say_something_else",
			Version: "version_that_called_generate_task",
			BuildId: "sample_build",
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
		Config:   sampleProjYmlTaskGroups,
	}
	s.NoError(sampleBuild.Insert())
	s.NoError(v.Insert())

	g := sampleGeneratedProjectAddToBVOnly
	g.TaskID = "task_that_called_generate_task"
	p, v, t, pm, err := g.NewVersion()
	s.NoError(err)
	s.NoError(g.Save(context.Background(), p, v, t, pm))
	s.Equal(1, v.ConfigUpdateNumber)
	tasks := []task.Task{}
	err = db.FindAllQ(task.Collection, db.Query(bson.M{}), &tasks)
	s.NoError(err)
	err = db.FindAllQ(task.Collection, db.Query(bson.M{"display_name": "task_that_has_dependencies"}), &tasks)
	s.NoError(err)
	s.Len(tasks[0].DependsOn, 3)
	expected := map[string]bool{"say-hi-task-id": false, "say-bye-task-id": false, "say_something_else": false}
	for _, dependency := range tasks[0].DependsOn {
		expected[dependency.TaskId] = true
	}
	for taskName, expect := range expected {
		s.True(expect, fmt.Sprintf("%s should be a dependency but wasn't", taskName))
	}
}

func (s *GenerateSuite) TestSaveNewTaskWithExistingExecutionTask() {
	taskThatExists := task.Task{
		Id:      "task_that_called_generate_task",
		Version: "version_that_called_generate_task",
	}
	taskDisplayGen := task.Task{
		Id:          "my_display_task_gen",
		DisplayName: "my_display_task_gen",
		Version:     "version_that_called_generate_task",
	}
	sampleBuild := build.Build{
		Id:           "sample_build",
		BuildVariant: "my_build_variant",
		Version:      "version_that_called_generate_task",
	}
	v := &Version{
		Id:       "version_that_called_generate_task",
		BuildIds: []string{"sample_build"},
		Config:   smallYml,
	}
	s.NoError(taskThatExists.Insert())
	s.NoError(taskDisplayGen.Insert())
	s.NoError(sampleBuild.Insert())
	s.NoError(v.Insert())

	g := smallGeneratedProject
	g.TaskID = "task_that_called_generate_task"
	p, v, t, pm, err := g.NewVersion()
	s.Require().NoError(err)
	s.NoError(g.Save(context.Background(), p, v, t, pm))
	s.Equal(1, v.ConfigUpdateNumber)

	tasks := []task.Task{}
	s.NoError(db.FindAllQ(task.Collection, db.Query(bson.M{}), &tasks))
	s.NoError(db.FindAllQ(task.Collection, db.Query(bson.M{"display_name": "my_display_task_gen"}), &tasks))
	s.Len(tasks, 1)
	s.NoError(db.FindAllQ(task.Collection, db.Query(bson.M{"display_name": "my_display_task"}), &tasks))
	s.Len(tasks, 1)
}

func (s *GenerateSuite) TestMergeGeneratedProjectsWithNoTasks() {
	projects := []GeneratedProject{smallGeneratedProject}
	merged := MergeGeneratedProjects(projects)
	s.Require().NotNil(merged)
	s.Require().Len(merged.BuildVariants, 1)
	s.Len(merged.BuildVariants[0].DisplayTasks, 1)
}
