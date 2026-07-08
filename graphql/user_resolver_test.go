package graphql

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserSubscriptionsOwnership(t *testing.T) {
	require.NoError(t, db.ClearCollections(event.SubscriptionsCollection, user.Collection, evergreen.ScopeCollection, evergreen.RoleCollection))
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(event.SubscriptionsCollection, user.Collection, evergreen.ScopeCollection, evergreen.RoleCollection))
	})

	const targetUser = "target-user"
	sub := event.Subscription{
		ID:           "sub-id",
		ResourceType: event.ResourceTypeTask,
		Trigger:      "outcome",
		Owner:        targetUser,
		OwnerType:    event.OwnerTypePerson,
		Selectors:    []event.Selector{{Type: event.SelectorObject, Data: "task"}},
		Subscriber:   event.Subscriber{Type: event.EmailSubscriberType, Target: "target@example.com"},
	}
	require.NoError(t, sub.Upsert(t.Context()))

	roleManager := evergreen.GetEnvironment().RoleManager()
	require.NoError(t, roleManager.AddScope(t.Context(), gimlet.Scope{
		ID:        "superuser-scope",
		Type:      evergreen.SuperUserResourceType,
		Resources: []string{evergreen.SuperUserPermissionsID},
	}))
	require.NoError(t, roleManager.UpdateRole(t.Context(), gimlet.Role{
		ID:          "superuser",
		Scope:       "superuser-scope",
		Permissions: gimlet.Permissions{evergreen.PermissionAdminSettings: evergreen.AdminSettingsEdit.Value},
	}))

	config := New("/graphql")
	obj := &restModel.APIDBUser{UserID: utility.ToStringPtr(targetUser)}

	for name, testCase := range map[string]struct {
		requester     *user.DBUser
		expectAllowed bool
	}{
		"OwnerCanViewOwnSubscriptions": {
			requester:     &user.DBUser{Id: targetUser},
			expectAllowed: true,
		},
		"OtherUserIsForbidden": {
			requester:     &user.DBUser{Id: "other-user"},
			expectAllowed: false,
		},
		"SuperuserCanView": {
			requester:     &user.DBUser{Id: "admin-user", SystemRoles: []string{"superuser"}},
			expectAllowed: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := gimlet.AttachUser(t.Context(), testCase.requester)
			subs, err := config.Resolvers.User().Subscriptions(ctx, obj)
			if !testCase.expectAllowed {
				require.Error(t, err)
				assert.Nil(t, subs)
				return
			}

			require.NoError(t, err)
			require.Len(t, subs, 1)
			assert.Equal(t, sub.ID, utility.FromStringPtr(subs[0].ID))
		})
	}
}
