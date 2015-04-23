package thirdparty

import (
	"10gen.com/mci/util"
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"net/http"
	"testing"
)

type JiraTestSuite struct{}

type stubHttpGet struct {
	res *http.Response
	err error
}

func (self stubHttpGet) doGet(url string, username string, password string) (*http.Response, error) {
	return self.res, self.err
}

func (self *JiraTestSuite) TestNetworkFail(t *testing.T) {
	Convey("With a JIRA rest interface with broken network", t, func() {
		stub := stubHttpGet{nil, fmt.Errorf("Generic network error")}

		jira := JiraHandler{stub, testConfig.Jira.Host, testConfig.Jira.Username, testConfig.Jira.Password}

		Convey("fetching tickets should return a non-nil err", func() {
			ticket, err := jira.GetJIRATicket("BF-1")
			So(ticket, ShouldBeNil)
			So(err.Error(), ShouldEqual, "Generic network error")
		})
	})
}

func (self *JiraTestSuite) TestUnauthorized(t *testing.T) {
	Convey("With a JIRA rest interface that makes an unauthorized response", t, func() {
		stub := stubHttpGet{&http.Response{}, nil}

		stub.res.StatusCode = 401
		stub.res.Status = "401 Unauthorized"

		jira := JiraHandler{stub, testConfig.Jira.Host, testConfig.Jira.Username, testConfig.Jira.Password}

		Convey("fetching tickets should return 401 unauth error", func() {
			ticket, err := jira.GetJIRATicket("BF-1")
			So(ticket, ShouldBeNil)
			So(err.Error(), ShouldEqual, "HTTP request returned unexpected status `401 Unauthorized`")
		})
	})
}

func (self *JiraTestSuite) TestIntegration(t *testing.T) {
	Convey("With a JIRA rest interface that makes a valid request", t, func() {
		jira := JiraHandler{liveHttpGet{}, testConfig.Jira.Host, testConfig.Jira.Username, testConfig.Jira.Password}

		Convey("the request for a ticket should return a valid ticket response", func() {
			ticket, err := jira.GetJIRATicket("BF-1")
			util.HandleTestingErr(err, t, "Failed to get ticket from JIRA")
			So(ticket.Key, ShouldEqual, "BF-1")
			So(ticket.Fields.Project.Name, ShouldEqual, "Build Failures")
		})
	})
}
