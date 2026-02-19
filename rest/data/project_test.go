package data

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/parameterstore/fakeparameter"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/githubapp"
	ghAppAuthModel "github.com/evergreen-ci/evergreen/model/githubapp"
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
			Id: projectId,
		}
		projects := []*model.ProjectRef{
			{
				Id:          "projectA",
				Enabled:     true,
				CommitQueue: model.CommitQueueParams{Enabled: utility.TruePtr()},
				Owner:       "evergreen-ci",
				Repo:        "gimlet",
				Branch:      "main",
			},
			{
				Id:          "projectB",
				Enabled:     true,
				CommitQueue: model.CommitQueueParams{Enabled: utility.TruePtr()},
				Owner:       "evergreen-ci",
				Repo:        "evergreen",
				Branch:      "main",
			},
			{
				Id:          "projectC",
				Enabled:     true,
				CommitQueue: model.CommitQueueParams{Enabled: utility.TruePtr()},
				Owner:       "mongodb",
				Repo:        "mongo",
				Branch:      "main",
			},
			{
				Id: "projectD",
			},
			{
				Id: "projectE",
			},
			{
				Id: "projectF",
			},
			projWithVars,
		}

		for _, p := range projects {
			if err := p.Insert(t.Context()); err != nil {
				return err
			}
			if _, err := model.GetNewRevisionOrderNumber(t.Context(), p.Id); err != nil {
				return err
			}
		}

		projVars := &model.ProjectVars{
			Id:          projectId,
			Vars:        map[string]string{"a": "1", "b": "3", "d": "4"},
			PrivateVars: map[string]bool{"b": true},
		}
		s.NoError(projVars.Insert(t.Context()))

		repoWithVars := &model.RepoRef{
			ProjectRef: model.ProjectRef{
				Id: repoProjectId,
			},
		}
		s.Require().NoError(repoWithVars.Replace(t.Context()))
		repoVars := &model.ProjectVars{
			Id:          repoProjectId,
			Vars:        map[string]string{"a": "a_from_repo", "c": "new"},
			PrivateVars: map[string]bool{"a": true},
		}
		s.NoError(repoVars.Insert(t.Context()))

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
			s.NoError(model.LogProjectModified(t.Context(), projectId, username, &before, &after))
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
	events, err := GetProjectEventLog(s.T().Context(), projectId, time.Now(), 0)
	s.NoError(err)
	s.Len(events, projEventCount)
	for _, eventLog := range events {
		s.Len(eventLog.Before.Aliases, 1)
		s.Len(eventLog.After.Aliases, 1)
		s.NotEmpty(eventLog.Before.Aliases[0].ID)
		s.NotEmpty(eventLog.After.Aliases[0].ID)
		s.Nil(eventLog.Before.ProjectRef.PeriodicBuilds)
		s.Nil(eventLog.Before.ProjectRef.WorkstationConfig.SetupCommands)
		s.NotNil(eventLog.After.ProjectRef.WorkstationConfig.SetupCommands)
		s.Empty(eventLog.After.ProjectRef.WorkstationConfig.SetupCommands)
		s.Equal(evergreen.RedactedBeforeValue, eventLog.Before.Vars.Vars["hello"])
		s.Equal(evergreen.RedactedAfterValue, eventLog.After.Vars.Vars["hello"])
		s.Equal(evergreen.RedactedBeforeValue, eventLog.Before.Vars.Vars["world"])
		s.Equal(evergreen.RedactedAfterValue, eventLog.After.Vars.Vars["world"])
		s.Empty(utility.FromStringPtr(eventLog.Before.GithubAppAuth.PrivateKey))
		s.Equal(evergreen.RedactedAfterValue, utility.FromStringPtr(eventLog.After.GithubAppAuth.PrivateKey))
	}

	// No error for empty events
	events, err = GetProjectEventLog(s.T().Context(), "projectA", time.Now(), 0)
	s.NoError(err)
	s.Empty(events)
}

