package data

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch patch by project route

type ProjectConnectorGetSuite struct {
	ctx      DBProjectConnector
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	evergreen.SetEnvironment(env)
	s.setup = func() error {
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
		after.GithubHooksEnabled = false

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

func (s *ProjectConnectorGetSuite) SetupSuite() { s.Require().NoError(s.setup()) }

func (s *ProjectConnectorGetSuite) TearDownSuite() {
	s.Require().NoError(s.teardown())
}

func (s *ProjectConnectorGetSuite) TestFetchTooManyAsc() {
	projects, err := model.FindProjects("", 8, 1)
	s.NoError(err)
	s.NotNil(projects)
	s.Len(projects, 7)
}

func (s *ProjectConnectorGetSuite) TestFetchTooManyDesc() {
	projects, err := model.FindProjects("zzz", 8, -1)
	s.NoError(err)
	s.NotNil(projects)
	s.Len(projects, 7)
}

func (s *ProjectConnectorGetSuite) TestFetchExactNumber() {
	projects, err := model.FindProjects("", 3, 1)
	s.NoError(err)
	s.NotNil(projects)
	s.Len(projects, 3)
}

func (s *ProjectConnectorGetSuite) TestFetchTooFewAsc() {
	projects, err := model.FindProjects("", 2, 1)
	s.NoError(err)
	s.NotNil(projects)
	s.Len(projects, 2)
}

func (s *ProjectConnectorGetSuite) TestFetchTooFewDesc() {
	projects, err := model.FindProjects("zzz", 2, -1)
	s.NoError(err)
	s.NotNil(projects)
	s.Len(projects, 2)
}

func (s *ProjectConnectorGetSuite) TestFetchKeyWithinBoundAsc() {
	projects, err := model.FindProjects("projectB", 1, 1)
	s.NoError(err)
	s.Len(projects, 1)
}

func (s *ProjectConnectorGetSuite) TestFetchKeyWithinBoundDesc() {
	projects, err := model.FindProjects("projectD", 1, -1)
	s.NoError(err)
	s.Len(projects, 1)
}

func (s *ProjectConnectorGetSuite) TestFetchKeyOutOfBoundAsc() {
	projects, err := model.FindProjects("zzz", 1, 1)
	s.NoError(err)
	s.Len(projects, 0)
}

func (s *ProjectConnectorGetSuite) TestFetchKeyOutOfBoundDesc() {
	projects, err := model.FindProjects("aaa", 1, -1)
	s.NoError(err)
	s.Len(projects, 0)
}

func (s *ProjectConnectorGetSuite) TestGetProjectEvents() {
	events, err := GetProjectEventLog(projectId, time.Now(), 0)
	s.NoError(err)
	s.Equal(projEventCount, len(events))
}

func (s *ProjectConnectorGetSuite) TestGetProjectWithCommitQueueByOwnerRepoAndBranch() {
	projRef, err := model.FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch("octocat", "hello-world", "main")
	s.NoError(err)
	s.Nil(projRef)

	projRef, err = model.FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch("evergreen-ci", "evergreen", "main")
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
	projectSettingsEvent, err := model.GetProjectSettings(projRef)
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
	projectSettingsEvent, err := model.GetProjectSettings(projRef)
	s.Nil(err)
	s.NotNil(projectSettingsEvent)
	s.False(projectSettingsEvent.GithubHooksEnabled)
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
		Vars:         map[string]string{"b": "2", "c": "3"},
		PrivateVars:  map[string]bool{"b": false, "c": true},
		VarsToDelete: varsToDelete,
	}
	s.NoError(UpdateProjectVars(projectId, &newVars, false))
	s.Equal(newVars.Vars["b"], "") // can't unredact previously redacted  variables
	s.Equal(newVars.Vars["c"], "")
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	evergreen.SetEnvironment(env)
	require.NoError(t, db.ClearCollections(model.ProjectVarsCollection, event.AllLogCollection))

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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	evergreen.SetEnvironment(env)
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
