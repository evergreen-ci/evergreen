package event

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func TestLogUserRolesEvent(t *testing.T) {
	defer func() {
		require.NoError(t, db.Clear(EventCollection))
	}()
	t.Run("InvalidUserEventType", func(t *testing.T) {
		assert.Error(t, LogUserEvent(t.Context(), "user", "invalid", []string{"role"}, []string{}))
	})
	t.Run("LogRoleChangeEvent", func(t *testing.T) {
		before := []string{"role1", "role2"}
		after := []string{"role1", "role2", "role3"}
		assert.NoError(t, LogUserEvent(t.Context(), "user", UserEventTypeRolesUpdate, before, after))

		e := &EventLogEntry{}
		q := db.Query(bson.M{
			"r_type": ResourceTypeUser,
			"e_type": UserEventTypeRolesUpdate,
			bsonutil.GetDottedKeyName("data", "user"):   "user",
			bsonutil.GetDottedKeyName("data", "before"): before,
			bsonutil.GetDottedKeyName("data", "after"):  after,
		})
		err := db.FindOneQ(t.Context(), EventCollection, q, e)
		require.NoError(t, err)
	})
}
