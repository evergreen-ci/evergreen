package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/testutil"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	mgobson "gopkg.in/mgo.v2/bson"
)

func TestFindOneProjectRef(t *testing.T) {
	evergreen.GetEnvironment().Settings().LoggerConfig.DefaultLogger = "buildlogger"
	assert := assert.New(t)
	require.NoError(t, db.Clear(ProjectRefCollection),
		"Error clearing collection")
	projectRef := &ProjectRef{
		Owner:     "mongodb",
		Repo:      "mci",
		Branch:    "main",
		Enabled:   utility.TruePtr(),
		BatchTime: 10,
		Id:        "ident",
	}
	assert.Nil(projectRef.Insert())

	projectRefFromDB, err := FindBranchProjectRef("ident")
	assert.Nil(err)
	assert.NotNil(projectRefFromDB)

	assert.Equal(projectRefFromDB.Owner, "mongodb")
	assert.Equal(projectRefFromDB.Repo, "mci")
	assert.Equal(projectRefFromDB.Branch, "main")
	assert.True(projectRefFromDB.IsEnabled())
	assert.Equal(projectRefFromDB.BatchTime, 10)
	assert.Equal(projectRefFromDB.Id, "ident")
	assert.Equal(projectRefFromDB.DefaultLogger, "buildlogger")
}

func TestFindMergedProjectRef(t *testing.T) {
	require.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection, ParserProjectCollection, VersionCollection),
		"Error clearing collection")

	v1 := Version{
		Id:         "ident",
		Identifier: "ident",
		Requester:  evergreen.GitTagRequester,
	}
	assert.NoError(t, v1.Insert())

	parserProject := &ParserProject{
		Id:                 "ident",
		DeactivatePrevious: utility.TruePtr(),
		TaskAnnotationSettings: &evergreen.AnnotationsSettings{
			FileTicketWebHook: evergreen.WebHook{
				Endpoint: "random2",
			},
		},
	}
	assert.NoError(t, parserProject.Insert())

	projectRef := &ProjectRef{
		Owner:                 "mongodb",
		RepoRefId:             "mongodb_mci",
		BatchTime:             10,
		Id:                    "ident",
		Admins:                []string{"john.smith", "john.doe"},
		UseRepoSettings:       true,
		Enabled:               utility.FalsePtr(),
		PatchingDisabled:      utility.FalsePtr(),
		RepotrackerDisabled:   utility.TruePtr(),
		DeactivatePrevious:    utility.FalsePtr(),
		PRTestingEnabled:      nil,
		GitTagVersionsEnabled: nil,
		GitTagAuthorizedTeams: []string{},
		PatchTriggerAliases: []patch.PatchTriggerDefinition{
			{ChildProject: "a different branch"},
		},
		CommitQueue:       CommitQueueParams{Enabled: nil, Message: "using repo commit queue"},
		WorkstationConfig: WorkstationConfig{GitClone: utility.TruePtr()},
		TaskSync:          TaskSyncOptions{ConfigEnabled: utility.FalsePtr()},
	}
	assert.NoError(t, projectRef.Insert())
	repoRef := &RepoRef{ProjectRef{
		Id:                    "mongodb_mci",
		Repo:                  "mci",
		Branch:                "main",
		SpawnHostScriptPath:   "my-path",
		Admins:                []string{"john.liu"},
		Enabled:               utility.TruePtr(),
		PatchingDisabled:      nil,
		GitTagVersionsEnabled: utility.FalsePtr(),
		PRTestingEnabled:      utility.TruePtr(),
		GitTagAuthorizedTeams: []string{"my team"},
		GitTagAuthorizedUsers: []string{"my user"},
		PatchTriggerAliases: []patch.PatchTriggerDefinition{
			{Alias: "global patch trigger"},
		},
		TaskSync:          TaskSyncOptions{ConfigEnabled: utility.TruePtr(), PatchEnabled: utility.TruePtr()},
		CommitQueue:       CommitQueueParams{Enabled: utility.TruePtr()},
		WorkstationConfig: WorkstationConfig{SetupCommands: []WorkstationSetupCommand{{Command: "my-command"}}},
	}}
	assert.NoError(t, repoRef.Upsert())

	mergedProject, err := FindMergedProjectRef("ident", "ident", true)
	assert.NoError(t, err)
	require.NotNil(t, mergedProject)
	assert.Equal(t, "ident", mergedProject.Id)
	require.Len(t, mergedProject.Admins, 2)
	assert.Contains(t, mergedProject.Admins, "john.smith")
	assert.Contains(t, mergedProject.Admins, "john.doe")
	assert.NotContains(t, mergedProject.Admins, "john.liu")
	assert.False(t, *mergedProject.Enabled)
	assert.False(t, mergedProject.IsPatchingDisabled())
	assert.True(t, mergedProject.UseRepoSettings)
	assert.True(t, mergedProject.IsRepotrackerDisabled())
	assert.False(t, mergedProject.IsGitTagVersionsEnabled())
	assert.False(t, mergedProject.IsGithubChecksEnabled())
	assert.True(t, mergedProject.IsPRTestingEnabled())
	assert.Equal(t, "my-path", mergedProject.SpawnHostScriptPath)
	assert.False(t, utility.FromBoolPtr(mergedProject.TaskSync.ConfigEnabled))
	assert.True(t, utility.FromBoolPtr(mergedProject.TaskSync.PatchEnabled))
	assert.Len(t, mergedProject.GitTagAuthorizedTeams, 0) // empty lists take precedent
	assert.Len(t, mergedProject.GitTagAuthorizedUsers, 1)
	require.Len(t, mergedProject.PatchTriggerAliases, 1)
	assert.Empty(t, mergedProject.PatchTriggerAliases[0].Alias)
	assert.Equal(t, "a different branch", mergedProject.PatchTriggerAliases[0].ChildProject)

	assert.True(t, mergedProject.CommitQueue.IsEnabled())
	assert.Equal(t, "using repo commit queue", mergedProject.CommitQueue.Message)

	assert.True(t, mergedProject.WorkstationConfig.ShouldGitClone())
	assert.Len(t, mergedProject.WorkstationConfig.SetupCommands, 1)
	assert.True(t, *mergedProject.DeactivatePrevious)
	assert.Equal(t, "random2", mergedProject.TaskAnnotationSettings.FileTicketWebHook.Endpoint)
}

func TestGetBatchTimeDoesNotExceedMaxBatchTime(t *testing.T) {
	assert := assert.New(t)

	projectRef := &ProjectRef{
		Owner:     "mongodb",
		Repo:      "mci",
		Branch:    "main",
		Enabled:   utility.TruePtr(),
		BatchTime: maxBatchTime + 1,
		Id:        "ident",
	}

	emptyVariant := &BuildVariant{}
	emptyTask := &BuildVariantTaskUnit{}

	assert.Equal(projectRef.getBatchTimeForVariant(emptyVariant), maxBatchTime,
		"ProjectRef.getBatchTimeForVariant() is not capping BatchTime to MaxInt32")

	assert.Equal(projectRef.getBatchTimeForTask(emptyTask), maxBatchTime,
		"ProjectRef.getBatchTimeForTask() is not capping BatchTime to MaxInt32")

	projectRef.BatchTime = 55
	assert.Equal(projectRef.getBatchTimeForVariant(emptyVariant), 55,
		"ProjectRef.getBatchTimeForVariant() is not returning the correct BatchTime")

	assert.Equal(projectRef.getBatchTimeForTask(emptyTask), 55,
		"ProjectRef.getBatchTimeForVariant() is not returning the correct BatchTime")

}

func TestGetActivationTimeForTask(t *testing.T) {
	assert.NoError(t, db.ClearCollections(VersionCollection))
	prevTime := time.Date(2020, time.June, 9, 0, 0, 0, 0, time.UTC) // Tuesday
	batchTime := 60
	projectRef := &ProjectRef{Id: "mci"}
	bvt := &BuildVariantTaskUnit{
		BatchTime: &batchTime,
		Name:      "myTask",
		Variant:   "bv1",
	}

	versionWithoutTask := Version{
		Id:         "v1",
		Identifier: projectRef.Id,
		Requester:  evergreen.RepotrackerVersionRequester,
		BuildVariants: []VersionBuildStatus{
			{
				BuildVariant:     "bv1",
				ActivationStatus: ActivationStatus{Activated: true, ActivateAt: time.Now()},
				BatchTimeTasks: []BatchTimeTaskStatus{
					{
						TaskName:         "a different task",
						ActivationStatus: ActivationStatus{ActivateAt: time.Now(), Activated: true},
					},
				},
			},
		},
	}
	versionWithTask := Version{
		Id:         "v2",
		Identifier: projectRef.Id,
		Requester:  evergreen.RepotrackerVersionRequester,
		BuildVariants: []VersionBuildStatus{
			{
				BuildVariant:     "bv1",
				ActivationStatus: ActivationStatus{Activated: false, ActivateAt: prevTime.Add(-1 * time.Hour)},
				BatchTimeTasks: []BatchTimeTaskStatus{
					{
						TaskName:         "myTask",
						ActivationStatus: ActivationStatus{ActivateAt: prevTime, Activated: true},
					},
					{
						TaskName:         "notMyTask",
						ActivationStatus: ActivationStatus{ActivateAt: time.Now(), Activated: true},
					},
				},
			},
			{
				BuildVariant:     "bv_unrelated",
				ActivationStatus: ActivationStatus{Activated: true, ActivateAt: time.Now()},
			},
		},
	}
	assert.NoError(t, versionWithoutTask.Insert())
	assert.NoError(t, versionWithTask.Insert())

	activationTime, err := projectRef.GetActivationTimeForTask(bvt)
	assert.NoError(t, err)
	assert.True(t, activationTime.Equal(prevTime.Add(time.Hour)))
}

