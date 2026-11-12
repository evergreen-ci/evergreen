package model

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestRepoOnlyViewPermissionWithoutAttachedProjectsSucceeds(t *testing.T) {
	ctx := t.Context()
	collections := []string{ProjectRefCollection, RepoRefCollection, evergreen.ScopeCollection, evergreen.RoleCollection}
	require.NoError(t, db.ClearCollections(collections...))
	t.Cleanup(func() { require.NoError(t, db.ClearCollections(collections...)) })
	require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))

	repo := &RepoRef{ProjectRef: ProjectRef{Id: "repo", Owner: "owner", Repo: "repo"}}
	require.NoError(t, repo.Add(ctx, nil))
	usr := &user.DBUser{}
	hasPermission, err := UserHasRepoViewPermission(ctx, usr, repo.Id)
	require.NoError(t, err)
	assert.False(t, hasPermission)

	rm := evergreen.GetEnvironment().RoleManager()
	role := gimlet.Role{
		ID:    "repoViewer",
		Scope: GetRepoOnlyScope(repo.Id),
		Permissions: gimlet.Permissions{
			evergreen.PermissionProjectSettings: evergreen.ProjectSettingsView.Value,
		},
	}
	require.NoError(t, rm.UpdateRole(ctx, role))
	usr.SystemRoles = []string{role.ID}

	hasPermission, err = UserHasRepoViewPermission(ctx, usr, repo.Id)
	require.NoError(t, err)
	assert.True(t, hasPermission)
}

