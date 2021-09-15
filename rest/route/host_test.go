package route

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	serviceutil "github.com/evergreen-ci/evergreen/service/testutil"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

////////////////////////////////////////////////////////////////////////

type HostsChangeStatusesSuite struct {
	route *hostsChangeStatusesHandler
	sc    *data.MockConnector

	suite.Suite
}

func TestHostsChangeStatusesSuite(t *testing.T) {
	suite.Run(t, new(HostsChangeStatusesSuite))
}

func (s *HostsChangeStatusesSuite) SetupTest() {
	s.sc = getMockHostsConnector()
	s.route = makeChangeHostsStatuses(s.sc).(*hostsChangeStatusesHandler)
}

func (s *HostsChangeStatusesSuite) TestParseValidStatus() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["root"])

	json := []byte(`{"host1": {"status": "quarantined"}, "host2": {"status": "decommissioned"}, "host4": {"status": "terminated"}}`)
	req, _ := http.NewRequest("PATCH", "http://example.com/api/rest/v2/hosts", bytes.NewBuffer(json))
	err := s.route.Parse(ctx, req)
	s.NoError(err)
	s.Equal("quarantined", s.route.HostToStatus["host1"].Status)
	s.Equal("decommissioned", s.route.HostToStatus["host2"].Status)
	s.Equal("terminated", s.route.HostToStatus["host4"].Status)
}

func (s *HostsChangeStatusesSuite) TestParseInValidStatus() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["root"])

	json := []byte(`{"host1": {"status": "This is an invalid state"}, "host2": {"status": "decommissioned"}, "host4": {"status": "terminated"}}`)
	req, _ := http.NewRequest("PATCH", "http://example.com/api/rest/v2/hosts", bytes.NewBuffer(json))
	err := s.route.Parse(ctx, req)

	s.Error(err)
	s.EqualError(err, fmt.Sprintf("Invalid host status '%s' for host '%s'", s.route.HostToStatus["host1"].Status, "host1"))
}

func (s *HostsChangeStatusesSuite) TestParseMissingPayload() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["root"])

	json := []byte(``)
	req, _ := http.NewRequest("PATCH", "http://example.com/api/rest/v2/hosts/host1", bytes.NewBuffer(json))
	err := s.route.Parse(ctx, req)
	s.Error(err)
	s.EqualError(err, "Argument read error: unexpected end of JSON input")
}

func (s *HostsChangeStatusesSuite) TestRunHostsValidStatusesChange() {
	h := s.route.Factory().(*hostsChangeStatusesHandler)
	h.HostToStatus = map[string]hostStatus{
		"host2": hostStatus{Status: "decommissioned"},
		"host4": hostStatus{Status: "terminated"},
	}

	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user0"])
	res := h.Run(ctx)
	s.Equal(http.StatusOK, res.Status())
}

func (s *HostsChangeStatusesSuite) TestRunHostNotStartedByUser() {
	h := s.route.Factory().(*hostsChangeStatusesHandler)
	h.HostToStatus = map[string]hostStatus{
		"host3": hostStatus{Status: "decommissioned"},
		"host4": hostStatus{Status: "terminated"},
	}

	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user1"])
	res := h.Run(ctx)
	s.Equal(http.StatusUnauthorized, res.Status())
}

func (s *HostsChangeStatusesSuite) TestRunSuperUserSetStatusAnyHost() {
	h := s.route.Factory().(*hostsChangeStatusesHandler)
	h.HostToStatus = map[string]hostStatus{
		"host3": hostStatus{Status: "decommissioned"},
		"host2": hostStatus{Status: "terminated"},
	}

	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["root"])
	res := h.Run(ctx)
	s.Equal(http.StatusOK, res.Status())
}

func (s *HostsChangeStatusesSuite) TestRunTerminatedOnTerminatedHost() {
	h := s.route.Factory().(*hostsChangeStatusesHandler)
	h.HostToStatus = map[string]hostStatus{
		"host1": hostStatus{Status: "terminated"},
	}

	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user0"])
	res := h.Run(ctx)
	s.Equal(http.StatusBadRequest, res.Status())
}

func (s *HostsChangeStatusesSuite) TestRunHostRunningOnTerminatedHost() {
	h := s.route.Factory().(*hostsChangeStatusesHandler)
	h.HostToStatus = map[string]hostStatus{
		"host1": hostStatus{Status: "running"},
	}

	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user0"])
	res := h.Run(ctx)
	s.Equal(http.StatusBadRequest, res.Status())
}

