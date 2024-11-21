package data

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	cocoaMock "github.com/evergreen-ci/cocoa/mock"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/parameterstore/fakeparameter"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/githubapp"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch patch by project route

type ProjectConnectorGetSuite struct {
	setup    func() error
	teardown func() error
	suite.Suite
}

const (
	projectId      = "mci2"
	repoProjectId  = "repo_mci"
	username       = "me"
	projEventCount = 10
)

func getMockProjectSettings() model.ProjectSettings {
	return model.ProjectSettings{
		ProjectRef: model.ProjectRef{
			Owner:          "admin",
			Enabled:        true,
			Id:             projectId,
			Admins:         []string{},
			PeriodicBuilds: nil,
			WorkstationConfig: model.WorkstationConfig{
				SetupCommands: nil,
				GitClone:      nil,
			},
		},
		GitHubAppAuth:      githubapp.GithubAppAuth{},
		GithubHooksEnabled: true,
		Vars: model.ProjectVars{
			Id:          projectId,
			Vars:        map[string]string{"hello": "world", "world": "hello", "beep": "boop"},
			PrivateVars: map[string]bool{"world": true},
		},
		Aliases: []model.ProjectAlias{{
			ID:        mgobson.ObjectIdHex("5bedc72ee4055d31f0340b1d"),
			ProjectID: projectId,
			Alias:     "alias1",
			Variant:   "ubuntu",
			Task:      "subcommand",
		},
		},
		Subscriptions: []event.Subscription{{
			ID:           "subscription1",
			ResourceType: "project",
			Owner:        "admin",
			Subscriber: event.Subscriber{
				Type:   event.GithubPullRequestSubscriberType,
				Target: event.GithubPullRequestSubscriber{},
			},
		},
		},
	}
}

func TestProjectConnectorGetSuite(t *testing.T) {
	s := new(ProjectConnectorGetSuite)
	s.setup = func() error {
		s.Require().NoError(db.ClearCollections(model.ProjectRefCollection, model.ProjectVarsCollection, fakeparameter.Collection))

		projWithVars := &model.ProjectRef{
			Id:                    projectId,
			ParameterStoreEnabled: true,
		}
		projects := []*model.ProjectRef{
			{
				Id:                    "projectA",
				Enabled:               true,
				CommitQueue:           model.CommitQueueParams{Enabled: utility.TruePtr()},
				Owner:                 "evergreen-ci",
				Repo:                  "gimlet",
				Branch:                "main",
				ParameterStoreEnabled: true,
			},
			{
				Id:                    "projectB",
				Enabled:               true,
				CommitQueue:           model.CommitQueueParams{Enabled: utility.TruePtr()},
				Owner:                 "evergreen-ci",
				Repo:                  "evergreen",
				Branch:                "main",
				ParameterStoreEnabled: true,
			},
			{
				Id:                    "projectC",
				Enabled:               true,
				CommitQueue:           model.CommitQueueParams{Enabled: utility.TruePtr()},
				Owner:                 "mongodb",
				Repo:                  "mongo",
				Branch:                "main",
				ParameterStoreEnabled: true,
			},
			{
				Id:                    "projectD",
				ParameterStoreEnabled: true,
			},
			{
				Id:                    "projectE",
				ParameterStoreEnabled: true,
			},
			{
				Id:                    "projectF",
				ParameterStoreEnabled: true,
			},
			projWithVars,
		}

		for _, p := range projects {
			if err := p.Insert(); err != nil {
				return err
			}
			if _, err := model.GetNewRevisionOrderNumber(p.Id); err != nil {
				return err
			}
		}

		projVars := &model.ProjectVars{
			Id:          projectId,
			Vars:        map[string]string{"a": "1", "b": "3", "d": "4"},
			PrivateVars: map[string]bool{"b": true},
		}
		s.NoError(projVars.Insert())
		checkAndSetProjectVarsSynced(t, projWithVars, false)

		repoWithVars := &model.RepoRef{
			ProjectRef: model.ProjectRef{
				Id:                    repoProjectId,
				ParameterStoreEnabled: true,
			},
		}
		s.Require().NoError(repoWithVars.Upsert())
		repoVars := &model.ProjectVars{
			Id:          repoProjectId,
			Vars:        map[string]string{"a": "a_from_repo", "c": "new"},
			PrivateVars: map[string]bool{"a": true},
		}
		s.NoError(repoVars.Insert())
		checkAndSetProjectVarsSynced(t, &repoWithVars.ProjectRef, true)

		before := getMockProjectSettings()
		after := getMockProjectSettings()
		after.GitHubAppAuth = githubapp.GithubAppAuth{
			AppID:      12345,
			PrivateKey: []byte("secret"),
		}
		after.GithubHooksEnabled = false
		after.ProjectRef.WorkstationConfig.SetupCommands = []model.WorkstationSetupCommand{}
		after.Vars = model.ProjectVars{
			Id:          projectId,
			Vars:        map[string]string{"hello": "another_world", "world": "changed", "beep": "boop"},
			PrivateVars: map[string]bool{"world": true},
		}
		s.NotEmpty(before.Aliases[0].ID)
		s.NotEmpty(after.Aliases[0].ID)

		s.Require().NoError(db.ClearCollections(event.EventCollection))
		for i := 0; i < projEventCount; i++ {
			s.NoError(model.LogProjectModified(projectId, username, &before, &after))
		}

		return nil
	}

	s.teardown = func() error {
		return db.ClearCollections(model.ProjectRefCollection, model.ProjectVarsCollection, fakeparameter.Collection)
	}

	suite.Run(t, s)
}

