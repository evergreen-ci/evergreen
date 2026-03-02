package data

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/parameterstore/fakeparameter"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/githubapp"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v70/github"
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
			config, err := evergreen.GetConfig(ctx)
			assert.NoError(t, err)
			config.GithubOrgs = []string{ref.Owner}
			assert.NoError(t, config.Set(ctx))

			assert.Empty(t, ref.SpawnHostScriptPath)
			ref.SpawnHostScriptPath = "my script path"
			ref.Owner = "something different"
			apiProjectRef := restModel.APIProjectRef{}
			assert.NoError(t, apiProjectRef.BuildFromService(t.Context(), ref.ProjectRef))

			// Appends ProjectHealthView field when building from service
			assert.Equal(t, model.ProjectHealthViewFailed, apiProjectRef.ProjectHealthView)

			apiChanges := &restModel.APIProjectSettings{
				ProjectRef: apiProjectRef,
			}

			// Shouldn't succeed if the new owner isn't in the config.
			settings, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageGeneralSection, true, "me")
			assert.Error(t, err)
			assert.Nil(t, settings)

			config.GithubOrgs = append(config.GithubOrgs, ref.Owner) // Add the new owner
			assert.NoError(t, config.Set(ctx))

			// Ensure that we're saving settings without a special case
			settings, err = SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageGeneralSection, true, "me")
			assert.NoError(t, err)
			assert.NotNil(t, settings)
			repoRefFromDB, err := model.FindOneRepoRef(t.Context(), ref.Id)
			assert.NoError(t, err)
			assert.NotNil(t, repoRefFromDB)
			assert.NotEmpty(t, repoRefFromDB.SpawnHostScriptPath)
			assert.NotEqual(t, "something different", repoRefFromDB) // we don't change this
		},
		model.ProjectPageAccessSection: func(t *testing.T, ref model.RepoRef) {
			newAdmin := user.DBUser{
				Id: "newAdmin",
			}
			require.NoError(t, newAdmin.Insert(t.Context()))
			ref.Restricted = utility.TruePtr() // should also flip the project that defaults to this repo
			ref.Admins = []string{"oldAdmin", newAdmin.Id}
			apiProjectRef := restModel.APIProjectRef{}
			assert.NoError(t, apiProjectRef.BuildFromService(t.Context(), ref.ProjectRef))
			apiChanges := &restModel.APIProjectSettings{
				ProjectRef: apiProjectRef,
			}
			settings, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageAccessSection, true, "me")
			assert.NoError(t, err)
			assert.NotNil(t, settings)
			repoRefFromDb, err := model.FindOneRepoRef(t.Context(), ref.Id)
			assert.NoError(t, err)
			assert.NotNil(t, repoRefFromDb)
			assert.True(t, repoRefFromDb.IsRestricted())
			assert.Equal(t, repoRefFromDb.Admins, ref.Admins)

			// should be restricted
			projectThatDefaults, err := model.FindMergedProjectRef(t.Context(), "myId", "", true)
			assert.NoError(t, err)
			assert.NotNil(t, projectThatDefaults)
			assert.True(t, projectThatDefaults.IsRestricted())

			// should not be restricted
			projectThatDoesNotDefault, err := model.FindMergedProjectRef(t.Context(), "myId2", "", true)
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

			newAdminFromDB, err := user.FindOneById(t.Context(), "newAdmin")
			assert.NoError(t, err)
			assert.NotNil(t, newAdminFromDB)
			assert.Contains(t, newAdminFromDB.Roles(), model.GetRepoAdminRole(ref.Id))
		},
		"Removes and adds admin with error": func(t *testing.T, ref model.RepoRef) {
			newAdmin := user.DBUser{
				Id: "newAdmin",
			}
			require.NoError(t, newAdmin.Insert(t.Context()))
			ref.Admins = []string{"nonexistent", newAdmin.Id}
			apiProjectRef := restModel.APIProjectRef{}
			assert.NoError(t, apiProjectRef.BuildFromService(t.Context(), ref.ProjectRef))
			apiChanges := &restModel.APIProjectSettings{
				ProjectRef: apiProjectRef,
			}
			settings, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageAccessSection, true, "me")
			// should still add newAdmin and delete oldAdmin even with errors
			require.Error(t, err)
			assert.Contains(t, err.Error(), "no user 'nonexistent' found")
			assert.NotNil(t, settings)
			repoRefFromDb, err := model.FindOneRepoRef(t.Context(), ref.Id)
			assert.NoError(t, err)
			assert.NotNil(t, repoRefFromDb)
			assert.Equal(t, []string{newAdmin.Id}, repoRefFromDb.Admins)

			newAdminFromDB, err := user.FindOneById(t.Context(), "newAdmin")
			assert.NoError(t, err)
			assert.NotNil(t, newAdminFromDB)
			assert.Contains(t, newAdminFromDB.Roles(), model.GetRepoAdminRole(ref.Id))

			oldAdminFromDB, err := user.FindOneById(t.Context(), "oldAdmin")
			assert.NoError(t, err)
			assert.NotNil(t, oldAdminFromDB)
			assert.NotContains(t, oldAdminFromDB.Roles(), model.GetRepoAdminRole(ref.Id))
		},
		model.ProjectPageVariablesSection: func(t *testing.T, ref model.RepoRef) {
			// remove a variable, modify a variable, add a variable
			updatedVars := model.ProjectVars{
				Id:          ref.Id,
				Vars:        map[string]string{"it": "me", "banana": "phone"},
				PrivateVars: map[string]bool{"banana": true},
			}
			apiProjectVars := restModel.APIProjectVars{}
			apiProjectVars.BuildFromService(updatedVars)
			apiChanges := &restModel.APIProjectSettings{
				Vars: apiProjectVars,
			}
			settings, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageVariablesSection, true, "me")
			assert.NoError(t, err)
			assert.NotNil(t, settings)
			varsFromDb, err := model.FindOneProjectVars(t.Context(), updatedVars.Id)
			assert.NoError(t, err)
			assert.NotNil(t, varsFromDb)
			assert.Equal(t, "me", varsFromDb.Vars["it"])
			assert.Equal(t, "phone", varsFromDb.Vars["banana"])
			assert.Equal(t, "", varsFromDb.Vars["hello"])
			assert.False(t, varsFromDb.PrivateVars["it"])
			assert.False(t, varsFromDb.PrivateVars["hello"])
			assert.True(t, varsFromDb.PrivateVars["banana"])
		},
	} {
		assert.NoError(t, db.ClearCollections(model.ProjectRefCollection, model.ProjectVarsCollection, fakeparameter.Collection,
			event.SubscriptionsCollection, event.EventCollection, evergreen.ScopeCollection, user.Collection))
		require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))

		repoRef := model.RepoRef{ProjectRef: model.ProjectRef{
			Id:         "myRepoId",
			Owner:      "evergreen-ci",
			Repo:       "evergreen",
			Restricted: utility.FalsePtr(),
			Admins:     []string{"oldAdmin"},
		}}
		assert.NoError(t, repoRef.Replace(t.Context()))

		pRefThatDefaults := model.ProjectRef{
			Id:        "myId",
			Owner:     "evergreen-ci",
			Repo:      "evergreen",
			RepoRefId: "myRepoId",
			Admins:    []string{"oldAdmin"},
		}
		assert.NoError(t, pRefThatDefaults.Replace(t.Context()))

		pRefThatDoesNotDefault := model.ProjectRef{
			Id:    "myId2",
			Owner: "evergreen-ci",
			Repo:  "evergreen",
		}
		assert.NoError(t, pRefThatDoesNotDefault.Replace(t.Context()))

		pVars := model.ProjectVars{
			Id:          repoRef.Id,
			Vars:        map[string]string{"hello": "world", "it": "adele"},
			PrivateVars: map[string]bool{"hello": true},
		}
		assert.NoError(t, pVars.Insert(t.Context()))

		// add scopes
		allProjectsScope := gimlet.Scope{
			ID:        evergreen.AllProjectsScope,
			Resources: []string{},
		}
		assert.NoError(t, rm.AddScope(t.Context(), allProjectsScope))
		restrictedScope := gimlet.Scope{
			ID:          evergreen.RestrictedProjectsScope,
			Resources:   []string{},
			ParentScope: evergreen.AllProjectsScope,
		}
		assert.NoError(t, rm.AddScope(t.Context(), restrictedScope))
		unrestrictedScope := gimlet.Scope{
			ID:          evergreen.UnrestrictedProjectsScope,
			Resources:   []string{pRefThatDefaults.Id, pRefThatDoesNotDefault.Id},
			ParentScope: evergreen.AllProjectsScope,
		}
		assert.NoError(t, rm.AddScope(t.Context(), unrestrictedScope))
		adminScope := gimlet.Scope{
			ID:        "project_scope",
			Resources: []string{pRefThatDefaults.Id},
			Type:      evergreen.ProjectResourceType,
		}
		assert.NoError(t, rm.AddScope(t.Context(), adminScope))

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
		require.NoError(t, rm.UpdateRole(t.Context(), adminRole))
		oldAdmin := user.DBUser{
			Id:          "oldAdmin",
			SystemRoles: []string{"admin"},
		}
		require.NoError(t, oldAdmin.Insert(t.Context()))

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
			config, err := evergreen.GetConfig(ctx)
			assert.NoError(t, err)
			config.GithubOrgs = []string{ref.Owner}
			assert.NoError(t, config.Set(ctx))
			ref.SpawnHostScriptPath = "my script path"
			ref.Owner = "something different"
			apiProjectRef := restModel.APIProjectRef{}
			assert.NoError(t, apiProjectRef.BuildFromService(t.Context(), ref))
			apiChanges := &restModel.APIProjectSettings{
				ProjectRef: apiProjectRef,
			}
			// Shouldn't succeed if the new owner isn't in the config.
			settings, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageGeneralSection, false, "me")
			assert.Error(t, err)
			assert.Nil(t, settings)

			config.GithubOrgs = append(config.GithubOrgs, ref.Owner) // Add the new owner
			assert.NoError(t, config.Set(ctx))

			// Ensure that we're saving settings without a special case
			settings, err = SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageGeneralSection, false, "me")
			assert.NoError(t, err)
			assert.NotNil(t, settings)
			assert.Equal(t, "myRepoId", utility.FromStringPtr(settings.ProjectRef.RepoRefId))
			pRefFromDB, err := model.FindBranchProjectRef(ctx, ref.Id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDB)
			assert.NotEmpty(t, pRefFromDB.SpawnHostScriptPath)
			assert.NotEqual(t, "something different", pRefFromDB.Owner) // because use repo settings is true, we don't change this
		},
		"github conflicts with enabling": func(t *testing.T, ref model.ProjectRef) {
			conflictingRef := model.ProjectRef{
				Identifier:          "conflicting-project",
				Owner:               ref.Owner,
				Repo:                ref.Repo,
				Branch:              ref.Branch,
				Enabled:             true,
				PRTestingEnabled:    utility.TruePtr(),
				GithubChecksEnabled: utility.TruePtr(),
				CommitQueue: model.CommitQueueParams{
					Enabled: utility.TruePtr(),
				},
			}
			assert.NoError(t, conflictingRef.Insert(t.Context()))
			ref.PRTestingEnabled = utility.TruePtr()
			ref.GithubChecksEnabled = utility.TruePtr()
			assert.NoError(t, ref.Replace(t.Context()))
			ref.Enabled = true
			apiProjectRef := restModel.APIProjectRef{}
			assert.NoError(t, apiProjectRef.BuildFromService(t.Context(), ref))
			apiChanges := &restModel.APIProjectSettings{
				ProjectRef: apiProjectRef,
			}
			_, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageGeneralSection, false, "me")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "PR testing (projects: conflicting-project) and commit checks (projects: conflicting-project)")
			assert.NotContains(t, err.Error(), "the commit queue")
		},
		"invalid URL should error when saving": func(t *testing.T, ref model.ProjectRef) {
			apiProjectRef := restModel.APIProjectRef{
				ExternalLinks: []restModel.APIExternalLink{
					{
						URLTemplate: utility.ToStringPtr("invalid URL template"),
						DisplayName: utility.ToStringPtr("display name"),
					},
				},
			}
			apiChanges := &restModel.APIProjectSettings{
				ProjectRef: apiProjectRef,
			}
			settings, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPagePluginSection, false, "me")
			require.Error(t, err)
			assert.Nil(t, settings)
			assert.Contains(t, err.Error(), "validating external links")
		},
		"valid URL should succeed when saving": func(t *testing.T, ref model.ProjectRef) {
			apiProjectRef := restModel.APIProjectRef{
				ExternalLinks: []restModel.APIExternalLink{
					{
						URLTemplate: utility.ToStringPtr("https://arnars.com/{version_id}"),
						DisplayName: utility.ToStringPtr("A link"),
					},
				},
			}
			apiChanges := &restModel.APIProjectSettings{
				ProjectRef: apiProjectRef,
			}
			settings, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPagePluginSection, false, "me")
			require.NoError(t, err)
			assert.NotNil(t, settings)
		},
		"enabling performance plugin should fail if id and identifier are different": func(t *testing.T, ref model.ProjectRef) {
			// Set identifier
			apiProjectRef := restModel.APIProjectRef{
				Identifier: utility.ToStringPtr("different identifier"),
			}
			apiChanges := &restModel.APIProjectSettings{
				ProjectRef: apiProjectRef,
			}
			settings, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageGeneralSection, false, "me")
			require.NoError(t, err)
			assert.NotNil(t, settings)

			// Try enabling performance plugin
			apiProjectRef = restModel.APIProjectRef{
				PerfEnabled: utility.TruePtr(),
			}
			apiChanges = &restModel.APIProjectSettings{
				ProjectRef: apiProjectRef,
			}
			settings, err = SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPagePluginSection, false, "me")
			require.Error(t, err)
			assert.Nil(t, settings)
			assert.Contains(t, err.Error(), "cannot enable performance plugin")
		},
		"enabling performance plugin should succeed if id and identifier are the same": func(t *testing.T, ref model.ProjectRef) {
			// Try enabling performance plugin
			apiProjectRef := restModel.APIProjectRef{
				PerfEnabled: utility.TruePtr(),
			}
			apiChanges := &restModel.APIProjectSettings{
				ProjectRef: apiProjectRef,
			}
			settings, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPagePluginSection, false, "me")
			require.NoError(t, err)
			assert.NotNil(t, settings)
		},
		"github conflicts on Commit Queue page when defaulting to repo": func(t *testing.T, ref model.ProjectRef) {
			conflictingRef := model.ProjectRef{
				Identifier:          "conflicting-project",
				Owner:               ref.Owner,
				Repo:                ref.Repo,
				Branch:              ref.Branch,
				Enabled:             true,
				PRTestingEnabled:    utility.TruePtr(),
				GithubChecksEnabled: utility.TruePtr(),
				CommitQueue: model.CommitQueueParams{
					Enabled: utility.TruePtr(),
				},
			}
			assert.NoError(t, conflictingRef.Insert(t.Context()))

			changes := model.ProjectRef{
				Id:                  ref.Id,
				PRTestingEnabled:    nil,
				GithubChecksEnabled: utility.FalsePtr(),
			}
			apiProjectRef := restModel.APIProjectRef{}
			assert.NoError(t, apiProjectRef.BuildFromService(t.Context(), changes))
			apiChanges := &restModel.APIProjectSettings{
				ProjectRef: apiProjectRef,
			}
			_, err := SaveProjectSettingsForSection(ctx, changes.Id, apiChanges, model.ProjectPageGithubAndCQSection, false, "me")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "PR testing (projects: conflicting-project)")
			assert.NotContains(t, err.Error(), "the commit queue")
			assert.NotContains(t, err.Error(), "commit checks")
		},
		model.ProjectPageGithubPermissionsSection: func(t *testing.T, ref model.ProjectRef) {
			apiChanges := &restModel.APIProjectSettings{
				ProjectRef: restModel.APIProjectRef{
					GitHubDynamicTokenPermissionGroups: []restModel.APIGitHubDynamicTokenPermissionGroup{
						{
							Name: utility.ToStringPtr("some-group"),
							Permissions: map[string]string{
								"actions": "read",
							},
						},
						{
							Name:        utility.ToStringPtr("other-group"),
							Permissions: map[string]string{}, // Should have no permissions.
						},
						{
							Name:           utility.ToStringPtr("all-group"),
							Permissions:    map[string]string{},
							AllPermissions: utility.TruePtr(), // Should have all permissions.
						},
					},
				},
			}
			settings, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageGithubPermissionsSection, false, "me")
			require.NoError(t, err)
			require.NotNil(t, settings)

			pRefFromDB, err := model.FindBranchProjectRef(ctx, ref.Id)
			require.NoError(t, err)
			require.NotNil(t, pRefFromDB)
			require.NotNil(t, pRefFromDB.GitHubDynamicTokenPermissionGroups)
			require.Len(t, pRefFromDB.GitHubDynamicTokenPermissionGroups, 3)

			assert.Equal(t, "some-group", pRefFromDB.GitHubDynamicTokenPermissionGroups[0].Name)
			require.NotNil(t, pRefFromDB.GitHubDynamicTokenPermissionGroups[0].Permissions)
			assert.Equal(t, "read", utility.FromStringPtr(pRefFromDB.GitHubDynamicTokenPermissionGroups[0].Permissions.Actions))

			assert.Equal(t, "other-group", pRefFromDB.GitHubDynamicTokenPermissionGroups[1].Name)
			require.NotNil(t, pRefFromDB.GitHubDynamicTokenPermissionGroups[1].Permissions)
			assert.False(t, pRefFromDB.GitHubDynamicTokenPermissionGroups[1].AllPermissions)

			assert.Equal(t, "all-group", pRefFromDB.GitHubDynamicTokenPermissionGroups[2].Name)
			require.NotNil(t, pRefFromDB.GitHubDynamicTokenPermissionGroups[2].Permissions)
			assert.True(t, pRefFromDB.GitHubDynamicTokenPermissionGroups[2].AllPermissions)
		},
		model.ProjectPageGithubAppSettingsSection: func(t *testing.T, ref model.ProjectRef) {
			// Should be able to save GitHub app credentials.
			apiChanges := &restModel.APIProjectSettings{
				GithubAppAuth: restModel.APIGithubAppAuth{
					AppID:      12345,
					PrivateKey: utility.ToStringPtr("my_secret"),
				},
			}
			settings, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageGithubAppSettingsSection, false, "me")
			assert.NoError(t, err)
			assert.NotNil(t, settings)

			githubAppFromDB, err := githubapp.FindOneGitHubAppAuth(t.Context(), ref.Id)
			assert.NoError(t, err)
			require.NotNil(t, githubAppFromDB)
			assert.Equal(t, int64(12345), githubAppFromDB.AppID)
			assert.Equal(t, githubAppFromDB.PrivateKey, []byte("my_secret"))

			// Should be able to update GitHub app credentials.
			apiChanges = &restModel.APIProjectSettings{
				GithubAppAuth: restModel.APIGithubAppAuth{
					AppID:      12345,
					PrivateKey: utility.ToStringPtr("my_new_secret"),
				},
			}
			settings, err = SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageGithubAppSettingsSection, false, "me")
			assert.NoError(t, err)
			assert.NotNil(t, settings)

			githubAppFromDB, err = githubapp.FindOneGitHubAppAuth(t.Context(), ref.Id)
			assert.NoError(t, err)
			require.NotNil(t, githubAppFromDB)
			assert.Equal(t, int64(12345), githubAppFromDB.AppID)
			assert.Equal(t, githubAppFromDB.PrivateKey, []byte("my_new_secret"))

			// Should not update if the private key string is {REDACTED}.
			apiChanges = &restModel.APIProjectSettings{
				GithubAppAuth: restModel.APIGithubAppAuth{
					AppID:      67890,
					PrivateKey: utility.ToStringPtr(evergreen.RedactedValue),
				},
			}
			settings, err = SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageGithubAppSettingsSection, false, "me")
			assert.NoError(t, err)
			assert.NotNil(t, settings)

			githubAppFromDB, err = githubapp.FindOneGitHubAppAuth(t.Context(), ref.Id)
			assert.NoError(t, err)
			require.NotNil(t, githubAppFromDB)
			assert.Equal(t, int64(12345), githubAppFromDB.AppID)
			assert.Equal(t, githubAppFromDB.PrivateKey, []byte("my_new_secret"))

			// Should be able to clear GitHub app credentials.
			apiChanges = &restModel.APIProjectSettings{
				GithubAppAuth: restModel.APIGithubAppAuth{
					AppID:      0,
					PrivateKey: utility.ToStringPtr(""),
				},
			}
			settings, err = SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageGithubAppSettingsSection, false, "me")
			assert.NoError(t, err)
			assert.NotNil(t, settings)

			githubAppFromDB, err = githubapp.FindOneGitHubAppAuth(t.Context(), ref.Id)
			assert.NoError(t, err)
			assert.Nil(t, githubAppFromDB)

			// Invalid requester should return an error.
			apiChanges = &restModel.APIProjectSettings{
				ProjectRef: restModel.APIProjectRef{
					GitHubPermissionGroupByRequester: map[string]string{
						"invalid-requester": "permission-group",
					},
				},
			}
			settings, err = SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageGithubAppSettingsSection, false, "me")
			assert.Error(t, err)
			assert.Nil(t, settings)

			// Invalid permission group (i.e. nonexistent permission group) should return an error.
			apiChanges = &restModel.APIProjectSettings{
				ProjectRef: restModel.APIProjectRef{
					GitHubPermissionGroupByRequester: map[string]string{
						evergreen.GitTagRequester: "nonexistent-permission-group",
					},
				},
			}
			settings, err = SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageGithubAppSettingsSection, false, "me")
			assert.Error(t, err)
			assert.Nil(t, settings)

			// Should be able to save with a valid requester and existing permission group.
			apiChanges = &restModel.APIProjectSettings{
				ProjectRef: restModel.APIProjectRef{
					GitHubPermissionGroupByRequester: map[string]string{
						evergreen.GitTagRequester: "permission-group",
					},
				},
			}
			settings, err = SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageGithubAppSettingsSection, false, "me")
			assert.NoError(t, err)
			assert.NotNil(t, settings)

			pRefFromDB, err := model.FindBranchProjectRef(ctx, ref.Id)
			assert.NoError(t, err)
			require.NotNil(t, pRefFromDB)
			require.NotNil(t, pRefFromDB.GitHubPermissionGroupByRequester)
			assert.Len(t, pRefFromDB.GitHubPermissionGroupByRequester, 1)
			assert.Equal(t, "permission-group", pRefFromDB.GitHubPermissionGroupByRequester[evergreen.GitTagRequester])

			// Should be able to save the field as nil.
			apiChanges = &restModel.APIProjectSettings{
				ProjectRef: restModel.APIProjectRef{
					GitHubPermissionGroupByRequester: nil,
				},
			}
			settings, err = SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageGithubAppSettingsSection, false, "me")
			assert.NoError(t, err)
			assert.NotNil(t, settings)

			pRefFromDB, err = model.FindBranchProjectRef(ctx, ref.Id)
			assert.NoError(t, err)
			require.NotNil(t, pRefFromDB)
			assert.Nil(t, pRefFromDB.GitHubPermissionGroupByRequester)
		},
		model.ProjectPageAccessSection: func(t *testing.T, ref model.ProjectRef) {
			newAdmin := user.DBUser{
				Id: "newAdmin",
			}
			require.NoError(t, newAdmin.Insert(t.Context()))
			ref.Restricted = nil // should now default to the repo value
			ref.Admins = []string{"oldAdmin", newAdmin.Id}
			apiProjectRef := restModel.APIProjectRef{}
			assert.NoError(t, apiProjectRef.BuildFromService(t.Context(), ref))
			apiChanges := &restModel.APIProjectSettings{
				ProjectRef: apiProjectRef,
			}
			settings, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageAccessSection, false, "me")
			assert.NoError(t, err)
			assert.NotNil(t, settings)
			pRefFromDB, err := model.FindBranchProjectRef(ctx, ref.Id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDB)
			assert.Nil(t, pRefFromDB.Restricted)
			assert.Equal(t, pRefFromDB.Admins, ref.Admins)

			mergedProject, err := model.FindMergedProjectRef(t.Context(), ref.Id, "", true)
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

			newAdminFromDB, err := user.FindOneById(t.Context(), "newAdmin")
			assert.NoError(t, err)
			assert.NotNil(t, newAdminFromDB)
			assert.Contains(t, newAdminFromDB.Roles(), "admin")
		},
		"Removes and adds admin with error": func(t *testing.T, ref model.ProjectRef) {
			newAdmin := user.DBUser{
				Id: "newAdmin",
			}
			require.NoError(t, newAdmin.Insert(t.Context()))
			ref.Admins = []string{"nonexistent", newAdmin.Id}
			apiProjectRef := restModel.APIProjectRef{}
			assert.NoError(t, apiProjectRef.BuildFromService(t.Context(), ref))
			apiChanges := &restModel.APIProjectSettings{
				ProjectRef: apiProjectRef,
			}
			settings, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageAccessSection, false, "me")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "no user 'nonexistent' found")
			assert.NotNil(t, settings)
			pRefFromDB, err := model.FindBranchProjectRef(ctx, ref.Id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDB)
			// should still add newAdmin and delete oldAdmin even with errors
			assert.Equal(t, []string{newAdmin.Id}, pRefFromDB.Admins)

			newAdminFromDB, err := user.FindOneById(t.Context(), "newAdmin")
			assert.NoError(t, err)
			assert.NotNil(t, newAdminFromDB)
			assert.Contains(t, newAdminFromDB.Roles(), "admin")

			oldAdminFromDB, err := user.FindOneById(t.Context(), "oldAdmin")
			assert.NoError(t, err)
			assert.NotNil(t, oldAdminFromDB)
			assert.NotContains(t, oldAdminFromDB.Roles(), model.GetRepoAdminRole(ref.Id))
		},
		"errors saving enabled project with no branch": func(t *testing.T, ref model.ProjectRef) {
			ref.Enabled = true
			ref.Branch = ""
			apiProjectRef := restModel.APIProjectRef{}
			assert.NoError(t, apiProjectRef.BuildFromService(t.Context(), ref))
			apiChanges := &restModel.APIProjectSettings{
				ProjectRef: apiProjectRef,
			}
			settings, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageGeneralSection, false, "me")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "branch not set on enabled project")
			assert.Nil(t, settings)
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
			assert.Equal(t, "", settings.Vars.Vars["banana"])
			assert.Equal(t, "", settings.Vars.Vars["change"])
			assert.Equal(t, "", settings.Vars.Vars["private"])
			varsFromDb, err := model.FindOneProjectVars(t.Context(), ref.Id)
			assert.NoError(t, err)
			assert.NotNil(t, varsFromDb)
			assert.Equal(t, "me", varsFromDb.Vars["it"])
			assert.Equal(t, "phone", varsFromDb.Vars["banana"])
			assert.Equal(t, "", varsFromDb.Vars["hello"])
			assert.Equal(t, "forever", varsFromDb.Vars["private"]) // ensure un-edited private variables are unchanged
			assert.Equal(t, "is good", varsFromDb.Vars["change"])  // ensure edited private variables are changed
			assert.False(t, varsFromDb.PrivateVars["it"])
			assert.False(t, varsFromDb.PrivateVars["hello"])
			assert.True(t, varsFromDb.PrivateVars["banana"])
			assert.True(t, varsFromDb.PrivateVars["private"])
			assert.True(t, varsFromDb.PrivateVars["change"])
		},
		model.ProjectPageNotificationsSection: func(t *testing.T, ref model.ProjectRef) {
			// When saving a webhook that has redacted values, it should not update to the redacted
			// values but stay as the existing values.

			// This subscription just makes sure we don't accidentally affect other subscriptions
			// when saving subscriptions.
			t.Run("SaveRedactedWebhookSecretAndHeader", func(t *testing.T) {
				noiseSubscription := event.Subscription{
					ID:           "existingSub1",
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
				assert.NoError(t, apiSub.BuildFromService(noiseSubscription))

				webhookSubscriber := restModel.APIWebhookSubscriber{
					URL:    utility.ToStringPtr("http://example.com"),
					Secret: utility.ToStringPtr("super_secret_2"),
					Headers: []restModel.APIWebhookHeader{
						{
							Key:   utility.ToStringPtr("Key"),
							Value: utility.ToStringPtr("A new value"),
						},
						{
							Key:   utility.ToStringPtr("Authorization"),
							Value: utility.ToStringPtr(evergreen.RedactedValue), // This is testing that the webhook stays redacted.
						},
					},
				}
				webhookSubscription := restModel.APISubscription{
					ID:           utility.ToStringPtr("existingSub2"),
					Owner:        utility.ToStringPtr(ref.Id),
					OwnerType:    utility.ToStringPtr(string(event.OwnerTypeProject)),
					ResourceType: utility.ToStringPtr(event.ResourceTypeTask),
					Trigger:      utility.ToStringPtr(event.TriggerSuccess),
					Selectors: []restModel.APISelector{
						{
							Type: utility.ToStringPtr("id"),
							Data: utility.ToStringPtr("1234"),
						},
					},
					Subscriber: restModel.APISubscriber{
						Type:              utility.ToStringPtr(event.EvergreenWebhookSubscriberType),
						Target:            webhookSubscriber,
						WebhookSubscriber: &webhookSubscriber,
					},
				}
				apiChanges := &restModel.APIProjectSettings{
					Subscriptions: []restModel.APISubscription{apiSub, webhookSubscription},
				}
				settings, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageNotificationsSection, false, "me")
				require.NoError(t, err)
				require.NotNil(t, settings)
				subsFromDb, err := event.FindSubscriptionsByOwner(t.Context(), ref.Id, event.OwnerTypeProject)
				require.NoError(t, err)
				require.Len(t, subsFromDb, 2)
				assert.Equal(t, event.TriggerSuccess, subsFromDb[0].Trigger)
				// Check if webhooks Authorization header is kept as before.
				webhookAPI, ok := subsFromDb[1].Subscriber.Target.(*event.WebhookSubscriber)
				require.True(t, ok)
				assert.Equal(t, "A new value", webhookAPI.Headers[0].Value, "webhook headers should persist after saving")
				assert.Equal(t, "a_very_super_secret", webhookAPI.Headers[1].Value, "Authorization header should not be changed when saving as redacted value")
				assert.Equal(t, "super_secret_2", string(webhookAPI.Secret), "webhook secret should be updated to the new value")
			})

			// This should save these new values that are not redacted values.
			// Also the noise subscription should be removed from the database.
			t.Run("SaveNewWebhookSecretAndHeader", func(t *testing.T) {
				webhookSubscriber := restModel.APIWebhookSubscriber{
					URL:    utility.ToStringPtr("http://example.com"),
					Secret: utility.ToStringPtr("super_secret_3"),
					Headers: []restModel.APIWebhookHeader{
						{
							Key:   utility.ToStringPtr("Key"),
							Value: utility.ToStringPtr("A new value"),
						},
						{
							Key:   utility.ToStringPtr("Authorization"),
							Value: utility.ToStringPtr("a_different_secret"),
						},
					},
				}
				webhookSubscription := restModel.APISubscription{
					ID:           utility.ToStringPtr("existingSub2"),
					Owner:        utility.ToStringPtr(ref.Id),
					OwnerType:    utility.ToStringPtr(string(event.OwnerTypeProject)),
					ResourceType: utility.ToStringPtr(event.ResourceTypeTask),
					Trigger:      utility.ToStringPtr(event.TriggerSuccess),
					Selectors: []restModel.APISelector{
						{
							Type: utility.ToStringPtr("id"),
							Data: utility.ToStringPtr("1234"),
						},
					},
					Subscriber: restModel.APISubscriber{
						Type:              utility.ToStringPtr(event.EvergreenWebhookSubscriberType),
						Target:            webhookSubscriber,
						WebhookSubscriber: &webhookSubscriber,
					},
				}
				apiChanges := &restModel.APIProjectSettings{
					Subscriptions: []restModel.APISubscription{webhookSubscription},
				}
				settings, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageNotificationsSection, false, "me")
				require.NoError(t, err)
				require.NotNil(t, settings)
				subsFromDb, err := event.FindSubscriptionsByOwner(t.Context(), ref.Id, event.OwnerTypeProject)
				require.NoError(t, err)
				require.Len(t, subsFromDb, 1)
				// Check if webhooks Authorization header is the new value.
				webhookAPI, ok := subsFromDb[0].Subscriber.Target.(*event.WebhookSubscriber)
				require.True(t, ok)
				assert.Equal(t, "A new value", webhookAPI.Headers[0].Value, "webhook headers should persist after saving")
				assert.Equal(t, "a_different_secret", webhookAPI.Headers[1].Value, "Authorization header should be updated to the new value")
				assert.Equal(t, "super_secret_3", string(webhookAPI.Secret), "webhook secret should be updated to the new value")
			})
		},
		model.ProjectPageTriggersSection: func(t *testing.T, ref model.ProjectRef) {
			upstreamProject := model.ProjectRef{
				Id:      "upstreamProject",
				Enabled: true,
			}
			assert.NoError(t, upstreamProject.Insert(t.Context()))
			apiProjectRef := restModel.APIProjectRef{
				Triggers: []restModel.APITriggerDefinition{
					{
						Project:           utility.ToStringPtr(upstreamProject.Id),
						Level:             utility.ToStringPtr(model.ProjectTriggerLevelTask),
						TaskRegex:         utility.ToStringPtr(".*"),
						BuildVariantRegex: utility.ToStringPtr(".*"),
						ConfigFile:        utility.ToStringPtr("myConfigFile"),
					},
				},
			}
			apiChanges := &restModel.APIProjectSettings{
				ProjectRef: apiProjectRef,
			}
			settings, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageTriggersSection, false, "me")
			assert.Error(t, err)
			assert.Nil(t, settings)

			_, err = model.GetNewRevisionOrderNumber(t.Context(), ref.Id)
			assert.NoError(t, err)
			settings, err = SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageTriggersSection, false, "me")
			assert.NoError(t, err)
			assert.NotNil(t, settings)
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
		model.ProjectPageViewsAndFiltersSection: func(t *testing.T, ref model.ProjectRef) {
			assert.Nil(t, ref.ParsleyFilters)

			// fail - empty expression
			apiProjectRef := restModel.APIProjectRef{
				ParsleyFilters: []restModel.APIParsleyFilter{
					{
						Expression:    utility.ToStringPtr(""),
						CaseSensitive: utility.FalsePtr(),
						ExactMatch:    utility.FalsePtr(),
					},
				},
			}
			apiChanges := &restModel.APIProjectSettings{
				ProjectRef: apiProjectRef,
			}
			settings, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageViewsAndFiltersSection, false, "me")
			require.Error(t, err)
			assert.Nil(t, settings)
			assert.Contains(t, err.Error(), "invalid Parsley filters: filter expression must be non-empty")

			// fail - invalid regular expression
			apiProjectRef = restModel.APIProjectRef{
				ParsleyFilters: []restModel.APIParsleyFilter{
					{
						Expression:    utility.ToStringPtr("*"),
						CaseSensitive: utility.FalsePtr(),
						ExactMatch:    utility.FalsePtr(),
					},
				},
			}
			apiChanges = &restModel.APIProjectSettings{
				ProjectRef: apiProjectRef,
			}
			settings, err = SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageViewsAndFiltersSection, false, "me")
			require.Error(t, err)
			assert.Nil(t, settings)
			assert.Contains(t, err.Error(), "invalid Parsley filters: filter expression '*' is invalid regexp")

			// fail - duplicate filters
			apiProjectRef = restModel.APIProjectRef{
				ParsleyFilters: []restModel.APIParsleyFilter{
					{
						Expression:    utility.ToStringPtr("dupe"),
						CaseSensitive: utility.FalsePtr(),
						ExactMatch:    utility.FalsePtr(),
					},
					{
						Expression:    utility.ToStringPtr("dupe"),
						CaseSensitive: utility.FalsePtr(),
						ExactMatch:    utility.FalsePtr(),
					},
				},
			}
			apiChanges = &restModel.APIProjectSettings{
				ProjectRef: apiProjectRef,
			}
			settings, err = SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageViewsAndFiltersSection, false, "me")
			require.Error(t, err)
			assert.Nil(t, settings)
			assert.Contains(t, err.Error(), "invalid Parsley filters: duplicate filter with expression 'dupe'")

			// success
			apiProjectRef = restModel.APIProjectRef{
				ParsleyFilters: []restModel.APIParsleyFilter{
					{
						Expression:    utility.ToStringPtr("filter1"),
						CaseSensitive: utility.FalsePtr(),
						ExactMatch:    utility.FalsePtr(),
					},
					{
						Expression:    utility.ToStringPtr("filter2"),
						CaseSensitive: utility.FalsePtr(),
						ExactMatch:    utility.FalsePtr(),
					},
				},
			}
			apiChanges = &restModel.APIProjectSettings{
				ProjectRef: apiProjectRef,
			}
			settings, err = SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageViewsAndFiltersSection, false, "me")
			assert.NoError(t, err)
			assert.NotNil(t, settings)

			projectFromDB, err := model.FindBranchProjectRef(ctx, ref.Id)
			assert.NoError(t, err)
			assert.NotNil(t, projectFromDB)
			assert.Len(t, projectFromDB.ParsleyFilters, 2)
		},
		model.ProjectPageTestSelectionSection: func(t *testing.T, ref model.ProjectRef) {
			apiProjectRef := restModel.APIProjectRef{
				TestSelection: restModel.APITestSelectionSettings{
					Allowed:        utility.ToBoolPtr(true),
					DefaultEnabled: utility.ToBoolPtr(false),
				},
			}
			apiChanges := &restModel.APIProjectSettings{
				ProjectRef: apiProjectRef,
			}
			settings, err := SaveProjectSettingsForSection(ctx, ref.Id, apiChanges, model.ProjectPageTestSelectionSection, false, "me")
			assert.NoError(t, err)
			assert.NotNil(t, settings)

			projectFromDB, err := model.FindBranchProjectRef(ctx, ref.Id)
			assert.NoError(t, err)
			assert.NotNil(t, projectFromDB)
			assert.Equal(t, true, utility.FromBoolPtr(projectFromDB.TestSelection.Allowed))
			assert.Equal(t, false, utility.FromBoolPtr(projectFromDB.TestSelection.DefaultEnabled))
		},
	} {
		assert.NoError(t, db.ClearCollections(model.ProjectRefCollection, model.ProjectVarsCollection, fakeparameter.Collection,
			event.SubscriptionsCollection, event.EventCollection, evergreen.ScopeCollection, user.Collection,
			model.RepositoriesCollection, evergreen.ConfigCollection))
		require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))

		pRef := model.ProjectRef{
			Id:                  "myId",
			Identifier:          "myId",
			Owner:               "evergreen-ci",
			Repo:                "evergreen",
			Branch:              "main",
			Restricted:          utility.FalsePtr(),
			RepoRefId:           "myRepoId",
			Admins:              []string{"oldAdmin"},
			RepotrackerDisabled: utility.TruePtr(),
			GitHubDynamicTokenPermissionGroups: []model.GitHubDynamicTokenPermissionGroup{
				{
					Name: "permission-group",
					Permissions: github.InstallationPermissions{
						Actions: utility.ToStringPtr("read"),
					},
				},
			},
		}
		assert.NoError(t, pRef.Insert(t.Context()))

		repoRef := model.RepoRef{ProjectRef: model.ProjectRef{
			Id:               pRef.RepoRefId,
			Restricted:       utility.TruePtr(),
			PRTestingEnabled: utility.TruePtr(),
		}}
		assert.NoError(t, repoRef.Replace(t.Context()))

		pVars := model.ProjectVars{
			Id:          pRef.Id,
			Vars:        map[string]string{"hello": "world", "it": "adele", "private": "forever", "change": "inevitable"},
			PrivateVars: map[string]bool{"hello": true, "private": true, "change": true},
		}
		assert.NoError(t, pVars.Insert(t.Context()))

		// add scopes
		allProjectsScope := gimlet.Scope{
			ID:        evergreen.AllProjectsScope,
			Resources: []string{},
		}
		assert.NoError(t, rm.AddScope(t.Context(), allProjectsScope))
		restrictedScope := gimlet.Scope{
			ID:          evergreen.RestrictedProjectsScope,
			Resources:   []string{repoRef.Id},
			ParentScope: evergreen.AllProjectsScope,
		}
		assert.NoError(t, rm.AddScope(t.Context(), restrictedScope))
		unrestrictedScope := gimlet.Scope{
			ID:          evergreen.UnrestrictedProjectsScope,
			Resources:   []string{pRef.Id},
			ParentScope: evergreen.AllProjectsScope,
		}
		assert.NoError(t, rm.AddScope(t.Context(), unrestrictedScope))
		adminScope := gimlet.Scope{
			ID:        "project_scope",
			Resources: []string{pRef.Id},
			Type:      evergreen.ProjectResourceType,
		}
		assert.NoError(t, rm.AddScope(t.Context(), adminScope))

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
		require.NoError(t, rm.UpdateRole(t.Context(), adminRole))
		oldAdmin := user.DBUser{
			Id:          "oldAdmin",
			SystemRoles: []string{"admin"},
		}
		require.NoError(t, oldAdmin.Insert(t.Context()))

		existingSub := event.Subscription{
			ID:           "existingSub1",
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
		assert.NoError(t, existingSub.Upsert(t.Context()))
		existingSub2 := event.Subscription{
			ID:           "existingSub2",
			Owner:        pRef.Id,
			OwnerType:    event.OwnerTypeProject,
			ResourceType: event.ResourceTypeTask,
			Trigger:      event.TriggerFailure,
			Selectors: []event.Selector{
				{Type: "id", Data: "1234"},
			},
			Subscriber: event.Subscriber{
				Type: event.EvergreenWebhookSubscriberType,
				Target: &event.WebhookSubscriber{
					URL:    "http://example.com",
					Secret: []byte("super_secret_1"),
					Headers: []event.WebhookHeader{
						{
							Key:   "Key",
							Value: "Value",
						},
						{
							Key:   "Authorization",
							Value: "a_very_super_secret",
						},
					},
				},
			},
		}
		assert.NoError(t, existingSub2.Upsert(t.Context()))
		t.Run(name, func(t *testing.T) {
			test(t, pRef)
		})
	}
}

