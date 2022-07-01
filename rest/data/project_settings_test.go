package data

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveProjectSettingsForSectionForRepo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	rm := env.RoleManager()

	for name, test := range map[string]func(t *testing.T, ref model.RepoRef){
		model.ProjectPageGeneralSection: func(t *testing.T, ref model.RepoRef) {
			assert.Empty(t, ref.SpawnHostScriptPath)

			ref.SpawnHostScriptPath = "my script path"
			ref.Owner = "something different"
			apiProjectRef := restModel.APIProjectRef{}
			assert.NoError(t, apiProjectRef.BuildFromService(ref.ProjectRef))
			apiChanges := &restModel.APIProjectSettings{
				ProjectRef: apiProjectRef,
			}
			// ensure that we're saving settings without a special case
			settings, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageGeneralSection, true, "me")
			assert.NoError(t, err)
			assert.NotNil(t, settings)
			repoRefFromDB, err := model.FindOneRepoRef(ref.Id)
			assert.NoError(t, err)
			assert.NotNil(t, repoRefFromDB)
			assert.NotEmpty(t, repoRefFromDB.SpawnHostScriptPath)
			assert.NotEqual(t, repoRefFromDB, "something different") // we don't change this
		},
		model.ProjectPageAccessSection: func(t *testing.T, ref model.RepoRef) {
			newAdmin := user.DBUser{
				Id: "newAdmin",
			}
			require.NoError(t, newAdmin.Insert())
			ref.Restricted = utility.TruePtr() // should also flip the project that defaults to this repo
			ref.Admins = []string{"oldAdmin", newAdmin.Id}
			apiProjectRef := restModel.APIProjectRef{}
			assert.NoError(t, apiProjectRef.BuildFromService(ref.ProjectRef))
			apiChanges := &restModel.APIProjectSettings{
				ProjectRef: apiProjectRef,
			}
			settings, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageAccessSection, true, "me")
			assert.NoError(t, err)
			assert.NotNil(t, settings)
			repoRefFromDb, err := model.FindOneRepoRef(ref.Id)
			assert.NoError(t, err)
			assert.NotNil(t, repoRefFromDb)
			assert.True(t, repoRefFromDb.IsRestricted())
			assert.Equal(t, repoRefFromDb.Admins, ref.Admins)

			// should be restricted
			projectThatDefaults, err := model.FindMergedProjectRef("myId", "", true)
			assert.NoError(t, err)
			assert.NotNil(t, projectThatDefaults)
			assert.True(t, projectThatDefaults.IsRestricted())

			// should not be restricted
			projectThatDoesNotDefault, err := model.FindMergedProjectRef("myId2", "", true)
			assert.NoError(t, err)
			assert.NotNil(t, projectThatDoesNotDefault)
			assert.False(t, projectThatDoesNotDefault.IsRestricted())

			restrictedScope, err := rm.GetScope(ctx, evergreen.RestrictedProjectsScope)
			assert.NoError(t, err)
			assert.NotNil(t, restrictedScope)
			assert.Contains(t, restrictedScope.Resources, projectThatDefaults.Id)

			unrestrictedScope, err := rm.GetScope(ctx, evergreen.UnrestrictedProjectsScope)
			assert.NoError(t, err)
			assert.NotNil(t, unrestrictedScope)
			assert.NotContains(t, unrestrictedScope.Resources, projectThatDefaults.Id)

			newAdminFromDB, err := user.FindOneById("newAdmin")
			assert.NoError(t, err)
			assert.NotNil(t, newAdminFromDB)
			assert.Contains(t, newAdminFromDB.Roles(), model.GetRepoAdminRole(ref.Id))
		},
		"Removes and adds admin with error": func(t *testing.T, ref model.RepoRef) {
			newAdmin := user.DBUser{
				Id: "newAdmin",
			}
			require.NoError(t, newAdmin.Insert())
			ref.Admins = []string{"nonexistent", newAdmin.Id}
			apiProjectRef := restModel.APIProjectRef{}
			assert.NoError(t, apiProjectRef.BuildFromService(ref.ProjectRef))
			apiChanges := &restModel.APIProjectSettings{
				ProjectRef: apiProjectRef,
			}
			settings, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageAccessSection, true, "me")
			// should still add newAdmin and delete oldAdmin even with errors
			require.Error(t, err)
			assert.Contains(t, err.Error(), "no user 'nonexistent' found")
			assert.NotNil(t, settings)
			repoRefFromDb, err := model.FindOneRepoRef(ref.Id)
			assert.NoError(t, err)
			assert.NotNil(t, repoRefFromDb)
			assert.Equal(t, []string{newAdmin.Id}, repoRefFromDb.Admins)

			newAdminFromDB, err := user.FindOneById("newAdmin")
			assert.NoError(t, err)
			assert.NotNil(t, newAdminFromDB)
			assert.Contains(t, newAdminFromDB.Roles(), model.GetRepoAdminRole(ref.Id))

			oldAdminFromDB, err := user.FindOneById("oldAdmin")
			assert.NoError(t, err)
			assert.NotNil(t, oldAdminFromDB)
			assert.NotContains(t, oldAdminFromDB.Roles(), model.GetRepoAdminRole(ref.Id))
		},
		model.ProjectPageVariablesSection: func(t *testing.T, ref model.RepoRef) {
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
			settings, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageVariablesSection, true, "me")
			assert.NoError(t, err)
			assert.NotNil(t, settings)
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
			event.SubscriptionsCollection, event.LegacyEventLogCollection, evergreen.ScopeCollection, user.Collection))
		require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))

		repoRef := model.RepoRef{ProjectRef: model.ProjectRef{
			Id:         "myRepoId",
			Owner:      "evergreen-ci",
			Repo:       "evergreen",
			Restricted: utility.FalsePtr(),
			Admins:     []string{"oldAdmin"},
		}}
		assert.NoError(t, repoRef.Upsert())

		pRefThatDefaults := model.ProjectRef{
			Id:        "myId",
			Owner:     "evergreen-ci",
			Repo:      "evergreen",
			RepoRefId: "myRepoId",
			Admins:    []string{"oldAdmin"},
		}
		assert.NoError(t, pRefThatDefaults.Upsert())

		pRefThatDoesNotDefault := model.ProjectRef{
			Id:    "myId2",
			Owner: "evergreen-ci",
			Repo:  "evergreen",
		}
		assert.NoError(t, pRefThatDoesNotDefault.Upsert())

		pVars := model.ProjectVars{
			Id:          repoRef.Id,
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
			Resources:   []string{},
			ParentScope: evergreen.AllProjectsScope,
		}
		assert.NoError(t, rm.AddScope(restrictedScope))
		unrestrictedScope := gimlet.Scope{
			ID:          evergreen.UnrestrictedProjectsScope,
			Resources:   []string{pRefThatDefaults.Id, pRefThatDoesNotDefault.Id},
			ParentScope: evergreen.AllProjectsScope,
		}
		assert.NoError(t, rm.AddScope(unrestrictedScope))
		adminScope := gimlet.Scope{
			ID:        "project_scope",
			Resources: []string{pRefThatDefaults.Id},
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
			test(t, repoRef)
		})
	}
}

