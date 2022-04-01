package graphql

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

func init() {
	testutil.Setup()
}

func TestFilterGeneralSubscriptions(t *testing.T) {
	usr := &user.DBUser{}
	usr.Settings.Notifications = user.NotificationPreferences{
		PatchFinishID: "patch_finish_id",
		CommitQueueID: "commit_queue_id",
	}

	t.Run("NoGeneralSubscriptions", func(t *testing.T) {
		subs := []event.Subscription{
			{ID: "123455"},
			{ID: "abcdef"},
		}
		filteredSubIDs := removeGeneralSubscriptions(usr, subs)
		assert.ElementsMatch(t, []string{"123455", "abcdef"}, filteredSubIDs)
	})

	t.Run("OnlyGeneralSubscriptions", func(t *testing.T) {
		subs := []event.Subscription{
			{ID: "patch_finish_id"},
			{ID: "commit_queue_id"},
		}
		filteredSubIDs := removeGeneralSubscriptions(usr, subs)
		assert.Empty(t, filteredSubIDs)
	})

	t.Run("MixGeneralSubscriptions", func(t *testing.T) {
		subs := []event.Subscription{
			{ID: "patch_finish_id"},
			{ID: "123456"},
		}
		filteredSubIDs := removeGeneralSubscriptions(usr, subs)
		assert.ElementsMatch(t, []string{"123456"}, filteredSubIDs)
	})
}