func (s *ProjectConnectorGetSuite) SetupSuite() { s.Require().NoError(s.setup()) }

func (s *ProjectConnectorGetSuite) TearDownSuite() {
	s.Require().NoError(s.teardown())
}

func (s *ProjectConnectorGetSuite) TestGetProjectEvents() {
	events, err := GetProjectEventLog(projectId, time.Now(), 0)
	s.NoError(err)
	s.Equal(projEventCount, len(events))
	for _, eventLog := range events {
		s.Len(eventLog.Before.Aliases, 1)
		s.Len(eventLog.After.Aliases, 1)
		s.NotEmpty(eventLog.Before.Aliases[0].ID)
		s.NotEmpty(eventLog.After.Aliases[0].ID)
		s.Nil(eventLog.Before.ProjectRef.PeriodicBuilds)
		s.Nil(eventLog.Before.ProjectRef.WorkstationConfig.SetupCommands)
		s.NotNil(eventLog.After.ProjectRef.WorkstationConfig.SetupCommands)
		s.Len(eventLog.After.ProjectRef.WorkstationConfig.SetupCommands, 0)
		s.Equal(evergreen.RedactedBeforeValue, eventLog.Before.Vars.Vars["hello"])
		s.Equal(evergreen.RedactedAfterValue, eventLog.After.Vars.Vars["hello"])
		s.Equal(evergreen.RedactedBeforeValue, eventLog.Before.Vars.Vars["world"])
		s.Equal(evergreen.RedactedAfterValue, eventLog.After.Vars.Vars["world"])
		s.Empty(utility.FromStringPtr(eventLog.Before.GithubAppAuth.PrivateKey))
		s.Equal(evergreen.RedactedAfterValue, utility.FromStringPtr(eventLog.After.GithubAppAuth.PrivateKey))
	}

	// No error for empty events
	events, err = GetProjectEventLog("projectA", time.Now(), 0)
	s.NoError(err)
	s.Equal(0, len(events))
}

