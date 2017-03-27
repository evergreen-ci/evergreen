package thirdparty

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
)

type stubHttp struct {
	res *http.Response
	err error
}

func (self stubHttp) doGet(url string, username string, password string) (*http.Response, error) {
	return self.res, self.err
}

func (self stubHttp) doPost(url string, username string, password string, content interface{}) (*http.Response, error) {
	return self.res, self.err
}

func (self stubHttp) doPut(url string, username string, password string, content interface{}) (*http.Response, error) {
	return self.res, self.err
}

func TestJiraNetworkFail(t *testing.T) {
	Convey("With a JIRA rest interface with broken network", t, func() {
		stub := stubHttp{nil, errors.New("Generic network error")}

		jira := JiraHandler{stub, testConfig.Jira.Host, testConfig.Jira.Username, testConfig.Jira.Password}

		Convey("fetching tickets should return a non-nil err", func() {
			ticket, err := jira.GetJIRATicket("BF-1")
			So(ticket, ShouldBeNil)
			So(err.Error(), ShouldEqual, "Generic network error")
		})
	})
}

func TestJiraUnauthorized(t *testing.T) {
	Convey("With a JIRA rest interface that makes an unauthorized response", t, func() {
		stub := stubHttp{&http.Response{}, nil}

		stub.res.StatusCode = 401
		stub.res.Status = "401 Unauthorized"
		stub.res.Body = ioutil.NopCloser(&bytes.Buffer{})

		jira := JiraHandler{stub, testConfig.Jira.Host, testConfig.Jira.Username, testConfig.Jira.Password}

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
		jira := JiraHandler{liveHttp{}, testConfig.Jira.Host, testConfig.Jira.Username, testConfig.Jira.Password}

		Convey("the request for a ticket should return a valid ticket response", func() {
			ticket, err := jira.GetJIRATicket("BF-1")
			So(err, ShouldBeNil)
			So(ticket.Key, ShouldEqual, "BF-1")
			So(ticket.Fields.Project.Name, ShouldEqual, "Build Failures")
		})
	})
}
