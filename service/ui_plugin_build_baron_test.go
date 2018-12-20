package service

import (
	"bytes"
	"context"
	"io/ioutil"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

var (
	ticket1 = thirdparty.JiraTicket{
		Key: "BF-1",
		Fields: &thirdparty.TicketFields{
			Summary: "ticket #1",
			Created: "2018-04-16T01:01:01",
			Updated: "2018-04-17T01:01:01",
			Status:  &thirdparty.JiraStatus{Name: "Open"},
		},
	}

	ticket2 = thirdparty.JiraTicket{
		Key: "BF-2",
		Fields: &thirdparty.TicketFields{
			Summary: "ticket #2",
			Created: "2018-04-16T02:02:02",
			Updated: "2018-04-17T02:02:02",
			Status:  &thirdparty.JiraStatus{Name: "Closed"},
		},
	}

	ticket3 = thirdparty.JiraTicket{
		Key: "BF-3",
		Fields: &thirdparty.TicketFields{
			Summary:    "ticket #3",
			Resolution: &thirdparty.JiraResolution{Name: "Fixed"},
			Created:    "2018-04-16T03:03:03",
			Updated:    "2018-04-17T03:03:03",
			Status:     &thirdparty.JiraStatus{Name: "Resolved"},
		},
	}
)

func TestAltEndpointParseResponseData(t *testing.T) {
	assert := assert.New(t)

	altEndpoint := altEndpointSuggest{nil, 0}
	data := bfSuggestionResponse{}
	rawJSON := `{
		"task_id": "my_task",
		"execution": 0,
		"status": "ok",
		"suggestions": [
		{
			"test_name": "all.js",
			"issues": [
			{
				"key": "BF-1",
				"summary": "ticket #1",
				"status": "Open",
				"created_date": "2018-04-16T01:01:01",
				"updated_date": "2018-04-17T01:01:01"
			},
			{
				"key": "BF-2",
				"summary": "ticket #2",
				"status": "Closed",
				"created_date": "2018-04-16T02:02:02",
				"updated_date": "2018-04-17T02:02:02"
			},
			{
				"key": "BF-3",
				"summary": "ticket #3",
				"status": "Resolved",
				"resolution": "Fixed",
				"created_date": "2018-04-16T03:03:03",
				"updated_date": "2018-04-17T03:03:03"
			}
			]
		}
		]
	}`
	err := util.ReadJSONInto(ioutil.NopCloser(bytes.NewBufferString(rawJSON)), &data)
	assert.Nil(err)

	tickets, err := altEndpoint.parseResponseData(data)

	assert.Nil(err)
	assert.Equal([]thirdparty.JiraTicket{ticket1, ticket2, ticket3}, tickets,
		"expected JIRA tickets for all suggestions to be returned")

	altEndpoint = altEndpointSuggest{nil, 0}
	data = bfSuggestionResponse{}
	rawJSON = `{
		"task_id": "my_task",
		"execution": 0,
		"status": "ok",
		"suggestions": [
		{
			"test_name": "all.js",
			"issues": [
			{
				"key": "BF-1",
				"summary": "ticket #1",
				"status": "Open",
				"created_date": "2018-04-16T01:01:01",
				"updated_date": "2018-04-17T01:01:01"
			},
			{
				"key": "BF-3",
				"summary": "ticket #3",
				"status": "Resolved",
				"resolution": "Fixed",
				"created_date": "2018-04-16T03:03:03",
				"updated_date": "2018-04-17T03:03:03"
			}
			]
		},
		{
			"test_name": "all2.js",
			"issues": [
			{
				"key": "BF-2",
				"summary": "ticket #2",
				"status": "Closed",
				"created_date": "2018-04-16T02:02:02",
				"updated_date": "2018-04-17T02:02:02"
			}
			]
		}
		]
	}`
	err = util.ReadJSONInto(ioutil.NopCloser(bytes.NewBufferString(rawJSON)), &data)
	assert.Nil(err)

	tickets, err = altEndpoint.parseResponseData(data)

	assert.Nil(err)
	assert.Equal([]thirdparty.JiraTicket{ticket1, ticket3, ticket2}, tickets,
		"expected JIRA tickets for all tests to be returned")

	altEndpoint = altEndpointSuggest{nil, 0}
	data = bfSuggestionResponse{}
	rawJSON = `{
		"task_id": "my_task",
		"execution": 0,
		"status": "ok",
		"suggestions": []
	}`
	err = util.ReadJSONInto(ioutil.NopCloser(bytes.NewBufferString(rawJSON)), &data)
	assert.Nil(err)

	tickets, err = altEndpoint.parseResponseData(data)

	assert.EqualError(err, "no suggestions found",
		"expected an error to be return if no suggestions were made")
	assert.Nil(tickets)

	altEndpoint = altEndpointSuggest{nil, 0}
	data = bfSuggestionResponse{}
	rawJSON = `{
		"task_id": "my_task",
		"execution": 0,
		"status": "scheduled"
	}`
	err = util.ReadJSONInto(ioutil.NopCloser(bytes.NewBufferString(rawJSON)), &data)
	assert.Nil(err)

	tickets, err = altEndpoint.parseResponseData(data)

	assert.EqualError(err, "Build Baron suggestions weren't ready: status=scheduled",
		"expected an error to be returned if suggestions weren't ready yet")
	assert.Nil(tickets)
}

type mockSuggest struct {
	Tickets []thirdparty.JiraTicket
	Error   error
}

func (ms *mockSuggest) Suggest(ctx context.Context, t *task.Task) ([]thirdparty.JiraTicket, error) {
	return ms.Tickets, ms.Error
}

func (ms *mockSuggest) GetTimeout() time.Duration {
	return time.Duration(0)
}

func TestRaceSuggesters(t *testing.T) {
	assert := assert.New(t)

	fallback := &mockSuggest{[]thirdparty.JiraTicket{ticket1}, nil}
	altEndpoint := &mockSuggest{nil, errors.New("Build Baron suggestions returned an error")}
	multiSource := multiSourceSuggest{fallback, altEndpoint}

	tickets, source, err := multiSource.Suggest(&task.Task{})
	assert.Nil(err)
	assert.Equal(jiraSource, source)
	assert.Equal([]thirdparty.JiraTicket{ticket1}, tickets,
		"expected fallback result to be returned")

	fallback = &mockSuggest{[]thirdparty.JiraTicket{ticket1}, nil}
	altEndpoint = &mockSuggest{[]thirdparty.JiraTicket{ticket2, ticket3}, nil}
	multiSource1 := multiSourceSuggest{fallback, altEndpoint}

	tickets, source, err = multiSource1.Suggest(&task.Task{})
	assert.Nil(err)
	assert.Equal(bfSuggestionSource, source)
	assert.Equal([]thirdparty.JiraTicket{ticket2, ticket3}, tickets,
		"expected alternative endpoint result to be returned")

	fallback = &mockSuggest{nil, errors.New("Error from fallback")}
	altEndpoint = &mockSuggest{nil, errors.New("Error from alternative endpoint")}
	multiSource2 := multiSourceSuggest{fallback, altEndpoint}

	tickets, source, err = multiSource2.Suggest(&task.Task{})
	assert.EqualError(err, "Error from fallback",
		"expected error from fallback to be returned since both failed")
	assert.Nil(tickets)
	assert.Equal(jiraSource, source)
}

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
	uis := UIServer{
		Settings: evergreen.Settings{
			Ui: evergreen.UIConfig{
				Url: "www.example.com",
			},
		},
	}

	n, err := uis.makeNotification("MCI", &t1)
	assert.NoError(err)
	assert.NotNil(n)
	assert.EqualValues(event.JIRAIssueSubscriber{
		Project:   "MCI",
		IssueType: jiraIssueType,
	}, n.Subscriber.Target)
	// test that creating another ticket creates another notification
	n, err = uis.makeNotification("MCI", &t1)
	assert.NoError(err)
	assert.NotNil(n)
}