func TestReplacingHiddenProjectUpdatesRepoAndRestrictionScopes(t *testing.T) {
	for _, tc := range []struct {
		name              string
		attached          bool
		repoRestricted    bool
		projectRestricted *bool
		creatorRoles      []string
		wantUnauthorized  bool
	}{
		{name: "StandaloneProjectLosesRepoAccess"},
		{name: "StandaloneRestrictedProjectLosesUnrestrictedAccess", projectRestricted: utility.TruePtr()},
		{name: "AttachedRestrictedProjectLosesRepoEditorAccess", attached: true, projectRestricted: utility.TruePtr()},
		{name: "AttachedProjectKeepsRepoAccess", attached: true},
		{name: "AttachedProjectInheritsRepoRestriction", attached: true, repoRestricted: true},
		{name: "AttachedProjectOverridesRepoRestriction", attached: true, repoRestricted: true, projectRestricted: utility.FalsePtr()},
		{name: "AttachedProjectByRepoAdminKeepsRepoAccess", attached: true, creatorRoles: []string{GetRepoAdminRole("repo")}},
		{name: "AttachedProjectByNonRepoAdminShouldError", attached: true, creatorRoles: []string{GetProjectAdminRole("hidden")}, wantUnauthorized: true},
		{name: "StandaloneProjectByProjectAdminLosesRepoAccess", creatorRoles: []string{GetProjectAdminRole("hidden")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			collections := []string{ProjectRefCollection, RepoRefCollection, ProjectVarsCollection, evergreen.ScopeCollection, evergreen.RoleCollection, user.Collection}
			require.NoError(t, db.ClearCollections(collections...))
			t.Cleanup(func() { require.NoError(t, db.ClearCollections(collections...)) })
			require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))
			rm := evergreen.GetEnvironment().RoleManager()
			for _, scopeID := range []string{evergreen.UnrestrictedProjectsScope, evergreen.RestrictedProjectsScope} {
				require.NoError(t, rm.AddScope(ctx, gimlet.Scope{ID: scopeID, Type: evergreen.ProjectResourceType, Resources: []string{}}))
			}
			repo := &RepoRef{ProjectRef: ProjectRef{Id: "repo", Owner: "owner", Repo: "repo", Restricted: &tc.repoRestricted}}
			require.NoError(t, repo.Add(ctx, nil))
			hidden := &ProjectRef{Id: "hidden", Owner: repo.Owner, Repo: repo.Repo, Branch: "branch", RepoRefId: repo.Id, Hidden: utility.TruePtr()}
			require.NoError(t, hidden.Add(ctx, nil))
			editorRole := gimlet.Role{ID: "repoEditor", Scope: GetUnrestrictedBranchProjectsScope(repo.Id), Permissions: gimlet.Permissions{evergreen.PermissionTasks: evergreen.TasksBasic.Value}}
			require.NoError(t, rm.UpdateRole(ctx, editorRole))
			editor := &user.DBUser{SystemRoles: []string{editorRole.ID}}
			admin := &user.DBUser{SystemRoles: []string{GetRepoAdminRole(repo.Id)}}
			permission := gimlet.PermissionOpts{Resource: hidden.Id, ResourceType: evergreen.ProjectResourceType, Permission: evergreen.PermissionTasks, RequiredLevel: evergreen.TasksBasic.Value}
			assert.True(t, admin.HasPermission(ctx, permission))
			assert.Equal(t, !tc.repoRestricted, editor.HasPermission(ctx, permission))

			project := &ProjectRef{Id: "newID", Identifier: "tracked", Owner: repo.Owner, Repo: repo.Repo, Branch: hidden.Branch, Restricted: tc.projectRestricted}
			if tc.attached {
				project.RepoRefId = repo.Id
			}
			var creator *user.DBUser
			if tc.creatorRoles != nil {
				creator = &user.DBUser{Id: "creator", SystemRoles: tc.creatorRoles}
				require.NoError(t, creator.Insert(ctx))
			}
			err := project.Add(ctx, creator)
			if tc.wantUnauthorized {
				require.Error(t, err)
				assert.True(t, errors.Is(err, ErrRepoRefUnauthorized), "expected an authorization error, got '%s'", err)
				stored, err := FindBranchProjectRef(ctx, hidden.Id)
				require.NoError(t, err)
				require.NotNil(t, stored)
				assert.True(t, stored.IsHidden())
				assert.Empty(t, stored.Identifier)
				assert.True(t, admin.HasPermission(ctx, permission))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, hidden.Id, project.Id)
			stored, err := FindBranchProjectRef(ctx, project.Id)
			require.NoError(t, err)
			require.NotNil(t, stored)
			assert.False(t, stored.IsHidden())
			assert.Equal(t, project.RepoRefId, stored.RepoRefId)
			restricted := utility.FromBoolPtr(tc.projectRestricted)
			if tc.attached && tc.projectRestricted == nil {
				restricted = tc.repoRestricted
			}
			assert.Equal(t, tc.attached, admin.HasPermission(ctx, permission))
			assert.Equal(t, tc.attached && !restricted, editor.HasPermission(ctx, permission))
			for scopeID, shouldContain := range map[string]bool{
				evergreen.UnrestrictedProjectsScope: !restricted,
				evergreen.RestrictedProjectsScope:   restricted,
			} {
				scope, err := rm.GetScope(ctx, scopeID)
				require.NoError(t, err)
				require.NotNil(t, scope)
				assert.Equal(t, shouldContain, utility.StringSliceContains(scope.Resources, project.Id))
			}
		})
	}
}

