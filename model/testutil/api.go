package testutil

import (
	"io/ioutil"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
	yaml "gopkg.in/yaml.v2"
)

type TestModelData struct {
	Task       *task.Task
	Build      *build.Build
	Host       *host.Host
	TaskConfig *model.TaskConfig
}

func CleanupAPITestData() error {
	// Ignore errs here because the ns might just not exist.
	testCollections := []string{
		task.Collection, build.Collection, host.Collection,
		distro.Collection, version.Collection, patch.Collection,
		model.PushlogCollection, model.ProjectVarsCollection, model.TaskQueuesCollection,
		manifest.Collection, model.ProjectRefCollection}

	if err := db.ClearCollections(testCollections...); err != nil {
		return errors.Wrap(err, "Failed to clear test data collection")
	}

	return nil
}

func SetupAPITestData(testConfig *evergreen.Settings, taskDisplayName string, variant string, projectFile string, patchMode PatchTestMode) (*TestModelData, error) {
	if err := CleanupAPITestData(); err != nil {
		return nil, errors.WithStack(err)
	}

	// Read in the project configuration
	projectConfig, err := ioutil.ReadFile(projectFile)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read project config")
	}

	modelData := &TestModelData{}

	// Unmarshall the project configuration into a struct
	project := &model.Project{}
	if err := model.LoadProjectInto(projectConfig, "test", project); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal project config")
	}

	// create a build variant for this project
	bv := model.BuildVariant{
		Name: variant,
		Tasks: []model.BuildVariantTaskUnit{{
			Name: taskDisplayName,
		}},
	}

	project.BuildVariants = append(project.BuildVariants, bv)
	// Marshall the project YAML for storage
	projectYamlBytes, err := yaml.Marshal(project)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal project config")
	}

	// Create the ref for the project
	projectRef := &model.ProjectRef{
		Identifier:  project.DisplayName,
		Owner:       project.Owner,
		Repo:        project.Repo,
		RepoKind:    project.RepoKind,
		Branch:      project.Branch,
		Enabled:     project.Enabled,
		BatchTime:   project.BatchTime,
		LocalConfig: string(projectConfig),
	}
	if err := projectRef.Insert(); err != nil {
		return nil, errors.Wrap(err, "failed to insert projectRef")
	}

	// Save the project variables
	projectVars := &model.ProjectVars{
		Id: project.DisplayName,
		Vars: map[string]string{
			"aws_key":    testConfig.Providers.AWS.Id,
			"aws_secret": testConfig.Providers.AWS.Secret,
			"fetch_key":  "fetch_expansion_value",
		},
	}
	if _, err := projectVars.Upsert(); err != nil {
		return nil, errors.Wrap(err, "problem inserting project variables")
	}

	// Create and insert two tasks
	taskOne := &task.Task{
		Id:           "testTaskId",
		BuildId:      "testBuildId",
		DistroId:     "test-distro-one",
		BuildVariant: variant,
		Project:      projectRef.Identifier,
		DisplayName:  taskDisplayName,
		HostId:       "testHost",
		Secret:       "testTaskSecret",
		Version:      "testVersionId",
		Status:       evergreen.TaskDispatched,
		Requester:    evergreen.RepotrackerVersionRequester,
	}
	if patchMode != NoPatch {
		taskOne.Requester = evergreen.PatchVersionRequester
	}
	if err := taskOne.Insert(); err != nil {
		return nil, errors.Wrap(err, "failed to insert taskOne")
	}
	modelData.Task = taskOne

	taskTwo := &task.Task{
		Id:           "testTaskIdTwo",
		BuildId:      "testBuildId",
		DistroId:     "test-distro-one",
		BuildVariant: variant,
		Project:      project.DisplayName,
		DisplayName:  taskDisplayName,
		HostId:       "",
		Secret:       "testTaskSecret",
		Version:      "testVersionId",
		Status:       evergreen.TaskUndispatched,
		Requester:    evergreen.RepotrackerVersionRequester,
		Activated:    true,
	}
	if err := taskTwo.Insert(); err != nil {
		return nil, errors.Wrap(err, "failed to insert taskTwo")
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
	if err := taskQueue.Save(); err != nil {
		return nil, errors.Wrap(err, "failed to insert task queue")
	}

	// Insert the version document
	v := &version.Version{
		Id:       taskOne.Version,
		BuildIds: []string{taskOne.BuildId},
		Config:   string(projectYamlBytes),
	}
	if err := v.Insert(); err != nil {
		return nil, errors.Wrap(err, "failed to insert version: ")
	}

	// Insert the build that contains the tasks
	build := &build.Build{
		Id: "testBuildId",
		Tasks: []build.TaskCache{
			build.NewTaskCache(taskOne.Id, taskOne.DisplayName, true),
			build.NewTaskCache(taskTwo.Id, taskTwo.DisplayName, true),
		},
		Version: v.Id,
	}
	if err := build.Insert(); err != nil {
		return nil, errors.Wrap(err, "failed to insert build")
	}
	modelData.Build = build

	workDir, err := ioutil.TempDir("", "agent_test_")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create working directory")
	}

	// Insert the host info for running the tests
	testHost := &host.Host{
		Id:   "testHost",
		Host: "testHost",
		Distro: distro.Distro{
			Id:         "test-distro-one",
			WorkDir:    workDir,
			Expansions: []distro.Expansion{{"distro_exp", "DISTRO_EXP"}},
		},
		Provider:      evergreen.HostTypeStatic,
		RunningTask:   taskOne.Id,
		Secret:        "testHostSecret",
		StartedBy:     evergreen.User,
		AgentRevision: evergreen.BuildRevision,
		Status:        evergreen.HostRunning,
	}
	if err := testHost.Insert(); err != nil {
		return nil, errors.Wrap(err, "failed to insert host")
	}
	modelData.Host = testHost

	session, _, err := db.GetGlobalSessionFactory().GetSession()
	if err != nil {
		return nil, errors.Wrap(err, "couldn't get db session!")
	}

	config, err := model.NewTaskConfig(&testHost.Distro, v, project,
		taskOne, projectRef)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create task config")
	}
	modelData.TaskConfig = config

	// Remove any logs for our test task from previous runs.
	_, err = session.DB(model.TaskLogDB).C(model.TaskLogCollection).
		RemoveAll(bson.M{"t_id": bson.M{"$in": []string{taskOne.Id, taskTwo.Id}}})
	if err != nil {
		return nil, errors.Wrap(err, "failed to remove logs")
	}

	return modelData, nil
}
