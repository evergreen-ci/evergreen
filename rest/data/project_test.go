package data

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	mgobson "gopkg.in/mgo.v2/bson"
)

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch patch by project route

type ProjectConnectorGetSuite struct {
	ctx      Connector
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
			Owner:   "admin",
			Enabled: utility.TruePtr(),
			Private: utility.TruePtr(),
			Id:      projectId,
			Admins:  []string{},
		},
		GitHubHooksEnabled: true,
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
		s.ctx = &DBConnector{}

		s.Require().NoError(db.ClearCollections(model.ProjectRefCollection, model.ProjectVarsCollection))

		projects := []*model.ProjectRef{
			{
				Id:          "projectA",
				Private:     utility.FalsePtr(),
				Enabled:     utility.TruePtr(),
				CommitQueue: model.CommitQueueParams{Enabled: utility.TruePtr()},
				Owner:       "evergreen-ci",
				Repo:        "gimlet",
				Branch:      "main",
			},
			{
				Id:          "projectB",
				Private:     utility.TruePtr(),
				Enabled:     utility.TruePtr(),
				CommitQueue: model.CommitQueueParams{Enabled: utility.TruePtr()},
				Owner:       "evergreen-ci",
				Repo:        "evergreen",
				Branch:      "main",
			},
			{
				Id:          "projectC",
				Private:     utility.TruePtr(),
				Enabled:     utility.TruePtr(),
				CommitQueue: model.CommitQueueParams{Enabled: utility.TruePtr()},
				Owner:       "mongodb",
				Repo:        "mongo",
				Branch:      "main",
			},
			{Id: "projectD", Private: utility.FalsePtr()},
			{Id: "projectE", Private: utility.FalsePtr()},
			{Id: "projectF", Private: utility.TruePtr()},
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
			Vars:        map[string]string{"a": "1", "b": "3"},
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
		after.GitHubHooksEnabled = false

		h :=
			event.EventLogEntry{
				Timestamp:    time.Now(),
				ResourceType: model.EventResourceTypeProject,
				EventType:    model.EventTypeProjectModified,
				ResourceId:   projectId,
				Data: &model.ProjectChangeEvent{
					User:   username,
					Before: before,
					After:  after,
				},
			}

		s.Require().NoError(db.ClearCollections(event.AllLogCollection))
		logger := event.NewDBEventLogger(event.AllLogCollection)
		for i := 0; i < projEventCount; i++ {
			eventShallowCpy := h
			s.NoError(logger.LogEvent(&eventShallowCpy))
		}

		return nil
	}

	s.teardown = func() error {
		return db.Clear(model.ProjectRefCollection)
	}

	suite.Run(t, s)
}

