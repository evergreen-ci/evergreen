package graphql

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequirePatchScheduleAccess(t *testing.T) {
	for testName, testCase := range map[string]struct {
		roles       []string
		isPR        bool
		isFinalized bool
		forbidden   bool
	}{
		"PatchSubmitShouldAllowAuthorizingPendingPRFromAnotherAuthor": {
			roles: []string{"edit_patch"},
			isPR:  true,
		},
		"NoRolesShouldForbidPendingPR": {
			isPR:      true,
			forbidden: true,
		},
		"PatchSubmitShouldForbidReconfiguringFinalizedPRFromAnotherAuthor": {
			roles:       []string{"edit_patch"},
			isPR:        true,
			isFinalized: true,
			forbidden:   true,
		},
		"PatchSubmitShouldForbidSchedulingCLIPatchFromAnotherAuthor": {
			roles:     []string{"edit_patch"},
			forbidden: true,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			setupPermissions(t)
			t.Cleanup(func() {
				require.NoError(t, db.ClearCollections(patch.Collection, evergreen.RoleCollection, evergreen.ScopeCollection))
			})
			usr := &user.DBUser{Id: testUser, SystemRoles: testCase.roles}
			ctx := gimlet.AttachUser(t.Context(), usr)

			p := &patch.Patch{Id: bson.NewObjectId(), Project: "project_id", Author: "other_author"}
			if testCase.isPR {
				p.GithubPatchData.HeadOwner = "other_author"
			}
			if testCase.isFinalized {
				p.Version = p.Id.Hex()
			}
			require.NoError(t, p.Insert(t.Context()))

			nextCalled := false
			next := func(context.Context) (any, error) {
				nextCalled = true
				return nil, nil
			}
			_, err := RequirePatchScheduleAccess(ctx, map[string]any{"patchId": p.Id.Hex()}, next)
			if testCase.forbidden {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "does not have permission")
				assert.False(t, nextCalled)
				return
			}
			require.NoError(t, err)
			assert.True(t, nextCalled)
		})
	}
}