func (s *ProjectConnectorGetSuite) TestFindProjectVarsById() {
	// redact private variables
	res, err := FindProjectVarsById(projectId, "", true)
	s.NoError(err)
	s.Require().NotNil(res)
	s.Equal("1", res.Vars["a"])
	s.Equal("", res.Vars["b"])
	s.True(res.PrivateVars["b"])

	// not redacted
	res, err = FindProjectVarsById(projectId, "", false)
	s.NoError(err)
	s.Require().NotNil(res)
	s.Equal("1", res.Vars["a"])
	s.Equal("3", res.Vars["b"])
	s.Equal("", res.Vars["c"])

	// test with repo
	res, err = FindProjectVarsById(projectId, repoProjectId, true)
	s.NoError(err)
	s.Require().NotNil(res)
	s.Equal("1", res.Vars["a"])
	s.Equal("", res.Vars["b"])
	s.True(res.PrivateVars["b"])
	s.False(res.PrivateVars["a"])
	s.Equal("new", res.Vars["c"])

	res, err = FindProjectVarsById("", repoProjectId, true)
	s.NoError(err)
	s.Equal("", res.Vars["a"])
	s.Equal("new", res.Vars["c"])
	s.True(res.PrivateVars["a"])

	res, err = FindProjectVarsById("", repoProjectId, false)
	s.NoError(err)
	s.Equal("a_from_repo", res.Vars["a"])
	s.Equal("", res.Vars["b"])
	s.Equal("new", res.Vars["c"])
	s.True(res.PrivateVars["a"])

	_, err = FindProjectVarsById("non-existent", "also-non-existent", false)
	s.Error(err)
}

func checkParametersMatchVars(ctx context.Context, t *testing.T, pm model.ParameterMappings, vars map[string]string) {
	assert.Len(t, pm, len(vars), "each project var should have exactly one corresponding parameter")
	fakeParams, err := fakeparameter.FindByIDs(ctx, pm.ParameterNames()...)
	assert.NoError(t, err)
	assert.Len(t, fakeParams, len(vars), "number of parameters for project vars should match number of project vars defined")

	paramNamesMap := pm.ParameterNameMap()
	for _, fakeParam := range fakeParams {
		varName := paramNamesMap[fakeParam.Name].Name
		assert.NotEmpty(t, varName, "parameter should have corresponding project variable")
		assert.Equal(t, vars[varName], fakeParam.Value, "project variable value should match the parameter value")
	}
}

func checkAndSetProjectVarsSynced(t *testing.T, projRef *model.ProjectRef, isRepoRef bool) {
	var dbProjRef *model.ProjectRef
	var err error
	if isRepoRef {
		dbRepoRef, err := model.FindOneRepoRef(projRef.Id)
		require.NoError(t, err)
		require.NotZero(t, dbRepoRef)
		dbProjRef = &dbRepoRef.ProjectRef
	} else {
		dbProjRef, err = model.FindBranchProjectRef(projRef.Id)
		require.NoError(t, err)
		require.NotZero(t, dbProjRef)
	}
	assert.True(t, dbProjRef.ParameterStoreVarsSynced, "parameter store vars must be synced but they aren't")

	projRef.ParameterStoreVarsSynced = true
}

func checkParametersNamespacedByProject(t *testing.T, vars model.ProjectVars) {
	projectID := vars.Id
	commonAndProjectIDPrefix := fmt.Sprintf("/%s/%s/", strings.TrimSuffix(strings.TrimPrefix(evergreen.GetEnvironment().Settings().Providers.AWS.ParameterStore.Prefix, "/"), "/"), projectID)
	for _, pm := range vars.Parameters {
		assert.True(t, strings.HasPrefix(pm.ParameterName, commonAndProjectIDPrefix), "parameter name '%s' should have standard prefix '%s'", pm.ParameterName, commonAndProjectIDPrefix)
	}
}