func TestSaveProjectSettingsForSection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	rm := env.RoleManager()

	for name, test := range map[string]func(t *testing.T, ref model.ProjectRef){
		model.ProjectPageGeneralSection: func(t *testing.T, ref model.ProjectRef) {
			assert.Empty(t, ref.SpawnHostScriptPath)

			ref.SpawnHostScriptPath = "my script path"
			ref.Owner = "something different"
			apiProjectRef := restModel.APIProjectRef{}
			assert.NoError(t, apiProjectRef.BuildFromService(ref))
			apiChanges := &restModel.APIProjectSettings{
				ProjectRef: apiProjectRef,
			}
			// ensure that we're saving settings without a special case
			settings, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageGeneralSection, false, "me")
			assert.NoError(t, err)
			assert.NotNil(t, settings)
			assert.Equal(t, "myRepoId", utility.FromStringPtr(settings.ProjectRef.RepoRefId))
			pRefFromDB, err := model.FindBranchProjectRef(ref.Id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDB)
			assert.NotEmpty(t, pRefFromDB.SpawnHostScriptPath)
			assert.NotEqual(t, pRefFromDB.Owner, "something different") // because use repo settings is true, we don't change this
		},
		"github conflicts with enabling": func(t *testing.T, ref model.ProjectRef) {
			conflictingRef := model.ProjectRef{
				Owner:               ref.Owner,
				Repo:                ref.Repo,
				Branch:              ref.Branch,
				Enabled:             utility.TruePtr(),
				PRTestingEnabled:    utility.TruePtr(),
				GithubChecksEnabled: utility.TruePtr(),
				CommitQueue: model.CommitQueueParams{
					Enabled: utility.TruePtr(),
				},
			}
			assert.NoError(t, conflictingRef.Insert())
			ref.PRTestingEnabled = utility.TruePtr()
			ref.GithubChecksEnabled = utility.TruePtr()
			assert.NoError(t, ref.Upsert())
			ref.Enabled = utility.TruePtr()
			apiProjectRef := restModel.APIProjectRef{}
			assert.NoError(t, apiProjectRef.BuildFromService(ref))
			apiChanges := &restModel.APIProjectSettings{
				ProjectRef: apiProjectRef,
			}
			_, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageGeneralSection, false, "me")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "PR testing and commit checks")
			assert.NotContains(t, err.Error(), "the commit queue")
		},
		"github conflicts on Commit Queue page when defaulting to repo": func(t *testing.T, ref model.ProjectRef) {
			conflictingRef := model.ProjectRef{
				Owner:               ref.Owner,
				Repo:                ref.Repo,
				Branch:              ref.Branch,
				Enabled:             utility.TruePtr(),
				PRTestingEnabled:    utility.TruePtr(),
				GithubChecksEnabled: utility.TruePtr(),
				CommitQueue: model.CommitQueueParams{
					Enabled: utility.TruePtr(),
				},
			}
			assert.NoError(t, conflictingRef.Insert())

			changes := model.ProjectRef{
				Id:                  ref.Id,
				PRTestingEnabled:    nil,
				GithubChecksEnabled: utility.FalsePtr(),
			}
			apiProjectRef := restModel.APIProjectRef{}
			assert.NoError(t, apiProjectRef.BuildFromService(changes))
			apiChanges := &restModel.APIProjectSettings{
				ProjectRef: apiProjectRef,
			}
			_, err := SaveProjectSettingsForSection(ctx, changes.Id, apiChanges, model.ProjectPageGithubAndCQSection, false, "me")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "PR testing")
			assert.NotContains(t, err.Error(), "the commit queue")
			assert.NotContains(t, err.Error(), "commit checks")
		},
		model.ProjectPageAccessSection: func(t *testing.T, ref model.ProjectRef) {
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
			settings, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageAccessSection, false, "me")
			assert.NoError(t, err)
			assert.NotNil(t, settings)
			pRefFromDB, err := model.FindBranchProjectRef(ref.Id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDB)
			assert.Nil(t, pRefFromDB.Restricted)
			assert.Equal(t, pRefFromDB.Admins, ref.Admins)

			mergedProject, err := model.FindMergedProjectRef(ref.Id, "", true)
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
		"Removes and adds admin with error": func(t *testing.T, ref model.ProjectRef) {
			newAdmin := user.DBUser{
				Id: "newAdmin",
			}
			require.NoError(t, newAdmin.Insert())
			ref.Admins = []string{"nonexistent", newAdmin.Id}
			apiProjectRef := restModel.APIProjectRef{}
			assert.NoError(t, apiProjectRef.BuildFromService(ref))
			apiChanges := &restModel.APIProjectSettings{
				ProjectRef: apiProjectRef,
			}
			settings, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageAccessSection, false, "me")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "no user 'nonexistent' found")
			assert.NotNil(t, settings)
			pRefFromDB, err := model.FindBranchProjectRef(ref.Id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDB)
			// should still add newAdmin and delete oldAdmin even with errors
			assert.Equal(t, []string{newAdmin.Id}, pRefFromDB.Admins)

			newAdminFromDB, err := user.FindOneById("newAdmin")
			assert.NoError(t, err)
			assert.NotNil(t, newAdminFromDB)
			assert.Contains(t, newAdminFromDB.Roles(), "admin")

			oldAdminFromDB, err := user.FindOneById("oldAdmin")
			assert.NoError(t, err)
			assert.NotNil(t, oldAdminFromDB)
			assert.NotContains(t, oldAdminFromDB.Roles(), model.GetRepoAdminRole(ref.Id))
		},
		model.ProjectPageVariablesSection: func(t *testing.T, ref model.ProjectRef) {
			// remove a variable, modify a variable, delete/add a private variable, add a variable, leave a private variable unchanged
			apiProjectVars := restModel.APIProjectVars{
				Vars:            map[string]string{"it": "me", "banana": "phone", "change": "is good", "private": ""},
				PrivateVarsList: []string{"banana", "private", "change"},
			}
			apiChanges := &restModel.APIProjectSettings{
				Vars: apiProjectVars,
			}
			settings, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageVariablesSection, false, "me")
			assert.NoError(t, err)
			assert.NotNil(t, settings)
			// Confirm that private variables are redacted.
			assert.Equal(t, settings.Vars.Vars["banana"], "")
			assert.Equal(t, settings.Vars.Vars["change"], "")
			assert.Equal(t, settings.Vars.Vars["private"], "")
			varsFromDb, err := model.FindOneProjectVars(ref.Id)
			assert.NoError(t, err)
			assert.NotNil(t, varsFromDb)
			assert.Equal(t, varsFromDb.Vars["it"], "me")
			assert.Equal(t, varsFromDb.Vars["banana"], "phone")
			assert.Equal(t, varsFromDb.Vars["hello"], "")
			assert.Equal(t, varsFromDb.Vars["private"], "forever") // ensure un-edited private variables are unchanged
			assert.Equal(t, varsFromDb.Vars["change"], "is good")  // ensure edited private variables are changed
			assert.False(t, varsFromDb.PrivateVars["it"])
			assert.False(t, varsFromDb.PrivateVars["hello"])
			assert.True(t, varsFromDb.PrivateVars["banana"])
			assert.True(t, varsFromDb.PrivateVars["private"])
			assert.True(t, varsFromDb.PrivateVars["change"])
		},
		model.ProjectPageNotificationsSection: func(t *testing.T, ref model.ProjectRef) {
			newSubscription := event.Subscription{
				Owner:        ref.Id,
				OwnerType:    event.OwnerTypeProject,
				ResourceType: event.ResourceTypeTask,
				Trigger:      event.TriggerSuccess,
				Selectors: []event.Selector{
					{Type: "id", Data: "1234"},
				},
				Subscriber: event.Subscriber{
					Type:   event.EmailSubscriberType,
					Target: "a@gmail.com",
				},
			}
			apiSub := restModel.APISubscription{}
			assert.NoError(t, apiSub.BuildFromService(newSubscription))
			apiChanges := &restModel.APIProjectSettings{
				Subscriptions: []restModel.APISubscription{apiSub},
			}
			settings, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageNotificationsSection, false, "me")
			assert.NoError(t, err)
			assert.NotNil(t, settings)
			subsFromDb, err := event.FindSubscriptionsByOwner(ref.Id, event.OwnerTypeProject)
			assert.NoError(t, err)
			require.Len(t, subsFromDb, 1)
			assert.Equal(t, subsFromDb[0].Trigger, event.TriggerSuccess)
		},
		model.ProjectPageWorkstationsSection: func(t *testing.T, ref model.ProjectRef) {
			assert.Nil(t, ref.WorkstationConfig.SetupCommands)
			apiProjectRef := restModel.APIProjectRef{
				WorkstationConfig: restModel.APIWorkstationConfig{
					GitClone:      utility.TruePtr(),
					SetupCommands: []restModel.APIWorkstationSetupCommand{}, // empty list should still save
				},
			}
			apiChanges := &restModel.APIProjectSettings{
				ProjectRef: apiProjectRef,
			}
			settings, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageWorkstationsSection, false, "me")
			assert.NoError(t, err)
			assert.NotNil(t, settings)
			assert.NotNil(t, settings.ProjectRef.WorkstationConfig.SetupCommands)
			assert.Empty(t, settings.ProjectRef.WorkstationConfig.SetupCommands)
			assert.True(t, utility.FromBoolPtr(settings.ProjectRef.WorkstationConfig.GitClone))
		},
	} {
		assert.NoError(t, db.ClearCollections(model.ProjectRefCollection, model.ProjectVarsCollection,
			event.SubscriptionsCollection, event.LegacyEventLogCollection, evergreen.ScopeCollection, user.Collection))
		require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))

		pRef := model.ProjectRef{
			Id:         "myId",
			Owner:      "evergreen-ci",
			Repo:       "evergreen",
			Branch:     "main",
			Restricted: utility.FalsePtr(),
			RepoRefId:  "myRepoId",
			Admins:     []string{"oldAdmin"},
		}
		assert.NoError(t, pRef.Insert())
		repoRef := model.RepoRef{ProjectRef: model.ProjectRef{
			Id:               pRef.RepoRefId,
			Restricted:       utility.TruePtr(),
			PRTestingEnabled: utility.TruePtr(),
		}}
		assert.NoError(t, repoRef.Upsert())

		pVars := model.ProjectVars{
			Id:          pRef.Id,
			Vars:        map[string]string{"hello": "world", "it": "adele", "private": "forever", "change": "inevitable"},
			PrivateVars: map[string]bool{"hello": true, "private": true, "change": true},
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

		existingSub := event.Subscription{
			Owner:        pRef.Id,
			OwnerType:    event.OwnerTypeProject,
			ResourceType: event.ResourceTypeTask,
			Trigger:      event.TriggerFailure,
			Selectors: []event.Selector{
				{Type: "id", Data: "1234"},
			},
			Subscriber: event.Subscriber{
				Type:   event.EmailSubscriberType,
				Target: "a@gmail.com",
			},
		}
		assert.NoError(t, existingSub.Upsert())
		t.Run(name, func(t *testing.T) {
			test(t, pRef)
		})
	}
}