func TestGetActivationTimeWithCron(t *testing.T) {
	prevTime := time.Date(2020, time.June, 9, 0, 0, 0, 0, time.UTC) // Tuesday
	for name, test := range map[string]func(t *testing.T){
		"Empty": func(t *testing.T) {
			_, err := GetActivationTimeWithCron(prevTime, "")
			assert.Error(t, err)
		},
		"InvalidBatchSyntax": func(t *testing.T) {
			batchStr := "* * *"
			_, err := GetActivationTimeWithCron(prevTime, batchStr)
			assert.Error(t, err)
		},
		"EveryHourEveryDay": func(t *testing.T) {
			batchStr := "0 * * * *"
			res, err := GetActivationTimeWithCron(prevTime, batchStr)
			assert.NoError(t, err)
			assert.Equal(t, prevTime.Add(time.Hour), res)
		},
		"SpecifyDOW": func(t *testing.T) {
			batchStr := "0 0 ? * MON,WED,FRI"
			res, err := GetActivationTimeWithCron(prevTime, batchStr)
			assert.NoError(t, err)
			assert.Equal(t, prevTime.Add(time.Hour*24), res) // i.e. Wednesday

			newRes, err := GetActivationTimeWithCron(res, batchStr) // i.e. Friday
			assert.NoError(t, err)
			assert.Equal(t, res.Add(time.Hour*48), newRes)
		},
		"1and15thOfTheMonth": func(t *testing.T) {
			batchStr := "0 0 1,15 *"
			res, err := GetActivationTimeWithCron(prevTime, batchStr)
			assert.NoError(t, err)
			assert.Equal(t, prevTime.Add(time.Hour*24*6), res)
		},
		"Descriptor": func(t *testing.T) {
			batchStr := "@daily"
			res, err := GetActivationTimeWithCron(prevTime, batchStr)
			assert.NoError(t, err)
			assert.Equal(t, prevTime.Add(time.Hour*24), res)
		},
		"Interval": func(t *testing.T) {
			batchStr := "@every 2h"
			_, err := GetActivationTimeWithCron(prevTime, batchStr)
			assert.Error(t, err)
		},
	} {
		t.Run(name, test)
	}
}

func TestChangeOwnerRepo(t *testing.T) {
	require.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection, evergreen.ScopeCollection,
		evergreen.RoleCollection, user.Collection, evergreen.ConfigCollection))
	env := evergreen.GetEnvironment()
	_ = env.DB().RunCommand(nil, map[string]string{"create": evergreen.ScopeCollection})
	settings := testutil.TestConfig()
	settings.GithubOrgs = []string{"evergreen-ci"}
	settings.GithubOrgs = []string{"newOwner"}
	assert.NoError(t, evergreen.UpdateConfig(settings))

	evergreen.SetEnvironment(env)
	pRef := ProjectRef{
		Id:              "myProject",
		Owner:           "evergreen-ci",
		Repo:            "evergreen",
		Admins:          []string{"me"},
		RepoRefId:       "myRepo",
		UseRepoSettings: true,
	}
	assert.NoError(t, pRef.Insert())
	repoRef := RepoRef{ProjectRef{
		Id: "myRepo",
	}}
	assert.NoError(t, repoRef.Upsert())
	u := &user.DBUser{Id: "me",
		SystemRoles: []string{GetViewRepoRole("myRepo")},
	}
	assert.NoError(t, u.Insert())
	pRef.Owner = "newOwner"
	pRef.Repo = "newRepo"
	assert.NoError(t, pRef.ChangeOwnerRepo(u))

	pRefFromDB, err := FindBranchProjectRef(pRef.Id)
	assert.NoError(t, err)
	assert.NotNil(t, pRefFromDB)
	assert.NotEqual(t, pRefFromDB.RepoRefId, "myRepo")
	assert.Equal(t, pRefFromDB.Owner, "newOwner")
	assert.Equal(t, pRefFromDB.Repo, "newRepo")

	userFromDB, err := user.FindOneById("me")
	assert.NoError(t, err)
	assert.Len(t, userFromDB.SystemRoles, 2)
	assert.Contains(t, userFromDB.SystemRoles, GetRepoAdminRole(pRefFromDB.RepoRefId))
	assert.Contains(t, userFromDB.SystemRoles, GetViewRepoRole(pRefFromDB.RepoRefId))
}

func TestAttachToRepo(t *testing.T) {
	require.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection, evergreen.ScopeCollection,
		evergreen.RoleCollection, user.Collection))
	env := evergreen.GetEnvironment()
	_ = env.DB().RunCommand(nil, map[string]string{"create": evergreen.ScopeCollection})

	pRef := ProjectRef{
		Id:     "myProject",
		Owner:  "evergreen-ci",
		Repo:   "evergreen",
		Admins: []string{"me"},
	}
	assert.NoError(t, pRef.Insert())

	u := &user.DBUser{Id: "me"}
	assert.NoError(t, u.Insert())
	assert.NoError(t, pRef.AttachToRepo(u))
	assert.True(t, pRef.UseRepoSettings)
	assert.NotEmpty(t, pRef.RepoRefId)

	pRefFromDB, err := FindBranchProjectRef(pRef.Id)
	assert.NoError(t, err)
	assert.NotNil(t, pRefFromDB)
	assert.True(t, pRefFromDB.UseRepoSettings)
	assert.NotEmpty(t, pRefFromDB.RepoRefId)

	u, err = user.FindOneById("me")
	assert.NoError(t, err)
	assert.NotNil(t, u)
	assert.Contains(t, u.Roles(), GetViewRepoRole(pRefFromDB.RepoRefId))
	assert.Contains(t, u.Roles(), GetRepoAdminRole(pRefFromDB.RepoRefId))
}

