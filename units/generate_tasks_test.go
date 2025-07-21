package units

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
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
    },
    {
      "name": "task-group-task1"
    },
    {
      "name": "task-group-task2"
    }
  ],
  "task_groups": [
      {
          "name": "my_task_group",
          "max_hosts": 1,
          "tasks": [
            "task-group-task1",
            "task-group-task2",
          ]
      },
  ]
}
`}

var dependOnGeneratedTasksConfig = `
buildvariants:
  - display_name: ! Enterprise Windows
    name: testBV1
    run_on:
      - ubuntu1604-test
    tasks:
      - name: placeholder
    depends_on:
      - name: version_gen
        variant: generate-tasks-for-version

  - display_name: Generate Tasks for Version
    name: generate-tasks-for-version
    run_on:
      - ubuntu1604-test
    tasks:
      - name: version_gen

  - name: testBV2
    display_name: "~ Shared Library Enterprise RHEL 8.0 v4 Toolchain Clang C++20 DEBUG"
    run_on:
      - ubuntu1604-test
    tasks:
      - name: dependencyTask

tasks:
  - name: placeholder
    depends_on:
      - name: version_gen
        variant: generate-tasks-for-version
    commands:
      - command: shell.exec
        params:
          working_dir: src
          script: |
            echo "noop2"
  - name: dependencyTask
    commands:
      - command: shell.exec
        params:
          script: |
            echo "noop2"

  - name: version_gen
    commands:
      - command: generate.tasks
        params:
          files:
            - src/evergreen.json
`

var omitGeneratedTasksConfig = `
buildvariants:
  - display_name: ! Enterprise Windows
    name: testBV1
    run_on:
      - ubuntu1604-test
    tasks:
      - name: placeholder
    depends_on:
      - name: version_gen
        variant: generate-tasks-for-version
        omit_generated_tasks: true

  - display_name: Generate Tasks for Version
    name: generate-tasks-for-version
    run_on:
      - ubuntu1604-test
    tasks:
      - name: version_gen

  - name: testBV2
    display_name: "~ Shared Library Enterprise RHEL 8.0 v4 Toolchain Clang C++20 DEBUG"
    run_on:
      - ubuntu1604-test
    tasks:
      - name: dependencyTask

tasks:
  - name: placeholder
    depends_on:
      - name: version_gen
        variant: generate-tasks-for-version
    commands:
      - command: shell.exec
        params:
          working_dir: src
          script: |
            echo "noop2"
  - name: dependencyTask
    commands:
      - command: shell.exec
        params:
          script: |
            echo "noop2"

  - name: version_gen
    commands:
      - command: generate.tasks
        params:
          files:
            - src/evergreen.json
`

var sampleGeneratedProject2 = []string{`
{
  "buildvariants": [
    {
      "name": "testBV1",
      "tasks": [
        {
          "name": "shouldDependOnVersionGen",
          "activate": false
        }
      ],
    },
    {
      "name": "testBV2",
      "tasks": [
        {
          "name": "shouldDependOnDependencyTask",
          "activate": false
        }
      ],
    }
  ],
  "tasks":  [
    {
      "name": "shouldDependOnVersionGen",
      "commands": [
        {
          "command": "shell.exec",
          "params":
          {
            "working_dir": "src",
            "script": "echo noop"
          }
        }
      ]
    },
    {
      "name": "shouldDependOnDependencyTask",
      "commands": [
        {
          "command": "shell.exec",
          "params":
          {
            "working_dir": "src",
            "script": "echo noop"
          }
        }
      ],
      "depends_on": [
        {
          "name": "dependencyTask"
        }
      ]
    }
  ]
}
`}

var shouldGenerateNewBVConfig = `
buildvariants:
  - display_name: Generate Tasks for Version
    name: generate-tasks-for-version
    run_on:
      - ubuntu1604-test
    tasks:
      - name: version_gen

  - name: testBV3
    display_name: TestBV3
    run_on:
      - ubuntu1604-test
    tasks:
      - name: placeholder
    depends_on:
      - name: version_gen
        variant: generate-tasks-for-version
      - name: dependencyTask
        variant: testBV4

  - name: testBV4
    display_name: TestBV4
    run_on:
      - ubuntu1604-test
    tasks:
      - name: dependencyTask
      - name: dependencyTaskShouldActivate
      - name: shouldNotActivate
      - name: depOfShouldNotActivate
      - name: shouldActivate

  - name: testBV5
    display_name: TestBV5
    run_on:
      - ubuntu1604-test
    tasks:
      - name: placeholder
    depends_on:
      - name: dependencyTaskShouldActivate
        variant: testBV4

