package route

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/model"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
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
	env   evergreen.Environment
	suite.Suite
}

func TestHostsChangeStatusesSuite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))
	s := &HostsChangeStatusesSuite{
		env: env,
	}
	suite.Run(t, s)
}

func (s *HostsChangeStatusesSuite) SetupTest() {
	setupMockHostsConnector(s.T(), s.env)
	s.route = makeChangeHostsStatuses().(*hostsChangeStatusesHandler)
}

func (s *HostsChangeStatusesSuite) TestParseValidStatus() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "root"})

	json := []byte(`{"host1": {"status": "quarantined"}, "host2": {"status": "decommissioned"}, "host4": {"status": "terminated"}}`)
	req, _ := http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/hosts", bytes.NewBuffer(json))
	err := s.route.Parse(ctx, req)
	s.NoError(err)
	s.Equal("quarantined", s.route.HostToStatus["host1"].Status)
	s.Equal("decommissioned", s.route.HostToStatus["host2"].Status)
	s.Equal("terminated", s.route.HostToStatus["host4"].Status)
}

func (s *HostsChangeStatusesSuite) TestParseInValidStatus() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "root"})

	json := []byte(`{"host1": {"status": "This is an invalid state"}, "host2": {"status": "decommissioned"}, "host4": {"status": "terminated"}}`)
	req, _ := http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/hosts", bytes.NewBuffer(json))
	err := s.route.Parse(ctx, req)

	s.Error(err)
	s.EqualError(err, fmt.Sprintf("invalid host status '%s' for host '%s'", s.route.HostToStatus["host1"].Status, "host1"))
}

func (s *HostsChangeStatusesSuite) TestParseMissingPayload() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "root"})

	json := []byte(``)
	req, _ := http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/hosts/host1", bytes.NewBuffer(json))
	err := s.route.Parse(ctx, req)
	s.Error(err)
	s.EqualError(err, "reading host-status mapping from JSON request body: unexpected end of JSON input")
}

