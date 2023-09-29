package data

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMakeJiraTicket(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, db.ClearCollections(task.Collection, model.VersionCollection, build.Collection, model.ProjectRefCollection, notification.Collection))
	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection, model.VersionCollection, build.Collection, model.ProjectRefCollection, notification.Collection))
	}()

	t1 := task.Task{
		Id:      "t1",
		Version: "v",
		Project: "proj",
		BuildId: "b",
	}
	assert.NoError(t, t1.Insert())
	v := model.Version{
		Id:       "v",
		Revision: "1234567890",
	}
	assert.NoError(t, v.Insert())
	b := build.Build{
		Id: "b",
	}
	assert.NoError(t, b.Insert())
	p := model.ProjectRef{
		Identifier: "proj",
	}
	assert.NoError(t, p.Insert())

	evgSettings := evergreen.Settings{
		Ui: evergreen.UIConfig{
			Url: "www.example.com",
		},
	}

	checkNotificationCreated := func(t *testing.T, n *notification.Notification, expectedSub event.JIRAIssueSubscriber) {
		assert.NotZero(t, n.ID)
		assert.Equal(t, event.JIRAIssueSubscriberType, n.Subscriber.Type)
		jiraSub, ok := n.Subscriber.Target.(event.JIRAIssueSubscriber)
		require.True(t, ok)
		assert.EqualValues(t, expectedSub, jiraSub)

		dbNotification, err := notification.Find(n.ID)
		require.NoError(t, err)
		require.NotZero(t, dbNotification)
		assert.Equal(t, n.Subscriber.Type, dbNotification.Subscriber.Type)
		dbJiraSub, ok := n.Subscriber.Target.(event.JIRAIssueSubscriber)
		require.True(t, ok)
		assert.EqualValues(t, expectedSub, dbJiraSub)
	}

	t.Run("MakeJiraNotificationSucceedsWithDefaultIssueType", func(t *testing.T) {
		n0, err := makeJiraNotification(ctx, &evgSettings, &t1, jiraTicketOptions{project: "EVG"})
		assert.NoError(t, err)
		require.NotNil(t, n0)
		checkNotificationCreated(t, n0, event.JIRAIssueSubscriber{
			Project:   "EVG",
			IssueType: defaultJiraIssueType,
		})

		// test that creating another ticket creates another notification
		n1, err := makeJiraNotification(ctx, &evgSettings, &t1, jiraTicketOptions{project: "EVG"})
		assert.NoError(t, err)
		require.NotNil(t, n1)
		assert.NotEqual(t, n1.ID, n0.ID)
		checkNotificationCreated(t, n1, event.JIRAIssueSubscriber{
			Project:   "EVG",
			IssueType: defaultJiraIssueType,
		})
	})
	t.Run("MakeJiraNotificationSucceedsWithExplicitIssueType", func(t *testing.T) {
		n, err := makeJiraNotification(ctx, &evgSettings, &t1, jiraTicketOptions{
			project:   "EVG",
			issueType: "Bug",
		})
		assert.NoError(t, err)
		require.NotZero(t, n)
		checkNotificationCreated(t, n, event.JIRAIssueSubscriber{
			Project:   "EVG",
			IssueType: "Bug",
		})
	})
	t.Run("MakeJiraNotificationFailsWithoutProject", func(t *testing.T) {
		n, err := makeJiraNotification(ctx, &evgSettings, &t1, jiraTicketOptions{})
		assert.Error(t, err)
		assert.Zero(t, n)
	})

}