func TestMockProjectConnectorGetSuite(t *testing.T) {
	s := new(ProjectConnectorGetSuite)
	s.setup = func() error {
		projectId := "mci2"
		beforeSettings := restModel.APIProjectSettings{
			ProjectRef: restModel.APIProjectRef{
				Owner:      utility.ToStringPtr("admin"),
				Enabled:    utility.TruePtr(),
				Private:    utility.TruePtr(),
				Identifier: utility.ToStringPtr(projectId),
				Admins:     []*string{},
			},
			GitHubWebhooksEnabled: true,
			Vars: restModel.APIProjectVars{
				Vars:        map[string]string{},
				PrivateVars: map[string]bool{},
			},
			Aliases: []restModel.APIProjectAlias{{
				Alias:   utility.ToStringPtr("alias1"),
				Variant: utility.ToStringPtr("ubuntu"),
				Task:    utility.ToStringPtr("subcommand"),
			},
			},
			Subscriptions: []restModel.APISubscription{{
				ID:           utility.ToStringPtr("subscription1"),
				ResourceType: utility.ToStringPtr("project"),
				Owner:        utility.ToStringPtr("admin"),
				Subscriber: restModel.APISubscriber{
					Type:   utility.ToStringPtr(event.GithubPullRequestSubscriberType),
					Target: restModel.APIGithubPRSubscriber{},
				},
			},
			},
		}

		afterSettings := beforeSettings
		afterSettings.ProjectRef.Enabled = utility.FalsePtr()

		projectEvents := []restModel.APIProjectEvent{}
		for i := 0; i < projEventCount; i++ {
			projectEvents = append(projectEvents, restModel.APIProjectEvent{
				Timestamp: restModel.ToTimePtr(time.Now().Add(time.Second * time.Duration(-i))),
				User:      utility.ToStringPtr("me"),
				Before:    beforeSettings,
				After:     afterSettings,
			})
		}

		s.ctx = &MockConnector{MockProjectConnector: MockProjectConnector{
			CachedProjects: []model.ProjectRef{
				{
					Id:          "projectA",
					Private:     utility.FalsePtr(),
					CommitQueue: model.CommitQueueParams{Enabled: utility.TruePtr()},
					Owner:       "evergreen-ci",
					Repo:        "gimlet",
					Branch:      "main",
				},
				{
					Id:          "projectB",
					Private:     utility.TruePtr(),
					CommitQueue: model.CommitQueueParams{Enabled: utility.TruePtr()},
					Owner:       "evergreen-ci",
					Repo:        "evergreen",
					Branch:      "main",
				},
				{
					Id:          "projectC",
					Private:     utility.TruePtr(),
					CommitQueue: model.CommitQueueParams{Enabled: utility.TruePtr()},
					Owner:       "evergreen-ci",
					Repo:        "evergreen",
					Branch:      "main",
				},
				{Id: "projectD", Private: utility.FalsePtr()},
				{Id: "projectE", Private: utility.FalsePtr()},
				{Id: "projectF", Private: utility.TruePtr()},
				{Id: projectId},
			},
			CachedEvents: projectEvents,
			CachedVars: []*model.ProjectVars{
				{
					Id:          projectId,
					Vars:        map[string]string{"a": "1", "b": "3"},
					PrivateVars: map[string]bool{"b": true},
				},
				{
					Id:          repoProjectId,
					Vars:        map[string]string{"a": "a_from_repo", "c": "new"},
					PrivateVars: map[string]bool{"a": true},
				},
			},
		}}

		return nil
	}

	s.teardown = func() error { return nil }

	suite.Run(t, s)
}

func (s *ProjectConnectorGetSuite) SetupSuite() { s.Require().NoError(s.setup()) }

func (s *ProjectConnectorGetSuite) TearDownSuite() {
	s.Require().NoError(s.teardown())
}

func (s *ProjectConnectorGetSuite) TestFetchTooManyAsc() {
	projects, err := s.ctx.FindProjects("", 8, 1)
	s.NoError(err)
	s.NotNil(projects)
	s.Len(projects, 7)
}

func (s *ProjectConnectorGetSuite) TestFetchTooManyDesc() {
	projects, err := s.ctx.FindProjects("zzz", 8, -1)
	s.NoError(err)
	s.NotNil(projects)
	s.Len(projects, 7)
}

func (s *ProjectConnectorGetSuite) TestFetchExactNumber() {
	projects, err := s.ctx.FindProjects("", 3, 1)
	s.NoError(err)
	s.NotNil(projects)
	s.Len(projects, 3)
}

func (s *ProjectConnectorGetSuite) TestFetchTooFewAsc() {
	projects, err := s.ctx.FindProjects("", 2, 1)
	s.NoError(err)
	s.NotNil(projects)
	s.Len(projects, 2)
}

func (s *ProjectConnectorGetSuite) TestFetchTooFewDesc() {
	projects, err := s.ctx.FindProjects("zzz", 2, -1)
	s.NoError(err)
	s.NotNil(projects)
	s.Len(projects, 2)
}

func (s *ProjectConnectorGetSuite) TestFetchKeyWithinBoundAsc() {
	projects, err := s.ctx.FindProjects("projectB", 1, 1)
	s.NoError(err)
	s.Len(projects, 1)
}

func (s *ProjectConnectorGetSuite) TestFetchKeyWithinBoundDesc() {
	projects, err := s.ctx.FindProjects("projectD", 1, -1)
	s.NoError(err)
	s.Len(projects, 1)
}

func (s *ProjectConnectorGetSuite) TestFetchKeyOutOfBoundAsc() {
	projects, err := s.ctx.FindProjects("zzz", 1, 1)
	s.NoError(err)
	s.Len(projects, 0)
}

func (s *ProjectConnectorGetSuite) TestFetchKeyOutOfBoundDesc() {
	projects, err := s.ctx.FindProjects("aaa", 1, -1)
	s.NoError(err)
	s.Len(projects, 0)
}