func TestPromoteVarsToRepo(t *testing.T) {
	for name, test := range map[string]func(t *testing.T, ref model.ProjectRef){
		"SuccessfullyPromotesAllVariables": func(t *testing.T, ref model.ProjectRef) {
			varsToPromote := []string{"a", "b", "c"}
			err := PromoteVarsToRepo(t.Context(), ref.Id, varsToPromote, "u")
			assert.NoError(t, err)

			projectVarsFromDB, err := model.FindOneProjectVars(t.Context(), ref.Id)
			assert.NoError(t, err)
			assert.Empty(t, projectVarsFromDB.Vars)
			assert.Empty(t, projectVarsFromDB.PrivateVars)
			assert.Empty(t, projectVarsFromDB.AdminOnlyVars)

			repoVarsFromDB, err := model.FindOneProjectVars(t.Context(), ref.RepoRefId)
			assert.NoError(t, err)
			assert.Len(t, repoVarsFromDB.Vars, 4)
			assert.Len(t, repoVarsFromDB.PrivateVars, 2)
			assert.Len(t, repoVarsFromDB.AdminOnlyVars, 1)
			assert.Equal(t, "1", repoVarsFromDB.Vars["a"])
			assert.Equal(t, "2", repoVarsFromDB.Vars["b"])
			assert.Equal(t, "3", repoVarsFromDB.Vars["c"])

			projectEvents, err := model.MostRecentProjectEvents(t.Context(), ref.Id, 10)
			assert.NoError(t, err)
			assert.Len(t, projectEvents, 1)

			repoEvents, err := model.MostRecentProjectEvents(t.Context(), ref.RepoRefId, 10)
			assert.NoError(t, err)
			assert.Len(t, repoEvents, 1)
		},
		"SuccessfullyPromotesSomeVariables": func(t *testing.T, ref model.ProjectRef) {
			varsToPromote := []string{"a", "b"}
			err := PromoteVarsToRepo(t.Context(), ref.Id, varsToPromote, "u")
			assert.NoError(t, err)

			varsFromDB, err := model.FindOneProjectVars(t.Context(), ref.Id)
			assert.NoError(t, err)
			assert.Len(t, varsFromDB.Vars, 1)
			assert.Equal(t, "3", varsFromDB.Vars["c"])
			assert.Empty(t, varsFromDB.PrivateVars)
			assert.Empty(t, varsFromDB.AdminOnlyVars)

			repoVarsFromDB, err := model.FindOneProjectVars(t.Context(), ref.RepoRefId)
			assert.NoError(t, err)
			assert.Len(t, repoVarsFromDB.Vars, 3)
			assert.Len(t, repoVarsFromDB.PrivateVars, 2)
			assert.Len(t, repoVarsFromDB.AdminOnlyVars, 1)
			assert.NotContains(t, repoVarsFromDB.Vars, "c")
			assert.Equal(t, "1", repoVarsFromDB.Vars["a"])
			assert.Equal(t, "2", repoVarsFromDB.Vars["b"])

			projectEvents, err := model.MostRecentProjectEvents(t.Context(), ref.Id, 10)
			assert.NoError(t, err)
			assert.Len(t, projectEvents, 1)

			repoEvents, err := model.MostRecentProjectEvents(t.Context(), ref.RepoRefId, 10)
			assert.NoError(t, err)
			assert.Len(t, repoEvents, 1)
		},
		"CorrectlyPromotesNoVariables": func(t *testing.T, ref model.ProjectRef) {
			varsToPromote := []string{}
			err := PromoteVarsToRepo(t.Context(), ref.Id, varsToPromote, "u")
			assert.NoError(t, err)

			varsFromDB, err := model.FindOneProjectVars(t.Context(), ref.Id)
			assert.NoError(t, err)
			assert.Len(t, varsFromDB.Vars, 3)
			assert.Equal(t, "1", varsFromDB.Vars["a"])
			assert.Equal(t, "2", varsFromDB.Vars["b"])
			assert.Equal(t, "3", varsFromDB.Vars["c"])
			assert.Len(t, varsFromDB.PrivateVars, 1)
			assert.True(t, varsFromDB.PrivateVars["a"])
			assert.Empty(t, varsFromDB.AdminOnlyVars)

			repoVarsFromDB, err := model.FindOneProjectVars(t.Context(), ref.RepoRefId)
			assert.NoError(t, err)
			assert.Len(t, repoVarsFromDB.Vars, 1)
			assert.Len(t, repoVarsFromDB.PrivateVars, 1)
			assert.True(t, repoVarsFromDB.PrivateVars["d"])
			assert.True(t, repoVarsFromDB.AdminOnlyVars["d"])

			projectEvents, err := model.MostRecentProjectEvents(t.Context(), ref.Id, 10)
			assert.NoError(t, err)
			assert.Empty(t, projectEvents)

			repoEvents, err := model.MostRecentProjectEvents(t.Context(), ref.RepoRefId, 10)
			assert.NoError(t, err)
			assert.Empty(t, repoEvents)
		},
		"FailsOnUnattachedRepo": func(t *testing.T, ref model.ProjectRef) {
			varsToPromote := []string{"test"}
			err := PromoteVarsToRepo(t.Context(), "pUnattached", varsToPromote, "u")
			assert.Error(t, err)
		},
		"IgnoresNonexistentVars": func(t *testing.T, ref model.ProjectRef) {
			varsToPromote := []string{"test"}
			err := PromoteVarsToRepo(t.Context(), ref.Id, varsToPromote, "u")
			assert.NoError(t, err)

			varsFromDB, err := model.FindOneProjectVars(t.Context(), ref.Id)
			assert.NoError(t, err)
			assert.Len(t, varsFromDB.Vars, 3)
			assert.Equal(t, "1", varsFromDB.Vars["a"])
			assert.Equal(t, "2", varsFromDB.Vars["b"])
			assert.Equal(t, "3", varsFromDB.Vars["c"])
			assert.Len(t, varsFromDB.PrivateVars, 1)
			assert.True(t, varsFromDB.PrivateVars["a"])
			assert.Empty(t, varsFromDB.AdminOnlyVars)

			repoVarsFromDB, err := model.FindOneProjectVars(t.Context(), ref.RepoRefId)
			assert.NoError(t, err)
			assert.Len(t, repoVarsFromDB.Vars, 1)
			assert.Len(t, repoVarsFromDB.PrivateVars, 1)
			assert.True(t, repoVarsFromDB.PrivateVars["d"])
			assert.True(t, repoVarsFromDB.AdminOnlyVars["d"])

			projectEvents, err := model.MostRecentProjectEvents(t.Context(), ref.Id, 10)
			assert.NoError(t, err)
			assert.Empty(t, projectEvents)

			repoEvents, err := model.MostRecentProjectEvents(t.Context(), ref.RepoRefId, 10)
			assert.NoError(t, err)
			assert.Empty(t, repoEvents)
		},
	} {
		assert.NoError(t, db.ClearCollections(model.ProjectRefCollection, model.ProjectVarsCollection, fakeparameter.Collection,
			user.Collection, model.RepoRefCollection, event.EventCollection))
		require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))

		repoRef := model.RepoRef{ProjectRef: model.ProjectRef{
			Id:         "rId",
			Owner:      "evergreen-ci",
			Repo:       "evergreen",
			Restricted: utility.FalsePtr(),
			Admins:     []string{"u"},
		}}
		assert.NoError(t, repoRef.Replace(t.Context()))

		rVars := model.ProjectVars{
			Id:            repoRef.Id,
			Vars:          map[string]string{"d": "4"},
			PrivateVars:   map[string]bool{"d": true},
			AdminOnlyVars: map[string]bool{"d": true},
		}
		assert.NoError(t, rVars.Insert(t.Context()))

		pRef := model.ProjectRef{
			Id:         "pId",
			Owner:      "evergreen-ci",
			Repo:       "evergreen",
			Branch:     "main",
			Restricted: utility.FalsePtr(),
			Admins:     []string{"u"},
			RepoRefId:  "rId",
		}
		assert.NoError(t, pRef.Insert(t.Context()))

		pUnattached := model.ProjectRef{
			Id:         "pUnattached",
			Owner:      "evergreen-ci",
			Repo:       "evergreen",
			Branch:     "main",
			Restricted: utility.FalsePtr(),
		}
		assert.NoError(t, pUnattached.Insert(t.Context()))

		pVars := model.ProjectVars{
			Id:            pRef.Id,
			Vars:          map[string]string{"a": "1", "b": "2", "c": "3"},
			PrivateVars:   map[string]bool{"a": true},
			AdminOnlyVars: map[string]bool{},
		}
		assert.NoError(t, pVars.Insert(t.Context()))

		usr := user.DBUser{
			Id:          "u",
			SystemRoles: []string{"admin"},
		}
		require.NoError(t, usr.Insert(t.Context()))

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
		"SuccessfullyCopiesProject": func(t *testing.T, ref model.ProjectRef) {
			copyProjectOpts := restModel.CopyProjectOpts{
				ProjectIdToCopy:      ref.Id,
				NewProjectIdentifier: "myNewProject",
				NewProjectId:         "12345",
			}
			newProject, err := CopyProject(ctx, env, copyProjectOpts)
			assert.NoError(t, err)
			require.NotNil(t, newProject)
			assert.Equal(t, "myNewProject", utility.FromStringPtr(newProject.Identifier))
			assert.Equal(t, "12345", utility.FromStringPtr(newProject.Id))

			dbProjRef, err := model.FindBranchProjectRef(ctx, utility.FromStringPtr(newProject.Id))
			require.NoError(t, err)
			require.NotZero(t, dbProjRef)
		},
		"CopiesProjectWithPartialError": func(t *testing.T, ref model.ProjectRef) {
			copyProjectOpts := restModel.CopyProjectOpts{
				ProjectIdToCopy:      "myIdTwo",
				NewProjectIdentifier: "mySecondProject",
			}
			newProject, err := CopyProject(ctx, env, copyProjectOpts)
			assert.Error(t, err)
			require.NotNil(t, newProject)
			assert.Equal(t, "mySecondProject", utility.FromStringPtr(newProject.Identifier))
		},
		"DoesNotCopyProjectWithFatalError": func(t *testing.T, ref model.ProjectRef) {
			copyProjectOpts := restModel.CopyProjectOpts{
				ProjectIdToCopy:      "nonexistentId",
				NewProjectIdentifier: "myThirdProject",
			}
			newProject, err := CopyProject(ctx, env, copyProjectOpts)
			assert.Error(t, err)
			assert.Nil(t, newProject)
		},
	} {
		assert.NoError(t, db.ClearCollections(model.ProjectRefCollection, model.ProjectVarsCollection, fakeparameter.Collection, model.ProjectAliasCollection,
			event.SubscriptionsCollection, event.EventCollection, evergreen.ScopeCollection, user.Collection))
		require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))

		pRef := model.ProjectRef{
			Id:         "myId",
			Owner:      "evergreen-ci",
			Repo:       "evergreen",
			Branch:     "main",
			Restricted: utility.FalsePtr(),
			Enabled:    true,
			Admins:     []string{"oldAdmin"},
		}
		assert.NoError(t, pRef.Insert(t.Context()))

		pRefInvalidAdmin := model.ProjectRef{
			Id:         "myIdTwo",
			Owner:      "evergreen-ci",
			Repo:       "spruce",
			Branch:     "main",
			Restricted: utility.FalsePtr(),
			Admins:     []string{"unknownAdmin"},
		}
		assert.NoError(t, pRefInvalidAdmin.Insert(t.Context()))

		pVars := model.ProjectVars{
			Id:          pRef.Id,
			Vars:        map[string]string{"hello": "world", "it": "adele", "private": "forever", "change": "inevitable"},
			PrivateVars: map[string]bool{"hello": true, "private": true, "change": true},
		}
		assert.NoError(t, pVars.Insert(t.Context()))
		// add scopes
		allProjectsScope := gimlet.Scope{
			ID:        evergreen.AllProjectsScope,
			Resources: []string{},
		}
		assert.NoError(t, rm.AddScope(t.Context(), allProjectsScope))
		unrestrictedScope := gimlet.Scope{
			ID:          evergreen.UnrestrictedProjectsScope,
			Resources:   []string{pRef.Id},
			ParentScope: evergreen.AllProjectsScope,
		}
		assert.NoError(t, rm.AddScope(t.Context(), unrestrictedScope))
		adminScope := gimlet.Scope{
			ID:        "project_scope",
			Resources: []string{pRef.Id},
			Type:      evergreen.ProjectResourceType,
		}
		assert.NoError(t, rm.AddScope(t.Context(), adminScope))

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
		require.NoError(t, rm.UpdateRole(t.Context(), adminRole))
		oldAdmin := user.DBUser{
			Id:          "oldAdmin",
			SystemRoles: []string{"admin"},
		}
		require.NoError(t, oldAdmin.Insert(t.Context()))

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
		assert.NoError(t, existingSub.Upsert(t.Context()))
		t.Run(name, func(t *testing.T) {
			test(t, pRef)
		})
	}
}