func (s *HostsChangeStatusesSuite) TestRunHostQuarantinedOnTerminatedHost() {
	h := s.route.Factory().(*hostsChangeStatusesHandler)
	h.HostToStatus = map[string]hostStatus{
		"host1": hostStatus{Status: "quarantined"},
	}

	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user0"])
	res := h.Run(ctx)
	s.Equal(http.StatusBadRequest, res.Status())
}

func (s *HostsChangeStatusesSuite) TestRunHostDecommissionedOnTerminatedHost() {
	h := s.route.Factory().(*hostsChangeStatusesHandler)
	h.HostToStatus = map[string]hostStatus{
		"host1": hostStatus{Status: "decommissioned"},
	}

	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user0"])
	res := h.Run(ctx)
	s.Equal(http.StatusBadRequest, res.Status())
}

func (s *HostsChangeStatusesSuite) TestRunWithInvalidHost() {
	h := s.route.Factory().(*hostsChangeStatusesHandler)
	h.HostToStatus = map[string]hostStatus{
		"invalid": hostStatus{Status: "decommissioned"},
	}

	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user0"])
	res := h.Run(ctx)
	s.Equal(http.StatusNotFound, res.Status())
}

////////////////////////////////////////////////////////////////////////

type HostModifySuite struct {
	route *hostModifyHandler
	sc    *data.MockConnector

	suite.Suite
}

func TestHostModifySuite(t *testing.T) {
	suite.Run(t, new(HostModifySuite))
}

func (s *HostModifySuite) SetupTest() {
	s.sc = getMockHostsConnector()
	s.route = makeHostModifyRouteManager(s.sc, evergreen.GetEnvironment()).(*hostModifyHandler)
}

func (s *HostModifySuite) TestRunHostNotFound() {
	h := s.route.Factory().(*hostModifyHandler)
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user0"])

	h.hostID = "host-invalid"
	h.options = &host.HostModifyOptions{}
	res := h.Run(ctx)
	s.Equal(http.StatusNotFound, res.Status())
}

func (s *HostModifySuite) TestRunHostNotStartedByUser() {
	h := s.route.Factory().(*hostModifyHandler)
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user1"])

	h.hostID = "host1"
	h.options = &host.HostModifyOptions{}
	res := h.Run(ctx)
	s.Equal(http.StatusUnauthorized, res.Status())
}

func (s *HostModifySuite) TestRunValidHost() {
	h := s.route.Factory().(*hostModifyHandler)
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user0"])

	h.hostID = "host2"
	h.options = &host.HostModifyOptions{}
	res := h.Run(ctx)
	s.Equal(http.StatusOK, res.Status())
}

func (s *HostModifySuite) TestParse() {
	h := s.route.Factory().(*hostModifyHandler)
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user0"])

	// empty
	r, err := http.NewRequest("PATCH", "/hosts/my-host", bytes.NewReader(nil))
	s.NoError(err)
	r = gimlet.SetURLVars(r, map[string]string{"host_id": "my-host"})
	s.Error(h.Parse(ctx, r))

	// not empty
	options := host.HostModifyOptions{
		AddInstanceTags: []host.Tag{
			{
				Key:   "banana",
				Value: "yellow",
			},
		},
	}
	jsonBody, err := json.Marshal(options)
	s.NoError(err)
	buffer := bytes.NewBuffer(jsonBody)

	r, err = http.NewRequest("PATCH", "/hosts/my-host", buffer)
	s.NoError(err)
	r = gimlet.SetURLVars(r, map[string]string{"host_id": "my-host"})

	s.NoError(h.Parse(ctx, r))

	s.Require().Len(h.options.AddInstanceTags, 1)
	s.Equal("banana", h.options.AddInstanceTags[0].Key)
	s.Equal("yellow", h.options.AddInstanceTags[0].Value)
}

////////////////////////////////////////////////////////////////////////

type HostSuite struct {
	route *hostIDGetHandler
	sc    *data.MockConnector

	suite.Suite
}

func TestHostSuite(t *testing.T) {
	suite.Run(t, new(HostSuite))
}

func (s *HostSuite) SetupSuite() {
	s.sc = getMockHostsConnector()
}

func (s *HostSuite) SetupTest() {
	s.route = &hostIDGetHandler{
		sc: s.sc,
	}
}

func (s *HostSuite) TestFindByIdFirst() {
	s.route.hostID = "host1"
	res := s.route.Run(context.TODO())
	s.NotNil(res)
	s.Equal(http.StatusOK, res.Status())

	h, ok := res.Data().(*model.APIHost)
	s.True(ok)
	s.NotNil(res)

	s.True(ok)
	s.Equal(utility.ToStringPtr("host1"), h.Id)
}

