package buildbaron

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/smartystreets/goconvey/convey/reporting"
)

func init() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	reporting.QuietMode()
}

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

func TestTaskToJQL(t *testing.T) {
	Convey("Given a task with with two failed tests and one successful test, "+
		"the jql should contain only the failed test names", t, func() {
		task1 := task.Task{}
		task1.LocalTestResults = []task.TestResult{
			{Status: "fail", TestFile: "foo.js"},
			{Status: "success", TestFile: "bar.js"},
			{Status: "fail", TestFile: "baz.js"},
		}
		task1.DisplayName = "foobar"
		jQL1 := taskToJQL(&task1, []string{"PRJ"})
		referenceJQL1 := fmt.Sprintf(JQLBFQuery, "PRJ", "text~\"foo.js\" or text~\"baz.js\"")
		So(jQL1, ShouldEqual, referenceJQL1)
	})

	Convey("Given a task with with oo failed tests, "+
		"the jql should contain only the failed task name", t, func() {
		task2 := task.Task{}
		task2.LocalTestResults = []task.TestResult{}
		task2.DisplayName = "foobar"
		jQL2 := taskToJQL(&task2, []string{"PRJ"})
		referenceJQL2 := fmt.Sprintf(JQLBFQuery, "PRJ", "text~\"foobar\"")
		So(jQL2, ShouldEqual, referenceJQL2)
	})
}

func TestBuildBaronPluginConfigure(t *testing.T) {
	assert := assert.New(t)

	bbPlugin := BuildBaronPlugin{nil, thirdparty.JiraHandler{}}
	assert.Nil(bbPlugin.Configure(map[string]interface{}{
		"Host":     "host",
		"Username": "user",
		"Password": "pass",
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketCreateProject:  "BFG",
				TicketSearchProjects: []string{"BF", "BFG"},
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{nil, thirdparty.JiraHandler{}}
	assert.Nil(bbPlugin.Configure(map[string]interface{}{
		"Host":     "host",
		"Username": "user",
		"Password": "pass",
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj1": evergreen.BuildBaronProject{
				TicketCreateProject:  "BFG",
				TicketSearchProjects: []string{"BF", "BFG"},
			},
			"proj2": evergreen.BuildBaronProject{
				TicketCreateProject:  "BFG",
				TicketSearchProjects: []string{"BF", "BFG"},
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{nil, thirdparty.JiraHandler{}}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Username": "user",
		"Password": "pass",
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketCreateProject:  "BFG",
				TicketSearchProjects: []string{"BF", "BFG"},
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{nil, thirdparty.JiraHandler{}}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Host":     "host",
		"Password": "pass",
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketCreateProject:  "BFG",
				TicketSearchProjects: []string{"BF", "BFG"},
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{nil, thirdparty.JiraHandler{}}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Host":     "host",
		"Username": "user",
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketCreateProject:  "BFG",
				TicketSearchProjects: []string{"BF", "BFG"},
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{nil, thirdparty.JiraHandler{}}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Host":     "host",
		"Username": "user",
		"Password": "pass",
		"Projects": map[string]evergreen.BuildBaronProject{},
	}))

	bbPlugin = BuildBaronPlugin{nil, thirdparty.JiraHandler{}}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Host":     "host",
		"Username": "user",
		"Password": "pass",
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketCreateProject: "BFG",
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{nil, thirdparty.JiraHandler{}}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Host":     "host",
		"Username": "user",
		"Password": "pass",
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketSearchProjects: []string{"BF", "BFG"},
			},
		},
	}))
}