func (s *ProjectConnectorGetSuite) TestUpdateProjectVars() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	//successful update
	varsToDelete := []string{"a"}
	newVars := restModel.APIProjectVars{
		Vars:         map[string]string{"b": "2", "c": "3", "d": ""},
		PrivateVars:  map[string]bool{"b": false, "c": true},
		VarsToDelete: varsToDelete,
	}
	s.NoError(UpdateProjectVars(projectId, &newVars, false))

	s.Empty(newVars.Vars["b"]) // can't unredact previously redacted variables
	s.Empty(newVars.Vars["c"])
	s.Equal("4", newVars.Vars["d"]) // can't overwrite a value with the empty string
	_, ok := newVars.Vars["a"]
	s.False(ok)

	s.Equal(newVars.PrivateVars["b"], true)
	s.Equal(newVars.PrivateVars["c"], true)
	_, ok = newVars.PrivateVars["a"]
	s.False(ok)

	dbNewVars, err := model.FindOneProjectVars(projectId)
	s.NoError(err)
	s.Require().NotZero(dbNewVars)

	s.Len(dbNewVars.Vars, 3)
	s.Equal("2", dbNewVars.Vars["b"])
	s.Equal("3", dbNewVars.Vars["c"])
	s.Equal("4", dbNewVars.Vars["d"])
	_, ok = dbNewVars.Vars["a"]
	s.False(ok)

	s.True(dbNewVars.PrivateVars["b"])
	s.True(dbNewVars.PrivateVars["c"])
	s.False(dbNewVars.PrivateVars["a"])

	checkParametersMatchVars(ctx, s.T(), dbNewVars.Parameters, dbNewVars.Vars)
	checkParametersNamespacedByProject(s.T(), *dbNewVars)

	newProjRef := model.ProjectRef{
		Id:                    "new_project",
		ParameterStoreEnabled: true,
	}
	s.Require().NoError(newProjRef.Insert())
	// successful upsert
	s.NoError(UpdateProjectVars(newProjRef.Id, &newVars, false))

	dbUpsertedVars, err := model.FindOneProjectVars(newProjRef.Id)
	s.NoError(err)
	s.Require().NotZero(dbUpsertedVars)

	s.Len(dbUpsertedVars.Vars, 1)
	s.Equal("4", dbUpsertedVars.Vars["d"])

	checkParametersMatchVars(ctx, s.T(), dbUpsertedVars.Parameters, dbUpsertedVars.Vars)
	checkParametersNamespacedByProject(s.T(), *dbNewVars)
}

func TestUpdateProjectVarsByValue(t *testing.T) {
	require.NoError(t, db.ClearCollections(model.ProjectVarsCollection, fakeparameter.Collection, event.EventCollection))

	pRef := model.ProjectRef{
		Id:                    projectId,
		ParameterStoreEnabled: true,
	}
	require.NoError(t, pRef.Insert())

	vars := &model.ProjectVars{
		Id:          pRef.Id,
		Vars:        map[string]string{"a": "1", "b": "3"},
		PrivateVars: map[string]bool{"b": true},
	}
	require.NoError(t, vars.Insert())
	checkAndSetProjectVarsSynced(t, &pRef, false)

	resp, err := model.UpdateProjectVarsByValue("1", "11", "user", true, false)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, []string{"a"}, resp[projectId])

	res, err := FindProjectVarsById(projectId, "", false)
	assert.NoError(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, "1", res.Vars["a"])

	resp, err = model.UpdateProjectVarsByValue("1", "11", username, false, false)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, []string{"a"}, resp[projectId])

	res, err = FindProjectVarsById(projectId, "", false)
	assert.NoError(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, "11", res.Vars["a"])

	resp, err = model.UpdateProjectVarsByValue("3", "33", username, false, false)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, []string{"b"}, resp[projectId])

	res, err = FindProjectVarsById(projectId, "", false)
	assert.NoError(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, "33", res.Vars["b"])

	projectEvents, err := model.MostRecentProjectEvents(projectId, 5)
	assert.NoError(t, err)
	require.Len(t, projectEvents, 2)

	assert.NotNil(t, projectEvents[0].Data)
	eventData := projectEvents[0].Data.(*model.ProjectChangeEvent)

	assert.Equal(t, username, eventData.User)
	assert.Equal(t, evergreen.RedactedBeforeValue, eventData.Before.Vars.Vars["b"])
	assert.True(t, eventData.Before.Vars.PrivateVars["b"])
	assert.Equal(t, evergreen.RedactedAfterValue, eventData.After.Vars.Vars["b"])
	assert.True(t, eventData.After.Vars.PrivateVars["b"])

	require.NotNil(t, projectEvents[1].Data)
	eventData = projectEvents[1].Data.(*model.ProjectChangeEvent)

	assert.Equal(t, username, eventData.User)
	assert.Equal(t, evergreen.RedactedBeforeValue, eventData.Before.Vars.Vars["a"])
	assert.Equal(t, evergreen.RedactedAfterValue, eventData.After.Vars.Vars["a"])
}

