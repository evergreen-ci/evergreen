package units

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var sampleBaseProject = `
functions:
  get-project:
    command: shell.exec
    params:
       shell: bash
       script: |
         echo "get-project function"
  set-up-credentials:
    command: shell.exec
    params:
       shell: bash
       script: |
         echo "set-up-credentials function"
  run-make:
    command: shell.exec
    params:
       shell: bash
       script: |
         echo "fun-make function"

tasks:
  - name: generate-lint
    commands:
      - command: generate.tasks
        params:
          files:
            - example/path/to/generate.json
  - name: dist
    commands:
      - command: shell.exec
        params:
          script: |
            echo "this is a task"

buildvariants:
  - name: ubuntu1604
    display_name: Ubuntu 16.04
    run_on:
      - ubuntu1604-test
    tasks:
      - name: "dist"

  - name: race-detector
    display_name: Race Detector
    run_on:
      - archlinux-test
    tasks:
      - name: "dist"
      - name: generate-lint
`

var sampleGeneratedProject = []json.RawMessage{json.RawMessage(`
{
  "buildvariants": [
    {
      "name": "race-detector",
      "tasks": [
        {
          "name": "lint-command"
        },
        {
          "name": "lint-rest-route"
        }
      ],
      "display_tasks": [
          {
              "name": "my_display_task",
              "execution_tasks": [
                  "lint-command",
                  "lint-rest-route"
              ]
          }
      ]
    }
  ],
  "tasks": [
    {
      "commands": [
        {
          "func": "get-project"
        },
        {
          "func": "set-up-credentials"
        },
        {
          "func": "run-make",
          "vars": {
            "target": "lint-command"
          }
        }
      ],
      "name": "lint-command"
    },
    {
      "commands": [
        {
          "func": "get-project"
        },
        {
          "func": "set-up-credentials"
        },
        {
          "func": "run-make",
          "vars": {
            "target": "lint-rest-route"
          }
        }
      ],
      "name": "lint-rest-route"
    }
  ],
  "task_groups": [
      {
          "name": "my_task_group",
          "max_hosts": 1,
          "tasks": [
            "lint-command",
            "lint-rest-route",
          ]
      },
  ]
}
`)}

func TestGenerateTasks(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(model.VersionCollection, build.Collection, task.Collection, distro.Collection))
	defer require.NoError(db.ClearCollections(model.VersionCollection, build.Collection, task.Collection, distro.Collection))
	randomVersion := model.Version{
		Id:         "random_version",
		Identifier: "mci",
		Config:     sampleBaseProject,
		BuildIds:   []string{"sample_build_id"},
	}
	require.NoError(randomVersion.Insert())
	sampleVersion := model.Version{
		Id:         "sample_version",
		Identifier: "mci",
		Config:     sampleBaseProject,
		BuildIds:   []string{"sample_build_id"},
	}
	require.NoError(sampleVersion.Insert())
	sampleBuild := build.Build{
		Id:           "sample_build_id",
		BuildVariant: "race-detector",
		Version:      "sample_version",
	}
	require.NoError(sampleBuild.Insert())
	sampleTask := task.Task{
		Id:            "sample_task",
		Version:       "sample_version",
		BuildId:       "sample_build_id",
		Project:       "mci",
		DisplayName:   "sample_task",
		GeneratedJSON: sampleGeneratedProject,
		Status:        evergreen.TaskStarted,
	}
	sampleDistros := []distro.Distro{
		distro.Distro{
			Id: "ubuntu1604-test",
		},
		distro.Distro{
			Id: "archlinux-test",
		},
	}
	for _, d := range sampleDistros {
		require.NoError(d.Insert())
	}
	require.NoError(sampleTask.Insert())
	j := NewGenerateTasksJob("sample_task", "1")
	j.Run(context.Background())
	assert.NoError(j.Error())
	tasks := []task.Task{}
	assert.NoError(db.FindAllQ(task.Collection, task.ByBuildId("sample_build_id"), &tasks))
	assert.Len(tasks, 4)
	all_tasks := map[string]bool{
		"sample_task":     false,
		"lint-command":    false,
		"lint-rest-route": false,
		"my_display_task": false,
	}
	for _, t := range tasks {
		assert.Equal("sample_version", t.Version)
		assert.Equal("mci", t.Project)
		all_tasks[t.DisplayName] = true
		if t.Version == "my_display_task" {
			assert.Len(t.ExecutionTasks, 1)
		}
	}
	for _, v := range all_tasks {
		assert.True(v)
	}

	// Make sure first project was not changed
	v, err := model.VersionFindOneId("random_version")
	assert.NoError(err)
	p := model.Project{}
	err = model.LoadProjectInto([]byte(v.Config), "mci", &p)
	assert.NoError(err)
	assert.Len(p.Tasks, 2)
	assert.Len(p.BuildVariants[0].Tasks, 1)
	assert.Len(p.BuildVariants[1].Tasks, 2)

	// Verify second project was changed
	v, err = model.VersionFindOneId("sample_version")
	assert.NoError(err)
	p = model.Project{}
	err = model.LoadProjectInto([]byte(v.Config), "mci", &p)
	assert.NoError(err)
	assert.Len(p.Tasks, 4)
	assert.Len(p.BuildVariants[0].Tasks, 1)
	assert.Len(p.BuildVariants[1].Tasks, 4)
	assert.Len(p.TaskGroups, 1)
	assert.Len(p.TaskGroups[0].Tasks, 2)

	b, err := build.FindOneId("sample_build_id")
	assert.NoError(err)
	assert.Equal("mci_race_detector_display_my_display_task__01_01_01_00_00_00", b.Tasks[0].Id)
}

func TestParseProjects(t *testing.T) {
	assert := assert.New(t)
	parsed, err := parseProjects(sampleGeneratedProject)
	assert.NoError(err)
	assert.Len(parsed, 1)
	assert.Len(parsed[0].BuildVariants, 1)
	assert.Equal(parsed[0].BuildVariants[0].Name, "race-detector")
	assert.Equal("my_display_task", parsed[0].BuildVariants[0].DisplayTasks[0].Name)
	assert.Equal("lint-command", parsed[0].BuildVariants[0].DisplayTasks[0].ExecutionTasks[0])
	assert.Equal("lint-rest-route", parsed[0].BuildVariants[0].DisplayTasks[0].ExecutionTasks[1])
	assert.Len(parsed[0].BuildVariants[0].DisplayTasks, 1)
	assert.Len(parsed[0].Tasks, 2)
	assert.Equal(parsed[0].Tasks[0].Name, "lint-command")
	assert.Equal(parsed[0].Tasks[1].Name, "lint-rest-route")
}
