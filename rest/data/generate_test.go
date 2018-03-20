package data

import (
	"encoding/json"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var sampleBaseProject = `
function:
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
  ]
}
`)}

func TestParseProjects(t *testing.T) {
	assert := assert.New(t)
	parsed, err := ParseProjects(sampleGeneratedProject)
	assert.NoError(err)
	assert.Len(parsed, 1)
	assert.Len(parsed[0].BuildVariants, 1)
	assert.Equal(parsed[0].BuildVariants[0].Name, "race-detector")
	assert.Len(parsed[0].Tasks, 2)
	assert.Equal(parsed[0].Tasks[0].Name, "lint-command")
	assert.Equal(parsed[0].Tasks[1].Name, "lint-rest-route")
}

func TestGenerateTasks(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(version.Collection, build.Collection, task.Collection))
	defer require.NoError(db.ClearCollections(version.Collection, task.Collection))
	sampleVersion := version.Version{
		Id:         "sample_version",
		Identifier: "mci",
		Config:     sampleBaseProject,
		BuildIds:   []string{"sample_build_id"},
	}
	require.NoError(sampleVersion.Insert())
	sampleBuild := build.Build{
		Id:           "sample_build_id",
		BuildVariant: "race-detector",
	}
	require.NoError(sampleBuild.Insert())
	sampleTask := task.Task{
		Id:          "sample_task",
		Version:     "sample_version",
		BuildId:     "sample_build_id",
		Project:     "mci",
		DisplayName: "sample_task",
	}
	require.NoError(sampleTask.Insert())
	gc := GenerateConnector{}
	assert.NoError(gc.GenerateTasks("sample_task", sampleGeneratedProject))
	tasks, err := task.Find(task.ByBuildId("sample_build_id"))
	assert.NoError(err)
	assert.Len(tasks, 3)
	all_tasks := map[string]bool{
		"sample_task":     false,
		"lint-command":    false,
		"lint-rest-route": false,
	}
	for _, t := range tasks {
		assert.Equal("sample_version", t.Version)
		assert.Equal("mci", t.Project)
		all_tasks[t.DisplayName] = true
	}
	for _, v := range all_tasks {
		assert.True(v)
	}
}
