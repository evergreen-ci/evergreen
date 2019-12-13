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

	t.Run("InvalidRoleChangeOperation", func(t *testing.T) {
		assert.Error(t, LogUserRolesEvent("user", "role", "invalid"))
	})
	t.Run("LogEvent", func(t *testing.T) {
		assert.NoError(t, LogUserRolesEvent("user", "role", AddRole))

		e := &EventLogEntry{}
		err := db.FindOne(
			AllLogCollection,
			bson.M{
				"r_type": ResourceTypeUserRoles,
				"e_type": EventTypeUserRoles,
				bsonutil.GetDottedKeyName("data", "user"):      "user",
				bsonutil.GetDottedKeyName("data", "role_id"):   "role",
				bsonutil.GetDottedKeyName("data", "operation"): AddRole,
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

	t.Run("InvalidRoleChangeOperation", func(t *testing.T) {
		assert.Error(t, LogRoleEvent(&gimlet.Role{}, nil, "invalid"))
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
		assert.NoError(t, LogRoleEvent(before, after, UpdateRole))

		e := &EventLogEntry{}
		err := db.FindOne(
			AllLogCollection,
			bson.M{
				"r_type": ResourceTypeRole,
				"e_type": EventTypeRole,
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
				bsonutil.GetDottedKeyName("data", "operation"):             UpdateRole,
			},
			nil,
			[]string{},
			e,
		)
		require.NoError(t, err)
	})
}