func (s *HostSuite) TestFindByIdLast() {
	s.route.hostID = "host2"
	res := s.route.Run(context.TODO())
	s.NotNil(res)
	s.Equal(http.StatusOK, res.Status())

	h, ok := res.Data().(*model.APIHost)
	s.True(ok)
	s.NotNil(res)

	s.True(ok)
	s.Equal(utility.ToStringPtr("host2"), h.Id)

}

func (s *HostSuite) TestFindByIdFail() {
	s.route.hostID = "host5"
	res := s.route.Run(context.TODO())
	s.NotNil(res)
	s.Equal(http.StatusNotFound, res.Status(), "%+v", res)

}

func (s *HostSuite) TestBuildFromServiceHost() {
	host := s.sc.MockHostConnector.CachedHosts[0]
	apiHost := model.APIHost{}
	s.NoError(apiHost.BuildFromService(host))
	s.Equal(apiHost.Id, utility.ToStringPtr(host.Id))
	s.Equal(apiHost.HostURL, utility.ToStringPtr(host.Host))
	s.Equal(apiHost.Provisioned, host.Provisioned)
	s.Equal(apiHost.StartedBy, utility.ToStringPtr(host.StartedBy))
	s.Equal(apiHost.Provider, utility.ToStringPtr(host.Provider))
	s.Equal(apiHost.User, utility.ToStringPtr(host.User))
	s.Equal(apiHost.Status, utility.ToStringPtr(host.Status))
	s.Equal(apiHost.UserHost, host.UserHost)
	s.Equal(apiHost.Distro.Id, utility.ToStringPtr(host.Distro.Id))
	s.Equal(apiHost.Distro.Provider, utility.ToStringPtr(host.Distro.Provider))
	s.Equal(apiHost.Distro.ImageId, utility.ToStringPtr(""))
}

////////////////////////////////////////////////////////////////////////

type hostTerminateHostHandlerSuite struct {
	rm *hostTerminateHandler
	sc *data.MockConnector

	suite.Suite
}

func TestTerminateHostHandler(t *testing.T) {
	s := &hostTerminateHostHandlerSuite{}
	suite.Run(t, s)
}

func (s *hostTerminateHostHandlerSuite) SetupTest() {
	s.sc = getMockHostsConnector()
	s.rm = makeTerminateHostRoute(s.sc).(*hostTerminateHandler)
}

func (s *hostTerminateHostHandlerSuite) TestExecuteWithNoUserPanics() {
	s.PanicsWithValue("no user attached to request", func() {
		_ = s.rm.Run(context.TODO())

	})
}

func (s *hostTerminateHostHandlerSuite) TestExecuteWithInvalidHost() {
	s.rm.hostID = "host-that-doesn't-exist"

	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user0"])
	resp := s.rm.Run(ctx)
	s.NotEqual(http.StatusOK, resp.Status())
}

func (s *hostTerminateHostHandlerSuite) TestExecuteWithTerminatedHost() {
	h := s.rm.Factory().(*hostTerminateHandler)
	h.hostID = "host1"

	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user0"])

	resp := h.Run(ctx)
	s.Equal(http.StatusBadRequest, resp.Status())

	apiErr := resp.Data().(gimlet.ErrorResponse)
	s.Equal(http.StatusBadRequest, apiErr.StatusCode)
	s.Equal(evergreen.HostTerminated, s.sc.CachedHosts[0].Status)
}

func (s *hostTerminateHostHandlerSuite) TestExecuteWithUninitializedHost() {
	h := s.rm.Factory().(*hostTerminateHandler)
	h.hostID = "host3"

	s.Equal(evergreen.HostUninitialized, s.sc.CachedHosts[2].Status)
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user0"])

	resp := h.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())
	s.Equal(evergreen.HostTerminated, s.sc.CachedHosts[2].Status)
}

func (s *hostTerminateHostHandlerSuite) TestExecuteWithRunningHost() {
	h := s.rm.Factory().(*hostTerminateHandler)
	h.hostID = "host2"

	s.Equal(evergreen.HostRunning, s.sc.CachedHosts[1].Status)
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user0"])

	resp := h.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())
	s.Equal(evergreen.HostRunning, s.sc.CachedHosts[1].Status)
}

func (s *hostTerminateHostHandlerSuite) TestSuperUserCanTerminateAnyHost() {
	h := s.rm.Factory().(*hostTerminateHandler)
	h.hostID = "host3"

	s.Equal(evergreen.HostRunning, s.sc.CachedHosts[1].Status)
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["root"])

	resp := h.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())
	s.Equal(evergreen.HostTerminated, s.sc.CachedHosts[2].Status)
}

