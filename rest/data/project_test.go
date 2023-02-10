package data

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/service/secretsmanager"
	cocoaMock "github.com/evergreen-ci/cocoa/mock"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
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
			Enabled:        utility.TruePtr(),
			Id:             projectId,
			Admins:         []string{},
			Repo:           evergreen.PublicRepoName,
			PeriodicBuilds: nil,
			WorkstationConfig: model.WorkstationConfig{
				SetupCommands: nil,
				GitClone:      nil,
			},
		},
		GithubHooksEnabled: true,
		Vars: model.ProjectVars{
			Id:          projectId,
			Vars:        map[string]string{},
			PrivateVars: map[string]bool{},
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
		s.Require().NoError(db.ClearCollections(model.ProjectRefCollection, model.ProjectVarsCollection))

		projects := []*model.ProjectRef{
			{
				Id:          "projectA",
				Enabled:     utility.TruePtr(),
				CommitQueue: model.CommitQueueParams{Enabled: utility.TruePtr()},
				Owner:       "evergreen-ci",
				Repo:        evergreen.PublicRepoName,
				Branch:      "main",
			},
			{
				Id:          "projectB",
				Enabled:     utility.TruePtr(),
				CommitQueue: model.CommitQueueParams{Enabled: utility.TruePtr()},
				Owner:       "evergreen-ci",
				Branch:      "main",
			},
			{
				Id:          "projectC",
				Enabled:     utility.TruePtr(),
				CommitQueue: model.CommitQueueParams{Enabled: utility.TruePtr()},
				Owner:       "mongodb",
				Repo:        "mongo",
				Branch:      "main",
			},
			{Id: "projectD", Repo: evergreen.PublicRepoName},
			{Id: "projectE", Repo: evergreen.PublicRepoName},
			{Id: "projectF"},
			{Id: projectId},
		}

		for _, p := range projects {
			if err := p.Insert(); err != nil {
				return err
			}
			if _, err := model.GetNewRevisionOrderNumber(p.Id); err != nil {
				return err
			}
		}

		vars := &model.ProjectVars{
			Id:          projectId,
			Vars:        map[string]string{"a": "1", "b": "3", "d": "4"},
			PrivateVars: map[string]bool{"b": true},
		}
		s.NoError(vars.Insert())
		vars = &model.ProjectVars{
			Id:          repoProjectId,
			Vars:        map[string]string{"a": "a_from_repo", "c": "new"},
			PrivateVars: map[string]bool{"a": true},
		}
		s.NoError(vars.Insert())
		before := getMockProjectSettings()
		after := getMockProjectSettings()
		after.GithubHooksEnabled = false
		after.ProjectRef.WorkstationConfig.SetupCommands = []model.WorkstationSetupCommand{}
		s.NotEmpty(before.Aliases[0].ID)
		s.NotEmpty(after.Aliases[0].ID)

		h :=
			event.EventLogEntry{
				Timestamp:    time.Now(),
				ResourceType: event.EventResourceTypeProject,
				EventType:    event.EventTypeProjectModified,
				ResourceId:   projectId,
				Data: &model.ProjectChangeEvent{
					User: username,
					Before: model.ProjectSettingsEvent{
						PeriodicBuildsDefault:      true,
						WorkstationCommandsDefault: true,
						ProjectSettings:            before,
					},
					After: model.ProjectSettingsEvent{
						ProjectSettings: after,
					},
				},
			}

		s.Require().NoError(db.ClearCollections(event.EventCollection))
		for i := 0; i < projEventCount; i++ {
			eventShallowCpy := h
			s.NoError(eventShallowCpy.Log())
		}

		return nil
	}

	s.teardown = func() error {
		return db.Clear(model.ProjectRefCollection)
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

func (s *ProjectConnectorGetSuite) TestUpdateProjectVars() {
	//successful update
	varsToDelete := []string{"a"}
	newVars := restModel.APIProjectVars{
		Vars:         map[string]string{"b": "2", "c": "3", "d": ""},
		PrivateVars:  map[string]bool{"b": false, "c": true},
		VarsToDelete: varsToDelete,
	}
	s.NoError(UpdateProjectVars(projectId, &newVars, false))
	s.Equal(newVars.Vars["b"], "") // can't unredact previously redacted  variables
	s.Equal(newVars.Vars["c"], "")
	s.Equal(newVars.Vars["d"], "4") // can't overwrite a value with the empty string
	_, ok := newVars.Vars["a"]
	s.False(ok)

	s.Equal(newVars.PrivateVars["b"], true)
	s.Equal(newVars.PrivateVars["c"], true)
	_, ok = newVars.PrivateVars["a"]
	s.False(ok)

	// successful upsert
	s.NoError(UpdateProjectVars("not-an-id", &newVars, false))
}

func TestUpdateProjectVarsByValue(t *testing.T) {
	require.NoError(t, db.ClearCollections(model.ProjectVarsCollection, event.EventCollection))

	vars := &model.ProjectVars{
		Id:          projectId,
		Vars:        map[string]string{"a": "1", "b": "3"},
		PrivateVars: map[string]bool{"b": true},
	}
	require.NoError(t, vars.Insert())

	resp, err := model.UpdateProjectVarsByValue("1", "11", "user", true)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, []string{"a"}, resp[projectId])

	res, err := FindProjectVarsById(projectId, "", false)
	assert.NoError(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, "1", res.Vars["a"])

	resp, err = model.UpdateProjectVarsByValue("1", "11", username, false)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, []string{"a"}, resp[projectId])

	res, err = FindProjectVarsById(projectId, "", false)
	assert.NoError(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, "11", res.Vars["a"])

	projectEvents, err := model.MostRecentProjectEvents(projectId, 5)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(projectEvents))

	assert.NotNil(t, projectEvents[0].Data)
	eventData := projectEvents[0].Data.(*model.ProjectChangeEvent)

	assert.Equal(t, username, eventData.User)
	assert.Equal(t, "1", eventData.Before.Vars.Vars["a"])
	assert.Equal(t, "11", eventData.After.Vars.Vars["a"])
}