func TestBuildBaronPluginConfigureAltEndpoint(t *testing.T) {
	assert := assert.New(t)

	bbPlugin := BuildBaronPlugin{nil, thirdparty.JiraHandler{}}
	assert.Nil(bbPlugin.Configure(map[string]interface{}{
		"Host":     "host",
		"Username": "user",
		"Password": "pass",
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketCreateProject:            "BFG",
				TicketSearchProjects:           []string{"BF", "BFG"},
				AlternativeEndpointURL:         "https://evergreen.mongodb.com",
				AlternativeEndpointUsername:    "user",
				AlternativeEndpointPassword:    "pass",
				AlternativeEndpointTimeoutSecs: 10,
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{nil, thirdparty.JiraHandler{}}
	assert.Nil(bbPlugin.Configure(map[string]interface{}{
		"Host":     "host",
		"Username": "user",
		"Password": "pass",
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketCreateProject:            "BFG",
				TicketSearchProjects:           []string{"BF", "BFG"},
				AlternativeEndpointURL:         "https://evergreen.mongodb.com",
				AlternativeEndpointTimeoutSecs: 10,
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{nil, thirdparty.JiraHandler{}}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Host":     "host",
		"Username": "user",
		"Password": "pass",
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketCreateProject:         "BFG",
				TicketSearchProjects:        []string{"BF", "BFG"},
				AlternativeEndpointUsername: "user",
				AlternativeEndpointPassword: "pass",
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{nil, thirdparty.JiraHandler{}}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Host":     "host",
		"Username": "user",
		"Password": "pass",
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketCreateProject:            "BFG",
				TicketSearchProjects:           []string{"BF", "BFG"},
				AlternativeEndpointTimeoutSecs: 10,
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{nil, thirdparty.JiraHandler{}}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Host":     "host",
		"Username": "user",
		"Password": "pass",
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketCreateProject:            "BFG",
				TicketSearchProjects:           []string{"BF", "BFG"},
				AlternativeEndpointURL:         "://evergreen.mongodb.com",
				AlternativeEndpointTimeoutSecs: 10,
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{nil, thirdparty.JiraHandler{}}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Host":     "host",
		"Username": "user",
		"Password": "pass",
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketCreateProject:            "BFG",
				TicketSearchProjects:           []string{"BF", "BFG"},
				AlternativeEndpointURL:         "https://evergreen.mongodb.com",
				AlternativeEndpointPassword:    "pass",
				AlternativeEndpointTimeoutSecs: 10,
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{nil, thirdparty.JiraHandler{}}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Host":     "host",
		"Username": "user",
		"Password": "pass",
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketCreateProject:            "BFG",
				TicketSearchProjects:           []string{"BF", "BFG"},
				AlternativeEndpointURL:         "https://evergreen.mongodb.com",
				AlternativeEndpointTimeoutSecs: 0,
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{nil, thirdparty.JiraHandler{}}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Host":     "host",
		"Username": "user",
		"Password": "pass",
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketCreateProject:            "BFG",
				TicketSearchProjects:           []string{"BF", "BFG"},
				AlternativeEndpointURL:         "https://evergreen.mongodb.com",
				AlternativeEndpointTimeoutSecs: -1,
			},
		},
	}))
}

func TestAltEndpointProcessResponse(t *testing.T) {
	assert := assert.New(t)

	altEndpoint := altEndpointSuggest{evergreen.BuildBaronProject{}}
	tickets, err := altEndpoint.parseResponse(ioutil.NopCloser(bytes.NewBufferString(`{
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
		}`)))

	assert.Nil(err)
	assert.Equal(tickets, []thirdparty.JiraTicket{ticket1, ticket2, ticket3},
		"expected JIRA tickets for all suggestions to be returned")

	altEndpoint = altEndpointSuggest{evergreen.BuildBaronProject{}}
	tickets, err = altEndpoint.parseResponse(ioutil.NopCloser(bytes.NewBufferString(`{
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
		}`)))

	assert.Nil(err)
	assert.Equal(tickets, []thirdparty.JiraTicket{ticket1, ticket3, ticket2},
		"expected JIRA tickets for all tests to be returned")

	altEndpoint = altEndpointSuggest{evergreen.BuildBaronProject{}}
	tickets, err = altEndpoint.parseResponse(ioutil.NopCloser(bytes.NewBufferString(`{
		"task_id": "my_task",
		"execution": 0,
		"status": "ok",
		"suggestions": []
	}`)))

	assert.EqualError(err, "no suggestions found",
		"expected an error to be return if no suggestions were made")
	assert.Nil(tickets)

	altEndpoint = altEndpointSuggest{evergreen.BuildBaronProject{}}
	tickets, err = altEndpoint.parseResponse(ioutil.NopCloser(bytes.NewBufferString(`{
			"task_id": "my_task",
			"execution": 0,
			"status": "scheduled"
		}`)))

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

	tickets, err := raceSuggesters(fallback, altEndpoint, &task.Task{})
	assert.Nil(err)
	assert.Equal(tickets, []thirdparty.JiraTicket{ticket1},
		"expected fallback result to be returned")

	fallback = &mockSuggest{[]thirdparty.JiraTicket{ticket1}, nil}
	altEndpoint = &mockSuggest{[]thirdparty.JiraTicket{ticket2, ticket3}, nil}

	tickets, err = raceSuggesters(fallback, altEndpoint, &task.Task{})
	assert.Nil(err)
	assert.Equal(tickets, []thirdparty.JiraTicket{ticket2, ticket3},
		"expected alternative endpoint result to be returned")

	fallback = &mockSuggest{nil, errors.New("Error from fallback")}
	altEndpoint = &mockSuggest{nil, errors.New("Error from alternative endpoint")}

	tickets, err = raceSuggesters(fallback, altEndpoint, &task.Task{})
	assert.EqualError(err, "Error from fallback",
		"expected error from fallback to be returned since both failed")
	assert.Nil(tickets)
}