func (s *HostsChangeStatusesSuite) TestRunHostsValidStatusesChange() {
	h := s.route.Factory().(*hostsChangeStatusesHandler)
	h.HostToStatus = map[string]hostStatus{
		"host2": {Status: "decommissioned"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user0"})
	res := h.Run(ctx)
	s.Equal(http.StatusOK, res.Status())
}

func (s *HostsChangeStatusesSuite) TestRunHostNotStartedByUser() {
	h := s.route.Factory().(*hostsChangeStatusesHandler)
	h.HostToStatus = map[string]hostStatus{
		"host3": {Status: "decommissioned"},
		"host4": {Status: "terminated"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})
	res := h.Run(ctx)
	s.Equal(http.StatusUnauthorized, res.Status())
}

func (s *HostsChangeStatusesSuite) TestRunSuperUserSetStatusAnyHost() {
	h := s.route.Factory().(*hostsChangeStatusesHandler)
	h.HostToStatus = map[string]hostStatus{
		"host3": {Status: "decommissioned"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "root", SystemRoles: []string{"root"}})
	res := h.Run(ctx)
	s.Equal(http.StatusOK, res.Status())
}

func (s *HostsChangeStatusesSuite) TestRunTerminatedOnTerminatedHost() {
	h := s.route.Factory().(*hostsChangeStatusesHandler)
	h.HostToStatus = map[string]hostStatus{
		"host1": {Status: "terminated"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user0"})
	res := h.Run(ctx)
	s.Equal(http.StatusInternalServerError, res.Status())
}

func (s *HostsChangeStatusesSuite) TestRunHostRunningOnTerminatedHost() {
	h := s.route.Factory().(*hostsChangeStatusesHandler)
	h.HostToStatus = map[string]hostStatus{
		"host1": {Status: "running"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user0"})
	res := h.Run(ctx)
	s.Equal(http.StatusInternalServerError, res.Status())
}

func (s *HostsChangeStatusesSuite) TestRunHostQuarantinedOnTerminatedHost() {
	h := s.route.Factory().(*hostsChangeStatusesHandler)
	h.HostToStatus = map[string]hostStatus{
		"host1": {Status: "quarantined"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user0"})
	res := h.Run(ctx)
	s.Equal(http.StatusInternalServerError, res.Status())
}

func (s *HostsChangeStatusesSuite) TestRunHostDecommissionedOnTerminatedHost() {
	h := s.route.Factory().(*hostsChangeStatusesHandler)
	h.HostToStatus = map[string]hostStatus{
		"host1": {Status: "decommissioned"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user0"})
	res := h.Run(ctx)
	s.Equal(http.StatusInternalServerError, res.Status())
}

func (s *HostsChangeStatusesSuite) TestRunWithInvalidHost() {
	h := s.route.Factory().(*hostsChangeStatusesHandler)
	h.HostToStatus = map[string]hostStatus{
		"invalid": {Status: "decommissioned"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user0"})
	res := h.Run(ctx)
	s.Equal(http.StatusNotFound, res.Status())
}

////////////////////////////////////////////////////////////////////////

type HostModifySuite struct {
	route *hostModifyHandler
	env   evergreen.Environment
	suite.Suite
}

func TestHostModifySuite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))
	s := &HostModifySuite{
		env: env,
	}
	suite.Run(t, s)
}

func (s *HostModifySuite) SetupTest() {
	setupMockHostsConnector(s.T(), s.env)
	s.route = makeHostModifyRouteManager(s.env).(*hostModifyHandler)
}

func (s *HostModifySuite) TestRunHostNotFound() {
	h := s.route.Factory().(*hostModifyHandler)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user0"})

	h.hostID = "host-invalid"
	h.options = &host.HostModifyOptions{}
	res := h.Run(ctx)
	s.Equal(http.StatusNotFound, res.Status())
}

func (s *HostModifySuite) TestRunHostNotStartedByUser() {
	h := s.route.Factory().(*hostModifyHandler)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})

	h.hostID = "host1"
	h.options = &host.HostModifyOptions{}
	res := h.Run(ctx)
	s.Equal(http.StatusUnauthorized, res.Status())
}

func (s *HostModifySuite) TestRunValidHost() {
	h := s.route.Factory().(*hostModifyHandler)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user0"})

	h.hostID = "host2"
	h.options = &host.HostModifyOptions{}
	res := h.Run(ctx)
	s.Equal(http.StatusOK, res.Status())
}

func (s *HostModifySuite) TestParse() {
	h := s.route.Factory().(*hostModifyHandler)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user0"})

	// empty
	r, err := http.NewRequest(http.MethodPatch, "/hosts/my-host", bytes.NewReader(nil))
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

	r, err = http.NewRequest(http.MethodPatch, "/hosts/my-host", buffer)
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
	env   evergreen.Environment
	suite.Suite
}

func TestHostSuite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))
	s := &HostSuite{
		env: env,
	}
	suite.Run(t, s)
}

func (s *HostSuite) SetupTest() {
	setupMockHostsConnector(s.T(), s.env)
	s.route = &hostIDGetHandler{}
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host, err := host.FindOneId(ctx, "host1")
	s.NoError(err)
	apiHost := model.APIHost{}
	apiHost.BuildFromService(host, nil)
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
	rm  *hostTerminateHandler
	env evergreen.Environment
	suite.Suite
}

func TestTerminateHostHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))
	s := &hostTerminateHostHandlerSuite{
		env: env,
	}
	suite.Run(t, s)
}

func (s *hostTerminateHostHandlerSuite) SetupTest() {
	setupMockHostsConnector(s.T(), s.env)
	s.rm = makeTerminateHostRoute().(*hostTerminateHandler)
}

func (s *hostTerminateHostHandlerSuite) TestExecuteWithNoUserPanics() {
	s.PanicsWithValue("no user attached to request", func() {
		_ = s.rm.Run(context.TODO())

	})
}

func (s *hostTerminateHostHandlerSuite) TestExecuteWithInvalidHost() {
	s.rm.hostID = "host-that-doesn't-exist"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user0"})
	resp := s.rm.Run(ctx)
	s.NotEqual(http.StatusOK, resp.Status())
}

func (s *hostTerminateHostHandlerSuite) TestExecuteWithTerminatedHost() {
	h := s.rm.Factory().(*hostTerminateHandler)
	h.hostID = "host1"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user0"})

	resp := h.Run(ctx)
	s.Equal(http.StatusBadRequest, resp.Status())

	apiErr := resp.Data().(gimlet.ErrorResponse)
	s.Equal(http.StatusBadRequest, apiErr.StatusCode)
	foundHost, err := host.FindOneId(ctx, "host1")
	s.NoError(err)
	s.Equal(evergreen.HostTerminated, foundHost.Status)
}

func (s *hostTerminateHostHandlerSuite) TestExecuteWithUninitializedHost() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := s.rm.Factory().(*hostTerminateHandler)
	h.hostID = "host3"

	foundHost, err := host.FindOneId(ctx, "host3")
	s.NoError(err)
	s.Equal(evergreen.HostUninitialized, foundHost.Status)
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user0"})

	resp := h.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())
	foundHost, err = host.FindOneId(ctx, "host3")
	s.NoError(err)
	s.Equal(evergreen.HostTerminated, foundHost.Status)
}