func (s *hostTerminateHostHandlerSuite) TestRegularUserCannotTerminateAnyHost() {
	h := s.rm.Factory().(*hostTerminateHandler)
	h.hostID = "host2"

	s.Equal(evergreen.HostRunning, s.sc.CachedHosts[1].Status)
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user1"])

	resp := h.Run(ctx)
	s.NotEqual(http.StatusOK, resp.Status())
	s.Equal(http.StatusUnauthorized, resp.Status())
	s.Equal(evergreen.HostRunning, s.sc.CachedHosts[1].Status)
}

////////////////////////////////////////////////////////////////////////

type hostChangeRDPPasswordHandlerSuite struct {
	rm         gimlet.RouteHandler
	sc         *data.MockConnector
	env        evergreen.Environment
	sshKeyName string
	suite.Suite
}

func TestHostChangeRDPPasswordHandler(t *testing.T) {
	s := &hostChangeRDPPasswordHandlerSuite{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.env = testutil.NewEnvironment(ctx, t)

	s.env.Settings().Keys["ssh_key_name"] = "ssh_key"

	suite.Run(t, s)
}

func (s *hostChangeRDPPasswordHandlerSuite) SetupTest() {
	s.sc = getMockHostsConnector()
	s.rm = makeHostChangePassword(s.sc, s.env)
}

func (s *hostChangeRDPPasswordHandlerSuite) TestExecuteWithNoUserPanics() {
	s.PanicsWithValue("no user attached to request", func() {
		_ = s.rm.Run(context.TODO())
	})
}

func (s *hostChangeRDPPasswordHandlerSuite) TestExecute() {
	h := s.rm.Factory().(*hostChangeRDPPasswordHandler)
	h.hostID = "host2"
	h.rdpPassword = "Hunter2!"

	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user0"])

	resp := h.Run(ctx)
	s.Error(resp.Data().(gimlet.ErrorResponse))
	procs, err := s.env.JasperManager().List(ctx, options.All)
	s.NoError(err)
	s.NotEmpty(procs)
}

func (s *hostChangeRDPPasswordHandlerSuite) TestExecuteWithUninitializedHostFails() {
	h := s.rm.Factory().(*hostChangeRDPPasswordHandler)
	h.hostID = "host3"
	h.rdpPassword = "Hunter2!"

	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user0"])

	resp := h.Run(ctx)
	s.NotEqual(http.StatusOK, resp.Status())
}

func (s *hostChangeRDPPasswordHandlerSuite) TestExecuteWithInvalidHost() {
	h := s.rm.Factory().(*hostChangeRDPPasswordHandler)
	h.hostID = "host-that-doesn't-exist"

	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user0"])

	resp := h.Run(ctx)
	s.NotEqual(http.StatusOK, resp.Status())
}

func (s *hostChangeRDPPasswordHandlerSuite) TestParseAndValidateRejectsInvalidPasswords() {
	invalidPasswords := []*string{
		utility.ToStringPtr(""),
		utility.ToStringPtr("weak"),
		utility.ToStringPtr("stilltooweak1"),
		utility.ToStringPtr("火a111")}
	for _, password := range invalidPasswords {
		mod := model.APISpawnHostModify{
			HostID: utility.ToStringPtr("host1"),
			RDPPwd: password,
		}
		err := s.tryParseAndValidate(mod)

		s.Error(err)
		s.IsType(gimlet.ErrorResponse{}, err)
		apiErr := err.(gimlet.ErrorResponse)
		s.Equal(http.StatusBadRequest, apiErr.StatusCode)
	}
}

func (s *hostChangeRDPPasswordHandlerSuite) TestSuperUserCanChangeAnyHost() {
	h := s.rm.Factory().(*hostChangeRDPPasswordHandler)
	h.hostID = "host2"
	h.rdpPassword = "Hunter2!"

	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["root"])

	resp := h.Run(ctx)
	s.NotEqual(http.StatusOK, resp.Status())
	s.Error(resp.Data().(gimlet.ErrorResponse))
	procs, err := s.env.JasperManager().List(ctx, options.All)
	s.NoError(err)
	s.NotEmpty(procs)
}
func (s *hostChangeRDPPasswordHandlerSuite) TestRegularUserCannotChangeAnyHost() {
	h := s.rm.Factory().(*hostChangeRDPPasswordHandler)
	h.hostID = "host2"
	h.rdpPassword = "Hunter2!"

	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user1"])

	resp := h.Run(ctx)
	s.NotEqual(http.StatusOK, resp.Status())
}