func (s *ProjectConnectorGetSuite) TestFindProjectVarsById() {
	// redact private variables
	res, err := FindProjectVarsById(s.T().Context(), projectId, "", true)
	s.NoError(err)
	s.Require().NotNil(res)
	s.Equal("1", res.Vars["a"])
	s.Equal("", res.Vars["b"])
	s.True(res.PrivateVars["b"])

	// not redacted
	res, err = FindProjectVarsById(s.T().Context(), projectId, "", false)
	s.NoError(err)
	s.Require().NotNil(res)
	s.Equal("1", res.Vars["a"])
	s.Equal("3", res.Vars["b"])
	s.Equal("", res.Vars["c"])

	// test with repo
	res, err = FindProjectVarsById(s.T().Context(), projectId, repoProjectId, true)
	s.NoError(err)
	s.Require().NotNil(res)
	s.Equal("1", res.Vars["a"])
	s.Equal("", res.Vars["b"])
	s.True(res.PrivateVars["b"])
	s.False(res.PrivateVars["a"])
	s.Equal("new", res.Vars["c"])

	res, err = FindProjectVarsById(s.T().Context(), "", repoProjectId, true)
	s.NoError(err)
	s.Equal("", res.Vars["a"])
	s.Equal("new", res.Vars["c"])
	s.True(res.PrivateVars["a"])

	res, err = FindProjectVarsById(s.T().Context(), "", repoProjectId, false)
	s.NoError(err)
	s.Equal("a_from_repo", res.Vars["a"])
	s.Equal("", res.Vars["b"])
	s.Equal("new", res.Vars["c"])
	s.True(res.PrivateVars["a"])

	_, err = FindProjectVarsById(s.T().Context(), "non-existent", "also-non-existent", false)
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

func checkParametersNamespacedByProject(t *testing.T, vars model.ProjectVars) {
	projectID := vars.Id
	commonAndProjectIDPrefix := fmt.Sprintf("/%s/%s/", strings.TrimSuffix(strings.TrimPrefix(evergreen.GetEnvironment().Settings().ParameterStore.Prefix, "/"), "/"), model.GetVarsParameterPath(projectID))
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
	s.NoError(UpdateProjectVars(s.T().Context(), projectId, &newVars, false))

	s.Empty(newVars.Vars["b"]) // can't unredact previously redacted variables
	s.Empty(newVars.Vars["c"])
	s.Equal("4", newVars.Vars["d"]) // can't overwrite a value with the empty string
	_, ok := newVars.Vars["a"]
	s.False(ok)

	s.True(newVars.PrivateVars["b"])
	s.True(newVars.PrivateVars["c"])
	_, ok = newVars.PrivateVars["a"]
	s.False(ok)

	dbNewVars, err := model.FindOneProjectVars(s.T().Context(), projectId)
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
		Id: "new_project",
	}
	s.Require().NoError(newProjRef.Insert(s.T().Context()))
	// successful upsert
	s.NoError(UpdateProjectVars(s.T().Context(), newProjRef.Id, &newVars, false))

	dbUpsertedVars, err := model.FindOneProjectVars(s.T().Context(), newProjRef.Id)
	s.NoError(err)
	s.Require().NotZero(dbUpsertedVars)

	s.Len(dbUpsertedVars.Vars, 1)
	s.Equal("4", dbUpsertedVars.Vars["d"])

	checkParametersMatchVars(ctx, s.T(), dbUpsertedVars.Parameters, dbUpsertedVars.Vars)
	checkParametersNamespacedByProject(s.T(), *dbNewVars)
}

func (s *ProjectConnectorGetSuite) TestCopyProjectVars() {
	pRef := model.ProjectRef{
		Id: "project-copy",
	}
	s.Require().NoError(pRef.Insert(s.T().Context()))
	s.NoError(model.CopyProjectVars(s.T().Context(), projectId, pRef.Id))
	origProj, err := FindProjectVarsById(s.T().Context(), projectId, "", false)
	s.NoError(err)

	newProj, err := FindProjectVarsById(s.T().Context(), "project-copy", "", false)
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
	require.NoError(t, alias1.Upsert(t.Context()))
	alias2 := model.ProjectAlias{
		Alias:     "select_bv2",
		ProjectID: p.Identifier,
		Variant:   "^bv2$",
		Task:      ".*",
	}
	require.NoError(t, alias2.Upsert(t.Context()))

	variantTasks, err := GetProjectAliasResults(t.Context(), &p, alias1.Alias, false)
	assert.NoError(t, err)
	assert.Len(t, variantTasks, 1)
	assert.Len(t, variantTasks[0].Tasks, 1)
	assert.Equal(t, "task1", variantTasks[0].Tasks[0])
	variantTasks, err = GetProjectAliasResults(t.Context(), &p, alias2.Alias, false)
	assert.NoError(t, err)
	assert.Len(t, variantTasks, 1)
	assert.Len(t, variantTasks[0].Tasks, 2)
}