func TestReplacingHiddenProjectFailuresPreserveProjectAndScopes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		attached bool
	}{
		{name: "AttachedProjectReplaceFailurePreservesScopes", attached: true},
		{name: "StandaloneProjectReplaceFailurePreservesScopes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			collections := []string{ProjectRefCollection, RepoRefCollection, ProjectVarsCollection, evergreen.ScopeCollection, evergreen.RoleCollection, user.Collection}
			require.NoError(t, db.ClearCollections(collections...))
			t.Cleanup(func() { require.NoError(t, db.ClearCollections(collections...)) })
			require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))
			rm := evergreen.GetEnvironment().RoleManager()
			for _, scopeID := range []string{evergreen.UnrestrictedProjectsScope, evergreen.RestrictedProjectsScope} {
				require.NoError(t, rm.AddScope(ctx, gimlet.Scope{ID: scopeID, Type: evergreen.ProjectResourceType, Resources: []string{}}))
			}
			repo := &RepoRef{ProjectRef: ProjectRef{Id: "repo", Owner: "owner", Repo: "repo"}}
			require.NoError(t, repo.Add(ctx, nil))
			hidden := &ProjectRef{Id: "hidden", Owner: repo.Owner, Repo: repo.Repo, Branch: "branch", RepoRefId: repo.Id, Hidden: utility.TruePtr()}
			require.NoError(t, hidden.Add(ctx, nil))
			creator := &user.DBUser{Id: "creator", SystemRoles: []string{GetProjectAdminRole(hidden.Id), GetRepoAdminRole(repo.Id)}}
			require.NoError(t, creator.Insert(ctx))

			before, err := FindBranchProjectRef(ctx, hidden.Id)
			require.NoError(t, err)
			require.NotNil(t, before)
			beforeScopes := map[string]*gimlet.Scope{}
			for _, scopeID := range []string{
				evergreen.UnrestrictedProjectsScope,
				evergreen.RestrictedProjectsScope,
				GetRepoAdminScope(repo.Id),
				GetUnrestrictedBranchProjectsScope(repo.Id),
				"project_" + hidden.Id,
			} {
				scope, err := rm.GetScope(ctx, scopeID)
				require.NoError(t, err)
				require.NotNil(t, scope)
				beforeScopes[scopeID] = scope
			}
			project := &ProjectRef{Id: "newID", Identifier: "replacement", Owner: repo.Owner, Repo: repo.Repo, Branch: hidden.Branch}
			if tc.attached {
				project.RepoRefId = repo.Id
			}
			require.NoError(t, (&ProjectRef{Id: "occupied", Identifier: project.Identifier}).Insert(ctx))
			const indexName = "replacementFailureIdentifier"
			require.NoError(t, db.EnsureIndex(ProjectRefCollection, mongo.IndexModel{
				Keys:    bson.D{{Key: ProjectRefIdentifierKey, Value: 1}},
				Options: options.Index().SetName(indexName).SetUnique(true),
			}))
			t.Cleanup(func() {
				_, err := evergreen.GetEnvironment().DB().Collection(ProjectRefCollection).Indexes().DropOne(context.WithoutCancel(ctx), indexName)
				require.NoError(t, err)
			})

			err = project.Add(ctx, creator)
			require.Error(t, err)
			assert.True(t, mongo.IsDuplicateKeyError(err), "expected a duplicate identifier error, got '%s'", err)
			after, err := FindBranchProjectRef(ctx, hidden.Id)
			require.NoError(t, err)
			require.NotNil(t, after)
			assert.Equal(t, before, after)
			for scopeID, beforeScope := range beforeScopes {
				afterScope, err := rm.GetScope(ctx, scopeID)
				require.NoError(t, err)
				assert.Equal(t, beforeScope, afterScope, "scope '%s' should be unchanged", scopeID)
			}
		})
	}
}