func (s *hostChangeRDPPasswordHandlerSuite) tryParseAndValidate(mod model.APISpawnHostModify) error {
	r, err := makeMockHostRequest(mod)
	s.NoError(err)

	h := s.rm.Factory().(*hostChangeRDPPasswordHandler)

	return h.Parse(context.TODO(), r)
}

////////////////////////////////////////////////////////////////////////

type hostExtendExpirationHandlerSuite struct {
	rm gimlet.RouteHandler
	sc *data.MockConnector
	suite.Suite
}

func TestHostExtendExpirationHandler(t *testing.T) {
	s := &hostExtendExpirationHandlerSuite{}
	suite.Run(t, s)
}

func (s *hostExtendExpirationHandlerSuite) SetupTest() {
	s.sc = getMockHostsConnector()
	s.rm = makeExtendHostExpiration(s.sc)
}

func (s *hostExtendExpirationHandlerSuite) TestHostExtendExpirationWithNoUserPanics() {
	s.PanicsWithValue("no user attached to request", func() {
		_ = s.rm.Run(context.TODO())
	})
}

func (s *hostExtendExpirationHandlerSuite) TestExecuteWithInvalidHost() {
	h := s.rm.Factory().(*hostExtendExpirationHandler)
	h.hostID = "host-that-doesn't-exist"

	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user0"])
	resp := h.Run(ctx)
	s.NotEqual(http.StatusOK, resp.Status())
}

func (s *hostExtendExpirationHandlerSuite) TestParseAndValidateRejectsInvalidExpirations() {
	invalidExpirations := []*string{
		utility.ToStringPtr("not a number"),
		utility.ToStringPtr("0"),
		utility.ToStringPtr("9223372036854775807"),
		utility.ToStringPtr(""),
	}
	for _, extendBy := range invalidExpirations {
		mod := model.APISpawnHostModify{
			HostID:   utility.ToStringPtr("host1"),
			RDPPwd:   utility.ToStringPtr(""),
			AddHours: extendBy,
		}

		err := s.tryParseAndValidate(mod)
		s.Error(err)
		s.IsType(gimlet.ErrorResponse{}, err)
		apiErr := err.(gimlet.ErrorResponse)
		s.Equal(http.StatusBadRequest, apiErr.StatusCode)
	}
}

func (s *hostExtendExpirationHandlerSuite) TestExecuteWithLargeExpirationFails() {
	h := s.rm.Factory().(*hostExtendExpirationHandler)
	h.hostID = "host2"
	h.addHours = 9001 * time.Hour

	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user0"])
	resp := h.Run(ctx)
	s.NotEqual(http.StatusOK, resp.Status())
	apiErr := resp.Data().(gimlet.ErrorResponse)
	s.Equal(http.StatusBadRequest, apiErr.StatusCode)
}

func (s *hostExtendExpirationHandlerSuite) TestExecute() {
	expectedTime := s.sc.CachedHosts[1].ExpirationTime.Add(8 * time.Hour)

	h := s.rm.Factory().(*hostExtendExpirationHandler)
	h.hostID = "host2"
	h.addHours = 8 * time.Hour

	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user0"])
	resp := h.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())
	s.Equal(expectedTime, s.sc.CachedHosts[1].ExpirationTime)
}

func (s *hostExtendExpirationHandlerSuite) TestExecuteWithTerminatedHostFails() {
	expectedTime := s.sc.CachedHosts[0].ExpirationTime

	h := s.rm.Factory().(*hostExtendExpirationHandler)
	h.hostID = "host1"
	h.addHours = 8 * time.Hour

	ctx := gimlet.AttachUser(context.Background(), s.sc.MockUserConnector.CachedUsers["user0"])
	resp := h.Run(ctx)
	s.Equal(http.StatusBadRequest, resp.Status())
	s.Equal(expectedTime, s.sc.CachedHosts[0].ExpirationTime)
}

func (s *hostExtendExpirationHandlerSuite) TestSuperUserCanExtendAnyHost() {
	expectedTime := s.sc.CachedHosts[1].ExpirationTime.Add(8 * time.Hour)

	h := s.rm.Factory().(*hostExtendExpirationHandler)
	h.hostID = "host2"
	h.addHours = 8 * time.Hour

	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["root"])

	resp := h.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())
	s.Equal(expectedTime, s.sc.CachedHosts[1].ExpirationTime)
}