func TestCreateProject(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.ClearCollections(model.ProjectRefCollection, model.ProjectVarsCollection, fakeparameter.Collection, event.EventCollection, user.Collection, evergreen.ScopeCollection))
	}()

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, pRef model.ProjectRef, u user.DBUser){
		"Succeeds": func(ctx context.Context, t *testing.T, env *mock.Environment, pRef model.ProjectRef, u user.DBUser) {
			created, err := CreateProject(ctx, env, &pRef, &u)
			require.NoError(t, err)
			require.True(t, created)

			dbProjRef, err := model.FindBranchProjectRef(ctx, pRef.Id)
			require.NoError(t, err)
			require.NotZero(t, dbProjRef)
		},
		"FailsWithAlreadyExistingID": func(ctx context.Context, t *testing.T, env *mock.Environment, pRef model.ProjectRef, u user.DBUser) {
			require.NoError(t, pRef.Insert(t.Context()))
			pRef.Identifier = "some new identifier"
			created, err := CreateProject(ctx, env, &pRef, &u)
			require.Error(t, err)
			require.False(t, created)
		},
		"FailsWithAlreadyExistingIdentifier": func(ctx context.Context, t *testing.T, env *mock.Environment, pRef model.ProjectRef, u user.DBUser) {
			require.NoError(t, pRef.Insert(t.Context()))
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

			dbProjRef, err := model.FindBranchProjectRef(ctx, pRef.Identifier)
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

			dbProjRef, err := model.FindBranchProjectRef(ctx, pRef.Identifier)
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

			require.NoError(t, db.ClearCollections(model.ProjectRefCollection, model.ProjectVarsCollection, fakeparameter.Collection, event.EventCollection, user.Collection, evergreen.ScopeCollection))

			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))

			pRef := model.ProjectRef{
				Id:         "new_project",
				Identifier: "new project identifier",
				Owner:      "evergreen-ci",
				Repo:       "treepo",
			}

			adminUser := user.DBUser{
				Id: "the_evergreen_admin",
			}
			require.NoError(t, adminUser.Insert(t.Context()))

			tCase(tctx, t, env, pRef, adminUser)
		})
	}
}

func TestGetLegacyProjectEvents(t *testing.T) {
	require.NoError(t, db.ClearCollections(event.EventCollection))

	project := &model.ProjectRef{Id: projectId}
	require.NoError(t, project.Insert(t.Context()))

	before := getMockProjectSettings()
	after := getMockProjectSettings()

	// Use an interface{} to mimic legacy data that was not inserted as a ProjectSettingsEvent
	h := event.EventLogEntry{
		Timestamp:    time.Now(),
		ResourceType: event.EventResourceTypeProject,
		EventType:    event.EventTypeProjectModified,
		ResourceId:   projectId,
		Data: map[string]any{
			"user":   username,
			"before": model.NewProjectSettingsEvent(before),
			"after":  model.NewProjectSettingsEvent(after),
		},
	}

	require.NoError(t, h.Log(t.Context()))

	events, err := GetProjectEventLog(t.Context(), projectId, time.Now(), 0)
	require.NoError(t, err)
	require.Len(t, events, 1)
	eventLog := events[0]
	require.NotNil(t, eventLog)

	// Because this document does not use <Fieldname>Default flags, it returns empty arrays instead of nil
	require.NotNil(t, eventLog.Before.ProjectRef.PeriodicBuilds)
	require.Empty(t, eventLog.Before.ProjectRef.PeriodicBuilds)
	require.NotNil(t, eventLog.Before.ProjectRef.WorkstationConfig.SetupCommands)
	require.Empty(t, eventLog.Before.ProjectRef.WorkstationConfig.SetupCommands)
}