func TestDetachFromRepo(t *testing.T) {
	for name, test := range map[string]func(t *testing.T, pRef *ProjectRef, dbUser *user.DBUser){
		"project ref is updated correctly": func(t *testing.T, pRef *ProjectRef, dbUser *user.DBUser) {
			assert.NoError(t, pRef.DetachFromRepo(dbUser))

			pRefFromDB, err := FindBranchProjectRef(pRef.Id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDB)
			assert.False(t, pRefFromDB.UseRepoSettings)
			assert.Empty(t, pRefFromDB.RepoRefId)
			assert.NotNil(t, pRefFromDB.PRTestingEnabled)
			assert.False(t, pRefFromDB.IsPRTestingEnabled())
			assert.NotNil(t, pRefFromDB.GitTagVersionsEnabled)
			assert.True(t, pRefFromDB.IsGitTagVersionsEnabled())
			assert.True(t, pRefFromDB.IsGithubChecksEnabled())
			assert.Equal(t, pRefFromDB.GithubTriggerAliases, []string{"my_trigger"}) // why isn't this set to repo :O

			dbUser, err = user.FindOneById("me")
			assert.NoError(t, err)
			assert.NotNil(t, dbUser)
			assert.NotContains(t, dbUser.Roles(), GetViewRepoRole(pRefFromDB.RepoRefId))
		},
		"project variables are updated": func(t *testing.T, pRef *ProjectRef, dbUser *user.DBUser) {
			assert.NoError(t, pRef.DetachFromRepo(dbUser))

			vars, err := FindOneProjectVars(pRef.Id)
			assert.NoError(t, err)
			assert.NotNil(t, vars)
			assert.Equal(t, vars.Vars["project"], "only")
			assert.Equal(t, vars.Vars["in"], "both")    // not modified
			assert.Equal(t, vars.Vars["repo"], "only!") // added from repo
			assert.False(t, vars.PrivateVars["project"])
			assert.True(t, vars.PrivateVars["in"])
			assert.True(t, vars.PrivateVars["repo"]) // added from repo
		},
		"patch aliases": func(t *testing.T, pRef *ProjectRef, dbUser *user.DBUser) {
			// no patch aliases are copied if the project has a patch alias
			projectAlias := ProjectAlias{Alias: "myProjectAlias", ProjectID: pRef.Id}
			assert.NoError(t, projectAlias.Upsert())

			repoAlias := ProjectAlias{Alias: "myRepoAlias", ProjectID: pRef.RepoRefId}
			assert.NoError(t, repoAlias.Upsert())

			assert.NoError(t, pRef.DetachFromRepo(dbUser))
			aliases, err := FindAllAliasesForProject(pRef.Id)
			assert.NoError(t, err)
			assert.Len(t, aliases, 1)
			assert.Equal(t, aliases[0].Alias, projectAlias.Alias)

			// reattach to repo to test without project patch aliases
			assert.NoError(t, pRef.AttachToRepo(dbUser))
			assert.NotEmpty(t, pRef.RepoRefId)
			assert.True(t, pRef.UseRepoSettings)
			assert.NoError(t, RemoveProjectAlias(projectAlias.ID.Hex()))

			assert.NoError(t, pRef.DetachFromRepo(dbUser))
			aliases, err = FindAllAliasesForProject(pRef.Id)
			assert.NoError(t, err)
			assert.Len(t, aliases, 1)
			assert.Equal(t, aliases[0].Alias, repoAlias.Alias)

		},
		"internal aliases": func(t *testing.T, pRef *ProjectRef, dbUser *user.DBUser) {
			projectAliases := []ProjectAlias{
				{Alias: evergreen.GitTagAlias, Variant: "projectVariant"},
				{Alias: evergreen.CommitQueueAlias},
			}
			assert.NoError(t, UpsertAliasesForProject(projectAliases, pRef.Id))
			repoAliases := []ProjectAlias{
				{Alias: evergreen.GitTagAlias, Variant: "repoVariant"},
				{Alias: evergreen.GithubPRAlias},
			}
			assert.NoError(t, UpsertAliasesForProject(repoAliases, pRef.RepoRefId))

			assert.NoError(t, pRef.DetachFromRepo(dbUser))
			aliases, err := FindAllAliasesForProject(pRef.Id)
			assert.NoError(t, err)
			assert.Len(t, aliases, 3)
			gitTagCount := 0
			prCount := 0
			cqCount := 0
			for _, a := range aliases {
				if a.Alias == evergreen.GitTagAlias {
					gitTagCount += 1
					assert.Equal(t, a.Variant, projectAliases[0].Variant) // wasn't overwritten by repo
				}
				if a.Alias == evergreen.GithubPRAlias {
					prCount += 1
				}
				if a.Alias == evergreen.CommitQueueAlias {
					cqCount += 1
				}
			}
			assert.Equal(t, gitTagCount, 1)
			assert.Equal(t, prCount, 1)
			assert.Equal(t, cqCount, 1)
		},
		"subscriptions": func(t *testing.T, pRef *ProjectRef, dbUser *user.DBUser) {
			projectSubscription := event.Subscription{
				Owner:        pRef.Id,
				OwnerType:    event.OwnerTypeProject,
				ResourceType: event.ResourceTypeTask,
				Trigger:      event.TriggerOutcome,
				Selectors: []event.Selector{
					{Type: "id", Data: "1234"},
				},
				Subscriber: event.Subscriber{
					Type:   event.EmailSubscriberType,
					Target: "a@domain.invalid",
				},
			}
			assert.NoError(t, projectSubscription.Upsert())
			repoSubscription := event.Subscription{
				Owner:        pRef.RepoRefId,
				OwnerType:    event.OwnerTypeProject,
				ResourceType: event.ResourceTypeTask,
				Trigger:      event.TriggerFailure,
				Selectors: []event.Selector{
					{Type: "id", Data: "1234"},
				},
				Subscriber: event.Subscriber{
					Type:   event.EmailSubscriberType,
					Target: "a@domain.invalid",
				},
			}
			assert.NoError(t, repoSubscription.Upsert())
			assert.NoError(t, pRef.DetachFromRepo(dbUser))

			subs, err := event.FindSubscriptionsByOwner(pRef.Id, event.OwnerTypeProject)
			assert.NoError(t, err)
			require.Len(t, subs, 1)
			assert.Equal(t, subs[0].Owner, pRef.Id)
			assert.Equal(t, subs[0].Trigger, event.TriggerOutcome)

			// reattach to repo to test without subscription
			assert.NoError(t, pRef.AttachToRepo(dbUser))
			assert.NoError(t, event.RemoveSubscription(projectSubscription.ID))
			assert.NoError(t, pRef.DetachFromRepo(dbUser))

			subs, err = event.FindSubscriptionsByOwner(pRef.Id, event.OwnerTypeProject)
			assert.NoError(t, err)
			assert.Len(t, subs, 1)
			assert.Equal(t, subs[0].Owner, pRef.Id)
			assert.Equal(t, subs[0].Trigger, event.TriggerFailure)
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection, evergreen.ScopeCollection,
				evergreen.RoleCollection, user.Collection, event.SubscriptionsCollection, ProjectAliasCollection))
			env := evergreen.GetEnvironment()
			_ = env.DB().RunCommand(nil, map[string]string{"create": evergreen.ScopeCollection})

			pRef := &ProjectRef{
				Id:              "myProject",
				Owner:           "evergreen-ci",
				Repo:            "evergreen",
				Admins:          []string{"me"},
				UseRepoSettings: true,
				RepoRefId:       "myRepo",

				PeriodicBuilds:        []PeriodicBuildDefinition{}, // also shouldn't be overwritten
				PRTestingEnabled:      utility.FalsePtr(),          // neither of these should be changed when overwriting
				GitTagVersionsEnabled: utility.TruePtr(),
				GithubChecksEnabled:   nil, // for now this is defaulting to repo
				//GithubTriggerAliases:  nil,
			}
			assert.NoError(t, pRef.Insert())

			repoRef := RepoRef{ProjectRef{
				Id:                    pRef.RepoRefId,
				Owner:                 pRef.Owner,
				Repo:                  pRef.Repo,
				PRTestingEnabled:      utility.TruePtr(),
				GitTagVersionsEnabled: utility.FalsePtr(),
				GithubChecksEnabled:   utility.TruePtr(),
				GithubTriggerAliases:  []string{"my_trigger"},
				PeriodicBuilds: []PeriodicBuildDefinition{
					{ID: "my_build"},
				},
			}}
			assert.NoError(t, repoRef.Upsert())

			pVars := &ProjectVars{
				Id: pRef.Id,
				Vars: map[string]string{
					"project": "only",
					"in":      "both",
				},
				PrivateVars: map[string]bool{
					"in": true,
				},
			}
			_, err := pVars.Upsert()
			assert.NoError(t, err)

			repoVars := &ProjectVars{
				Id: repoRef.Id,
				Vars: map[string]string{
					"in":   "also the repo",
					"repo": "only!",
				},
				PrivateVars: map[string]bool{
					"repo": true,
				},
			}
			_, err = repoVars.Upsert()
			assert.NoError(t, err)

			u := &user.DBUser{
				Id:          "me",
				SystemRoles: []string{GetViewRepoRole("myRepo")},
			}
			assert.NoError(t, u.Insert())
			test(t, pRef, u)
		})
	}
}

