package testutil

import (
	"context"
	"os"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/parameterstore/fakeparameter"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/taskoutput"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

type TestModelData struct {
	Task       *task.Task
	Build      *build.Build
	Host       *host.Host
	Project    *model.Project
	ProjectRef *model.ProjectRef
}

func CleanupAPITestData() error {
	// Ignore errs here because the ns might just not exist.
	testCollections := []string{
		task.Collection, build.Collection, host.Collection,
		distro.Collection, model.VersionCollection, patch.Collection,
		model.PushlogCollection, model.ProjectVarsCollection, fakeparameter.Collection, model.TaskQueuesCollection,
		manifest.Collection, model.ProjectRefCollection, model.ParserProjectCollection}

	if err := db.ClearCollections(testCollections...); err != nil {
		return errors.Wrap(err, "clearing test data collection")
	}

	return nil
}

func SetupAPITestData(testConfig *evergreen.Settings, taskDisplayName string, variant string, projectFile string, patchMode PatchTestMode) (*TestModelData, error) {
	if err := CleanupAPITestData(); err != nil {
		return nil, errors.WithStack(err)
	}

	modelData := &TestModelData{}

	projectConfig, err := os.ReadFile(projectFile)
	if err != nil {
		return nil, errors.Wrap(err, "reading project config")
	}

	// Unmarshal the project configuration into a struct
	project := &model.Project{}
	ctx := context.Background()
	opts := &model.GetProjectOpts{
		Ref:          modelData.ProjectRef,
		ReadFileFrom: model.ReadFromLocal,
	}
	pp, err := model.LoadProjectInto(ctx, projectConfig, opts, "", project)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshalling project config")
	}
	modelData.Project = project

	// create a build variant for this project
	bv := model.BuildVariant{
		Name: variant,
		Tasks: []model.BuildVariantTaskUnit{{
			Name:    taskDisplayName,
			Variant: variant,
		}},
	}
	project.BuildVariants = append(project.BuildVariants, bv)

	// Marshal the parser project YAML for storage
	pp.AddBuildVariant(variant, "", "", nil, []string{taskDisplayName})

	// Create the ref for the project
	projectRef := &model.ProjectRef{
		Id:                    project.DisplayName,
		Owner:                 "evergreen-ci",
		Repo:                  "sample",
		Branch:                "main",
		Enabled:               true,
		ParameterStoreEnabled: true,
	}
	if err = projectRef.Insert(); err != nil {
		return nil, errors.Wrap(err, "inserting project ref")
	}
	modelData.ProjectRef = projectRef

	version := &model.Version{
		Id:         "sample_version",
		Identifier: project.DisplayName,
		Requester:  evergreen.RepotrackerVersionRequester,
	}
	if err = version.Insert(); err != nil {
		return nil, errors.Wrap(err, "inserting version")
	}

	versionParserProject := &model.ParserProject{}
	if err = util.UnmarshalYAMLWithFallback(projectConfig, &versionParserProject); err != nil {
		return nil, errors.Wrap(err, "unmarshalling parser project from YAML")
	}
	versionParserProject.Id = "sample_version"
	if err = versionParserProject.Insert(); err != nil {
		return nil, errors.Wrap(err, "inserting parser project")
	}

	// Save the project variables
	if len(testConfig.Providers.AWS.EC2Keys) == 0 {
		return nil, errors.New("no EC2 Keys in test config")
	}
	projectVars := &model.ProjectVars{
		Id: project.DisplayName,
		Vars: map[string]string{
			"aws_key":    testConfig.Providers.AWS.EC2Keys[0].Key,
			"aws_secret": testConfig.Providers.AWS.EC2Keys[0].Secret,
			"fetch_key":  "fetch_expansion_value",
		},
	}
	if _, err = projectVars.Upsert(); err != nil {
		return nil, errors.Wrap(err, "inserting project variables")
	}

	// Create and insert two tasks
	taskOne := &task.Task{
		Id:             "testTaskId",
		BuildId:        "testBuildId",
		DistroId:       "test-distro-one",
		BuildVariant:   variant,
		Project:        projectRef.Id,
		DisplayName:    taskDisplayName,
		HostId:         "testHost",
		Secret:         "testTaskSecret",
		Version:        "testVersionId",
		Status:         evergreen.TaskDispatched,
		TaskOutputInfo: &taskoutput.TaskOutput{},
	}
	if patchMode == NoPatch {
		taskOne.Requester = evergreen.RepotrackerVersionRequester
	} else if patchMode == MergePatch {
		taskOne.Requester = evergreen.MergeTestRequester
	} else {
		taskOne.Requester = evergreen.PatchVersionRequester
	}
	if err = taskOne.Insert(); err != nil {
		return nil, errors.Wrap(err, "inserting taskOne")
	}
	modelData.Task = taskOne

	taskTwo := &task.Task{
		Id:             "testTaskIdTwo",
		BuildId:        "testBuildId",
		DistroId:       "test-distro-one",
		BuildVariant:   variant,
		Project:        project.DisplayName,
		DisplayName:    taskDisplayName,
		HostId:         "",
		Secret:         "testTaskSecret",
		Version:        "testVersionId",
		Status:         evergreen.TaskUndispatched,
		Requester:      evergreen.RepotrackerVersionRequester,
		Activated:      true,
		TaskOutputInfo: &taskoutput.TaskOutput{},
	}
	if err = taskTwo.Insert(); err != nil {
		return nil, errors.Wrap(err, "inserting taskTwo")
	}

	// Set up a task queue for task end tests
	taskQueue := &model.TaskQueue{
		Distro: "test-distro-one",
		Queue: []model.TaskQueueItem{
			{
				Id:          "testTaskIdTwo",
				DisplayName: taskDisplayName,
			},
		},
	}
	if err = taskQueue.Save(); err != nil {
		return nil, errors.Wrap(err, "inserting task queue")
	}

	// Insert the version document
	v := &model.Version{
		Id:       taskOne.Version,
		BuildIds: []string{taskOne.BuildId},
	}
	if err = v.Insert(); err != nil {
		return nil, errors.Wrap(err, "inserting version")
	}
	pp.Id = taskOne.Version
	if err = pp.Insert(); err != nil {
		return nil, errors.Wrap(err, "inserting parser project")
	}

	// Insert the build that contains the tasks
	build := &build.Build{
		Id:      "testBuildId",
		Version: v.Id,
	}
	if err = build.Insert(); err != nil {
		return nil, errors.Wrap(err, "inserting build")
	}
	modelData.Build = build

	workDir, err := os.MkdirTemp("", "agent_test_")
	if err != nil {
		return nil, errors.Wrap(err, "creating working directory")
	}

	// Insert the host info for running the tests
	testHost := &host.Host{
		Id:   "testHost",
		Host: "testHost",
		Distro: distro.Distro{
			Id:         "test-distro-one",
			WorkDir:    workDir,
			Expansions: []distro.Expansion{{Key: "distro_exp", Value: "DISTRO_EXP"}},
		},
		Provider:      evergreen.HostTypeStatic,
		RunningTask:   taskOne.Id,
		Secret:        "testHostSecret",
		StartedBy:     evergreen.User,
		AgentRevision: evergreen.AgentVersion,
		Status:        evergreen.HostRunning,
	}
	if err = testHost.Insert(ctx); err != nil {
		return nil, errors.Wrap(err, "inserting host")
	}
	modelData.Host = testHost

	return modelData, nil
}
