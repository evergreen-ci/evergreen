package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/mongodb/grip"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
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
			},
			parserBV{
				Name: "another_variant",
				Tasks: parserBVTaskUnits{
					parserBVTaskUnit{
						Name: "another_task",
					},
				},
			},
		},
		Functions: map[string]*YAMLCommandSet{
			"new_function": &YAMLCommandSet{
				MultiCommand: []PluginCommandConf{},
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

buildvariants:
  - name: a_variant
    display_name: Variant Number One
    tasks:
    - name: "say-hi"

functions:
  a_function:
    command: shell.exec
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
        ],
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
            "name": "first"
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
	s.Require().NoError(db.ClearCollections(task.Collection, build.Collection, version.Collection))
}

func (s *GenerateSuite) TestParseProjectFromJSON() {
	g, err := ParseProjectFromJSON([]byte(sampleGenerateTasksYml))
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

func (s *GenerateSuite) TestaddGeneratedProjectToConfig() {
	cachedProject := projectMaps{}
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
}

func (s *GenerateSuite) TestSaveNewBuildsAndTasks() {
	t := task.Task{
		Id:      "task_that_called_generate_task",
		Version: "version_that_called_generate_task",
	}
	s.NoError(t.Insert())
	sampleBuild := build.Build{
		Id:           "sample_build",
		BuildVariant: "a_variant",
	}
	v := &version.Version{
		Id:       "version_that_called_generate_task",
		BuildIds: []string{"sample_build"},
		Config:   sampleProjYml,
	}
	s.NoError(sampleBuild.Insert())
	s.NoError(v.Insert())

	g := sampleGeneratedProject
	g.TaskID = "task_that_called_generate_task"
	s.NoError(g.AddGeneratedProjectToVersion())
	builds, err := build.Find(db.Query(bson.M{}))
	s.NoError(err)
	tasks, err := task.Find(db.Query(bson.M{}))
	s.NoError(err)
	s.Len(builds, 2)
	s.Len(tasks, 3)
}