func TestDefaultRepoBySection(t *testing.T) {
	for name, test := range map[string]func(t *testing.T, id string){
		ProjectPageGeneralSection: func(t *testing.T, id string) {
			assert.NoError(t, DefaultSectionToRepo(id, ProjectPageGeneralSection, "me"))

			pRefFromDb, err := FindBranchProjectRef(id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDb)
			assert.Equal(t, pRefFromDb.BatchTime, 0)
			assert.Nil(t, pRefFromDb.RepotrackerDisabled)
			assert.Nil(t, pRefFromDb.DeactivatePrevious)
			assert.Empty(t, pRefFromDb.RemotePath)
			assert.Nil(t, pRefFromDb.TaskSync.ConfigEnabled)
			assert.Nil(t, pRefFromDb.FilesIgnoredFromCache)
		},
		ProjectPageAccessSection: func(t *testing.T, id string) {
			assert.NoError(t, DefaultSectionToRepo(id, ProjectPageAccessSection, "me"))

			pRefFromDb, err := FindBranchProjectRef(id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDb)
			assert.Nil(t, pRefFromDb.Private)
			assert.Nil(t, pRefFromDb.Restricted)
			assert.Nil(t, pRefFromDb.Admins)
		},
		ProjectPageVariablesSection: func(t *testing.T, id string) {
			assert.NoError(t, DefaultSectionToRepo(id, ProjectPageVariablesSection, "me"))

			varsFromDb, err := FindOneProjectVars(id)
			assert.NoError(t, err)
			assert.NotNil(t, varsFromDb)
			assert.Nil(t, varsFromDb.Vars)
			assert.Nil(t, varsFromDb.PrivateVars)
			assert.Nil(t, varsFromDb.RestrictedVars)
			assert.NotEmpty(t, varsFromDb.Id)
		},
		ProjectPageGithubAndCQSection: func(t *testing.T, id string) {
			aliases, err := FindAllAliasesForProject(id)
			assert.NoError(t, err)
			assert.Len(t, aliases, 5)
			assert.NoError(t, DefaultSectionToRepo(id, ProjectPageGithubAndCQSection, "me"))

			pRefFromDb, err := FindBranchProjectRef(id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDb)
			assert.Nil(t, pRefFromDb.PRTestingEnabled)
			assert.Nil(t, pRefFromDb.GithubChecksEnabled)
			assert.Nil(t, pRefFromDb.GitTagAuthorizedUsers)
			aliases, err = FindAllAliasesForProject(id)
			assert.NoError(t, err)
			assert.Len(t, aliases, 1)
			// assert that only patch aliases are left
			for _, a := range aliases {
				assert.NotContains(t, evergreen.InternalAliases, a.Alias)
			}
		},
		ProjectPageNotificationsSection: func(t *testing.T, id string) {
			assert.NoError(t, DefaultSectionToRepo(id, ProjectPageNotificationsSection, "me"))
			pRefFromDb, err := FindBranchProjectRef(id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDb)
			assert.Nil(t, pRefFromDb.NotifyOnBuildFailure)
		},
		ProjectPagePatchAliasSection: func(t *testing.T, id string) {
			aliases, err := FindAllAliasesForProject(id)
			assert.NoError(t, err)
			assert.Len(t, aliases, 5)

			assert.NoError(t, DefaultSectionToRepo(id, ProjectPagePatchAliasSection, "me"))
			pRefFromDb, err := FindBranchProjectRef(id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDb)
			assert.Nil(t, pRefFromDb.PatchTriggerAliases)

			aliases, err = FindAllAliasesForProject(id)
			assert.NoError(t, err)
			assert.Len(t, aliases, 4)
			// assert that no patch aliases are left
			for _, a := range aliases {
				assert.Contains(t, evergreen.InternalAliases, a.Alias)
			}
		},
		ProjectPageTriggersSection: func(t *testing.T, id string) {
			assert.NoError(t, DefaultSectionToRepo(id, ProjectPageTriggersSection, "me"))
			pRefFromDb, err := FindBranchProjectRef(id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDb)
			assert.Nil(t, pRefFromDb.Triggers)
		},
		ProjectPageWorkstationsSection: func(t *testing.T, id string) {
			assert.NoError(t, DefaultSectionToRepo(id, ProjectPageWorkstationsSection, "me"))
			pRefFromDb, err := FindBranchProjectRef(id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDb)
			assert.Nil(t, pRefFromDb.WorkstationConfig.GitClone)
			assert.Nil(t, pRefFromDb.WorkstationConfig.SetupCommands)
		},
		ProjectPagePeriodicBuildsSection: func(t *testing.T, id string) {
			assert.NoError(t, DefaultSectionToRepo(id, ProjectPagePeriodicBuildsSection, "me"))
			pRefFromDb, err := FindBranchProjectRef(id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDb)
			assert.Nil(t, pRefFromDb.PeriodicBuilds)
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(ProjectRefCollection, ProjectVarsCollection, ProjectAliasCollection,
				event.SubscriptionsCollection, event.AllLogCollection))

			pRef := ProjectRef{
				Id:                    "my_project",
				Owner:                 "candy",
				Repo:                  "land",
				BatchTime:             10,
				RepotrackerDisabled:   utility.TruePtr(),
				DeactivatePrevious:    utility.FalsePtr(),
				RemotePath:            "path.yml",
				TaskSync:              TaskSyncOptions{ConfigEnabled: utility.TruePtr()},
				FilesIgnoredFromCache: []string{},
				Private:               utility.TruePtr(),
				Restricted:            utility.FalsePtr(),
				Admins:                []string{"annie"},
				PRTestingEnabled:      utility.TruePtr(),
				GithubChecksEnabled:   utility.FalsePtr(),
				GitTagAuthorizedUsers: []string{"anna"},
				NotifyOnBuildFailure:  utility.FalsePtr(),
				Triggers: []TriggerDefinition{
					{Project: "your_project"},
				},
				PatchTriggerAliases: []patch.PatchTriggerDefinition{
					{ChildProject: "your_project"},
				},
				WorkstationConfig: WorkstationConfig{
					GitClone: utility.TruePtr(),
					SetupCommands: []WorkstationSetupCommand{
						{Command: "expeliarmus"},
					},
				},
				PeriodicBuilds: []PeriodicBuildDefinition{
					{
						ID:         "so_occasional",
						ConfigFile: "build.yml",
					},
				},
			}
			assert.NoError(t, pRef.Insert())

			pVars := ProjectVars{
				Id:             pRef.Id,
				Vars:           map[string]string{"hello": "world"},
				PrivateVars:    map[string]bool{"hello": true},
				RestrictedVars: map[string]bool{"hello": true},
			}
			assert.NoError(t, pVars.Insert())

			aliases := []ProjectAlias{
				{
					ID:        mgobson.NewObjectId(),
					ProjectID: pRef.Id,
					Alias:     evergreen.GithubPRAlias,
					Variant:   "v",
					Task:      "t",
				},
				{
					ID:        mgobson.NewObjectId(),
					ProjectID: pRef.Id,
					Alias:     evergreen.GitTagAlias,
					Variant:   "v",
					Task:      "t",
				},
				{
					ID:        mgobson.NewObjectId(),
					ProjectID: pRef.Id,
					Alias:     evergreen.CommitQueueAlias,
					Variant:   "v",
					Task:      "t",
				},
				{
					ID:        mgobson.NewObjectId(),
					ProjectID: pRef.Id,
					Alias:     evergreen.GithubChecksAlias,
					Variant:   "v",
					Task:      "t",
				},
				{
					ID:        mgobson.NewObjectId(),
					ProjectID: pRef.Id,
					Alias:     "i am a patch alias!",
					Variant:   "v",
					Task:      "t",
				},
			}
			for _, a := range aliases {
				assert.NoError(t, a.Upsert())
			}
			test(t, pRef.Id)
		})
	}
}

func TestGroupProjectsByRepo(t *testing.T) {
	assert := assert.New(t)
	groupedProjects := GroupProjectsByRepo(
		[]ProjectRef{
			{Id: "projectB", RepoRefId: "mongo"},
			{Id: "projectC", RepoRefId: "mongo"},
			{Id: "projectD", RepoRefId: "mongo"},
			{Id: "projectE", RepoRefId: "gimlet"},
			{Id: "projectF", RepoRefId: "gimlet"},
		},
	)

	assert.Equal(2, len(groupedProjects["gimlet"]))
	assert.Equal(3, len(groupedProjects["mongo"]))

	assert.Equal("projectB", groupedProjects["mongo"][0].Id)
	assert.Equal("projectC", groupedProjects["mongo"][1].Id)
	assert.Equal("projectD", groupedProjects["mongo"][2].Id)

	assert.Equal("projectE", groupedProjects["gimlet"][0].Id)
	assert.Equal("projectF", groupedProjects["gimlet"][1].Id)
}

func TestFindProjectRefsByRepoAndBranch(t *testing.T) {
	evergreen.GetEnvironment().Settings().LoggerConfig.DefaultLogger = "buildlogger"
	assert := assert.New(t)
	require := require.New(t)

	assert.NoError(db.ClearCollections(ProjectRefCollection, RepoRefCollection))

	projectRefs, err := FindMergedEnabledProjectRefsByRepoAndBranch("mongodb", "mci", "main")
	assert.NoError(err)
	assert.Empty(projectRefs)

	projectRef := &ProjectRef{
		Owner:            "mongodb",
		Repo:             "mci",
		Branch:           "main",
		Enabled:          utility.FalsePtr(),
		BatchTime:        10,
		Id:               "iden_",
		PRTestingEnabled: utility.TruePtr(),
	}
	assert.NoError(projectRef.Insert())
	projectRefs, err = FindMergedEnabledProjectRefsByRepoAndBranch("mongodb", "mci", "main")
	assert.NoError(err)
	assert.Empty(projectRefs)

	projectRef.Id = "ident"
	projectRef.Enabled = utility.TruePtr()
	assert.NoError(projectRef.Insert())

	projectRefs, err = FindMergedEnabledProjectRefsByRepoAndBranch("mongodb", "mci", "main")
	assert.NoError(err)
	require.Len(projectRefs, 1)
	assert.Equal("ident", projectRefs[0].Id)
	assert.Equal("buildlogger", projectRefs[0].DefaultLogger)

	projectRef.Id = "ident2"
	assert.NoError(projectRef.Insert())
	projectRefs, err = FindMergedEnabledProjectRefsByRepoAndBranch("mongodb", "mci", "main")
	assert.NoError(err)
	assert.Len(projectRefs, 2)

	projectRef.Id = "uses_repo"
	projectRef.Enabled = nil
	projectRef.RepoRefId = "my_repo"
	projectRef.UseRepoSettings = true
	assert.NoError(projectRef.Insert())

	repoRef := RepoRef{ProjectRef{
		Id:      "my_repo",
		Enabled: utility.FalsePtr(),
	}}
	assert.NoError(repoRef.Upsert())

	projectRefs, err = FindMergedEnabledProjectRefsByRepoAndBranch("mongodb", "mci", "main")
	assert.NoError(err)
	assert.Len(projectRefs, 2)

	repoRef.Enabled = utility.TruePtr()
	assert.NoError(repoRef.Upsert())
	projectRefs, err = FindMergedEnabledProjectRefsByRepoAndBranch("mongodb", "mci", "main")
	assert.NoError(err)
	assert.Len(projectRefs, 3)

	projectRef.Enabled = utility.FalsePtr()
	assert.NoError(projectRef.Upsert())
	projectRefs, err = FindMergedEnabledProjectRefsByRepoAndBranch("mongodb", "mci", "main")
	assert.NoError(err)
	assert.Len(projectRefs, 2)
}