func (s *hostTerminateHostHandlerSuite) TestExecuteWithRunningHost() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := s.rm.Factory().(*hostTerminateHandler)
	h.hostID = "host2"

	mock := cloud.GetMockProvider()
	mock.Set(h.hostID, cloud.MockInstance{
		Status: cloud.StatusRunning,
	})

	foundHost, err := host.FindOneId(ctx, "host2")
	s.NoError(err)
	s.Equal(evergreen.HostRunning, foundHost.Status)
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user0"})

	resp := h.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())
	foundHost, err = host.FindOneId(ctx, "host2")
	s.NoError(err)
	s.Equal(evergreen.HostTerminated, foundHost.Status)
}

func (s *hostTerminateHostHandlerSuite) TestSuperUserCanTerminateAnyHost() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := s.rm.Factory().(*hostTerminateHandler)
	h.hostID = "host3"

	foundHost, err := host.FindOneId(ctx, "host2")
	s.NoError(err)
	s.Equal(evergreen.HostRunning, foundHost.Status)
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "root", SystemRoles: []string{"root"}})

	resp := h.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())
	foundHost, err = host.FindOneId(ctx, "host3")
	s.NoError(err)
	s.Equal(evergreen.HostTerminated, foundHost.Status)
}

func (s *hostTerminateHostHandlerSuite) TestRegularUserCannotTerminateAnyHost() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := s.rm.Factory().(*hostTerminateHandler)
	h.hostID = "host2"

	foundHost, err := host.FindOneId(ctx, "host2")
	s.NoError(err)
	s.Equal(evergreen.HostRunning, foundHost.Status)
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})

	resp := h.Run(ctx)
	s.NotEqual(http.StatusOK, resp.Status())
	s.Equal(http.StatusUnauthorized, resp.Status())
	foundHost, err = host.FindOneId(ctx, "host2")
	s.NoError(err)
	s.Equal(evergreen.HostRunning, foundHost.Status)
}

////////////////////////////////////////////////////////////////////////

type hostChangeRDPPasswordHandlerSuite struct {
	rm  gimlet.RouteHandler
	env evergreen.Environment
	suite.Suite
}

func TestHostChangeRDPPasswordHandler(t *testing.T) {
	s := &hostChangeRDPPasswordHandlerSuite{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))
	s.env = env

	setupMockHostsConnector(t, s.env)

	suite.Run(t, s)
}

func (s *hostChangeRDPPasswordHandlerSuite) SetupTest() {
	s.rm = makeHostChangePassword(s.env)

	keyFile, err := os.CreateTemp(s.T().TempDir(), "")
	s.Require().NoError(err)
	s.env.(*mock.Environment).EvergreenSettings.KanopySSHKeyPath = keyFile.Name()
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user0"})

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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user0"})

	resp := h.Run(ctx)
	s.NotEqual(http.StatusOK, resp.Status())
}