func TestUpdateProjectVarsByValueWithEnabledOnly(t *testing.T) {
	require.NoError(t, db.ClearCollections(model.ProjectVarsCollection, fakeparameter.Collection, event.EventCollection, model.ProjectRefCollection))

	enabledRef := &model.ProjectRef{
		Id:                    "enabledProject",
		Identifier:            "enabledProjectIdent",
		Enabled:               true,
		ParameterStoreEnabled: true,
	}
	require.NoError(t, enabledRef.Insert())
	disabledRef := &model.ProjectRef{
		Id:                    "disabledProject",
		Identifier:            "disabledProjectIdent",
		Enabled:               false,
		ParameterStoreEnabled: true,
	}
	require.NoError(t, disabledRef.Insert())
	repoRef := &model.RepoRef{
		ProjectRef: model.ProjectRef{
			Id:                    "repoProject",
			Identifier:            "repoProjectIdent",
			Enabled:               true,
			ParameterStoreEnabled: true,
		},
	}
	require.NoError(t, repoRef.Upsert())

	enabledVars := &model.ProjectVars{
		Id:          enabledRef.Id,
		Vars:        map[string]string{"a": "1", "b": "3"},
		PrivateVars: map[string]bool{"b": true},
	}
	require.NoError(t, enabledVars.Insert())
	checkAndSetProjectVarsSynced(t, enabledRef, false)
	disabledVars := &model.ProjectVars{
		Id:          disabledRef.Id,
		Vars:        map[string]string{"a": "1", "b": "3"},
		PrivateVars: map[string]bool{"b": true},
	}
	require.NoError(t, disabledVars.Insert())
	checkAndSetProjectVarsSynced(t, disabledRef, false)
	repoVars := &model.ProjectVars{
		Id:          repoRef.Id,
		Vars:        map[string]string{"a": "1", "b": "3"},
		PrivateVars: map[string]bool{"b": true},
	}
	require.NoError(t, repoVars.Insert())
	checkAndSetProjectVarsSynced(t, &repoRef.ProjectRef, true)

	resp, err := model.UpdateProjectVarsByValue("1", "11", "user", true, false)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Len(t, resp, 3) // All three projects considered
	assert.Equal(t, []string{"a"}, resp["enabledProjectIdent"])
	assert.Equal(t, []string{"a"}, resp["disabledProjectIdent"])
	assert.Equal(t, []string{"a"}, resp["repoProject"])

	resp, err = model.UpdateProjectVarsByValue("1", "11", "user", true, true)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Len(t, resp, 2) // Only two projects considered
	assert.Equal(t, []string{"a"}, resp["enabledProjectIdent"])
	assert.Equal(t, []string{"a"}, resp["repoProject"])
}

func (s *ProjectConnectorGetSuite) TestCopyProjectVars() {
	pRef := model.ProjectRef{
		Id:                    "project-copy",
		ParameterStoreEnabled: true,
	}
	s.Require().NoError(pRef.Insert())
	s.NoError(model.CopyProjectVars(projectId, pRef.Id))
	origProj, err := FindProjectVarsById(projectId, "", false)
	s.NoError(err)

	newProj, err := FindProjectVarsById("project-copy", "", false)
	s.NoError(err)

	s.Equal(origProj.PrivateVars, newProj.PrivateVars)
	s.Equal(origProj.Vars, newProj.Vars)
}