func TestCreateNewRepoRef(t *testing.T) {
	assert.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection, user.Collection,
		evergreen.ScopeCollection, ProjectVarsCollection, ProjectAliasCollection))
	_ = evergreen.GetEnvironment().DB().RunCommand(nil, map[string]string{"create": evergreen.ScopeCollection})
	doc1 := &ProjectRef{
		Id:                    "id1",
		Owner:                 "mongodb",
		Repo:                  "mongo",
		Branch:                "mci",
		Enabled:               utility.TruePtr(),
		FilesIgnoredFromCache: []string{"file1", "file2"},
		Admins:                []string{"bob", "other bob"},
		PRTestingEnabled:      utility.TruePtr(),
		RemotePath:            "evergreen.yml",
		NotifyOnBuildFailure:  utility.TruePtr(),
		CommitQueue:           CommitQueueParams{Message: "my message"},
		TaskSync:              TaskSyncOptions{PatchEnabled: utility.TruePtr()},
	}
	assert.NoError(t, doc1.Insert())
	doc2 := &ProjectRef{
		Id:                    "id2",
		Owner:                 "mongodb",
		Repo:                  "mongo",
		Branch:                "mci2",
		Enabled:               utility.TruePtr(),
		FilesIgnoredFromCache: []string{"file2"},
		Admins:                []string{"bob", "other bob"},
		PRTestingEnabled:      utility.TruePtr(),
		RemotePath:            "evergreen.yml",
		NotifyOnBuildFailure:  utility.FalsePtr(),
		GithubChecksEnabled:   utility.TruePtr(),
		CommitQueue:           CommitQueueParams{Message: "my message"},
		TaskSync:              TaskSyncOptions{PatchEnabled: utility.TruePtr(), ConfigEnabled: utility.TruePtr()},
	}
	assert.NoError(t, doc2.Insert())
	doc3 := &ProjectRef{
		Id:      "id3",
		Owner:   "mongodb",
		Repo:    "mongo",
		Branch:  "mci2",
		Enabled: utility.FalsePtr(),
	}
	assert.NoError(t, doc3.Insert())

	projectVariables := []ProjectVars{
		{
			Id: doc1.Id,
			Vars: map[string]string{
				"hello":        "world",
				"sdc":          "buggy",
				"violets":      "nah",
				"roses":        "red",
				"ever":         "green",
				"also":         "this one",
				"this is only": "in one doc",
			},
			PrivateVars: map[string]bool{
				"sdc": true,
			},
		},
		{
			Id: doc2.Id,
			Vars: map[string]string{
				"hello":   "world",
				"violets": "blue",
				"sdc":     "buggy",
				"ever":    "green",
			},
			RestrictedVars: map[string]bool{
				"ever": true,
			},
		},
		{
			Id: doc3.Id,
			Vars: map[string]string{
				"it's me": "adele",
			},
		},
	}
	for _, vars := range projectVariables {
		assert.NoError(t, vars.Insert())
	}

	projectAliases := ProjectAliases{
		ProjectAlias{
			ProjectID: doc1.Id,
			Task:      ".*",
			Variant:   ".*",
			Alias:     evergreen.GithubPRAlias,
		},
		ProjectAlias{
			ProjectID: doc2.Id,
			Task:      ".*",
			Variant:   ".*",
			Alias:     evergreen.GithubPRAlias,
		},
		ProjectAlias{
			ProjectID: doc1.Id,
			TaskTags:  []string{"t2"},
			Variant:   ".*",
			Alias:     evergreen.GithubChecksAlias,
		},
		ProjectAlias{
			ProjectID: doc2.Id,
			TaskTags:  []string{"t1"},
			Variant:   ".*",
			Alias:     evergreen.GithubChecksAlias,
		},
		ProjectAlias{
			ProjectID:   doc1.Id,
			Task:        ".*",
			VariantTags: []string{"v1"},
			Alias:       evergreen.GitTagAlias,
		},
		ProjectAlias{
			ProjectID:   doc2.Id,
			Task:        ".*",
			VariantTags: []string{"v1"},
			Alias:       evergreen.GitTagAlias,
		},
		ProjectAlias{
			ProjectID:  doc1.Id,
			RemotePath: "random",
			Alias:      "random-alias",
		},
	}
	for _, a := range projectAliases {
		assert.NoError(t, a.Upsert())
	}
	u := user.DBUser{Id: "me"}
	assert.NoError(t, u.Insert())
	// this will create the new repo ref
	assert.NoError(t, doc2.AddToRepoScope(&u))
	assert.NotEmpty(t, doc2.RepoRefId)

	repoRef, err := FindOneRepoRef(doc2.RepoRefId)
	assert.NoError(t, err)
	assert.NotNil(t, repoRef)

	assert.Equal(t, "mongodb", repoRef.Owner)
	assert.Equal(t, "mongo", repoRef.Repo)
	assert.Contains(t, repoRef.Admins, "bob")
	assert.Contains(t, repoRef.Admins, "other bob")
	assert.Contains(t, repoRef.Admins, "me")
	assert.Empty(t, repoRef.FilesIgnoredFromCache)
	assert.True(t, repoRef.IsEnabled())
	assert.True(t, repoRef.IsPRTestingEnabled())
	assert.Equal(t, "evergreen.yml", repoRef.RemotePath)
	assert.Nil(t, repoRef.NotifyOnBuildFailure)
	assert.Nil(t, repoRef.GithubChecksEnabled)
	assert.Equal(t, "my message", repoRef.CommitQueue.Message)
	assert.False(t, repoRef.TaskSync.IsPatchEnabled())

	projectVars, err := FindOneProjectVars(repoRef.Id)
	assert.NoError(t, err)
	assert.Len(t, projectVars.Vars, 3)
	assert.Len(t, projectVars.PrivateVars, 1)
	assert.Len(t, projectVars.RestrictedVars, 1)
	assert.Equal(t, "world", projectVars.Vars["hello"])
	assert.Equal(t, "buggy", projectVars.Vars["sdc"])
	assert.Equal(t, "green", projectVars.Vars["ever"])
	assert.True(t, projectVars.PrivateVars["sdc"])
	assert.True(t, projectVars.RestrictedVars["ever"])

	projectAliases, err = FindAliasesForRepo(repoRef.Id)
	assert.NoError(t, err)
	assert.Len(t, projectAliases, 2)
	for _, a := range projectAliases {
		assert.Empty(t, a.RemotePath)
		assert.Empty(t, a.GitTag)
		assert.Empty(t, a.TaskTags)
		if a.Alias == evergreen.GithubPRAlias {
			assert.Equal(t, ".*", a.Task)
			assert.Equal(t, ".*", a.Variant)
			assert.Empty(t, a.VariantTags)
		} else {
			assert.Equal(t, evergreen.GitTagAlias, a.Alias)
			assert.Equal(t, ".*", a.Task)
			assert.Contains(t, a.VariantTags, "v1")
		}
	}

	// verify that both the project and repo are part of the scope
	rm := evergreen.GetEnvironment().RoleManager()
	scope, err := rm.GetScope(context.TODO(), GetRepoAdminScope(repoRef.Id))
	assert.NoError(t, err)
	assert.NotNil(t, scope)
	assert.Contains(t, scope.Resources, repoRef.Id)
	assert.Contains(t, scope.Resources, doc2.Id)
	assert.NotContains(t, scope.Resources, doc1.Id)
}

