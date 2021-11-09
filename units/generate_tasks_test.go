package units

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	mgobson "gopkg.in/mgo.v2/bson"
)

var sampleBaseProject = `
functions:
  get-project:
    command: shell.exec
    params:
       shell: bash
       script: |
         echo "get-project function"
  setup-credentials:
    command: shell.exec
    params:
       shell: bash
       script: |
         echo "setup-credentials function"
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

var sampleGeneratedProject = []string{`
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
          "func": "setup-credentials"
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
          "func": "setup-credentials"
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
`}

func TestGenerateTasks(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(model.ProjectRefCollection, model.VersionCollection, build.Collection, task.Collection, distro.Collection, patch.Collection, model.ParserProjectCollection))
	defer require.NoError(db.ClearCollections(model.ProjectRefCollection, model.VersionCollection, build.Collection, task.Collection, distro.Collection, patch.Collection, model.ParserProjectCollection))
	randomVersion := model.Version{
		Id:         "random_version",
		Identifier: "mci",
		Config:     sampleBaseProject,
		BuildIds:   []string{"sample_build_id"},
	}
	require.NoError(randomVersion.Insert())
	randomPatch := patch.Patch{
		Id:      mgobson.NewObjectId(),
		Version: randomVersion.Id,
	}
	require.NoError(randomPatch.Insert())
	sampleVersion := model.Version{
		Id:         "sample_version",
		Identifier: "mci",
		Config:     sampleBaseProject,
		BuildIds:   []string{"sample_build_id"},
	}
	samplePatch := patch.Patch{
		Id:      mgobson.NewObjectId(),
		Version: sampleVersion.Id,
	}
	require.NoError(samplePatch.Insert())
	require.NoError(sampleVersion.Insert())
	sampleBuild := build.Build{
		Id:           "sample_build_id",
		BuildVariant: "race-detector",
		Version:      "sample_version",
	}
	require.NoError(sampleBuild.Insert())
	sampleTask := task.Task{
		Id:                    "sample_task",
		Version:               "sample_version",
		BuildId:               "sample_build_id",
		Project:               "mci",
		DisplayName:           "sample_task",
		GeneratedJSONAsString: sampleGeneratedProject,
		Status:                evergreen.TaskStarted,
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
	projectRef := model.ProjectRef{Id: "mci", Identifier: "mci_identifier"}
	require.NoError(projectRef.Insert())

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
	p, _, err := model.LoadProjectForVersion(v, "mci", true)
	assert.NoError(err)
	require.NotNil(p)
	assert.Len(p.Tasks, 2)
	require.Len(p.BuildVariants, 2)
	assert.Len(p.BuildVariants[0].Tasks, 1)
	assert.Len(p.BuildVariants[1].Tasks, 2)

	// Verify second project was changed
	v, err = model.VersionFindOneId("sample_version")
	assert.NoError(err)
	p, _, err = model.LoadProjectForVersion(v, "mci", true)
	assert.NoError(err)
	require.NotNil(p)
	assert.Len(p.Tasks, 4)
	require.Len(p.BuildVariants, 2)
	assert.Len(p.BuildVariants[0].Tasks, 1)
	assert.Len(p.BuildVariants[1].Tasks, 4)
	require.Len(p.TaskGroups, 1)
	assert.Len(p.TaskGroups[0].Tasks, 2)

	b, err := build.FindOneId("sample_build_id")
	assert.NoError(err)
	assert.Equal("mci_identifier_race_detector_display_my_display_task__01_01_01_00_00_00", b.Tasks[0].Id)
}

func TestParseProjects(t *testing.T) {
	assert := assert.New(t)
	parsed, err := parseProjectsAsString(sampleGeneratedProject)
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

func TestGenerateSkipsInvalidDependency(t *testing.T) {
	var sampleBaseProject = `
tasks:
  - name: generator
    commands:
      - command: generate.tasks
        params:
          files:
            - example/path/to/generate.json
  - name: patch_only_task
    patch_only: true
    commands:
    - command: shell.exec
  - name: new_task
    depends_on:
    - name: "*"
      variant: ubuntu1604
      status: "*"

buildvariants:
  - name: generator
    display_name: generator
    run_on:
      - ubuntu1604-test
    tasks:
      - name: "generator"
  - name: ubuntu1604
    display_name: Ubuntu 16.04
    run_on:
      - ubuntu1604-test
    tasks:
      - name: "patch_only_task"
`
	var sampleGeneratedProject2 = []string{`
{
  "buildvariants": [
    {
      "name": "ubuntu1604",
      "tasks": [
        {
          "name": "new_task"
        }
      ]
    }
  ]
}`}
	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(model.VersionCollection, build.Collection, task.Collection, distro.Collection,
		patch.Collection, model.ParserProjectCollection, model.ProjectRefCollection))
	sampleVersion := model.Version{
		Id:         "sample_version",
		Identifier: "mci",
		Config:     sampleBaseProject,
		Requester:  evergreen.RepotrackerVersionRequester,
		BuildIds:   []string{"sample_build_id"},
	}
	require.NoError(sampleVersion.Insert())
	sampleBuild := build.Build{
		Id:           "sample_build_id",
		BuildVariant: "race-detector",
		Version:      "sample_version",
		Requester:    evergreen.RepotrackerVersionRequester,
	}
	require.NoError(sampleBuild.Insert())
	sampleTask := task.Task{
		Id:                    "generator",
		Version:               sampleVersion.Id,
		BuildId:               "sample_build_id",
		Project:               "mci",
		DisplayName:           "generator",
		GeneratedJSONAsString: sampleGeneratedProject2,
		Status:                evergreen.TaskStarted,
		Requester:             evergreen.RepotrackerVersionRequester,
	}
	sampleDistros := []distro.Distro{
		distro.Distro{
			Id: "ubuntu1604-test",
		},
	}
	for _, d := range sampleDistros {
		require.NoError(d.Insert())
	}
	require.NoError(sampleTask.Insert())
	projectRef := model.ProjectRef{Id: "mci"}
	require.NoError(projectRef.Insert())

	j := NewGenerateTasksJob("generator", "1")
	j.Run(context.Background())
	assert.NoError(j.Error())

	tasks, err := task.Find(task.ByVersion(sampleVersion.Id))
	assert.NoError(err)
	foundGeneratedtask := false
	for _, dbTask := range tasks {
		if dbTask.DisplayName == "new_task" {
			foundGeneratedtask = true
			// the patch_only task isn't a dependency
			assert.Len(dbTask.DependsOn, 0)
		}
	}
	assert.True(foundGeneratedtask)
}

func TestMarkGeneratedTasksError(t *testing.T) {
	require.NoError(t, db.ClearCollections(model.ProjectRefCollection, model.VersionCollection, build.Collection, task.Collection, distro.Collection, patch.Collection, model.ParserProjectCollection))
	sampleTask := task.Task{
		Id:                    "sample_task",
		Version:               "sample_version",
		BuildId:               "sample_build_id",
		Project:               "mci",
		DisplayName:           "sample_task",
		GeneratedJSONAsString: sampleGeneratedProject,
		Status:                evergreen.TaskStarted,
	}
	require.NoError(t, sampleTask.Insert())

	j := NewGenerateTasksJob(sampleTask.Id, "1")
	j.Run(context.Background())
	assert.Error(t, j.Error())
	dbTask, err := task.FindOneId(sampleTask.Id)
	assert.NoError(t, err)
	assert.Equal(t, "unable to find version sample_version", dbTask.GenerateTasksError)
	assert.False(t, dbTask.GeneratedTasks)
}
