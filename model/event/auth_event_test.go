package event

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/anser/bsonutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func TestLogUserRolesEvent(t *testing.T) {
	defer db.Clear(AllLogCollection)

	t.Run("InvalidUserEventType", func(t *testing.T) {
		assert.Error(t, LogUserEvent("user", "invalid", []string{"role"}, []string{}))
	})
	t.Run("LogRoleChangeEvent", func(t *testing.T) {
		before := []string{"role1", "role2"}
		after := []string{"role1", "role2", "role3"}
		assert.NoError(t, LogUserEvent("user", UserEventTypeRolesUpdate, before, after))

		e := &EventLogEntry{}
		err := db.FindOne(
			AllLogCollection,
			bson.M{
				"r_type": ResourceTypeUser,
				"e_type": UserEventTypeRolesUpdate,
				bsonutil.GetDottedKeyName("data", "user"):   "user",
				bsonutil.GetDottedKeyName("data", "before"): before,
				bsonutil.GetDottedKeyName("data", "after"):  after,
			},
			nil,
			[]string{},
			e,
		)
		require.NoError(t, err)
	})
}

func TestLogRoleEvent(t *testing.T) {
	defer db.Clear(AllLogCollection)

	t.Run("InvalidRoleEventType", func(t *testing.T) {
		assert.Error(t, LogRoleEvent("invalid", &gimlet.Role{}, nil))
	})
	t.Run("LogEvent", func(t *testing.T) {
		before := &gimlet.Role{
			ID:          "role",
			Name:        "role",
			Scope:       "scope",
			Permissions: gimlet.Permissions{"project_settings": 10},
			Owners:      []string{"evergreen"},
		}
		after := &gimlet.Role{
			ID:          "role",
			Name:        "role",
			Scope:       "different_scope",
			Permissions: gimlet.Permissions{"project_settings": 10},
			Owners:      []string{"evergreen"},
		}
		assert.NoError(t, LogRoleEvent(RoleEventTypeUpdate, before, after))

		e := &EventLogEntry{}
		err := db.FindOne(
			AllLogCollection,
			bson.M{
				"r_type": ResourceTypeRole,
				"e_type": RoleEventTypeUpdate,
				bsonutil.GetDottedKeyName("data", "before", "_id"):         before.ID,
				bsonutil.GetDottedKeyName("data", "before", "name"):        before.Name,
				bsonutil.GetDottedKeyName("data", "before", "scope"):       before.Scope,
				bsonutil.GetDottedKeyName("data", "before", "permissions"): before.Permissions,
				bsonutil.GetDottedKeyName("data", "before", "owners"):      before.Owners,
				bsonutil.GetDottedKeyName("data", "after", "_id"):          after.ID,
				bsonutil.GetDottedKeyName("data", "after", "name"):         after.Name,
				bsonutil.GetDottedKeyName("data", "after", "scope"):        after.Scope,
				bsonutil.GetDottedKeyName("data", "after", "permissions"):  after.Permissions,
				bsonutil.GetDottedKeyName("data", "after", "owners"):       after.Owners,
			},
			nil,
			[]string{},
			e,
		)
		require.NoError(t, err)
	})
}
