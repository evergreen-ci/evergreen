package auth

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/rolemanager"
	"github.com/mongodb/anser/bsonutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/mgo.v2/bson"
)

func init() {
	if evergreen.GetEnvironment() == nil {
		testutil.Setup()
	}
}

func TestEventLoggingRoleManager(t *testing.T) {
	base := rolemanager.NewInMemoryRoleManager()
	rm := NewEventLoggingRoleManager(base)
	defer db.Clear(event.AllLogCollection)

	t.Run("RemoveRole", func(t *testing.T) {
		t.Run("DNE", func(t *testing.T) {
			assert.NoError(t, rm.DeleteRole("DNE"))
		})
		t.Run("Exists", func(t *testing.T) {
			role := gimlet.Role{
				ID:    "toDelete",
				Name:  "Should Delete",
				Scope: "delete",
			}
			require.NoError(t, base.UpdateRole(role))

			assert.NoError(t, rm.DeleteRole(role.ID))
			e := &event.EventLogEntry{}
			err := db.FindOne(
				event.AllLogCollection,
				bson.M{
					bsonutil.GetDottedKeyName("data", "before", "_id"):         role.ID,
					bsonutil.GetDottedKeyName("data", "before", "name"):        role.Name,
					bsonutil.GetDottedKeyName("data", "before", "scope"):       role.Scope,
					bsonutil.GetDottedKeyName("data", "before", "permissions"): role.Permissions,
					bsonutil.GetDottedKeyName("data", "before", "owners"):      role.Owners,
					bsonutil.GetDottedKeyName("data", "after"):                 nil,
					bsonutil.GetDottedKeyName("data", "operation"):             event.RemoveRole,
				},
				nil,
				[]string{},
				e,
			)
			assert.NoError(t, err)
		})
	})
	t.Run("UpdateRole", func(t *testing.T) {
		t.Run("AddNew", func(t *testing.T) {
			role := gimlet.Role{
				ID:    "new",
				Name:  "new role",
				Scope: "scope",
			}

			require.NoError(t, rm.UpdateRole(role))
			e := &event.EventLogEntry{}
			err := db.FindOne(
				event.AllLogCollection,
				bson.M{
					bsonutil.GetDottedKeyName("data", "before"):               nil,
					bsonutil.GetDottedKeyName("data", "after", "_id"):         role.ID,
					bsonutil.GetDottedKeyName("data", "after", "name"):        role.Name,
					bsonutil.GetDottedKeyName("data", "after", "scope"):       role.Scope,
					bsonutil.GetDottedKeyName("data", "after", "permissions"): role.Permissions,
					bsonutil.GetDottedKeyName("data", "after", "owners"):      role.Owners,
					bsonutil.GetDottedKeyName("data", "operation"):            event.AddRole,
				},
				nil,
				[]string{},
				e,
			)
			assert.NoError(t, err)
		})
		t.Run("UpdateExisting", func(t *testing.T) {
			role := gimlet.Role{
				ID:    "existing",
				Name:  "existing role",
				Scope: "scope",
			}
			require.NoError(t, base.UpdateRole(role))
			role.Owners = []string{"evergreen"}

			require.NoError(t, rm.UpdateRole(role))
			e := &event.EventLogEntry{}
			err := db.FindOne(
				event.AllLogCollection,
				bson.M{
					bsonutil.GetDottedKeyName("data", "before", "_id"):         role.ID,
					bsonutil.GetDottedKeyName("data", "before", "name"):        role.Name,
					bsonutil.GetDottedKeyName("data", "before", "scope"):       role.Scope,
					bsonutil.GetDottedKeyName("data", "before", "permissions"): role.Permissions,
					bsonutil.GetDottedKeyName("data", "before", "owners"):      []string{},
					bsonutil.GetDottedKeyName("data", "after", "_id"):          role.ID,
					bsonutil.GetDottedKeyName("data", "after", "name"):         role.Name,
					bsonutil.GetDottedKeyName("data", "after", "scope"):        role.Scope,
					bsonutil.GetDottedKeyName("data", "after", "permissions"):  role.Permissions,
					bsonutil.GetDottedKeyName("data", "after", "owners"):       role.Owners,
					bsonutil.GetDottedKeyName("data", "operation"):             event.UpdateRole,
				},
				nil,
				[]string{},
				e,
			)
			assert.NoError(t, err)
		})
	})
	t.Run("Clear", func(t *testing.T) {
		t.Run("Empty", func(t *testing.T) {
			require.NoError(t, base.Clear())
			require.NoError(t, rm.Clear())
			e := &event.EventLogEntry{}
			err := db.FindOne(
				event.AllLogCollection,
				bson.M{
					bsonutil.GetDottedKeyName("data", "before", "_id"): bson.M{"$ne": "toDelete"},
					bsonutil.GetDottedKeyName("data", "operation"):     event.RemoveRole,
				},
				nil,
				[]string{},
				e,
			)
			assert.Error(t, err)

		})
		t.Run("ExistingRoles", func(t *testing.T) {
			role1 := gimlet.Role{
				ID:    "role1",
				Name:  "first role",
				Scope: "scope",
			}
			role2 := gimlet.Role{
				ID:    "role2",
				Name:  "second role",
				Scope: "scope",
			}
			require.NoError(t, base.UpdateRole(role1))
			require.NoError(t, base.UpdateRole(role2))

			require.NoError(t, rm.Clear())
			for _, role := range []gimlet.Role{role1, role2} {
				e := &event.EventLogEntry{}
				err := db.FindOne(
					event.AllLogCollection,
					bson.M{
						bsonutil.GetDottedKeyName("data", "before", "_id"):         role.ID,
						bsonutil.GetDottedKeyName("data", "before", "name"):        role.Name,
						bsonutil.GetDottedKeyName("data", "before", "scope"):       role.Scope,
						bsonutil.GetDottedKeyName("data", "before", "permissions"): role.Permissions,
						bsonutil.GetDottedKeyName("data", "before", "owners"):      role.Owners,
						bsonutil.GetDottedKeyName("data", "after"):                 nil,
						bsonutil.GetDottedKeyName("data", "operation"):             event.RemoveRole,
					},
					nil,
					[]string{},
					e,
				)
				assert.NoError(t, err)
			}
		})
	})
}
