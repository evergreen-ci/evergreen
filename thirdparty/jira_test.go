package thirdparty

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
)

type mockHttp struct {
	res *http.Response
	err error
}

func (mock *mockHttp) RoundTrip(_ *http.Request) (*http.Response, error) {
	return mock.res, mock.err
}

func TestJiraNetworkFail(t *testing.T) {
	Convey("With a JIRA rest interface with broken network", t, func() {
		stub := &http.Client{Transport: &mockHttp{res: nil, err: errors.New("Generic network error")}}

		jira := JiraHandler{client: stub}

		Convey("fetching tickets should return a non-nil err", func() {
			ticket, err := jira.GetJIRATicket("BF-1")
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
			Body:       ioutil.NopCloser(&bytes.Buffer{}),
		}
		stub := &http.Client{Transport: &mockHttp{res: resp, err: nil}}

		jira := JiraHandler{client: stub}

		Convey("fetching tickets should return 401 unauth error", func() {
			ticket, err := jira.GetJIRATicket("BF-1")
			So(ticket, ShouldBeNil)
			So(err.Error(), ShouldEqual, "HTTP request returned unexpected status `401 Unauthorized`")
		})
	})
}

func TestJiraIntegration(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig, "TestJiraIntegration")
	Convey("With a JIRA rest interface that makes a valid request", t, func() {
		jira := NewJiraHandler(*testConfig.Jira.Export())

		Convey("the request for a ticket should return a valid ticket response", func() {
			ticket, err := jira.GetJIRATicket("BF-1")
			So(err, ShouldBeNil)
			So(ticket.Key, ShouldEqual, "BF-1")
			So(ticket.Fields.Project.Name, ShouldEqual, "Build Failures")
		})
	})
}