func (s *hostExtendExpirationHandlerSuite) TestRegularUserCannotExtendOtherUsersHosts() {
	expectedTime := s.sc.CachedHosts[1].ExpirationTime

	h := s.rm.Factory().(*hostExtendExpirationHandler)
	h.hostID = "host2"
	h.addHours = 8 * time.Hour

	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user1"])

	resp := h.Run(ctx)
	s.NotEqual(http.StatusOK, resp.Status())
	s.Equal(expectedTime, s.sc.CachedHosts[1].ExpirationTime)
}

func (s *hostExtendExpirationHandlerSuite) tryParseAndValidate(mod model.APISpawnHostModify) error {
	r, err := makeMockHostRequest(mod)
	s.NoError(err)

	h := s.rm.Factory().(*hostExtendExpirationHandler)

	return h.Parse(context.TODO(), r)
}

func makeMockHostRequest(mod model.APISpawnHostModify) (*http.Request, error) {
	data, err := json.Marshal(mod)
	if err != nil {
		return nil, err
	}

	var r *http.Request
	r, err = http.NewRequest("POST", fmt.Sprintf("https://example.com/hosts/%s", utility.FromStringPtr(mod.HostID)), bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	return r, nil
}

func getMockHostsConnector() *data.MockConnector {
	windowsDistro := distro.Distro{
		Id:       "windows",
		Arch:     "windows_amd64",
		Provider: evergreen.ProviderNameMock,
		SSHKey:   "ssh_key_name",
	}
	connector := &data.MockConnector{
		MockHostConnector: data.MockHostConnector{
			CachedHosts: []host.Host{
				{
					Id:             "host1",
					StartedBy:      "user0",
					Host:           "host1",
					Status:         evergreen.HostTerminated,
					CreationTime:   time.Now(),
					ExpirationTime: time.Now().Add(time.Hour),
					Distro:         windowsDistro,
				},
				{
					Id:             "host2",
					StartedBy:      "user0",
					Host:           "host2",
					Status:         evergreen.HostRunning,
					CreationTime:   time.Now(),
					ExpirationTime: time.Now().Add(time.Hour),
					Distro:         windowsDistro,
				},
				{
					Id:             "host3",
					StartedBy:      "user0",
					Host:           "host3",
					Status:         evergreen.HostUninitialized,
					CreationTime:   time.Now(),
					ExpirationTime: time.Now().Add(time.Hour),
					Distro:         windowsDistro,
				},
				{
					Id:             "host4",
					StartedBy:      "user0",
					Host:           "host4",
					Status:         evergreen.HostRunning,
					CreationTime:   time.Now(),
					ExpirationTime: time.Now().Add(time.Hour),
					Distro: distro.Distro{
						Id:       "linux",
						Arch:     "linux_amd64",
						Provider: evergreen.ProviderNameMock,
					},
				},
			},
		},
		MockUserConnector: data.MockUserConnector{
			CachedUsers: map[string]*user.DBUser{
				"user0": {
					Id:     "user0",
					APIKey: "user0-key",
				},
				"user1": {
					Id:     "user1",
					APIKey: "user1-key",
				},
				"root": {
					Id:          "root",
					APIKey:      "root-key",
					SystemRoles: []string{"root"},
				},
			},
		},
	}
	grip.Error(db.ClearCollections(evergreen.ScopeCollection, evergreen.RoleCollection))
	cmd := map[string]string{
		"create": evergreen.ScopeCollection,
	}
	grip.Error(evergreen.GetEnvironment().DB().RunCommand(nil, cmd).Err())
	rm := evergreen.GetEnvironment().RoleManager()
	grip.Error(rm.AddScope(gimlet.Scope{
		ID:        "root",
		Resources: []string{windowsDistro.Id},
		Type:      evergreen.DistroResourceType,
	}))
	grip.Error(rm.UpdateRole(gimlet.Role{
		ID:    "root",
		Scope: "root",
		Permissions: gimlet.Permissions{
			evergreen.PermissionHosts: evergreen.HostsEdit.Value,
		},
	}))
	return connector
}

func TestClearHostsHandler(t *testing.T) {
	assert.NoError(t, db.ClearCollections(host.Collection, host.VolumesCollection, user.Collection))
	h0 := host.Host{
		Id:           "h0",
		StartedBy:    "user0",
		UserHost:     true,
		Status:       evergreen.HostTerminated,
		CreationTime: time.Now(),
		Distro:       distro.Distro{Id: "ubuntu-1604", Provider: evergreen.ProviderNameMock},
	}
	h1 := host.Host{
		Id:           "h1",
		StartedBy:    "user0",
		UserHost:     true,
		Status:       evergreen.HostRunning,
		CreationTime: time.Now(),
		NoExpiration: true,
		Distro:       distro.Distro{Id: "ubuntu-1604", Provider: evergreen.ProviderNameMock},
	}
	h2 := host.Host{
		Id:           "h2",
		StartedBy:    "user0",
		UserHost:     true,
		Status:       evergreen.HostRunning,
		CreationTime: time.Now(),
		Distro:       distro.Distro{Id: "ubuntu-1604", Provider: evergreen.ProviderNameMock},
	}
	h3 := host.Host{
		Id:           "h3",
		StartedBy:    "user0",
		UserHost:     true,
		Status:       evergreen.HostTerminated,
		CreationTime: time.Now(),
		Distro:       distro.Distro{Id: "ubuntu-1804", Provider: evergreen.ProviderNameMock},
	}
	v1 := host.Volume{
		ID:           "v1",
		CreatedBy:    "user0",
		NoExpiration: true,
	}
	assert.NoError(t, h0.Insert())
	assert.NoError(t, h1.Insert())
	assert.NoError(t, h2.Insert())
	assert.NoError(t, h3.Insert())
	assert.NoError(t, v1.Insert())

	handler := offboardUserHandler{
		sc: &data.DBConnector{},
	}
	json := []byte(`{"email": "user0@mongodb.com"}`)
	ctx := gimlet.AttachUser(context.Background(), &user.DBUser{Id: "root"})
	req, _ := http.NewRequest("PATCH", "http://example.com/api/rest/v2/users/offboard_user?dry_run=true", bytes.NewBuffer(json))
	assert.Error(t, handler.Parse(ctx, req)) // user not inserted

	u := user.DBUser{Id: "user0"}
	assert.NoError(t, u.Insert())
	req, _ = http.NewRequest("PATCH", "http://example.com/api/rest/v2/users/offboard_user?dry_run=true", bytes.NewBuffer(json))
	assert.NoError(t, handler.Parse(ctx, req))
	assert.Equal(t, "user0", handler.user)
	assert.True(t, handler.dryRun)

	resp := handler.Run(ctx)
	require.Equal(t, http.StatusOK, resp.Status())
	res, ok := resp.Data().(model.APIOffboardUserResults)
	assert.True(t, ok)
	require.Len(t, res.TerminatedHosts, 1)
	assert.Equal(t, "h1", res.TerminatedHosts[0])
	require.Len(t, res.TerminatedVolumes, 1)
	assert.Equal(t, "v1", res.TerminatedVolumes[0])
	hostFromDB, err := host.FindOneByIdOrTag(h1.Id)
	assert.NoError(t, err)
	assert.NotNil(t, hostFromDB)
	assert.True(t, hostFromDB.NoExpiration)
}

func TestRemoveAdminHandler(t *testing.T) {
	assert.NoError(t, db.ClearCollections(host.Collection, host.VolumesCollection, dbModel.ProjectRefCollection))
	projectRef0 := &dbModel.ProjectRef{
		Owner:     "mongodb",
		Repo:      "test_repo0",
		Branch:    "main",
		Enabled:   utility.TruePtr(),
		BatchTime: 10,
		Id:        "test0",
		Admins:    []string{"user1", "user0"},
	}
	projectRef1 := &dbModel.ProjectRef{
		Owner:     "mongodb",
		Repo:      "test_repo1",
		Branch:    "main",
		Enabled:   utility.TruePtr(),
		BatchTime: 10,
		Id:        "test1",
		Admins:    []string{"user1", "user2"},
	}

	assert.NoError(t, projectRef0.Insert())
	assert.NoError(t, projectRef1.Insert())

	offboardedUser := "user0"
	env := evergreen.GetEnvironment()
	userManager := env.UserManager()

	handler := offboardUserHandler{
		sc:     &data.DBConnector{},
		dryRun: true,
		env:    env,
		user:   offboardedUser,
	}
	resp := handler.Run(gimlet.AttachUser(context.Background(), &user.DBUser{Id: "root"}))
	require.Equal(t, http.StatusOK, resp.Status())
	assert.Contains(t, projectRef0.Admins, offboardedUser)

	handler.dryRun = false
	handler.env.SetUserManager(serviceutil.MockUserManager{})
	resp = handler.Run(gimlet.AttachUser(context.Background(), &user.DBUser{Id: "root"}))
	require.Equal(t, http.StatusOK, resp.Status())
	env.SetUserManager(userManager)

	projectRefs, err := dbModel.FindAllMergedProjectRefs()
	assert.NoError(t, err)
	require.Len(t, projectRefs, 2)
	for _, projRef := range projectRefs {
		assert.NotContains(t, projRef.Admins, offboardedUser)
	}
}

func TestHostFilterGetHandler(t *testing.T) {
	connector := &data.MockConnector{
		MockHostConnector: data.MockHostConnector{
			CachedHosts: []host.Host{
				{
					Id:           "h0",
					StartedBy:    "user0",
					Status:       evergreen.HostTerminated,
					CreationTime: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
					Distro:       distro.Distro{Id: "ubuntu-1604", Provider: evergreen.ProviderNameMock},
				},
				{
					Id:           "h1",
					StartedBy:    "user0",
					Status:       evergreen.HostRunning,
					CreationTime: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
					Distro:       distro.Distro{Id: "ubuntu-1604", Provider: evergreen.ProviderNameMock},
				},
				{
					Id:           "h2",
					StartedBy:    "user1",
					Status:       evergreen.HostRunning,
					CreationTime: time.Date(2009, time.December, 10, 23, 0, 0, 0, time.UTC),
					Distro:       distro.Distro{Id: "ubuntu-1804", Provider: evergreen.ProviderNameMock},
				},
			},
		},
	}

	handler := hostFilterGetHandler{
		sc:     connector,
		params: model.APIHostParams{Status: evergreen.HostTerminated},
	}
	resp := handler.Run(gimlet.AttachUser(context.Background(), &user.DBUser{Id: "user0"}))
	assert.Equal(t, http.StatusOK, resp.Status())
	hosts := resp.Data().([]interface{})
	require.Len(t, hosts, 1)
	assert.Equal(t, "h0", utility.FromStringPtr(hosts[0].(*model.APIHost).Id))

	handler = hostFilterGetHandler{
		sc:     connector,
		params: model.APIHostParams{Distro: "ubuntu-1604"},
	}
	resp = handler.Run(gimlet.AttachUser(context.Background(), &user.DBUser{Id: "user0"}))
	assert.Equal(t, http.StatusOK, resp.Status())
	hosts = resp.Data().([]interface{})
	require.Len(t, hosts, 1)
	assert.Equal(t, "h1", utility.FromStringPtr(hosts[0].(*model.APIHost).Id))

	handler = hostFilterGetHandler{
		sc:     connector,
		params: model.APIHostParams{CreatedAfter: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)},
	}
	resp = handler.Run(gimlet.AttachUser(context.Background(), &user.DBUser{Id: "user1"}))
	assert.Equal(t, http.StatusOK, resp.Status())
	hosts = resp.Data().([]interface{})
	require.Len(t, hosts, 1)
	assert.Equal(t, "h2", utility.FromStringPtr(hosts[0].(*model.APIHost).Id))
}