func TestGetProjectAliasResults(t *testing.T) {
	require.NoError(t, db.ClearCollections(model.ProjectAliasCollection))
	p := model.Project{
		Identifier: "helloworld",
		BuildVariants: model.BuildVariants{
			{Name: "bv1", Tasks: []model.BuildVariantTaskUnit{{Name: "task1", Variant: "bv1"}}},
			{Name: "bv2", Tasks: []model.BuildVariantTaskUnit{{Name: "task2", Variant: "bv2"}, {Name: "task3", Variant: "bv2"}}},
		},
		Tasks: []model.ProjectTask{
			{Name: "task1"},
			{Name: "task2"},
			{Name: "task3"},
		},
	}
	alias1 := model.ProjectAlias{
		Alias:     "select_bv1",
		ProjectID: p.Identifier,
		Variant:   "^bv1$",
		Task:      ".*",
	}
	require.NoError(t, alias1.Upsert())
	alias2 := model.ProjectAlias{
		Alias:     "select_bv2",
		ProjectID: p.Identifier,
		Variant:   "^bv2$",
		Task:      ".*",
	}
	require.NoError(t, alias2.Upsert())

	variantTasks, err := GetProjectAliasResults(&p, alias1.Alias, false)
	assert.NoError(t, err)
	assert.Len(t, variantTasks, 1)
	assert.Len(t, variantTasks[0].Tasks, 1)
	assert.Equal(t, "task1", variantTasks[0].Tasks[0])
	variantTasks, err = GetProjectAliasResults(&p, alias2.Alias, false)
	assert.NoError(t, err)
	assert.Len(t, variantTasks, 1)
	assert.Len(t, variantTasks[0].Tasks, 2)
}

func TestCreateProject(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.ClearCollections(model.ProjectRefCollection, model.ProjectVarsCollection, fakeparameter.Collection, commitqueue.Collection, event.EventCollection, user.Collection, evergreen.ScopeCollection))

		cocoaMock.ResetGlobalSecretCache()
	}()

	smClient := &cocoaMock.SecretsManagerClient{}

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, pRef model.ProjectRef, u user.DBUser){
		"Succeeds": func(ctx context.Context, t *testing.T, env *mock.Environment, pRef model.ProjectRef, u user.DBUser) {
			created, err := CreateProject(ctx, env, &pRef, &u)
			require.NoError(t, err)
			require.True(t, created)

			dbProjRef, err := model.FindBranchProjectRef(pRef.Id)
			require.NoError(t, err)
			require.NotZero(t, dbProjRef)
			require.Len(t, dbProjRef.ContainerSecrets, 1, "should create pod secret for new project")
			assert.NotZero(t, dbProjRef.ContainerSecrets[0].Name)
			assert.Equal(t, model.ContainerSecretPodSecret, dbProjRef.ContainerSecrets[0].Type)
			assert.NotZero(t, dbProjRef.ContainerSecrets[0].ExternalName)
			assert.NotZero(t, dbProjRef.ContainerSecrets[0].ExternalID)

			getValOut, err := smClient.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
				SecretId: utility.ToStringPtr(dbProjRef.ContainerSecrets[0].ExternalID),
			})
			require.NoError(t, err, "new pod secret should be stored")
			assert.NotZero(t, utility.FromStringPtr(getValOut.SecretString))
		},
		"FailsWithAlreadyExistingID": func(ctx context.Context, t *testing.T, env *mock.Environment, pRef model.ProjectRef, u user.DBUser) {
			require.NoError(t, pRef.Insert())
			pRef.Identifier = "some new identifier"
			created, err := CreateProject(ctx, env, &pRef, &u)
			require.Error(t, err)
			require.False(t, created)
		},
		"FailsWithAlreadyExistingIdentifier": func(ctx context.Context, t *testing.T, env *mock.Environment, pRef model.ProjectRef, u user.DBUser) {
			require.NoError(t, pRef.Insert())
			pRef.Id = "some new ID"
			created, err := CreateProject(ctx, env, &pRef, &u)
			require.Error(t, err)
			require.False(t, created)
		},
		"SucceedsWithEmptyID": func(ctx context.Context, t *testing.T, env *mock.Environment, pRef model.ProjectRef, u user.DBUser) {
			pRef.Id = ""
			created, err := CreateProject(ctx, env, &pRef, &u)
			require.NoError(t, err)
			require.True(t, created)

			dbProjRef, err := model.FindBranchProjectRef(pRef.Identifier)
			require.NoError(t, err)
			require.NotZero(t, dbProjRef)
			assert.NotZero(t, dbProjRef.Id, "project ID should be set to something")
			assert.Equal(t, pRef.Identifier, dbProjRef.Identifier)
		},
		"SucceedsWithEmptyIdentifier": func(ctx context.Context, t *testing.T, env *mock.Environment, pRef model.ProjectRef, u user.DBUser) {
			pRef.Identifier = ""
			created, err := CreateProject(ctx, env, &pRef, &u)
			require.NoError(t, err)
			require.True(t, created)

			dbProjRef, err := model.FindBranchProjectRef(pRef.Identifier)
			require.NoError(t, err)
			require.NotZero(t, dbProjRef)
			assert.NotZero(t, dbProjRef.Id)
			assert.Equal(t, pRef.Identifier, dbProjRef.Identifier)
		},
		"SucceedsWithObjectIDAsProjectID": func(ctx context.Context, t *testing.T, env *mock.Environment, pRef model.ProjectRef, u user.DBUser) {
			pRef.Id = primitive.NewObjectID().Hex()
			created, err := CreateProject(ctx, env, &pRef, &u)
			require.NoError(t, err)
			require.True(t, created)
		},
		"SucceedsWithValidSpecialCharactersInProjectID": func(ctx context.Context, t *testing.T, env *mock.Environment, pRef model.ProjectRef, u user.DBUser) {
			pRef.Id = `(This 1) ~is-totally_fine.`
			created, err := CreateProject(ctx, env, &pRef, &u)
			require.NoError(t, err)
			require.True(t, created)
		},
		"FailsWithInvalidCharactersInProjectID": func(ctx context.Context, t *testing.T, env *mock.Environment, pRef model.ProjectRef, u user.DBUser) {
			pRef.Id = `^this / % is $ invalid*`
			created, err := CreateProject(ctx, env, &pRef, &u)
			require.Error(t, err)
			require.False(t, created)
		},
		"FailsWithInvalidCharactersInProjectIdentifier": func(ctx context.Context, t *testing.T, env *mock.Environment, pRef model.ProjectRef, u user.DBUser) {
			pRef.Identifier = `^this / % is $ invalid*`
			created, err := CreateProject(ctx, env, &pRef, &u)
			require.Error(t, err)
			require.False(t, created)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithCancel(context.Background())
			defer tcancel()

			require.NoError(t, db.ClearCollections(model.ProjectRefCollection, model.ProjectVarsCollection, fakeparameter.Collection, commitqueue.Collection, event.EventCollection, user.Collection, evergreen.ScopeCollection))

			cocoaMock.ResetGlobalSecretCache()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))

			pRef := model.ProjectRef{
				Id:                    "new_project",
				Identifier:            "new project identifier",
				Owner:                 "evergreen-ci",
				Repo:                  "treepo",
				ParameterStoreEnabled: true,
			}

			adminUser := user.DBUser{
				Id: "the_evergreen_admin",
			}
			require.NoError(t, adminUser.Insert())

			tCase(tctx, t, env, pRef, adminUser)
		})
	}
}