func (s *ProjectConnectorGetSuite) TestGetProjectEvents() {
	events, err := s.ctx.GetProjectEventLog(projectId, time.Now(), 0)
	s.NoError(err)
	s.Equal(projEventCount, len(events))
}

func (s *ProjectConnectorGetSuite) TestGetProjectWithCommitQueueByOwnerRepoAndBranch() {
	projRef, err := s.ctx.GetProjectWithCommitQueueByOwnerRepoAndBranch("octocat", "hello-world", "main")
	s.NoError(err)
	s.Nil(projRef)

	projRef, err = s.ctx.GetProjectWithCommitQueueByOwnerRepoAndBranch("evergreen-ci", "evergreen", "main")
	s.NoError(err)
	s.NotNil(projRef)
}

func (s *ProjectConnectorGetSuite) TestGetProjectSettings() {
	projRef := &model.ProjectRef{
		Owner:   "admin",
		Enabled: utility.TruePtr(),
		Private: utility.TruePtr(),
		Id:      projectId,
		Admins:  []string{},
		Repo:    "SomeRepo",
	}
	projectSettingsEvent, err := s.ctx.GetProjectSettings(projRef)
	s.NoError(err)
	s.NotNil(projectSettingsEvent)
}

func (s *ProjectConnectorGetSuite) TestGetProjectSettingsNoRepo() {
	projRef := &model.ProjectRef{
		Owner:   "admin",
		Enabled: utility.TruePtr(),
		Private: utility.TruePtr(),
		Id:      projectId,
		Admins:  []string{},
	}
	projectSettingsEvent, err := s.ctx.GetProjectSettings(projRef)
	s.NotNil(err)
	s.Nil(projectSettingsEvent)
}

func (s *ProjectConnectorGetSuite) TestFindProjectVarsById() {
	// redact private variables
	res, err := s.ctx.FindProjectVarsById(projectId, "", true)
	s.NoError(err)
	s.Require().NotNil(res)
	s.Equal("1", res.Vars["a"])
	s.Equal("", res.Vars["b"])
	s.True(res.PrivateVars["b"])

	// not redacted
	res, err = s.ctx.FindProjectVarsById(projectId, "", false)
	s.NoError(err)
	s.Require().NotNil(res)
	s.Equal("1", res.Vars["a"])
	s.Equal("3", res.Vars["b"])
	s.Equal("", res.Vars["c"])

	// test with repo
	res, err = s.ctx.FindProjectVarsById(projectId, repoProjectId, true)
	s.NoError(err)
	s.Require().NotNil(res)
	s.Equal("1", res.Vars["a"])
	s.Equal("", res.Vars["b"])
	s.True(res.PrivateVars["b"])
	s.False(res.PrivateVars["a"])
	s.Equal("new", res.Vars["c"])

	res, err = s.ctx.FindProjectVarsById("", repoProjectId, true)
	s.NoError(err)
	s.Equal("", res.Vars["a"])
	s.Equal("new", res.Vars["c"])
	s.True(res.PrivateVars["a"])

	res, err = s.ctx.FindProjectVarsById("", repoProjectId, false)
	s.NoError(err)
	s.Equal("a_from_repo", res.Vars["a"])
	s.Equal("", res.Vars["b"])
	s.Equal("new", res.Vars["c"])
	s.True(res.PrivateVars["a"])

	_, err = s.ctx.FindProjectVarsById("non-existent", "also-non-existent", false)
	s.Error(err)
}

func (s *ProjectConnectorGetSuite) TestUpdateProjectVars() {
	//successful update
	varsToDelete := []string{"a"}
	newVars := restModel.APIProjectVars{
		Vars:         map[string]string{"b": "2", "c": "3"},
		PrivateVars:  map[string]bool{"b": false, "c": true},
		VarsToDelete: varsToDelete,
	}
	s.NoError(s.ctx.UpdateProjectVars(projectId, &newVars, false))
	s.Equal(newVars.Vars["b"], "") // can't unredact previously redacted  variables
	s.Equal(newVars.Vars["c"], "")
	_, ok := newVars.Vars["a"]
	s.False(ok)

	s.Equal(newVars.PrivateVars["b"], true)
	s.Equal(newVars.PrivateVars["c"], true)
	_, ok = newVars.PrivateVars["a"]
	s.False(ok)

	// successful upsert
	s.NoError(s.ctx.UpdateProjectVars("not-an-id", &newVars, false))
}