func (s *hostChangeRDPPasswordHandlerSuite) TestExecuteWithInvalidHost() {
	h := s.rm.Factory().(*hostChangeRDPPasswordHandler)
	h.hostID = "host-that-doesn't-exist"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user0"})

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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "root", SystemRoles: []string{"root"}})

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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})

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
	suite.Suite
}

func TestHostExtendExpirationHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &hostExtendExpirationHandlerSuite{}
	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))
	setupMockHostsConnector(t, env)
	suite.Run(t, s)
}

func (s *hostExtendExpirationHandlerSuite) SetupTest() {
	s.rm = makeExtendHostExpiration()
}

func (s *hostExtendExpirationHandlerSuite) TestHostExtendExpirationWithNoUserPanics() {
	s.PanicsWithValue("no user attached to request", func() {
		_ = s.rm.Run(context.TODO())
	})
}

func (s *hostExtendExpirationHandlerSuite) TestExecuteWithInvalidHost() {
	h := s.rm.Factory().(*hostExtendExpirationHandler)
	h.hostID = "host-that-doesn't-exist"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user0"})
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user0"})
	resp := h.Run(ctx)
	s.NotEqual(http.StatusOK, resp.Status())
	apiErr := resp.Data().(gimlet.ErrorResponse)
	s.Equal(http.StatusBadRequest, apiErr.StatusCode)
}

func (s *hostExtendExpirationHandlerSuite) TestExecute() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	foundHost, err := host.FindOneId(ctx, "host2")
	s.NoError(err)
	expectedTime := foundHost.ExpirationTime.Add(1 * time.Hour)

	h := s.rm.Factory().(*hostExtendExpirationHandler)
	h.hostID = "host2"
	h.addHours = 1 * time.Hour

	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user0"})
	resp := h.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())
	foundHost, err = host.FindOneId(ctx, "host2")
	s.NoError(err)
	s.Equal(expectedTime, foundHost.ExpirationTime)
}

func (s *hostExtendExpirationHandlerSuite) TestExecuteWithTerminatedHostFails() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	foundHost, err := host.FindOneId(ctx, "host1")
	s.NoError(err)
	expectedTime := foundHost.ExpirationTime

	h := s.rm.Factory().(*hostExtendExpirationHandler)
	h.hostID = "host1"
	h.addHours = 1 * time.Hour

	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user0"})
	resp := h.Run(ctx)
	s.Equal(http.StatusBadRequest, resp.Status())
	foundHost, err = host.FindOneId(ctx, "host1")
	s.NoError(err)
	s.Equal(expectedTime, foundHost.ExpirationTime)
}

func (s *hostExtendExpirationHandlerSuite) TestSuperUserCanExtendAnyHost() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	foundHost, err := host.FindOneId(ctx, "host2")
	s.NoError(err)
	expectedTime := foundHost.ExpirationTime.Add(1 * time.Hour)

	h := s.rm.Factory().(*hostExtendExpirationHandler)
	h.hostID = "host2"
	h.addHours = 1 * time.Hour

	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "root", SystemRoles: []string{"root"}})

	resp := h.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())
	foundHost, err = host.FindOneId(ctx, "host2")
	s.NoError(err)
	s.Equal(expectedTime, foundHost.ExpirationTime)
}

func (s *hostExtendExpirationHandlerSuite) TestRegularUserCannotExtendOtherUsersHosts() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	foundHost, err := host.FindOneId(ctx, "host2")
	s.NoError(err)
	expectedTime := foundHost.ExpirationTime

	h := s.rm.Factory().(*hostExtendExpirationHandler)
	h.hostID = "host2"
	h.addHours = 1 * time.Hour

	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})

	resp := h.Run(ctx)
	s.NotEqual(http.StatusOK, resp.Status())
	foundHost, err = host.FindOneId(ctx, "host2")
	s.NoError(err)
	s.Equal(expectedTime, foundHost.ExpirationTime)
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
	r, err = http.NewRequest(http.MethodPost, fmt.Sprintf("https://example.com/hosts/%s", utility.FromStringPtr(mod.HostID)), bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	return r, nil
}