func TestGetLegacyProjectEvents(t *testing.T) {
	require.NoError(t, db.ClearCollections(event.EventCollection))

	project := &model.ProjectRef{Id: projectId}
	require.NoError(t, project.Insert())

	before := getMockProjectSettings()
	after := getMockProjectSettings()

	// Use an interface{} to mimic legacy data that was not inserted as a ProjectSettingsEvent
	h := event.EventLogEntry{
		Timestamp:    time.Now(),
		ResourceType: event.EventResourceTypeProject,
		EventType:    event.EventTypeProjectModified,
		ResourceId:   projectId,
		Data: map[string]interface{}{
			"user":   username,
			"before": model.NewProjectSettingsEvent(before),
			"after":  model.NewProjectSettingsEvent(after),
		},
	}

	require.NoError(t, h.Log())

	events, err := GetProjectEventLog(projectId, time.Now(), 0)
	require.NoError(t, err)
	require.Len(t, events, 1)
	eventLog := events[0]
	require.NotNil(t, eventLog)

	// Because this document does not use <Fieldname>Default flags, it returns empty arrays instead of nil
	require.NotNil(t, eventLog.Before.ProjectRef.PeriodicBuilds)
	require.Len(t, eventLog.Before.ProjectRef.PeriodicBuilds, 0)
	require.NotNil(t, eventLog.Before.ProjectRef.WorkstationConfig.SetupCommands)
	require.Len(t, eventLog.Before.ProjectRef.WorkstationConfig.SetupCommands, 0)
}