func TestFindOneProjectRefByRepoAndBranchWithPRTesting(t *testing.T) {
	evergreen.GetEnvironment().Settings().LoggerConfig.DefaultLogger = "buildlogger"
	assert := assert.New(t)   //nolint
	require := require.New(t) //nolint

	require.NoError(db.ClearCollections(ProjectRefCollection, RepoRefCollection, evergreen.ScopeCollection, evergreen.RoleCollection))
	env := evergreen.GetEnvironment()
	_ = env.DB().RunCommand(nil, map[string]string{"create": evergreen.ScopeCollection})

	projectRef, err := FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "main")
	assert.NoError(err)
	assert.Nil(projectRef)

	doc := &ProjectRef{
		Owner:            "mongodb",
		Repo:             "mci",
		Branch:           "main",
		Enabled:          utility.FalsePtr(),
		BatchTime:        10,
		Id:               "ident0",
		PRTestingEnabled: utility.FalsePtr(),
	}
	require.NoError(doc.Insert())

	// 1 disabled document = no match
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "main")
	assert.NoError(err)
	assert.Nil(projectRef)

	// 2 docs, 1 enabled, but the enabled one has pr testing disabled = no match
	doc.Id = "ident_"
	doc.PRTestingEnabled = utility.FalsePtr()
	doc.Enabled = utility.TruePtr()
	require.NoError(doc.Insert())
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "main")
	assert.NoError(err)
	require.Nil(projectRef)

	// 3 docs, 2 enabled, but only 1 has pr testing enabled = match
	doc.Id = "ident1"
	doc.PRTestingEnabled = utility.TruePtr()
	require.NoError(doc.Insert())
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "main")
	assert.NoError(err)
	require.NotNil(projectRef)
	assert.Equal("ident1", projectRef.Id)
	assert.Equal("buildlogger", projectRef.DefaultLogger)

	// 2 matching documents, we just return one of those projects
	doc.Id = "ident2"
	require.NoError(doc.Insert())
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "main")
	assert.NoError(err)
	assert.NotNil(projectRef)

	repoDoc := RepoRef{ProjectRef{
		Id:    "my_repo",
		Owner: "mongodb",
		Repo:  "mci",
	}}
	assert.NoError(repoDoc.Upsert())
	doc = &ProjectRef{
		Id:              "defaulting_project",
		Owner:           "mongodb",
		Repo:            "mci",
		Branch:          "mine",
		UseRepoSettings: true,
		RepoRefId:       repoDoc.Id,
	}
	assert.NoError(doc.Insert())
	doc2 := &ProjectRef{
		Id:               "hidden_project",
		Owner:            "mongodb",
		Repo:             "mci",
		Branch:           "mine",
		UseRepoSettings:  true,
		RepoRefId:        repoDoc.Id,
		Enabled:          utility.FalsePtr(),
		PRTestingEnabled: utility.FalsePtr(),
		Hidden:           utility.TruePtr(),
	}
	assert.NoError(doc2.Insert())

	// repo doesn't have PR testing enabled, so no project returned
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "mine")
	assert.NoError(err)
	assert.Nil(projectRef)

	repoDoc.Enabled = utility.TruePtr()
	assert.NoError(repoDoc.Upsert())
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "mine")
	assert.NoError(err)
	assert.Nil(projectRef)

	repoDoc.PRTestingEnabled = utility.TruePtr()
	assert.NoError(repoDoc.Upsert())
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "mine")
	assert.NoError(err)
	require.NotNil(projectRef)
	assert.Equal("defaulting_project", projectRef.Id)

	// project PR testing explicitly disabled
	doc.PRTestingEnabled = utility.FalsePtr()
	assert.NoError(doc.Upsert())
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "mine")
	assert.NoError(err)
	assert.Nil(projectRef)

	// project explicitly disabled
	doc.Enabled = utility.FalsePtr()
	doc.PRTestingEnabled = utility.TruePtr()
	assert.NoError(doc.Upsert())
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "mine")
	assert.NoError(err)
	assert.Nil(projectRef)

	// branch with no project doesn't work if repo not configured right
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "yours")
	assert.NoError(err)
	assert.Nil(projectRef)

	repoDoc.RemotePath = "my_path"
	assert.NoError(repoDoc.Upsert())
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "yours")
	assert.NoError(err)
	assert.NotNil(projectRef)
	assert.Equal("yours", projectRef.Branch)
	assert.True(projectRef.IsHidden())
	firstAttemptId := projectRef.Id

	// verify we return the same hidden project
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "yours")
	assert.NoError(err)
	require.NotNil(projectRef)
	assert.Equal(firstAttemptId, projectRef.Id)
}

func TestFindOneProjectRefWithCommitQueueByOwnerRepoAndBranch(t *testing.T) {
	evergreen.GetEnvironment().Settings().LoggerConfig.DefaultLogger = "buildlogger"
	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.ClearCollections(ProjectRefCollection, RepoRefCollection))

	projectRef, err := FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch("mongodb", "mci", "main")
	assert.NoError(err)
	assert.Nil(projectRef)

	doc := &ProjectRef{
		Owner:   "mongodb",
		Repo:    "mci",
		Branch:  "main",
		Id:      "mci",
		Enabled: utility.TruePtr(),
	}
	require.NoError(doc.Insert())

	projectRef, err = FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch("mongodb", "mci", "main")
	assert.NoError(err)
	assert.Nil(projectRef)

	doc.CommitQueue.Enabled = utility.TruePtr()
	require.NoError(db.Update(ProjectRefCollection, mgobson.M{ProjectRefIdKey: "mci"}, doc))

	projectRef, err = FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch("mongodb", "mci", "main")
	assert.NoError(err)
	assert.NotNil(projectRef)
	assert.Equal("mci", projectRef.Id)
	assert.Equal("buildlogger", projectRef.DefaultLogger)

	// doc defaults to repo, which is not enabled
	doc = &ProjectRef{
		Owner:           "mongodb",
		Repo:            "mci",
		Branch:          "not_main",
		Id:              "mci_main",
		RepoRefId:       "my_repo",
		UseRepoSettings: true,
	}
	repoDoc := &RepoRef{ProjectRef{Id: "my_repo"}}
	assert.NoError(doc.Insert())
	assert.NoError(repoDoc.Upsert())

	projectRef, err = FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch("mongodb", "mci", "not_main")
	assert.NoError(err)
	assert.Nil(projectRef)

	// doc defaults to repo, which is enabled
	repoDoc.Enabled = utility.TruePtr()
	repoDoc.CommitQueue.Enabled = utility.TruePtr()
	assert.NoError(repoDoc.Upsert())

	projectRef, err = FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch("mongodb", "mci", "not_main")
	assert.NoError(err)
	assert.NotNil(projectRef)
	assert.Equal("mci_main", projectRef.Id)
	assert.Equal("buildlogger", projectRef.DefaultLogger)

	// doc doesn't default to repo
	doc.CommitQueue.Enabled = utility.FalsePtr()
	assert.NoError(doc.Update())
	projectRef, err = FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch("mongodb", "mci", "not_main")
	assert.NoError(err)
	assert.Nil(projectRef)
}

func TestCanEnableCommitQueue(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.Clear(ProjectRefCollection))
	doc := &ProjectRef{
		Owner:   "mongodb",
		Repo:    "mci",
		Branch:  "main",
		Id:      "mci",
		Enabled: utility.TruePtr(),
		CommitQueue: CommitQueueParams{
			Enabled: utility.TruePtr(),
		},
	}
	require.NoError(doc.Insert())
	ok, err := doc.CanEnableCommitQueue()
	assert.NoError(err)
	assert.True(ok)

	doc2 := &ProjectRef{
		Owner:   "mongodb",
		Repo:    "mci",
		Branch:  "main",
		Id:      "not-mci",
		Enabled: utility.TruePtr(),
		CommitQueue: CommitQueueParams{
			Enabled: utility.FalsePtr(),
		},
	}
	require.NoError(doc2.Insert())
	ok, err = doc2.CanEnableCommitQueue()
	assert.NoError(err)
	assert.False(ok)
}

func TestFindMergedEnabledProjectRefsByOwnerAndRepo(t *testing.T) {
	require.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection))
	projectRefs, err := FindMergedEnabledProjectRefsByOwnerAndRepo("mongodb", "mci")
	assert.NoError(t, err)
	assert.Empty(t, projectRefs)

	repoRef := RepoRef{ProjectRef{
		Id:      "my_repo",
		Enabled: utility.TruePtr(),
	}}
	assert.NoError(t, repoRef.Upsert())
	doc := &ProjectRef{
		Enabled:         utility.TruePtr(),
		Owner:           "mongodb",
		Repo:            "mci",
		Branch:          "main",
		Identifier:      "mci",
		Id:              "1",
		RepoRefId:       repoRef.Id,
		UseRepoSettings: true,
	}
	assert.NoError(t, doc.Insert())
	doc.Enabled = nil
	doc.Id = "2"
	assert.NoError(t, doc.Insert())

	doc.Enabled = utility.FalsePtr()
	doc.Id = "3"
	assert.NoError(t, doc.Insert())

	doc.Enabled = utility.TruePtr()
	doc.RepoRefId = ""
	doc.UseRepoSettings = false
	doc.Id = "4"
	assert.NoError(t, doc.Insert())

	projectRefs, err = FindMergedEnabledProjectRefsByOwnerAndRepo("mongodb", "mci")
	assert.NoError(t, err)
	require.Len(t, projectRefs, 3)
	assert.NotEqual(t, projectRefs[0].Id, "3")
	assert.NotEqual(t, projectRefs[1].Id, "3")
	assert.NotEqual(t, projectRefs[2].Id, "3")
}

func TestFindProjectRefsWithCommitQueueEnabled(t *testing.T) {
	evergreen.GetEnvironment().Settings().LoggerConfig.DefaultLogger = "buildlogger"
	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.ClearCollections(ProjectRefCollection, RepoRefCollection))
	projectRefs, err := FindProjectRefsWithCommitQueueEnabled()
	assert.NoError(err)
	assert.Empty(projectRefs)

	repoRef := RepoRef{ProjectRef{
		Id:      "my_repo",
		Enabled: utility.TruePtr(),
		CommitQueue: CommitQueueParams{
			Enabled: utility.TruePtr(),
		},
	}}
	assert.NoError(repoRef.Upsert())
	doc := &ProjectRef{
		Enabled:         utility.TruePtr(),
		Owner:           "mongodb",
		Repo:            "mci",
		Branch:          "main",
		Identifier:      "mci",
		Id:              "1",
		RepoRefId:       repoRef.Id,
		UseRepoSettings: true,
		CommitQueue: CommitQueueParams{
			Enabled: utility.TruePtr(),
		},
	}
	require.NoError(doc.Insert())

	doc.Branch = "fix"
	doc.Id = "2"
	require.NoError(doc.Insert())

	doc.Identifier = "grip"
	doc.Repo = "grip"
	doc.Id = "3"
	doc.CommitQueue.Enabled = utility.FalsePtr()
	require.NoError(doc.Insert())

	projectRefs, err = FindProjectRefsWithCommitQueueEnabled()
	assert.NoError(err)
	require.Len(projectRefs, 2)
	assert.Equal("mci", projectRefs[0].Identifier)
	assert.Equal("buildlogger", projectRefs[0].DefaultLogger)
	assert.Equal("mci", projectRefs[1].Identifier)
	assert.Equal("buildlogger", projectRefs[1].DefaultLogger)

	doc.Id = "both_settings_from_repo"
	doc.Enabled = nil
	doc.CommitQueue.Enabled = nil
	assert.NoError(doc.Insert())
	projectRefs, err = FindProjectRefsWithCommitQueueEnabled()
	assert.NoError(err)
	assert.Len(projectRefs, 3)

	repoRef.CommitQueue.Enabled = utility.FalsePtr()
	assert.NoError(repoRef.Upsert())
	projectRefs, err = FindProjectRefsWithCommitQueueEnabled()
	assert.NoError(err)
	assert.Len(projectRefs, 2)
}