tasks:
  - name: placeholder
    depends_on:
      - name: version_gen
        variant: generate-tasks-for-version
    commands:
      - command: shell.exec
        params:
          working_dir: src
          script: |
            echo "noop2"

  - name: dependencyTask
    depends_on:
      - name: shouldNotActivate
      - name: shouldActivate
    commands:
      - command: shell.exec
        params:
          script: |
            echo "noop"

  - name: dependencyTaskShouldActivate
    commands:
      - command: shell.exec
        params:
          script: |
            echo "noop"
    depends_on:
      - name: shouldActivate

  - name: shouldNotActivate
    depends_on:
      - name: depOfShouldNotActivate
    commands:
      - command: shell.exec
        params:
          script: |
            echo "noop2"

  - name: depOfShouldNotActivate
    commands:
      - command: shell.exec
        params:
          script: |
            echo "noop2"

  - name: shouldActivate
    commands:
      - command: shell.exec
        params:
          script: |
            echo "noop2"

  - name: version_gen
    commands:
      - command: generate.tasks
        params:
          files:
            - src/evergreen.json
`

var sampleGeneratedProject3 = []string{`
{
  "buildvariants": [
    {
      "name": "testBV3",
      "tasks": [
        {
          "name": "shouldDependOnDependencyTask",
          "activate": false
        }
      ],
    },
    {
      "name": "testBV5",
      "tasks": [
        {
          "name": "shouldDependOnDependencyTask"
        }
      ]
    }
  ],
  "tasks":  [
    {
      "name": "shouldDependOnDependencyTask",
      "commands": [
        {
          "command": "shell.exec",
          "params":
          {
            "working_dir": "src",
            "script": "echo noop"
          }
        }
      ],
      "depends_on": [
        {
          "name": "dependencyTask"
        }
      ]
    }
  ]
}
`}

func TestGenerateTasksWithDifferentGeneratedJSONStorageMethods(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	testutil.ConfigureIntegrationTest(t, env.Settings())

	c := utility.GetHTTPClient()
	defer utility.PutHTTPClient(c)

	ppConf := env.Settings().Providers.AWS.ParserProject
	bucket, err := pail.NewS3BucketWithHTTPClient(ctx, c, pail.S3Options{
		Name:   ppConf.Bucket,
		Region: evergreen.DefaultEC2Region,
	})
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, bucket.RemovePrefix(ctx, ppConf.Prefix))
	}()

	for methodName, storageMethod := range map[string]evergreen.ParserProjectStorageMethod{
		"DB": evergreen.ProjectStorageMethodDB,
		"S3": evergreen.ProjectStorageMethodS3,
	} {
		t.Run(methodName, func(t *testing.T) {
			assert := assert.New(t)
			require := require.New(t)

			require.NoError(db.ClearCollections(model.ProjectRefCollection, model.VersionCollection, build.Collection, task.Collection, distro.Collection, patch.Collection, model.ParserProjectCollection))
			defer require.NoError(db.ClearCollections(model.ProjectRefCollection, model.VersionCollection, build.Collection, task.Collection, distro.Collection, patch.Collection, model.ParserProjectCollection))

			randomVersion := model.Version{
				Id:         "random_version",
				Identifier: "mci",
				BuildIds:   []string{"sample_build_id"},
			}
			require.NoError(randomVersion.Insert(t.Context()))
			randomPatch := patch.Patch{
				Id:      mgobson.NewObjectId(),
				Version: randomVersion.Id,
			}
			require.NoError(randomPatch.Insert(t.Context()))
			sampleVersion := model.Version{
				Id:         "sample_version",
				Identifier: "mci",
				BuildIds:   []string{"sample_build_id"},
			}
			samplePatch := patch.Patch{
				Id:      mgobson.NewObjectId(),
				Version: sampleVersion.Id,
			}
			require.NoError(samplePatch.Insert(t.Context()))
			require.NoError(sampleVersion.Insert(t.Context()))
			sampleBuild := build.Build{
				Id:           "sample_build_id",
				BuildVariant: "race-detector",
				Version:      "sample_version",
			}
			require.NoError(sampleBuild.Insert(t.Context()))

			pp := model.ParserProject{}
			err := util.UnmarshalYAMLWithFallback([]byte(sampleBaseProject), &pp)
			require.NoError(err)
			pp.Id = "sample_version"
			require.NoError(pp.Insert(t.Context()))
			pp.Id = "random_version"
			require.NoError(pp.Insert(t.Context()))

			sampleTask := task.Task{
				Id:          "sample_task",
				Version:     "sample_version",
				BuildId:     "sample_build_id",
				Project:     "mci",
				DisplayName: "sample_task",
				Status:      evergreen.TaskStarted,
			}
			require.NoError(sampleTask.Insert(t.Context()))

			require.NoError(task.GeneratedJSONInsert(ctx, env.Settings(), &sampleTask, sampleGeneratedProject, storageMethod))

			sampleDistros := []distro.Distro{
				{
					Id: "ubuntu1604-test",
				},
				{
					Id: "archlinux-test",
				},
			}
			for _, d := range sampleDistros {
				require.NoError(d.Insert(ctx))
			}
			projectRef := model.ProjectRef{Id: "mci", Identifier: "mci_identifier"}
			require.NoError(projectRef.Insert(t.Context()))

			j := NewGenerateTasksJob(env, sampleTask.Version, sampleTask.Id, "1")
			j.Run(ctx)
			assert.NoError(j.Error())
			tasks, err := task.FindAll(ctx, db.Query(task.ByVersion("sample_version")))
			assert.NoError(err)
			assert.Len(tasks, 4)
			allTasks := map[string]bool{
				"sample_task":     false,
				"lint-command":    false,
				"lint-rest-route": false,
				"my_display_task": false,
			}
			for _, t := range tasks {
				assert.Equal("sample_version", t.Version)
				assert.Equal("mci", t.Project)
				allTasks[t.DisplayName] = true
				if t.Version == "my_display_task" {
					assert.Len(t.ExecutionTasks, 1)
				}
			}
			for _, v := range allTasks {
				assert.True(v)
			}

			// Make sure first project was not changed
			v, err := model.VersionFindOneId(ctx, "random_version")
			assert.NoError(err)
			p, _, err := model.FindAndTranslateProjectForVersion(ctx, env.Settings(), v, false)
			assert.NoError(err)
			require.NotNil(p)
			assert.Len(p.Tasks, 2)
			require.Len(p.BuildVariants, 2)
			assert.Len(p.BuildVariants[0].Tasks, 1)
			assert.Len(p.BuildVariants[1].Tasks, 2)

			// Verify second project was changed
			v, err = model.VersionFindOneId(ctx, "sample_version")
			assert.NoError(err)
			p, _, err = model.FindAndTranslateProjectForVersion(ctx, env.Settings(), v, false)
			assert.NoError(err)
			require.NotNil(p)
			assert.Len(p.Tasks, 6)
			require.Len(p.BuildVariants, 2)
			assert.Len(p.BuildVariants[0].Tasks, 1)
			assert.Len(p.BuildVariants[1].Tasks, 4)
			require.Len(p.TaskGroups, 1)
			assert.Len(p.TaskGroups[0].Tasks, 2)

			b, err := build.FindOneId(t.Context(), "sample_build_id")
			assert.NoError(err)
			assert.Equal("mci_identifier_race_detector_display_my_display_task__01_01_01_00_00_00", b.Tasks[0].Id)
		})
	}
}

func TestGeneratedTasksAreNotDependencies(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(model.ProjectRefCollection, model.VersionCollection, build.Collection, task.Collection, distro.Collection, patch.Collection, model.ParserProjectCollection))
	defer require.NoError(db.ClearCollections(model.ProjectRefCollection, model.VersionCollection, build.Collection, task.Collection, distro.Collection, patch.Collection, model.ParserProjectCollection))
	v := model.Version{
		Id:         "sample_version",
		Identifier: "mci",
		BuildVariants: []model.VersionBuildStatus{{
			BuildVariant: "generate-tasks-for-version",
			BuildId:      "b1",
		}, {
			BuildVariant: "testBV1",
			BuildId:      "b2",
		}, {
			BuildVariant: "testBV2",
			BuildId:      "b3",
		}},
	}
	require.NoError(v.Insert(t.Context()))
	b1 := build.Build{
		Id:           "b1",
		BuildVariant: "generate-tasks-for-version",
		Version:      "sample_version",
		Activated:    true,
	}
	b2 := build.Build{
		Id:           "b2",
		BuildVariant: "testBV1",
		Version:      "sample_version",
		Activated:    true,
	}
	b3 := build.Build{
		Id:           "b3",
		BuildVariant: "testBV2",
		Version:      "sample_version",
		Activated:    true,
	}
	require.NoError(b1.Insert(t.Context()))
	require.NoError(b2.Insert(t.Context()))
	require.NoError(b3.Insert(t.Context()))

	pp := model.ParserProject{}
	err := util.UnmarshalYAMLWithFallback([]byte(omitGeneratedTasksConfig), &pp)
	require.NoError(err)
	pp.Id = "sample_version"
	require.NoError(pp.Insert(t.Context()))
	generateTask := task.Task{
		Id:                    "mci_identifier_generate_tasks_for_version_version_gen__01_01_01_00_00_00",
		Version:               "sample_version",
		BuildId:               "b1",
		Project:               "mci",
		DisplayName:           "version_gen",
		BuildVariant:          "generate-tasks-for-version",
		GeneratedJSONAsString: sampleGeneratedProject2,
		Status:                evergreen.TaskStarted,
	}
	require.NoError(generateTask.Insert(t.Context()))
	projectRef := model.ProjectRef{Id: "mci", Identifier: "mci_identifier"}
	require.NoError(projectRef.Insert(t.Context()))

	j := NewGenerateTasksJob(env, generateTask.Version, generateTask.Id, "1")
	j.Run(ctx)
	assert.NoError(j.Error())
	tasks, err := task.FindAll(ctx, db.Query(task.ByVersion("sample_version")))
	assert.NoError(err)
	assert.Len(tasks, 4)
	for _, foundTask := range tasks {
		switch foundTask.DisplayName {
		case "version_gen", "dependency_task":
			assert.Equal([]task.Dependency{}, foundTask.DependsOn)
		case "shouldDependOnVersionGen":
			assert.Equal([]task.Dependency{
				{
					TaskId:             "mci_identifier_generate_tasks_for_version_version_gen__01_01_01_00_00_00",
					Status:             evergreen.TaskSucceeded,
					OmitGeneratedTasks: true,
				},
			}, foundTask.DependsOn)
		case "shouldDependOnDependencyTask":
			assert.Equal([]task.Dependency{{TaskId: "mci_identifier_testBV2_dependencyTask__01_01_01_00_00_00", Status: evergreen.TaskSucceeded}}, foundTask.DependsOn)
		}
	}
	require.NoError(db.ClearCollections(task.Collection, model.ParserProjectCollection))

	// check that the generated tasks are included as dependencies by default
	pp = model.ParserProject{}
	err = util.UnmarshalYAMLWithFallback([]byte(dependOnGeneratedTasksConfig), &pp)
	require.NoError(err)
	pp.Id = "sample_version"
	require.NoError(pp.Insert(t.Context()))
	generateTaskWithoutFlag := task.Task{
		Id:                    "mci_identifier_generate_tasks_for_version_version_gen__01_01_01_00_00_00",
		Version:               "sample_version",
		BuildId:               "b1",
		Project:               "mci",
		DisplayName:           "version_gen",
		GeneratedJSONAsString: sampleGeneratedProject2,
		Status:                evergreen.TaskStarted,
	}
	require.NoError(generateTaskWithoutFlag.Insert(t.Context()))
	j = NewGenerateTasksJob(env, generateTask.Version, generateTask.Id, "1")
	j.Run(ctx)
	assert.NoError(j.Error())
	tasks, err = task.FindAll(ctx, db.Query(task.ByVersion("sample_version")))
	assert.NoError(err)
	assert.Len(tasks, 4)
	for _, foundTask := range tasks {
		switch foundTask.DisplayName {
		case "version_gen", "dependency_task":
			assert.Equal([]task.Dependency{}, foundTask.DependsOn)
		case "shouldDependOnVersionGen":
			assert.Equal([]task.Dependency{
				{TaskId: "mci_identifier_generate_tasks_for_version_version_gen__01_01_01_00_00_00", Status: evergreen.TaskSucceeded},
				{TaskId: "mci_identifier_testBV2_dependencyTask__01_01_01_00_00_00", Status: evergreen.TaskSucceeded},
			}, foundTask.DependsOn)
		case "shouldDependOnDependencyTask":
			assert.Equal([]task.Dependency{{TaskId: "mci_identifier_testBV2_dependencyTask__01_01_01_00_00_00", Status: evergreen.TaskSucceeded}}, foundTask.DependsOn)
		}
	}

	require.NoError(db.ClearCollections(task.Collection, model.ParserProjectCollection))

	// check that the generated tasks are included as dependencies by default
	pp = model.ParserProject{}
	err = util.UnmarshalYAMLWithFallback([]byte(shouldGenerateNewBVConfig), &pp)
	require.NoError(err)
	pp.Id = "sample_version"
	require.NoError(pp.Insert(t.Context()))
	generateTask = task.Task{
		Id:                    "mci_identifier_generate_tasks_for_version_version_gen__01_01_01_00_00_00",
		Version:               "sample_version",
		BuildId:               "b1",
		Project:               "mci",
		DisplayName:           "version_gen",
		GeneratedJSONAsString: sampleGeneratedProject3,
		Status:                evergreen.TaskStarted,
	}
	require.NoError(generateTask.Insert(t.Context()))
	j = NewGenerateTasksJob(env, generateTask.Version, generateTask.Id, "1")
	j.Run(ctx)
	assert.NoError(j.Error())
	tasks, err = task.FindAll(ctx, db.Query(task.ByVersion("sample_version")))
	assert.NoError(err)
	assert.Len(tasks, 8)
	// shouldActivate should be activated because although the inactive dependencyTask has it as a
	// dependency, dependencyTaskShouldActivate also has shouldActivate as a dependency, so it should
	// be overridden as active during the handle function's recursion.
	for _, foundTask := range tasks {
		if foundTask.BuildVariant == "testBV4" {
			if foundTask.DisplayName == "dependencyTask" || foundTask.DisplayName == "shouldNotActivate" || foundTask.DisplayName == "depOfShouldNotActivate" {
				assert.False(foundTask.Activated)
			} else {
				assert.True(foundTask.Activated)
			}
		}
	}
}

func TestParseProjects(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	assert := assert.New(t)
	parsed, err := parseProjectsAsString(ctx, sampleGeneratedProject)
	assert.NoError(err)
	assert.Len(parsed, 1)
	assert.Len(parsed[0].BuildVariants, 1)
	assert.Equal("race-detector", parsed[0].BuildVariants[0].Name)
	assert.Equal("my_display_task", parsed[0].BuildVariants[0].DisplayTasks[0].Name)
	assert.Equal("lint-command", parsed[0].BuildVariants[0].DisplayTasks[0].ExecutionTasks[0])
	assert.Equal("lint-rest-route", parsed[0].BuildVariants[0].DisplayTasks[0].ExecutionTasks[1])
	assert.Len(parsed[0].BuildVariants[0].DisplayTasks, 1)
	assert.Len(parsed[0].Tasks, 4)
	assert.Equal("lint-command", parsed[0].Tasks[0].Name)
	assert.Equal("lint-rest-route", parsed[0].Tasks[1].Name)
}

func TestGenerateSkipsInvalidDependency(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

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
      params:
        shell: bash
        script: |
          echo "how you doing"
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
		Requester:  evergreen.RepotrackerVersionRequester,
		BuildIds:   []string{"sample_build_id"},
	}
	require.NoError(sampleVersion.Insert(t.Context()))
	sampleBuild := build.Build{
		Id:           "sample_build_id",
		BuildVariant: "race-detector",
		Version:      "sample_version",
		Requester:    evergreen.RepotrackerVersionRequester,
	}
	require.NoError(sampleBuild.Insert(t.Context()))
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
		{
			Id: "ubuntu1604-test",
		},
	}
	sampleParserProject := model.ParserProject{}
	err := util.UnmarshalYAMLWithFallback([]byte(sampleBaseProject), &sampleParserProject)
	require.NoError(err)
	sampleParserProject.Id = "sample_version"
	require.NoError(sampleParserProject.Insert(t.Context()))

	for _, d := range sampleDistros {
		require.NoError(d.Insert(ctx))
	}
	require.NoError(sampleTask.Insert(t.Context()))
	projectRef := model.ProjectRef{Id: "mci"}
	require.NoError(projectRef.Insert(t.Context()))

	j := NewGenerateTasksJob(env, sampleTask.Version, sampleTask.Id, "1")
	j.Run(ctx)
	assert.NoError(j.Error())

	tasks, err := task.Find(ctx, task.ByVersion(sampleVersion.Id))
	assert.NoError(err)
	foundGeneratedtask := false
	for _, dbTask := range tasks {
		if dbTask.DisplayName == "new_task" {
			foundGeneratedtask = true
			// the patch_only task isn't a dependency
			assert.Empty(dbTask.DependsOn)
		}
	}
	assert.True(foundGeneratedtask)
}

func TestMarkGeneratedTasksError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

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
	require.NoError(t, sampleTask.Insert(t.Context()))

	j := NewGenerateTasksJob(env, sampleTask.Version, sampleTask.Id, "1")
	j.Run(ctx)
	assert.Error(t, j.Error())
	dbTask, err := task.FindOneId(ctx, sampleTask.Id)
	assert.NoError(t, err)
	require.NotZero(t, dbTask)
	assert.Equal(t, "version 'sample_version' not found", dbTask.GenerateTasksError)
	assert.False(t, dbTask.GeneratedTasks)
}