func TestEnsuringRepoPermissionScopesCreatesMissingScopesWithInheritedRestrictions(t *testing.T) {
	ctx := t.Context()
	rm := setupRepoPermissionScopeTest(t)
	repo := &RepoRef{ProjectRef: ProjectRef{Id: "repo", Restricted: utility.TruePtr()}}
	for _, project := range []ProjectRef{
		{Id: "inheritsRestriction", RepoRefId: repo.Id},
		{Id: "explicitlyUnrestricted", RepoRefId: repo.Id, Restricted: utility.FalsePtr()},
		{Id: "explicitlyRestricted", RepoRefId: repo.Id, Restricted: utility.TruePtr()},
		{Id: "notAttached"},
	} {
		require.NoError(t, project.Insert(ctx))
	}
	require.NoError(t, EnsureRepoPermissionScopes(ctx, rm, repo))
	require.NoError(t, EnsureRepoPermissionScopes(ctx, rm, repo))
	for kind, expectedResources := range map[RepoPermissionScope][]string{
		RepoOnlyScope:         {repo.Id},
		RepoAndProjectsScope:  {repo.Id, "inheritsRestriction", "explicitlyUnrestricted", "explicitlyRestricted"},
		RepoUnrestrictedScope: {"explicitlyUnrestricted"},
	} {
		scope, err := rm.GetScope(ctx, RepoPermissionScopes(repo.Id)[kind])
		require.NoError(t, err)
		require.NotNil(t, scope)
		assert.Equal(t, repo.Id, scope.Name)
		assert.Equal(t, evergreen.ProjectResourceType, scope.Type)
		assert.ElementsMatch(t, expectedResources, scope.Resources)
	}
}