func (s *ProjectConnectorGetSuite) TestCopyProjectVars() {
	s.NoError(model.CopyProjectVars(projectId, "project-copy"))
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
			{Name: "bv1", Tasks: []model.BuildVariantTaskUnit{{Name: "task1"}}},
			{Name: "bv2", Tasks: []model.BuildVariantTaskUnit{{Name: "task2"}, {Name: "task3"}}},
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
		assert.NoError(t, db.ClearCollections(model.ProjectRefCollection, model.ProjectVarsCollection, commitqueue.Collection, event.EventCollection, user.Collection))

		cocoaMock.ResetGlobalSecretCache()
	}()

	smClient := &cocoaMock.SecretsManagerClient{}
	defer func() {
		assert.NoError(t, smClient.Close(ctx))
	}()

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, pRef model.ProjectRef, u user.DBUser){
		"Succeeds": func(ctx context.Context, t *testing.T, env *mock.Environment, pRef model.ProjectRef, u user.DBUser) {
			require.NoError(t, CreateProject(ctx, env, &pRef, &u))

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
			assert.Error(t, CreateProject(ctx, env, &pRef, &u))
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithCancel(context.Background())
			defer tcancel()

			require.NoError(t, db.ClearCollections(model.ProjectRefCollection, model.ProjectVarsCollection, commitqueue.Collection, event.EventCollection, user.Collection))

			cocoaMock.ResetGlobalSecretCache()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))

			pRef := model.ProjectRef{
				Id:    "new_project",
				Owner: "evergreen-ci",
				Repo:  "treepo",
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
			"before": before,
			"after":  after,
		},
	}

	require.NoError(t, h.Log())

	events, err := GetProjectEventLog(projectId, time.Now(), 0)
	require.NoError(t, err)
	require.Equal(t, len(events), 1)
	eventLog := events[0]
	require.NotNil(t, eventLog)

	// Because this document does not use <Fieldname>Default flags, it returns empty arrays instead of nil
	require.NotNil(t, eventLog.Before.ProjectRef.PeriodicBuilds)
	require.Len(t, eventLog.Before.ProjectRef.PeriodicBuilds, 0)
	require.NotNil(t, eventLog.Before.ProjectRef.WorkstationConfig.SetupCommands)
	require.Len(t, eventLog.Before.ProjectRef.WorkstationConfig.SetupCommands, 0)
}

func TestRequestS3Creds(t *testing.T) {
	assert.NoError(t, db.ClearCollections(notification.Collection, evergreen.ConfigCollection))
	assert.Error(t, RequestS3Creds(""))
	assert.NoError(t, RequestS3Creds("identifier"))
	n, err := notification.FindUnprocessed()
	assert.NoError(t, err)
	assert.Len(t, n, 0)
	projectCreationConfig := evergreen.ProjectCreationConfig{
		JiraProject: "BUILD",
	}
	assert.NoError(t, projectCreationConfig.Set())
	assert.NoError(t, RequestS3Creds("identifier"))
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
	assert.Equal(t, []string{"Access"}, payload.Components)
}