func TestValidatePeriodicBuildDefinition(t *testing.T) {
	assert := assert.New(t)
	testCases := map[PeriodicBuildDefinition]bool{
		PeriodicBuildDefinition{
			IntervalHours: 24,
			ConfigFile:    "foo.yml",
			Alias:         "myAlias",
		}: true,
		PeriodicBuildDefinition{
			IntervalHours: 0,
			ConfigFile:    "foo.yml",
			Alias:         "myAlias",
		}: false,
		PeriodicBuildDefinition{
			IntervalHours: 24,
			ConfigFile:    "",
			Alias:         "myAlias",
		}: false,
		PeriodicBuildDefinition{
			IntervalHours: 24,
			ConfigFile:    "foo.yml",
			Alias:         "",
		}: true,
	}

	for testCase, shouldPass := range testCases {
		if shouldPass {
			assert.NoError(testCase.Validate())
		} else {
			assert.Error(testCase.Validate())
		}
		assert.NotEmpty(testCase.ID)
	}
}

func TestGetPatchTriggerAlias(t *testing.T) {
	projRef := ProjectRef{
		PatchTriggerAliases: []patch.PatchTriggerDefinition{{Alias: "a0"}},
	}

	alias, found := projRef.GetPatchTriggerAlias("a0")
	assert.True(t, found)
	assert.Equal(t, "a0", alias.Alias)

	alias, found = projRef.GetPatchTriggerAlias("a1")
	assert.False(t, found)
}

func TestFindDownstreamProjects(t *testing.T) {
	require.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection))
	evergreen.GetEnvironment().Settings().LoggerConfig.DefaultLogger = "buildlogger"

	repoRef := RepoRef{ProjectRef{
		Id:      "my_repo",
		Enabled: utility.TruePtr(),
	}}
	assert.NoError(t, repoRef.Upsert())

	proj1 := ProjectRef{
		Id:              "evergreen",
		RepoRefId:       repoRef.Id,
		UseRepoSettings: true,
		Enabled:         utility.TruePtr(),
		Triggers:        []TriggerDefinition{{Project: "grip"}},
	}
	require.NoError(t, proj1.Insert())

	proj2 := ProjectRef{
		Id:              "mci",
		RepoRefId:       repoRef.Id,
		UseRepoSettings: true,
		Enabled:         utility.FalsePtr(),
		Triggers:        []TriggerDefinition{{Project: "grip"}},
	}
	require.NoError(t, proj2.Insert())

	projects, err := FindDownstreamProjects("grip")
	assert.NoError(t, err)
	assert.Len(t, projects, 1)
	proj1.DefaultLogger = "buildlogger"
	assert.Equal(t, proj1, projects[0])

	proj1.Enabled = nil
	assert.NoError(t, proj1.Upsert())
	projects, err = FindDownstreamProjects("grip")
	assert.NoError(t, err)
	assert.Len(t, projects, 1)

	proj2.Enabled = nil
	assert.NoError(t, proj2.Upsert())
	projects, err = FindDownstreamProjects("grip")
	assert.NoError(t, err)
	assert.Len(t, projects, 2)
}

func TestAddPermissions(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(user.Collection, ProjectRefCollection, evergreen.ScopeCollection, evergreen.RoleCollection))
	_ = evergreen.GetEnvironment().DB().RunCommand(nil, map[string]string{"create": evergreen.ScopeCollection})
	u := user.DBUser{
		Id: "me",
	}
	assert.NoError(u.Insert())
	p := ProjectRef{
		Identifier: "myProject",
		Owner:      "mongodb",
		Repo:       "mongo",
		Branch:     "main",
		Hidden:     utility.TruePtr(),
	}
	assert.NoError(p.Add(&u))
	assert.NotEmpty(p.Id)
	assert.True(mgobson.IsObjectIdHex(p.Id))

	rm := evergreen.GetEnvironment().RoleManager()
	scope, err := rm.FindScopeForResources(evergreen.ProjectResourceType, p.Id)
	assert.NoError(err)
	assert.NotNil(scope)
	role, err := rm.FindRoleWithPermissions(evergreen.ProjectResourceType, []string{p.Id}, map[string]int{
		evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value,
		evergreen.PermissionTasks:           evergreen.TasksAdmin.Value,
		evergreen.PermissionPatches:         evergreen.PatchSubmit.Value,
		evergreen.PermissionLogs:            evergreen.LogsView.Value,
	})
	assert.NoError(err)
	assert.NotNil(role)
	dbUser, err := user.FindOneById(u.Id)
	assert.NoError(err)
	assert.Contains(dbUser.Roles(), fmt.Sprintf("admin_project_%s", p.Id))
	projectId := p.Id

	// check that an added project uses the hidden project's ID
	u = user.DBUser{Id: "you"}
	assert.NoError(u.Insert())
	p.Identifier = "differentProject"
	p.Id = ""
	assert.NoError(p.Add(&u))
	assert.NotEmpty(p.Id)
	assert.True(mgobson.IsObjectIdHex(p.Id))
	assert.Equal(projectId, p.Id)

	scope, err = rm.FindScopeForResources(evergreen.ProjectResourceType, p.Id)
	assert.NoError(err)
	assert.NotNil(scope)
	role, err = rm.FindRoleWithPermissions(evergreen.ProjectResourceType, []string{p.Id}, map[string]int{
		evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value,
		evergreen.PermissionTasks:           evergreen.TasksAdmin.Value,
		evergreen.PermissionPatches:         evergreen.PatchSubmit.Value,
		evergreen.PermissionLogs:            evergreen.LogsView.Value,
	})
	assert.NoError(err)
	assert.NotNil(role)
	dbUser, err = user.FindOneById(u.Id)
	assert.NoError(err)
	assert.Contains(dbUser.Roles(), fmt.Sprintf("admin_project_%s", p.Id))
}

func TestUpdateAdminRoles(t *testing.T) {
	require.NoError(t, db.ClearCollections(ProjectRefCollection, evergreen.ScopeCollection, evergreen.RoleCollection, user.Collection))
	env := evergreen.GetEnvironment()
	_ = env.DB().RunCommand(nil, map[string]string{"create": evergreen.ScopeCollection})
	rm := env.RoleManager()
	adminScope := gimlet.Scope{
		ID:        evergreen.AllProjectsScope,
		Type:      evergreen.ProjectResourceType,
		Resources: []string{"proj"},
	}
	require.NoError(t, rm.AddScope(adminScope))
	adminRole := gimlet.Role{
		ID:          "admin",
		Scope:       evergreen.AllProjectsScope,
		Permissions: adminPermissions,
	}
	require.NoError(t, rm.UpdateRole(adminRole))
	oldAdmin := user.DBUser{
		Id:          "oldAdmin",
		SystemRoles: []string{"admin"},
	}
	require.NoError(t, oldAdmin.Insert())
	newAdmin := user.DBUser{
		Id: "newAdmin",
	}
	require.NoError(t, newAdmin.Insert())
	p := ProjectRef{
		Id: "proj",
	}
	require.NoError(t, p.Insert())

	assert.NoError(t, p.UpdateAdminRoles([]string{newAdmin.Id}, []string{oldAdmin.Id}))
	oldAdminFromDB, err := user.FindOneById(oldAdmin.Id)
	assert.NoError(t, err)
	assert.Len(t, oldAdminFromDB.Roles(), 0)
	newAdminFromDB, err := user.FindOneById(newAdmin.Id)
	assert.NoError(t, err)
	assert.Len(t, newAdminFromDB.Roles(), 1)
}