func TestUpdateProjectVarsByValue(t *testing.T) {
	require.NoError(t, db.ClearCollections(model.ProjectVarsCollection, event.AllLogCollection))
	dc := &DBProjectConnector{}

	vars := &model.ProjectVars{
		Id:          projectId,
		Vars:        map[string]string{"a": "1", "b": "3"},
		PrivateVars: map[string]bool{"b": true},
	}
	require.NoError(t, vars.Insert())

	resp, err := dc.UpdateProjectVarsByValue("1", "11", "user", true)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "a", resp[projectId])

	res, err := dc.FindProjectVarsById(projectId, "", false)
	assert.NoError(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, "1", res.Vars["a"])

	resp, err = dc.UpdateProjectVarsByValue("1", "11", username, false)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "a", resp[projectId])

	res, err = dc.FindProjectVarsById(projectId, "", false)
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
	s.NoError(s.ctx.CopyProjectVars(projectId, "project-copy"))
	origProj, err := s.ctx.FindProjectVarsById(projectId, "", false)
	s.NoError(err)

	newProj, err := s.ctx.FindProjectVarsById("project-copy", "", false)
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

	dc := &DBProjectConnector{}
	variantTasks, err := dc.GetProjectAliasResults(&p, alias1.Alias, false)
	assert.NoError(t, err)
	assert.Len(t, variantTasks, 1)
	assert.Len(t, variantTasks[0].Tasks, 1)
	assert.Equal(t, "task1", variantTasks[0].Tasks[0])
	variantTasks, err = dc.GetProjectAliasResults(&p, alias2.Alias, false)
	assert.NoError(t, err)
	assert.Len(t, variantTasks, 1)
	assert.Len(t, variantTasks[0].Tasks, 2)
}

