package thirdparty

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	jiraclient "github.com/andygrunwald/go-jira"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
	"github.com/trivago/tgo/tcontainer"
)

type mockHttp struct {
	res *http.Response
	err error
}

func (mock *mockHttp) RoundTrip(_ *http.Request) (*http.Response, error) {
	return mock.res, mock.err
}

func testJiraHandler(t *testing.T, httpClient *http.Client) JiraHandler {
	t.Helper()
	jc, err := jiraclient.NewClient(httpClient, "http://example.com/")
	if err != nil {
		t.Fatal(err)
	}
	return JiraHandler{client: jc}
}

func TestJiraNetworkFail(t *testing.T) {
	Convey("With a JIRA rest interface with broken network", t, func() {
		stub := &http.Client{Transport: &mockHttp{res: nil, err: errors.New("Generic network error")}}

		jira := testJiraHandler(t, stub)

		Convey("fetching tickets should return a non-nil err", func() {
			ticket, err := jira.GetIssue(context.Background(), "BF-1")
			So(ticket, ShouldBeNil)
			So(strings.Contains(err.Error(), "Generic network error"), ShouldBeTrue)
		})
	})
}

func TestJiraUnauthorized(t *testing.T) {
	Convey("With a JIRA rest interface that makes an unauthorized response", t, func() {
		resp := &http.Response{
			StatusCode: http.StatusUnauthorized,
			Status:     "401 Unauthorized",
			Body:       io.NopCloser(&bytes.Buffer{}),
		}
		stub := &http.Client{Transport: &mockHttp{res: resp, err: nil}}

		jira := testJiraHandler(t, stub)

		Convey("fetching tickets should return an unauthorized error", func() {
			ticket, err := jira.GetIssue(context.Background(), "BF-1")
			So(ticket, ShouldBeNil)
			So(err, ShouldNotBeNil)
			So(strings.Contains(err.Error(), "401"), ShouldBeTrue)
		})
	})
}

// TestJiraIssueToJiraTicket_JSONRoundTripCustomFields checks that go-jira's IssueFields.MarshalJSON
// (Unknowns merged into JSON) round-trips into our JiraTicket types.
func TestJiraIssueToJiraTicket_JSONRoundTripCustomFields(t *testing.T) {
	issue := &jiraclient.Issue{
		Key: "X-1",
		Fields: &jiraclient.IssueFields{
			Summary: "s",
			Unknowns: tcontainer.MarshalMap{
				"customfield_12950": []string{"task-a", "task-b"},
			},
		},
	}
	ticket, err := jiraIssueToJiraTicket(issue)
	require.NoError(t, err)
	require.NotNil(t, ticket)
	require.Equal(t, "X-1", ticket.Key)
	require.Len(t, ticket.Fields.FailingTasks, 2)
	require.Equal(t, []string{"task-a", "task-b"}, ticket.Fields.FailingTasks)

	data, err := json.Marshal(issue)
	require.NoError(t, err)
	require.Contains(t, string(data), "customfield_12950")
}

func TestJiraIntegration(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig)
	Convey("With a JIRA rest interface that makes a valid request", t, func() {
		jira, err := NewJiraHandler(*testConfig.Jira.Export())
		So(err, ShouldBeNil)

		Convey("the request for a ticket should return a valid ticket response", func() {
			ticket, err := jira.GetIssue(t.Context(), "BF-1")
			So(err, ShouldBeNil)
			So(ticket.Key, ShouldEqual, "BF-1")

			failingTasks := []string{"auth_audit", "causally_consistent_hedged_reads_jscore_passthrough", "concurrency_simultaneous_replication_wiredtiger_cursor_sweeps", "embedded_sdk_run_tests", "multi_stmt_txn_jscore_passthrough_with_migration", "parallel", "unittests"}
			So(ticket.Fields.FailingTasks, ShouldHaveLength, len(failingTasks))
			for _, task := range failingTasks {
				So(ticket.Fields.FailingTasks, ShouldContain, task)
			}
		})
	})
}