func TestCopyProject(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "oldAdmin"})
	env := testutil.NewEnvironment(ctx, t)
	rm := env.RoleManager()

	for name, test := range map[string]func(t *testing.T, ref model.ProjectRef){
		"Successfully copies project": func(t *testing.T, ref model.ProjectRef) {
			copyProjectOpts := CopyProjectOpts{
				ProjectIdToCopy:      ref.Id,
				NewProjectIdentifier: "myNewProject",
				NewProjectId:         "12345",
			}
			newProject, err := CopyProject(ctx, copyProjectOpts)
			assert.NoError(t, err)
			assert.NotNil(t, newProject)
			assert.Equal(t, *newProject.Identifier, "myNewProject")
			assert.Equal(t, *newProject.Id, "12345")
		},
		"Copies project with partial error": func(t *testing.T, ref model.ProjectRef) {
			copyProjectOpts := CopyProjectOpts{
				ProjectIdToCopy:      "myIdTwo",
				NewProjectIdentifier: "mySecondProject",
			}
			newProject, err := CopyProject(ctx, copyProjectOpts)
			assert.Error(t, err)
			assert.NotNil(t, newProject)
			assert.Equal(t, *newProject.Identifier, "mySecondProject")
		},
		"Does not copy project with fatal error": func(t *testing.T, ref model.ProjectRef) {
			copyProjectOpts := CopyProjectOpts{
				ProjectIdToCopy:      "nonexistentId",
				NewProjectIdentifier: "myThirdProject",
			}
			newProject, err := CopyProject(ctx, copyProjectOpts)
			assert.Error(t, err)
			assert.Nil(t, newProject)
		},
	} {
		assert.NoError(t, db.ClearCollections(model.ProjectRefCollection, model.ProjectVarsCollection,
			event.SubscriptionsCollection, event.LegacyEventLogCollection, evergreen.ScopeCollection, user.Collection))
		require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))

		pRef := model.ProjectRef{
			Id:         "myId",
			Owner:      "evergreen-ci",
			Repo:       "evergreen",
			Branch:     "main",
			Restricted: utility.FalsePtr(),
			Admins:     []string{"oldAdmin"},
		}
		assert.NoError(t, pRef.Insert())

		pRefInvalidAdmin := model.ProjectRef{
			Id:         "myIdTwo",
			Owner:      "evergreen-ci",
			Repo:       "spruce",
			Branch:     "main",
			Restricted: utility.FalsePtr(),
			Admins:     []string{"unknownAdmin"},
		}
		assert.NoError(t, pRefInvalidAdmin.Insert())

		pVars := model.ProjectVars{
			Id:          pRef.Id,
			Vars:        map[string]string{"hello": "world", "it": "adele", "private": "forever", "change": "inevitable"},
			PrivateVars: map[string]bool{"hello": true, "private": true, "change": true},
		}
		assert.NoError(t, pVars.Insert())
		// add scopes
		allProjectsScope := gimlet.Scope{
			ID:        evergreen.AllProjectsScope,
			Resources: []string{},
		}
		assert.NoError(t, rm.AddScope(allProjectsScope))
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

		existingSub := event.Subscription{
			Owner:        pRef.Id,
			OwnerType:    event.OwnerTypeProject,
			ResourceType: event.ResourceTypeTask,
			Trigger:      event.TriggerFailure,
			Selectors: []event.Selector{
				{Type: "id", Data: "1234"},
			},
			Subscriber: event.Subscriber{
				Type:   event.EmailSubscriberType,
				Target: "a@gmail.com",
			},
		}
		assert.NoError(t, existingSub.Upsert())
		t.Run(name, func(t *testing.T) {
			test(t, pRef)
		})
	}
}
