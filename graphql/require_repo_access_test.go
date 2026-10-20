package graphql

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/require"
)

func TestRequireRepoAccess(t *testing.T) {
	setupPermissions(t)
	config := NewConfig()
	require.NotNil(t, config)

	usr, err := setupUser(t)
	require.NoError(t, err)
	require.NotNil(t, usr)

	ctx := gimlet.AttachUser(context.Background(), usr)
	require.NotNil(t, ctx)

	// The project is not yet attached to a repo; its owner/repo determine which
	// repo ref it would attach to.
	projectRef := model.ProjectRef{
		Id:         "project_id",
		Identifier: "project_identifier",
		Owner:      "evergreen-ci",
		Repo:       "spruce",
	}
	require.NoError(t, projectRef.Insert(t.Context()))

	obj := any(map[string]any{"projectId": projectRef.Id})

	// The cases run in order and share state; each setup builds on the
	// permissions and repo refs established by earlier cases.
	for _, tc := range []struct {
		name        string
		setup       func(t *testing.T)
		obj         any
		access      AccessLevel
		expectedErr string
		expectNext  bool
	}{
		{
			name:        "NonMapInputErrors",
			obj:         any(nil),
			access:      AccessLevelAdmin,
			expectedErr: "input: converting args into map",
		},
		{
			name:        "MissingProjectIDErrors",
			obj:         any(map[string]any{}),
			access:      AccessLevelAdmin,
			expectedErr: "input: project not specified",
		},
		{
			name:        "NonexistentProjectErrors",
			obj:         any(map[string]any{"projectId": "nonexistent"}),
			access:      AccessLevelAdmin,
			expectedErr: "input: project 'nonexistent' not found",
		},
		{
			// No repo ref exists yet and the user is not a project admin, so
			// they may not attach.
			name:        "NonProjectAdminCannotAttachToNewRepo",
			obj:         obj,
			access:      AccessLevelAdmin,
			expectedErr: "input: user 'test_user' must be an admin of project 'project_id' to attach it to a new repo",
		},
		{
			// A project admin may attach the project to a not-yet-existing repo.
			name: "ProjectAdminCanAttachToNewRepo",
			setup: func(t *testing.T) {
				require.NoError(t, usr.AddRole(t.Context(), "admin_project"))
			},
			obj:        obj,
			access:     AccessLevelAdmin,
			expectNext: true,
		},
		{
			// Once a repo ref exists, ADMIN access requires repo admin even for
			// a project admin.
			name: "AdminAccessRequiresRepoAdmin",
			setup: func(t *testing.T) {
				repoRef := model.RepoRef{ProjectRef: model.ProjectRef{
					Id:    "repo_id",
					Owner: "evergreen-ci",
					Repo:  "spruce",
				}}
				require.NoError(t, repoRef.Replace(t.Context()))
			},
			obj:         obj,
			access:      AccessLevelAdmin,
			expectedErr: "input: user 'test_user' is not an admin of repo 'evergreen-ci/spruce'",
		},
		{
			name:        "UnsupportedAccessLevelErrors",
			obj:         obj,
			access:      AccessLevelEdit,
			expectedErr: "input: invalid access level 'EDIT' for repo",
		},
		{
			// Grant the user repo edit access, making them a repo admin.
			name: "RepoAdminHasAdminAccess",
			setup: func(t *testing.T) {
				roleManager := evergreen.GetEnvironment().RoleManager()
				repoScope := gimlet.Scope{
					ID:        "repo_scope",
					Name:      "repo scope",
					Type:      evergreen.ProjectResourceType,
					Resources: []string{"repo_id"},
				}
				require.NoError(t, roleManager.AddScope(t.Context(), repoScope))
				repoAdminRole := gimlet.Role{
					ID:    "admin_repo",
					Scope: repoScope.ID,
					Permissions: map[string]int{
						evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value,
					},
				}
				require.NoError(t, roleManager.UpdateRole(t.Context(), repoAdminRole))
				require.NoError(t, usr.AddRole(t.Context(), "admin_repo"))
			},
			obj:        obj,
			access:     AccessLevelAdmin,
			expectNext: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setup != nil {
				tc.setup(t)
			}

			nextCalled := false
			next := func(rctx context.Context) (any, error) {
				nextCalled = true
				return nil, nil
			}

			res, err := config.Directives.RequireRepoAccess(ctx, tc.obj, next, tc.access)
			require.Nil(t, res)
			if tc.expectedErr == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tc.expectedErr)
			}
			require.Equal(t, tc.expectNext, nextCalled)
		})
	}
}