func TestConcurrentRepoPermissionScopeInitializationToleratesDuplicateKeys(t *testing.T) {
	rm := setupRepoPermissionScopeTest(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	var workers sync.WaitGroup
	t.Cleanup(func() {
		cancel()
		workers.Wait()
	})
	repo := &RepoRef{ProjectRef: ProjectRef{Id: "repo", Owner: "owner", Repo: "repo"}}
	require.NoError(t, repo.Add(ctx, nil))
	scopeID := GetRepoOnlyScope(repo.Id)
	scope, err := rm.GetScope(ctx, scopeID)
	require.NoError(t, err)
	require.NotNil(t, scope)
	require.NoError(t, rm.DeleteScope(ctx, *scope))

	lookedUp := make(chan struct{}, 2)
	resume := make(chan struct{})
	var duplicateKeys atomic.Int32
	manager := &repoPermissionScopeTestRoleManager{
		RoleManager: rm,
		getScope: func(ctx context.Context, id string) (*gimlet.Scope, error) {
			scope, err := rm.GetScope(ctx, id)
			if err != nil || scope != nil || id != scopeID {
				return scope, err
			}
			select {
			case lookedUp <- struct{}{}:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			select {
			case <-resume:
				return nil, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
		addScope: func(ctx context.Context, scope gimlet.Scope) error {
			err := rm.AddScope(ctx, scope)
			if mongo.IsDuplicateKeyError(err) {
				duplicateKeys.Add(1)
			}
			return err
		},
	}
	results := make(chan error, 2)
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			results <- EnsureRepoPermissionScopes(ctx, manager, repo)
		}()
	}
	for range 2 {
		select {
		case <-lookedUp:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	close(resume)
	for range 2 {
		require.NoError(t, <-results)
	}
	assert.EqualValues(t, 1, duplicateKeys.Load())
	scope, err = rm.GetScope(ctx, scopeID)
	require.NoError(t, err)
	require.NotNil(t, scope)
	assert.Equal(t, repo.Id, scope.Name)
	assert.Equal(t, []string{repo.Id}, scope.Resources)
}

func TestEnsuringRepoPermissionScopesPreservesConcurrentAttachments(t *testing.T) {
	ctx := t.Context()
	rm := setupRepoPermissionScopeTest(t)
	repo := &RepoRef{ProjectRef: ProjectRef{Id: "repo", Owner: "owner", Repo: "repo"}}
	require.NoError(t, repo.Add(ctx, nil))
	project := &ProjectRef{Id: "newlyAttached", RepoRefId: repo.Id}
	var attached bool
	manager := &repoPermissionScopeTestRoleManager{
		RoleManager: rm,
		getScope: func(ctx context.Context, id string) (*gimlet.Scope, error) {
			if id == GetUnrestrictedBranchProjectsScope(repo.Id) && !attached {
				if err := project.Insert(ctx); err != nil {
					return nil, err
				}
				for _, scopeID := range []string{GetRepoAdminScope(repo.Id), GetUnrestrictedBranchProjectsScope(repo.Id)} {
					if err := rm.AddResourceToScope(ctx, scopeID, project.Id); err != nil {
						return nil, err
					}
				}
				attached = true
			}
			return rm.GetScope(ctx, id)
		},
	}
	require.NoError(t, EnsureRepoPermissionScopes(ctx, manager, repo))
	assert.True(t, attached)
	for _, scopeID := range []string{GetRepoAdminScope(repo.Id), GetUnrestrictedBranchProjectsScope(repo.Id)} {
		scope, err := rm.GetScope(ctx, scopeID)
		require.NoError(t, err)
		require.NotNil(t, scope)
		assert.Contains(t, scope.Resources, project.Id)
	}
}

func TestEnsuringRepoPermissionScopesPropagatesCreationErrors(t *testing.T) {
	rm := setupRepoPermissionScopeTest(t)
	creationErr := errors.New("scope write unavailable")
	manager := &repoPermissionScopeTestRoleManager{
		RoleManager: rm,
		addScope: func(context.Context, gimlet.Scope) error {
			return creationErr
		},
	}
	err := EnsureRepoPermissionScopes(t.Context(), manager, &RepoRef{ProjectRef: ProjectRef{Id: "repo"}})
	require.ErrorIs(t, err, creationErr)
}

func setupRepoPermissionScopeTest(t *testing.T) gimlet.RoleManager {
	t.Helper()
	collections := []string{ProjectRefCollection, RepoRefCollection, evergreen.ScopeCollection, evergreen.RoleCollection}
	require.NoError(t, db.ClearCollections(collections...))
	t.Cleanup(func() { require.NoError(t, db.ClearCollections(collections...)) })
	require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))
	return evergreen.GetEnvironment().RoleManager()
}

type repoPermissionScopeTestRoleManager struct {
	gimlet.RoleManager
	getScope func(context.Context, string) (*gimlet.Scope, error)
	addScope func(context.Context, gimlet.Scope) error
}

func (m *repoPermissionScopeTestRoleManager) GetScope(ctx context.Context, id string) (*gimlet.Scope, error) {
	if m.getScope != nil {
		return m.getScope(ctx, id)
	}
	return m.RoleManager.GetScope(ctx, id)
}

func (m *repoPermissionScopeTestRoleManager) AddScope(ctx context.Context, scope gimlet.Scope) error {
	if m.addScope != nil {
		return m.addScope(ctx, scope)
	}
	return m.RoleManager.AddScope(ctx, scope)
}

func TestRepoPermissionScopeReconciliationRespectsInheritedRestrictions(t *testing.T) {
	ctx := t.Context()
	require.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection, evergreen.ScopeCollection, evergreen.RoleCollection))
	require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))
	rm := evergreen.GetEnvironment().RoleManager()
	repo := &RepoRef{ProjectRef: ProjectRef{Id: "repo", Owner: "owner", Repo: "repo", Restricted: utility.TruePtr()}}
	require.NoError(t, repo.Add(ctx, nil))
	projects := []ProjectRef{
		{Id: "inheritsRestriction", RepoRefId: repo.Id},
		{Id: "explicitlyUnrestricted", RepoRefId: repo.Id, Restricted: utility.FalsePtr()},
		{Id: "explicitlyRestricted", RepoRefId: repo.Id, Restricted: utility.TruePtr()},
		{Id: "notAttached", Owner: repo.Owner, Repo: repo.Repo},
	}
	for _, project := range projects {
		require.NoError(t, project.Insert(ctx))
	}
	// Repair the stale membership that existed before repo entitlements.
	require.NoError(t, rm.AddResourceToScope(ctx, GetUnrestrictedBranchProjectsScope(repo.Id), "inheritsRestriction"))
	require.NoError(t, ReconcileRepoPermissionScopes(ctx, rm, repo))
	scope, err := rm.GetScope(ctx, GetUnrestrictedBranchProjectsScope(repo.Id))
	require.NoError(t, err)
	require.NotNil(t, scope)
	assert.ElementsMatch(t, []string{"explicitlyUnrestricted"}, scope.Resources)
	adminScope, err := rm.GetScope(ctx, GetRepoAdminScope(repo.Id))
	require.NoError(t, err)
	require.NotNil(t, adminScope)
	assert.ElementsMatch(t, []string{repo.Id, "inheritsRestriction", "explicitlyRestricted", "explicitlyUnrestricted"}, adminScope.Resources)
	settingsScope, err := rm.GetScope(ctx, GetRepoOnlyScope(repo.Id))
	require.NoError(t, err)
	require.NotNil(t, settingsScope)
	assert.Equal(t, []string{repo.Id}, settingsScope.Resources)

	// Restriction changes preserve explicit branch overrides.
	repo.Restricted = utility.FalsePtr()
	branches, err := FindMergedProjectRefsForRepo(ctx, repo)
	require.NoError(t, err)
	require.NoError(t, repo.MakeUnrestricted(ctx, branches))
	scope, err = rm.GetScope(ctx, GetUnrestrictedBranchProjectsScope(repo.Id))
	require.NoError(t, err)
	require.NotNil(t, scope)
	assert.ElementsMatch(t, []string{"explicitlyUnrestricted", "inheritsRestriction"}, scope.Resources)
	repo.Restricted = utility.TruePtr()
	branches, err = FindMergedProjectRefsForRepo(ctx, repo)
	require.NoError(t, err)
	require.NoError(t, repo.MakeRestricted(ctx, branches))
	scope, err = rm.GetScope(ctx, GetUnrestrictedBranchProjectsScope(repo.Id))
	require.NoError(t, err)
	require.NotNil(t, scope)
	assert.ElementsMatch(t, []string{"explicitlyUnrestricted"}, scope.Resources)
	project := projects[0]
	require.NoError(t, project.RemoveFromRepoScope(ctx))
	scope, err = rm.GetScope(ctx, GetUnrestrictedBranchProjectsScope(repo.Id))
	require.NoError(t, err)
	require.NotNil(t, scope)
	assert.NotContains(t, scope.Resources, project.Id)
}