func TestUpdateAdminRolesError(t *testing.T) {
	require.NoError(t, db.ClearCollections(ProjectRefCollection, evergreen.ScopeCollection, evergreen.RoleCollection, user.Collection))
	env := evergreen.GetEnvironment()
	_ = env.DB().RunCommand(nil, map[string]string{"create": evergreen.ScopeCollection})
	rm := env.RoleManager()
	adminScope := gimlet.Scope{
		ID:        evergreen.AllProjectsScope,
		Type:      evergreen.ProjectResourceType,
		Resources: []string{"proj"},
	}
	require.NoError(t, rm.AddScope(adminScope))
	adminRole := gimlet.Role{
		ID:          "admin",
		Scope:       evergreen.AllProjectsScope,
		Permissions: adminPermissions,
	}
	require.NoError(t, rm.UpdateRole(adminRole))
	oldAdmin := user.DBUser{
		Id:          "oldAdmin",
		SystemRoles: []string{"admin"},
	}
	require.NoError(t, oldAdmin.Insert())
	newAdmin := user.DBUser{
		Id: "newAdmin",
	}
	require.NoError(t, newAdmin.Insert())
	p := ProjectRef{
		Id: "proj",
	}
	require.NoError(t, p.Insert())

	// check that the existing users have been added and removed while returning an error
	assert.Error(t, p.UpdateAdminRoles([]string{"nonexistent-user", newAdmin.Id}, []string{"nonexistent-user", oldAdmin.Id}))
	oldAdminFromDB, err := user.FindOneById(oldAdmin.Id)
	assert.NoError(t, err)
	assert.Len(t, oldAdminFromDB.Roles(), 0)
	newAdminFromDB, err := user.FindOneById(newAdmin.Id)
	assert.NoError(t, err)
	assert.Len(t, newAdminFromDB.Roles(), 1)
}

func TestUpdateNextPeriodicBuild(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.Clear(ProjectRefCollection))
	now := time.Now().Truncate(time.Second)
	p := ProjectRef{
		Id: "proj",
		PeriodicBuilds: []PeriodicBuildDefinition{
			{ID: "1", NextRunTime: now},
			{ID: "2", NextRunTime: now.Add(1 * time.Hour)},
		},
	}
	assert.NoError(p.Insert())

	assert.NoError(p.UpdateNextPeriodicBuild("2", now.Add(10*time.Hour)))
	dbProject, err := FindBranchProjectRef(p.Id)
	assert.NoError(err)
	assert.True(now.Equal(dbProject.PeriodicBuilds[0].NextRunTime))
	assert.True(now.Equal(p.PeriodicBuilds[0].NextRunTime))
	assert.True(now.Add(10 * time.Hour).Equal(dbProject.PeriodicBuilds[1].NextRunTime))
	assert.True(now.Add(10 * time.Hour).Equal(p.PeriodicBuilds[1].NextRunTime))
}

func TestGetProjectSetupCommands(t *testing.T) {
	p := ProjectRef{}
	p.WorkstationConfig.SetupCommands = []WorkstationSetupCommand{
		{Command: "c0"},
		{Command: "c1"},
	}

	cmds, err := p.GetProjectSetupCommands(apimodels.WorkstationSetupCommandOptions{})
	assert.NoError(t, err)
	assert.Len(t, cmds, 2)
	assert.Contains(t, cmds[0].String(), "c0")
	assert.Contains(t, cmds[1].String(), "c1")
}

func TestFindPeriodicProjects(t *testing.T) {
	assert.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection))

	repoRef := RepoRef{ProjectRef{
		Enabled:        utility.TruePtr(),
		Id:             "my_repo",
		PeriodicBuilds: []PeriodicBuildDefinition{{ID: "repo_def"}},
	}}
	assert.NoError(t, repoRef.Upsert())

	pRef := ProjectRef{
		Id:              "p1",
		RepoRefId:       "my_repo",
		UseRepoSettings: true,
		PeriodicBuilds:  []PeriodicBuildDefinition{},
	}
	assert.NoError(t, pRef.Insert())

	pRef.Id = "p2"
	pRef.PeriodicBuilds = []PeriodicBuildDefinition{{ID: "p1"}}
	assert.NoError(t, pRef.Insert())

	pRef.Id = "p3"
	pRef.PeriodicBuilds = nil
	assert.NoError(t, pRef.Insert())

	pRef.Id = "p4"
	pRef.Enabled = utility.FalsePtr()
	pRef.PeriodicBuilds = []PeriodicBuildDefinition{{ID: "p1"}}
	assert.NoError(t, pRef.Insert())

	projects, err := FindPeriodicProjects()
	assert.NoError(t, err)
	assert.Len(t, projects, 2)
}

func TestRemoveAdminFromProjects(t *testing.T) {
	assert.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection))

	pRef := ProjectRef{
		Id:     "my_project",
		Admins: []string{"me", "villain"},
	}
	pRef2 := ProjectRef{
		Id:     "your_project",
		Admins: []string{"you", "villain"},
	}
	pRef3 := ProjectRef{
		Id: "adminless_project",
	}
	repoRef := RepoRef{ProjectRef{
		Id:     "my_repo",
		Admins: []string{"villain"},
	}}
	repoRef2 := RepoRef{ProjectRef{
		Id:     "your_repo",
		Admins: []string{"villain"},
	}}
	repoRef3 := RepoRef{ProjectRef{
		Id: "adminless_repo",
	}}

	assert.NoError(t, pRef.Upsert())
	assert.NoError(t, pRef2.Upsert())
	assert.NoError(t, pRef3.Upsert())
	assert.NoError(t, repoRef.Upsert())
	assert.NoError(t, repoRef2.Upsert())
	assert.NoError(t, repoRef3.Upsert())

	assert.NoError(t, RemoveAdminFromProjects("villain"))

	// verify that we carry out multiple updates
	pRefFromDB, err := FindBranchProjectRef(pRef.Id)
	assert.NoError(t, err)
	assert.NotNil(t, pRefFromDB)
	assert.NotContains(t, pRefFromDB.Admins, "villain")
	pRefFromDB, err = FindBranchProjectRef(pRef2.Id)
	assert.NoError(t, err)
	assert.NotNil(t, pRefFromDB)
	assert.NotContains(t, pRefFromDB.Admins, "villain")
	pRefFromDB, err = FindBranchProjectRef(pRef3.Id)
	assert.NoError(t, err)
	assert.NotNil(t, pRefFromDB)
	assert.NotContains(t, pRefFromDB.Admins, "villain")

	repoRefFromDB, err := FindOneRepoRef(repoRef.Id)
	assert.NoError(t, err)
	assert.NotNil(t, repoRefFromDB)
	assert.NotContains(t, repoRefFromDB.Admins, "villain")
	repoRefFromDB, err = FindOneRepoRef(repoRef2.Id)
	assert.NoError(t, err)
	assert.NotNil(t, repoRefFromDB)
	assert.NotContains(t, repoRefFromDB.Admins, "villain")
	repoRefFromDB, err = FindOneRepoRef(repoRef3.Id)
	assert.NoError(t, err)
	assert.NotNil(t, repoRefFromDB)
	assert.NotContains(t, repoRefFromDB.Admins, "villain")
}

func TestPointers(t *testing.T) {
	assert.NoError(t, db.ClearCollections(ProjectRefCollection))
	ref := struct {
		MyString string            `bson:"my_str"`
		MyBool   bool              `bson:"my_bool"`
		MyStruct WorkstationConfig `bson:"config"`
	}{
		MyString: "this is a string",
		MyBool:   false,
		MyStruct: WorkstationConfig{GitClone: utility.TruePtr()},
	}

	assert.NoError(t, db.Insert(ProjectRefCollection, ref))

	pointerRef := struct {
		PtrString *string            `bson:"my_str"`
		PtrBool   *bool              `bson:"my_bool"`
		PtrStruct *WorkstationConfig `bson:"config"`
	}{}
	assert.NoError(t, db.FindOneQ(ProjectRefCollection, db.Query(bson.M{}), &pointerRef))
	assert.Equal(t, ref.MyString, *pointerRef.PtrString)
	assert.False(t, utility.FromBoolTPtr(pointerRef.PtrBool))
	assert.NotNil(t, pointerRef.PtrStruct)
	assert.True(t, pointerRef.PtrStruct.ShouldGitClone())
}

func TestMergeWithParserProject(t *testing.T) {
	require.NoError(t, db.ClearCollections(ProjectRefCollection, ParserProjectCollection),
		"Error clearing collection")

	projectRef := &ProjectRef{
		Owner:              "mongodb",
		Id:                 "ident",
		PerfEnabled:        utility.TruePtr(),
		DeactivatePrevious: utility.FalsePtr(),
		TaskAnnotationSettings: evergreen.AnnotationsSettings{
			FileTicketWebHook: evergreen.WebHook{
				Endpoint: "random1",
			},
		},
	}
	parserProject := &ParserProject{
		Id:                 "version1",
		DeactivatePrevious: utility.TruePtr(),
		TaskAnnotationSettings: &evergreen.AnnotationsSettings{
			FileTicketWebHook: evergreen.WebHook{
				Endpoint: "random2",
			},
		},
	}
	assert.NoError(t, projectRef.Insert())
	assert.NoError(t, parserProject.Insert())
	err := projectRef.MergeWithParserProject("version1")
	assert.NoError(t, err)
	require.NotNil(t, projectRef)
	assert.Equal(t, "ident", projectRef.Id)

	assert.True(t, *projectRef.DeactivatePrevious)
	assert.True(t, *projectRef.PerfEnabled)
	assert.Equal(t, "random2", projectRef.TaskAnnotationSettings.FileTicketWebHook.Endpoint)
}