func TestRequestS3Creds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, db.ClearCollections(notification.Collection, evergreen.ConfigCollection))
	assert.Error(t, RequestS3Creds(ctx, "", ""))
	assert.NoError(t, RequestS3Creds(ctx, "identifier", "user@email.com"))
	n, err := notification.FindUnprocessed()
	assert.NoError(t, err)
	assert.Len(t, n, 0)
	projectCreationConfig := evergreen.ProjectCreationConfig{
		JiraProject: "BUILD",
	}
	assert.NoError(t, projectCreationConfig.Set(ctx))
	assert.NoError(t, RequestS3Creds(ctx, "identifier", "user@email.com"))
	n, err = notification.FindUnprocessed()
	assert.NoError(t, err)
	assert.Len(t, n, 1)
	assert.Equal(t, event.JIRAIssueSubscriberType, n[0].Subscriber.Type)
	target := n[0].Subscriber.Target.(*event.JIRAIssueSubscriber)
	assert.Equal(t, "BUILD", target.Project)
	payload := n[0].Payload.(*message.JiraIssue)
	summary := "Create AWS key for s3 uploads for 'identifier' project"
	description := "Could you create an s3 key for the new [identifier|/project/identifier/settings/general] project?"
	assert.Equal(t, "BUILD", payload.Project)
	assert.Equal(t, summary, payload.Summary)
	assert.Equal(t, description, payload.Description)
	assert.Equal(t, "user@email.com", payload.Reporter)
}

func TestHideBranch(t *testing.T) {
	require.NoError(t, db.ClearCollections(model.RepoRefCollection, model.ProjectRefCollection, model.ProjectVarsCollection, fakeparameter.Collection, model.ProjectAliasCollection))

	repo := model.RepoRef{
		ProjectRef: model.ProjectRef{
			Id:    "repo_ref",
			Owner: "mongodb",
			Repo:  "test_repo",
		},
	}
	assert.NoError(t, repo.Upsert())

	project := &model.ProjectRef{
		Identifier:            projectId,
		Id:                    projectId,
		DisplayName:           "test_project",
		Owner:                 repo.Owner,
		Repo:                  repo.Repo,
		RepoRefId:             repo.Id,
		Branch:                "branch",
		Enabled:               true,
		Hidden:                utility.ToBoolPtr(false),
		ParameterStoreEnabled: true,
	}
	require.NoError(t, project.Upsert())

	alias := model.ProjectAlias{
		ProjectID: project.Id,
		Alias:     "select_bv1",
		Variant:   "^bv1$",
		Task:      ".*",
	}
	require.NoError(t, alias.Upsert())

	vars := &model.ProjectVars{
		Id:          project.Id,
		Vars:        map[string]string{"a": "1", "b": "3"},
		PrivateVars: map[string]bool{"b": true},
	}
	require.NoError(t, vars.Insert())
	checkAndSetProjectVarsSynced(t, project, false)

	err := HideBranch(project.Id)
	assert.NoError(t, err)

	hiddenProj, err := model.FindMergedProjectRef(project.Id, "", true)
	assert.NoError(t, err)
	skeletonProj := model.ProjectRef{
		Id:                       project.Id,
		Owner:                    repo.Owner,
		Repo:                     repo.Repo,
		Branch:                   project.Branch,
		RepoRefId:                repo.Id,
		Enabled:                  false,
		Hidden:                   utility.TruePtr(),
		ParameterStoreEnabled:    true,
		ParameterStoreVarsSynced: true,
	}
	assert.Equal(t, skeletonProj, *hiddenProj)

	projAliases, err := model.FindAliasesForProjectFromDb(project.Id)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(projAliases))

	skeletonProjVars := model.ProjectVars{
		Id:   project.Id,
		Vars: map[string]string{},
	}
	projVars, err := model.FindOneProjectVars(project.Id)
	assert.NoError(t, err)
	assert.Equal(t, skeletonProjVars, *projVars)
}
