package data

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
)

func TestMakeTicket(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, db.ClearCollections(task.Collection, model.VersionCollection, build.Collection, model.ProjectRefCollection))
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

	n, err := makeNotification(ctx, &evgSettings, "MCI", &t1)
	assert.NoError(t, err)
	assert.NotNil(t, n)
	assert.EqualValues(t, event.JIRAIssueSubscriber{
		Project:   "MCI",
		IssueType: jiraIssueType,
	}, n.Subscriber.Target)
	// test that creating another ticket creates another notification
	n, err = makeNotification(ctx, &evgSettings, "MCI", &t1)
	assert.NoError(t, err)
	assert.NotNil(t, n)
}