func TestNewUntrackedBranchesInheritRepoRestrictionsInScopes(t *testing.T) {
	ctx := t.Context()
	require.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection, ProjectVarsCollection, evergreen.ScopeCollection, evergreen.RoleCollection))
	require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))
	rm := evergreen.GetEnvironment().RoleManager()
	repo := &RepoRef{ProjectRef: ProjectRef{
		Id: "repo", Owner: "owner", Repo: "repo", Restricted: utility.TruePtr(),
		RemotePath: "evergreen.yml", PRTestingEnabled: utility.TruePtr(),
	}}
	require.NoError(t, repo.Add(ctx, nil))
	project, err := FindOneProjectRefByRepoAndBranchWithPRTesting(ctx, repo.Owner, repo.Repo, "untracked", patch.AutomatedCaller)
	require.NoError(t, err)
	require.NotNil(t, project)
	assert.True(t, project.IsHidden())
	merged, err := GetProjectRefMergedWithRepo(ctx, *project)
	require.NoError(t, err)
	require.NotNil(t, merged)
	assert.True(t, merged.IsRestricted())
	for _, scopeID := range []string{GetUnrestrictedBranchProjectsScope(repo.Id), evergreen.UnrestrictedProjectsScope} {
		scope, err := rm.GetScope(ctx, scopeID)
		require.NoError(t, err)
		if scope != nil {
			assert.NotContains(t, scope.Resources, project.Id)
		}
	}
	adminScope, err := rm.GetScope(ctx, GetRepoAdminScope(repo.Id))
	require.NoError(t, err)
	require.NotNil(t, adminScope)
	assert.Contains(t, adminScope.Resources, project.Id)
}
