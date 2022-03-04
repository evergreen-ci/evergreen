package data

import (
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
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection, model.VersionCollection, build.Collection, model.ProjectRefCollection))
	t1 := task.Task{
		Id:      "t1",
		Version: "v",
		Project: "proj",
		BuildId: "b",
	}
	assert.NoError(t1.Insert())
	v := model.Version{
		Id:       "v",
		Revision: "1234567890",
	}
	assert.NoError(v.Insert())
	b := build.Build{
		Id: "b",
	}
	assert.NoError(b.Insert())
	p := model.ProjectRef{
		Identifier: "proj",
	}
	assert.NoError(p.Insert())

	evgSettings := evergreen.Settings{
		Ui: evergreen.UIConfig{
			Url: "www.example.com",
		},
	}

	n, err := makeNotification(&evgSettings, "MCI", &t1)
	assert.NoError(err)
	assert.NotNil(n)
	assert.EqualValues(event.JIRAIssueSubscriber{
		Project:   "MCI",
		IssueType: jiraIssueType,
	}, n.Subscriber.Target)
	// test that creating another ticket creates another notification
	n, err = makeNotification(&evgSettings, "MCI", &t1)
	assert.NoError(err)
	assert.NotNil(n)
}