func TestSaveProjectSettingsForSection(t *testing.T) {
	dc := &DBProjectConnector{}
	ctx := context.Background()
	rm := evergreen.GetEnvironment().RoleManager()

	for name, test := range map[string]func(t *testing.T, ref model.ProjectRef){
		model.ProjectRefGeneralSection: func(t *testing.T, ref model.ProjectRef) {
			assert.Empty(t, ref.SpawnHostScriptPath)

			ref.SpawnHostScriptPath = "my script path"
			apiProjectRef := restModel.APIProjectRef{}
			assert.NoError(t, apiProjectRef.BuildFromService(ref))
			apiChanges := &restModel.APIProjectSettings{
				ProjectRef: apiProjectRef,
			}
			// ensure that we're saving settings without a special case
			assert.NoError(t, dc.SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectRefGeneralSection, "me"))
			pRefFromDB, err := model.FindOneProjectRef(ref.Id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDB)
			assert.NotEmpty(t, pRefFromDB.SpawnHostScriptPath)
		},
		model.ProjectRefAccessSection: func(t *testing.T, ref model.ProjectRef) {
			newAdmin := user.DBUser{
				Id: "newAdmin",
			}
			require.NoError(t, newAdmin.Insert())
			ref.Restricted = nil // should now default to the repo value
			ref.Admins = []string{"oldAdmin", newAdmin.Id}
			apiProjectRef := restModel.APIProjectRef{}
			assert.NoError(t, apiProjectRef.BuildFromService(ref))
			apiChanges := &restModel.APIProjectSettings{
				ProjectRef: apiProjectRef,
			}
			assert.NoError(t, dc.SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectRefAccessSection, "me"))
			pRefFromDB, err := model.FindOneProjectRef(ref.Id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDB)
			assert.Nil(t, pRefFromDB.Restricted)
			assert.Equal(t, pRefFromDB.Admins, ref.Admins)

			mergedProject, err := model.FindMergedProjectRef(ref.Id)
			assert.NoError(t, err)
			assert.NotNil(t, mergedProject)
			assert.True(t, mergedProject.IsRestricted())

			restrictedScope, err := rm.GetScope(ctx, evergreen.RestrictedProjectsScope)
			assert.NoError(t, err)
			assert.NotNil(t, restrictedScope)
			assert.Contains(t, restrictedScope.Resources, ref.Id)

			unrestrictedScope, err := rm.GetScope(ctx, evergreen.UnrestrictedProjectsScope)
			assert.NoError(t, err)
			assert.NotNil(t, unrestrictedScope)
			assert.NotContains(t, unrestrictedScope.Resources, ref.Id)

			newAdminFromDB, err := user.FindOneById("newAdmin")
			assert.NoError(t, err)
			assert.NotNil(t, newAdminFromDB)
			assert.Contains(t, newAdminFromDB.Roles(), "admin")
		},
		model.ProjectRefVariablesSection: func(t *testing.T, ref model.ProjectRef) {
			// remove a variable, modify a variable, add a variable
			updatedVars := &model.ProjectVars{
				Id:          ref.Id,
				Vars:        map[string]string{"it": "me", "banana": "phone"},
				PrivateVars: map[string]bool{"banana": true},
			}
			apiProjectVars := restModel.APIProjectVars{}
			assert.NoError(t, apiProjectVars.BuildFromService(updatedVars))
			apiChanges := &restModel.APIProjectSettings{
				Vars: apiProjectVars,
			}
			assert.NoError(t, dc.SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectRefVariablesSection, "me"))
			varsFromDb, err := model.FindOneProjectVars(updatedVars.Id)
			assert.NoError(t, err)
			assert.NotNil(t, varsFromDb)
			assert.Equal(t, varsFromDb.Vars["it"], "me")
			assert.Equal(t, varsFromDb.Vars["banana"], "phone")
			assert.Equal(t, varsFromDb.Vars["hello"], "")
			assert.False(t, varsFromDb.PrivateVars["it"])
			assert.False(t, varsFromDb.PrivateVars["hello"])
			assert.True(t, varsFromDb.PrivateVars["banana"])
		},
	} {
		assert.NoError(t, db.ClearCollections(model.ProjectRefCollection, model.ProjectVarsCollection,
			event.SubscriptionsCollection, event.AllLogCollection, evergreen.ScopeCollection, user.Collection))
		_ = evergreen.GetEnvironment().DB().RunCommand(nil, map[string]string{"create": evergreen.ScopeCollection})

		pRef := model.ProjectRef{
			Id:              "myId",
			Owner:           "evergreen-ci",
			Repo:            "evergreen",
			Restricted:      utility.FalsePtr(),
			UseRepoSettings: true,
			RepoRefId:       "myRepoId",
			Admins:          []string{"oldAdmin"},
		}
		assert.NoError(t, pRef.Insert())
		repoRef := model.RepoRef{ProjectRef: model.ProjectRef{
			Id:         pRef.RepoRefId,
			Restricted: utility.TruePtr(),
		}}
		assert.NoError(t, repoRef.Upsert())

		pVars := model.ProjectVars{
			Id:          pRef.Id,
			Vars:        map[string]string{"hello": "world", "it": "adele"},
			PrivateVars: map[string]bool{"hello": true},
		}
		assert.NoError(t, pVars.Insert())
		// add scopes
		allProjectsScope := gimlet.Scope{
			ID:        evergreen.AllProjectsScope,
			Resources: []string{},
		}
		assert.NoError(t, rm.AddScope(allProjectsScope))
		restrictedScope := gimlet.Scope{
			ID:          evergreen.RestrictedProjectsScope,
			Resources:   []string{repoRef.Id},
			ParentScope: evergreen.AllProjectsScope,
		}
		assert.NoError(t, rm.AddScope(restrictedScope))
		unrestrictedScope := gimlet.Scope{
			ID:          evergreen.UnrestrictedProjectsScope,
			Resources:   []string{pRef.Id},
			ParentScope: evergreen.AllProjectsScope,
		}
		assert.NoError(t, rm.AddScope(unrestrictedScope))
		adminScope := gimlet.Scope{
			ID:        "project_scope",
			Resources: []string{pRef.Id},
			Type:      evergreen.ProjectResourceType,
		}
		assert.NoError(t, rm.AddScope(adminScope))

		adminRole := gimlet.Role{
			ID:    "admin",
			Scope: adminScope.ID,
			Permissions: gimlet.Permissions{
				evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value,
				evergreen.PermissionTasks:           evergreen.TasksAdmin.Value,
				evergreen.PermissionPatches:         evergreen.PatchSubmit.Value,
				evergreen.PermissionLogs:            evergreen.LogsView.Value,
			},
		}
		require.NoError(t, rm.UpdateRole(adminRole))
		oldAdmin := user.DBUser{
			Id:          "oldAdmin",
			SystemRoles: []string{"admin"},
		}
		require.NoError(t, oldAdmin.Insert())

		t.Run(name, func(t *testing.T) {
			test(t, pRef)
		})
	}

}