func TestRequestS3Creds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, db.ClearCollections(notification.Collection, evergreen.ConfigCollection))
	assert.Error(t, RequestS3Creds(ctx, "", ""))
	assert.NoError(t, RequestS3Creds(ctx, "identifier", "user@email.com"))
	n, err := notification.FindUnprocessed(t.Context())
	assert.NoError(t, err)
	assert.Empty(t, n)
	projectCreationConfig := evergreen.ProjectCreationConfig{
		JiraProject: "BUILD",
	}
	assert.NoError(t, projectCreationConfig.Set(ctx))
	assert.NoError(t, RequestS3Creds(ctx, "identifier", "user@email.com"))
	n, err = notification.FindUnprocessed(t.Context())
	assert.NoError(t, err)
	assert.Len(t, n, 1)
	assert.Equal(t, event.JIRAIssueSubscriberType, n[0].Subscriber.Type)
	target := n[0].Subscriber.Target.(*event.JIRAIssueSubscriber)
	assert.Equal(t, "BUILD", target.Project)
	payload := n[0].Payload.(*message.JiraIssue)
	summary := "Create AWS bucket for s3 uploads for 'identifier' project"
	description := "Could you create an s3 bucket and role arn for the new [identifier|/project/identifier/settings/general] project?"
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
	assert.NoError(t, repo.Replace(t.Context()))

	project := &model.ProjectRef{
		Identifier:  projectId,
		Id:          projectId,
		DisplayName: "test_project",
		Owner:       repo.Owner,
		Repo:        repo.Repo,
		RepoRefId:   repo.Id,
		Branch:      "branch",
		Enabled:     true,
		Hidden:      utility.ToBoolPtr(false),
	}
	require.NoError(t, project.Replace(t.Context()))

	alias := model.ProjectAlias{
		ProjectID: project.Id,
		Alias:     "select_bv1",
		Variant:   "^bv1$",
		Task:      ".*",
	}
	require.NoError(t, alias.Upsert(t.Context()))

	vars := &model.ProjectVars{
		Id:          project.Id,
		Vars:        map[string]string{"a": "1", "b": "3"},
		PrivateVars: map[string]bool{"b": true},
	}
	require.NoError(t, vars.Insert(t.Context()))

	// Insert GitHub app and ensure it inserts a fake parameter.
	var ghAppID int64 = 90
	ghPrivateKey := []byte("secret")
	require.NoError(t, project.SetGithubAppCredentials(t.Context(), ghAppID, ghPrivateKey))

	oldGhAppAuth, err := ghAppAuthModel.FindOneGitHubAppAuth(t.Context(), project.Id)
	require.NoError(t, err)
	require.NotEmpty(t, oldGhAppAuth)

	fakeParams, err := fakeparameter.FindByIDs(t.Context(), oldGhAppAuth.PrivateKeyParameter)
	assert.NoError(t, err)
	require.Len(t, fakeParams, 1)
	assert.Equal(t, string(ghPrivateKey), fakeParams[0].Value)

	require.NoError(t, HideBranch(t.Context(), project.Id))

	t.Run("MergedProjectRefStripped", func(t *testing.T) {
		hiddenProj, err := model.FindMergedProjectRef(t.Context(), project.Id, "", true)
		require.NoError(t, err)
		skeletonProj := model.ProjectRef{
			Id:        project.Id,
			Owner:     repo.Owner,
			Repo:      repo.Repo,
			Branch:    project.Branch,
			RepoRefId: repo.Id,
			Enabled:   false,
			Hidden:    utility.TruePtr(),
		}
		assert.Equal(t, skeletonProj, *hiddenProj)
	})

	t.Run("AliasesDeleted", func(t *testing.T) {
		projAliases, err := model.FindAliasesForProjectFromDb(t.Context(), project.Id)
		require.NoError(t, err)
		assert.Empty(t, projAliases)
	})

	t.Run("VarsStripped", func(t *testing.T) {
		skeletonProjVars := model.ProjectVars{
			Id:   project.Id,
			Vars: map[string]string{},
		}
		projVars, err := model.FindOneProjectVars(t.Context(), project.Id)
		require.NoError(t, err)
		assert.Equal(t, skeletonProjVars, *projVars)
	})

	t.Run("GitHubAppDeleted", func(t *testing.T) {
		gh, err := ghAppAuthModel.FindOneGitHubAppAuth(t.Context(), project.Id)
		require.NoError(t, err)
		assert.Empty(t, gh)

		t.Run("FakeParameterDeleted", func(t *testing.T) {
			fakeParams, err := fakeparameter.FindByIDs(t.Context(), oldGhAppAuth.PrivateKeyParameter)
			require.NoError(t, err)
			assert.Empty(t, fakeParams)
		})
	})

}
