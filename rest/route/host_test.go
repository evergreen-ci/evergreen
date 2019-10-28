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
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/jasper/options"
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
	s.EqualError(err, "Argument read error: error attempting to unmarshal into *map[string]route.hostStatus: unexpected end of JSON input")
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
		"host4": hostStatus{Status: "terminated"},
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
	s.Equal(model.ToAPIString("host1"), h.Id)
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
	s.Equal(model.ToAPIString("host2"), h.Id)

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
	s.Equal(apiHost.Id, model.ToAPIString(host.Id))
	s.Equal(apiHost.HostURL, model.ToAPIString(host.Host))
	s.Equal(apiHost.Provisioned, host.Provisioned)
	s.Equal(apiHost.StartedBy, model.ToAPIString(host.StartedBy))
	s.Equal(apiHost.Type, model.ToAPIString(host.InstanceType))
	s.Equal(apiHost.User, model.ToAPIString(host.User))
	s.Equal(apiHost.Status, model.ToAPIString(host.Status))
	s.Equal(apiHost.UserHost, host.UserHost)
	s.Equal(apiHost.Distro.Id, model.ToAPIString(host.Distro.Id))
	s.Equal(apiHost.Distro.Provider, model.ToAPIString(host.Distro.Provider))
	s.Equal(apiHost.Distro.ImageId, model.ToAPIString(""))
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
	invalidPasswords := []model.APIString{
		model.ToAPIString(""),
		model.ToAPIString("weak"),
		model.ToAPIString("stilltooweak1"),
		model.ToAPIString("火a111")}
	for _, password := range invalidPasswords {
		mod := model.APISpawnHostModify{
			HostID: model.ToAPIString("host1"),
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
	invalidExpirations := []model.APIString{
		model.ToAPIString("not a number"),
		model.ToAPIString("0"),
		model.ToAPIString("9223372036854775807"),
		model.ToAPIString(""),
	}
	for _, extendBy := range invalidExpirations {
		mod := model.APISpawnHostModify{
			HostID:   model.ToAPIString("host1"),
			RDPPwd:   model.ToAPIString(""),
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
	r, err = http.NewRequest("POST", fmt.Sprintf("https://example.com/hosts/%s", model.FromAPIString(mod.HostID)), bytes.NewReader(data))
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
					ExpirationTime: time.Now().Add(time.Hour),
					Distro:         windowsDistro,
				},
				{
					Id:             "host2",
					StartedBy:      "user0",
					Host:           "host2",
					Status:         evergreen.HostRunning,
					ExpirationTime: time.Now().Add(time.Hour),
					Distro:         windowsDistro,
				},
				{
					Id:             "host3",
					StartedBy:      "user0",
					Host:           "host3",
					Status:         evergreen.HostUninitialized,
					ExpirationTime: time.Now().Add(time.Hour),
					Distro:         windowsDistro,
				},
				{
					Id:             "host4",
					StartedBy:      "user0",
					Host:           "host4",
					Status:         evergreen.HostRunning,
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
					Id:     "root",
					APIKey: "root-key",
				},
			},
		},
	}
	connector.SetSuperUsers([]string{"root"})
	return connector
}