func TestDisableHostHandler(t *testing.T) {
	hostID := "h1"
	connector := data.MockConnector{
		MockHostConnector: data.MockHostConnector{
			CachedHosts: []host.Host{
				{
					Id:     hostID,
					Status: evergreen.HostRunning,
				},
			},
		},
	}
	dh := disableHost{
		sc:     &connector,
		hostID: hostID,
	}

	responder := dh.Run(context.Background())
	assert.Equal(t, http.StatusOK, responder.Status())
	assert.Equal(t, evergreen.HostDecommissioned, connector.CachedHosts[0].Status)
}

func TestHostProvisioningOptionsGetHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, db.Clear(host.Collection))
	defer func() {
		assert.NoError(t, db.Clear(host.Collection))
	}()

	h := host.Host{
		Id: "host_id",
		Distro: distro.Distro{
			Arch: evergreen.ArchLinuxAmd64,
		},
	}
	require.NoError(t, h.Insert())

	t.Run("SucceedsWithValidHostID", func(t *testing.T) {
		rh := hostProvisioningOptionsGetHandler{
			sc:     &data.DBConnector{},
			hostID: h.Id,
		}
		resp := rh.Run(ctx)
		require.Equal(t, http.StatusOK, resp.Status())
		opts, ok := resp.Data().(model.APIHostProvisioningOptions)
		require.True(t, ok)
		assert.NotEmpty(t, opts.Content)
	})
	t.Run("FailsWithoutHostID", func(t *testing.T) {
		rh := hostProvisioningOptionsGetHandler{
			sc: &data.DBConnector{},
		}
		resp := rh.Run(ctx)
		assert.NotEqual(t, http.StatusOK, resp.Status())
	})
	t.Run("FailsWithInvalidHostID", func(t *testing.T) {
		rh := hostProvisioningOptionsGetHandler{
			sc:     &data.DBConnector{},
			hostID: "foo",
		}
		resp := rh.Run(ctx)
		assert.NotEqual(t, http.StatusOK, resp.Status())
	})
}