func setupMockHostsConnector(t *testing.T, env evergreen.Environment) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(evergreen.ScopeCollection, evergreen.RoleCollection, host.Collection, user.Collection))
	windowsDistro := distro.Distro{
		Id:       "windows",
		Arch:     "windows_amd64",
		Provider: evergreen.ProviderNameMock,
	}
	users := []user.DBUser{
		{
			Id:     "user0",
			APIKey: "user0-key",
		},
		{
			Id:     "user1",
			APIKey: "user1-key",
		},
		{
			Id:          "root",
			APIKey:      "root-key",
			SystemRoles: []string{"root"},
		},
	}
	hosts := []host.Host{
		{
			Id:             "host1",
			StartedBy:      "user0",
			Host:           "host1",
			Status:         evergreen.HostTerminated,
			CreationTime:   time.Date(2028, 7, 15, 0, 0, 0, 0, time.UTC),
			ExpirationTime: time.Date(2028, 7, 15, 0, 0, 0, 0, time.UTC).Add(time.Hour),
			Distro:         windowsDistro,
		},
		{
			Id:             "host2",
			StartedBy:      "user0",
			Host:           "host2",
			Status:         evergreen.HostRunning,
			CreationTime:   time.Date(2028, 7, 15, 0, 0, 0, 0, time.UTC),
			ExpirationTime: time.Date(2028, 7, 15, 0, 0, 0, 0, time.UTC).Add(time.Hour),
			Distro:         windowsDistro,
		},
		{
			Id:             "host3",
			StartedBy:      "user0",
			Host:           "host3",
			Status:         evergreen.HostUninitialized,
			CreationTime:   time.Date(2028, 7, 15, 0, 0, 0, 0, time.UTC),
			ExpirationTime: time.Date(2028, 7, 15, 0, 0, 0, 0, time.UTC).Add(time.Hour),
			Distro:         windowsDistro,
		},
		{
			Id:             "host4",
			StartedBy:      "user0",
			Host:           "host4",
			Status:         evergreen.HostRunning,
			CreationTime:   time.Date(2028, 7, 15, 0, 0, 0, 0, time.UTC),
			ExpirationTime: time.Date(2028, 7, 15, 0, 0, 0, 0, time.UTC).Add(time.Hour),
			Distro: distro.Distro{
				Id:       "linux",
				Arch:     "linux_amd64",
				Provider: evergreen.ProviderNameMock,
			},
		},
	}
	for _, hostToAdd := range hosts {
		grip.Error(hostToAdd.Insert(ctx))
	}
	for _, userToAdd := range users {
		grip.Error(userToAdd.Insert())
	}
	require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))
	rm := env.RoleManager()
	require.NoError(t, rm.AddScope(gimlet.Scope{
		ID:        "root",
		Resources: []string{windowsDistro.Id},
		Type:      evergreen.DistroResourceType,
	}))
	require.NoError(t, rm.UpdateRole(gimlet.Role{
		ID:    "root",
		Scope: "root",
		Permissions: gimlet.Permissions{
			evergreen.PermissionHosts:          evergreen.HostsEdit.Value,
			evergreen.PermissionDistroSettings: evergreen.DistroSettingsAdmin.Value,
		},
	}))
}

func TestHostFilterGetHandler(t *testing.T) {
	assert.NoError(t, db.ClearCollections(host.Collection))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	newHosts := []host.Host{
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
	}

	for _, hostToAdd := range newHosts {
		assert.NoError(t, hostToAdd.Insert(ctx))
	}
	handler := hostFilterGetHandler{
		params: model.APIHostParams{Status: evergreen.HostTerminated},
	}
	resp := handler.Run(gimlet.AttachUser(context.Background(), &user.DBUser{Id: "user0"}))
	assert.Equal(t, http.StatusOK, resp.Status())
	hosts := resp.Data().([]interface{})
	require.Len(t, hosts, 1)
	assert.Equal(t, "h0", utility.FromStringPtr(hosts[0].(*model.APIHost).Id))

	handler = hostFilterGetHandler{
		params: model.APIHostParams{Distro: "ubuntu-1604"},
	}
	resp = handler.Run(gimlet.AttachUser(context.Background(), &user.DBUser{Id: "user0"}))
	assert.Equal(t, http.StatusOK, resp.Status())
	hosts = resp.Data().([]interface{})
	require.Len(t, hosts, 1)
	assert.Equal(t, "h1", utility.FromStringPtr(hosts[0].(*model.APIHost).Id))

	handler = hostFilterGetHandler{
		params: model.APIHostParams{CreatedAfter: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)},
	}
	resp = handler.Run(gimlet.AttachUser(context.Background(), &user.DBUser{Id: "user1"}))
	assert.Equal(t, http.StatusOK, resp.Status())
	hosts = resp.Data().([]interface{})
	require.Len(t, hosts, 1)
	assert.Equal(t, "h2", utility.FromStringPtr(hosts[0].(*model.APIHost).Id))
}

func TestDisableHostHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assert.NoError(t, db.ClearCollections(host.Collection))
	hostID := "h1"
	hosts := []host.Host{
		{
			Id:     hostID,
			Status: evergreen.HostRunning,
		},
	}
	for _, hostToAdd := range hosts {
		assert.NoError(t, hostToAdd.Insert(ctx))
	}
	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))
	dh := disableHost{
		hostID: hostID,
		env:    env,
	}

	responder := dh.Run(context.Background())
	assert.Equal(t, http.StatusOK, responder.Status())
	foundHost, err := host.FindOneId(ctx, hostID)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.HostDecommissioned, foundHost.Status)
}

func TestHostIsUpPostHandler(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(host.Collection))
	}()

	generateFakeEC2InstanceID := func() string {
		return "i-" + utility.RandomString()
	}

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, rh *hostIsUpPostHandler, h *host.Host){
		"ConvertsBuildingIntentHostToStartingRealHost": func(ctx context.Context, t *testing.T, rh *hostIsUpPostHandler, h *host.Host) {
			require.NoError(t, h.Insert(ctx))
			instanceID := generateFakeEC2InstanceID()

			rh.params = restmodel.APIHostIsUpOptions{
				HostID:        h.Id,
				EC2InstanceID: instanceID,
			}
			resp := rh.Run(ctx)

			require.NotZero(t, resp)
			assert.Equal(t, resp.Status(), http.StatusOK)
			apiHost, ok := resp.Data().(*restmodel.APIHost)
			require.True(t, ok, resp.Data())
			require.NotZero(t, apiHost)
			assert.Equal(t, instanceID, utility.FromStringPtr(apiHost.Id))

			dbIntentHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			assert.Zero(t, dbIntentHost)

			realHost, err := host.FindOneId(ctx, instanceID)
			require.NoError(t, err)
			require.NotZero(t, realHost)
			assert.Equal(t, realHost.Status, evergreen.HostStarting, "intent host should be converted to real host when it's up")
		},
		"ConvertsFailedIntentHostToDecommissionedRealHost": func(ctx context.Context, t *testing.T, rh *hostIsUpPostHandler, h *host.Host) {
			h.Status = evergreen.HostBuildingFailed
			require.NoError(t, h.Insert(ctx))

			instanceID := generateFakeEC2InstanceID()

			rh.params = restmodel.APIHostIsUpOptions{
				HostID:        h.Id,
				EC2InstanceID: instanceID,
			}

			resp := rh.Run(ctx)

			require.NotZero(t, resp)
			assert.Equal(t, resp.Status(), http.StatusOK)
			apiHost, ok := resp.Data().(*restmodel.APIHost)
			require.True(t, ok, resp.Data())
			require.NotZero(t, apiHost)
			assert.Equal(t, instanceID, utility.FromStringPtr(apiHost.Id))
			assert.NotZero(t, resp)
			assert.Equal(t, resp.Status(), http.StatusOK)

			dbIntentHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			assert.Zero(t, dbIntentHost)

			realHost, err := host.FindOneId(ctx, instanceID)
			require.NoError(t, err)
			require.NotZero(t, realHost)
			assert.Equal(t, evergreen.HostDecommissioned, realHost.Status, "host that fails to build should be decommissioned when it comes up")
		},
		"ConvertsTerminatedHostIntoDecommissionedRealHost": func(ctx context.Context, t *testing.T, rh *hostIsUpPostHandler, h *host.Host) {
			h.Status = evergreen.HostTerminated
			require.NoError(t, h.Insert(ctx))

			instanceID := generateFakeEC2InstanceID()
			rh.params = restmodel.APIHostIsUpOptions{
				HostID:        h.Id,
				EC2InstanceID: instanceID,
			}

			resp := rh.Run(ctx)

			require.NotZero(t, resp)
			assert.Equal(t, resp.Status(), http.StatusOK)
			apiHost, ok := resp.Data().(*restmodel.APIHost)
			require.True(t, ok, resp.Data())
			require.NotZero(t, apiHost)
			assert.Equal(t, instanceID, utility.FromStringPtr(apiHost.Id))
			assert.NotZero(t, resp)
			assert.Equal(t, resp.Status(), http.StatusOK)

			dbIntentHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			assert.Zero(t, dbIntentHost)

			realHost, err := host.FindOneId(ctx, instanceID)
			require.NoError(t, err)
			require.NotZero(t, realHost)
			assert.Equal(t, realHost.Status, evergreen.HostDecommissioned, "already-terminated intent host should be decommissioned when it's up")
		},
		"NoopsForNonIntentHost": func(ctx context.Context, t *testing.T, rh *hostIsUpPostHandler, h *host.Host) {
			instanceID := generateFakeEC2InstanceID()
			h.Id = instanceID
			h.Status = evergreen.HostStarting
			require.NoError(t, h.Insert(ctx))

			rh.params = restmodel.APIHostIsUpOptions{
				HostID:        instanceID,
				EC2InstanceID: instanceID,
			}

			resp := rh.Run(ctx)

			require.NotZero(t, resp)
			assert.Equal(t, resp.Status(), http.StatusOK)
			apiHost, ok := resp.Data().(*restmodel.APIHost)
			require.True(t, ok, resp.Data())
			require.NotZero(t, apiHost)
			assert.Equal(t, instanceID, utility.FromStringPtr(apiHost.Id))
			assert.NotZero(t, resp)
			assert.Equal(t, resp.Status(), http.StatusOK)

			dbHost, err := host.FindOneId(ctx, instanceID)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostStarting, dbHost.Status, "host should not be modified if it's not an intent host")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			require.NoError(t, db.ClearCollections(host.Collection))

			rh, ok := makeHostIsUpPostHandler().(*hostIsUpPostHandler)
			require.True(t, ok)

			h := &host.Host{
				Id:       "evg-12345",
				Status:   evergreen.HostBuilding,
				Provider: evergreen.ProviderNameEc2Fleet,
				Distro: distro.Distro{
					Id:       "distro_id",
					Provider: evergreen.ProviderNameEc2Fleet,
				},
			}

			tCase(ctx, t, rh, h)
		})
	}
}

func TestHostProvisioningOptionsGetHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))
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
	require.NoError(t, h.Insert(ctx))

	t.Run("SucceedsWithValidHostID", func(t *testing.T) {
		rh := hostProvisioningOptionsGetHandler{
			hostID: h.Id,
			env:    env,
		}
		resp := rh.Run(ctx)
		require.Equal(t, http.StatusOK, resp.Status())
		opts, ok := resp.Data().(model.APIHostProvisioningOptions)
		require.True(t, ok)
		assert.NotEmpty(t, opts.Content)
	})
	t.Run("FailsWithoutHostID", func(t *testing.T) {
		rh := hostProvisioningOptionsGetHandler{
			env: env,
		}
		resp := rh.Run(ctx)
		assert.NotEqual(t, http.StatusOK, resp.Status())
	})
	t.Run("FailsWithInvalidHostID", func(t *testing.T) {
		rh := hostProvisioningOptionsGetHandler{
			hostID: "foo",
			env:    env,
		}
		resp := rh.Run(ctx)
		assert.NotEqual(t, http.StatusOK, resp.Status())
	})
}
