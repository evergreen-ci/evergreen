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
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/stretchr/testify/suite"
)

type HostSuite struct {
	sc   *data.MockConnector
	data data.MockHostConnector

	suite.Suite
}

func TestHostSuite(t *testing.T) {
	suite.Run(t, new(HostSuite))
}

func (s *HostSuite) SetupSuite() {
	s.data = data.MockHostConnector{
		CachedHosts: []host.Host{host.Host{Id: "host1"}, host.Host{Id: "host2"}},
	}

	s.sc = &data.MockConnector{
		MockHostConnector: s.data,
	}
}

func (s *HostSuite) TestFindByIdFirst() {
	handler := &hostIDGetHandler{hostId: "host1"}
	res, err := handler.Execute(context.TODO(), s.sc)
	s.NoError(err)
	s.NotNil(res)
	s.Equal(1, len(res.Result))

	h, ok := (res.Result[0]).(*model.APIHost)
	s.True(ok)
	s.Equal(model.APIString("host1"), h.Id)
}

func (s *HostSuite) TestFindByIdLast() {
	handler := &hostIDGetHandler{hostId: "host2"}
	res, err := handler.Execute(context.TODO(), s.sc)
	s.NoError(err)
	s.NotNil(res)
	s.Equal(1, len(res.Result))

	h, ok := (res.Result[0]).(*model.APIHost)
	s.True(ok)
	s.Equal(model.APIString("host2"), h.Id)
}

func (s *HostSuite) TestFindByIdFail() {
	handler := &hostIDGetHandler{hostId: "host3"}
	_, ok := handler.Execute(context.TODO(), s.sc)
	s.Error(ok)
}

type TestTerminateHostHandlerSuite struct {
	rm *RouteManager
	sc *data.MockConnector

	suite.Suite
}

func TestTerminateHostHandler(t *testing.T) {
	s := &TestTerminateHostHandlerSuite{}
	suite.Run(t, s)
}

func (s *TestTerminateHostHandlerSuite) SetupTest() {
	s.rm = getHostTerminateRouteManager("", 2)
	s.sc = getMockHostsConnector()
}

func (s *TestTerminateHostHandlerSuite) TestExecuteWithNoUserPanics() {
	s.PanicsWithValue("no user attached to request", func() {
		_, _ = s.rm.Methods[0].Execute(context.TODO(), s.sc)
	})
}

func (s *TestTerminateHostHandlerSuite) TestExecuteWithInvalidHost() {
	h := s.rm.Methods[0].Handler().(*hostTerminateHandler)
	h.hostID = "host-that-doesn't-exist"

	ctx := context.Background()
	ctx = context.WithValue(ctx, evergreen.RequestUser, s.sc.MockUserConnector.CachedUsers["user0"])
	data, err := h.Execute(ctx, s.sc)
	s.Empty(data.Result)
	s.Error(err)
}

func (s *TestTerminateHostHandlerSuite) TestExecuteWithTerminatedHost() {
	h := s.rm.Methods[0].Handler().(*hostTerminateHandler)
	h.hostID = "host1"

	ctx := context.Background()
	ctx = context.WithValue(ctx, evergreen.RequestUser, s.sc.MockUserConnector.CachedUsers["user0"])

	data, err := h.Execute(ctx, s.sc)
	s.Empty(data.Result)
	s.NotNil(err)
	s.IsType(new(rest.APIError), err)
	apiErr := err.(*rest.APIError)
	s.Equal(http.StatusBadRequest, apiErr.StatusCode)
	s.Equal(evergreen.HostTerminated, s.sc.CachedHosts[0].Status)
}

func (s *TestTerminateHostHandlerSuite) TestExecuteWithUninitializedHost() {
	h := s.rm.Methods[0].Handler().(*hostTerminateHandler)
	h.hostID = "host3"

	s.Equal(evergreen.HostUninitialized, s.sc.CachedHosts[2].Status)
	ctx := context.Background()
	ctx = context.WithValue(ctx, evergreen.RequestUser, s.sc.MockUserConnector.CachedUsers["user0"])

	data, err := h.Execute(ctx, s.sc)
	s.Empty(data.Result)
	s.Nil(err)
	s.Equal(evergreen.HostTerminated, s.sc.CachedHosts[2].Status)
}

func (s *TestTerminateHostHandlerSuite) TestExecuteWithRunningHost() {
	h := s.rm.Methods[0].Handler().(*hostTerminateHandler)
	h.hostID = "host2"

	s.Equal(evergreen.HostRunning, s.sc.CachedHosts[1].Status)
	ctx := context.Background()
	ctx = context.WithValue(ctx, evergreen.RequestUser, s.sc.MockUserConnector.CachedUsers["user0"])

	data, err := h.Execute(ctx, s.sc)
	s.Empty(data.Result)
	s.Nil(err)
	s.Equal(evergreen.HostRunning, s.sc.CachedHosts[1].Status)
}

func (s *TestTerminateHostHandlerSuite) TestSuperUserCanTerminateAnyHost() {
	h := s.rm.Methods[0].Handler().(*hostTerminateHandler)
	h.hostID = "host3"

	s.Equal(evergreen.HostRunning, s.sc.CachedHosts[1].Status)
	ctx := context.Background()
	ctx = context.WithValue(ctx, evergreen.RequestUser, s.sc.MockUserConnector.CachedUsers["root"])

	data, err := h.Execute(ctx, s.sc)
	s.Empty(data.Result)
	s.Nil(err)
	s.Equal(evergreen.HostTerminated, s.sc.CachedHosts[2].Status)
}

type TestHostChangeRDPPasswordHandlerSuite struct {
	rm *RouteManager
	sc *data.MockConnector
	suite.Suite
}

func TestHostChangeRDPPasswordHandler(t *testing.T) {
	s := &TestHostChangeRDPPasswordHandlerSuite{}
	suite.Run(t, s)
}

func (s *TestHostChangeRDPPasswordHandlerSuite) SetupTest() {
	s.rm = getHostChangeRDPPasswordRouteManager("", 2)
	s.sc = getMockHostsConnector()
}

func (s *TestHostChangeRDPPasswordHandlerSuite) TestExecuteWithNoUserPanics() {
	s.PanicsWithValue("no user attached to request", func() {
		_, _ = s.rm.Methods[0].Execute(context.TODO(), s.sc)
	})
}

func (s *TestHostChangeRDPPasswordHandlerSuite) TestExecute() {
	h := s.rm.Methods[0].Handler().(*hostChangeRDPPasswordHandler)
	h.hostID = "host2"
	h.rdpPassword = "Hunter2!"

	ctx := context.Background()
	ctx = context.WithValue(ctx, evergreen.RequestUser, s.sc.MockUserConnector.CachedUsers["user0"])

	data, err := h.Execute(ctx, s.sc)
	s.Empty(data.Result)
	s.Nil(err)
}

func (s *TestHostChangeRDPPasswordHandlerSuite) TestExecuteWithUninitializedHostFails() {
	h := s.rm.Methods[0].Handler().(*hostChangeRDPPasswordHandler)
	h.hostID = "host3"
	h.rdpPassword = "Hunter2!"

	ctx := context.Background()
	ctx = context.WithValue(ctx, evergreen.RequestUser, s.sc.MockUserConnector.CachedUsers["user0"])

	data, err := h.Execute(ctx, s.sc)
	s.Empty(data.Result)
	s.Error(err)
}

func (s *TestHostChangeRDPPasswordHandlerSuite) TestExecuteWithInvalidHost() {
	h := s.rm.Methods[0].Handler().(*hostChangeRDPPasswordHandler)
	h.hostID = "host-that-doesn't-exist"

	ctx := context.Background()
	ctx = context.WithValue(ctx, evergreen.RequestUser, s.sc.MockUserConnector.CachedUsers["user0"])

	data, err := h.Execute(ctx, s.sc)
	s.Empty(data.Result)
	s.Error(err)
}

func (s *TestHostChangeRDPPasswordHandlerSuite) TestParseAndValidateRejectsInvalidPasswords() {
	invalidPasswords := []model.APIString{"", "weak", "stilltooweak1", "火a111"}
	for _, password := range invalidPasswords {
		mod := model.APISpawnHostModify{
			HostID: "host1",
			RDPPwd: password,
		}
		err := s.tryParseAndValidate(mod)

		s.Error(err)
		s.IsType(new(rest.APIError), err)
		apiErr := err.(*rest.APIError)
		s.Equal(http.StatusBadRequest, apiErr.StatusCode)
	}
}

func (s *TestHostChangeRDPPasswordHandlerSuite) TestSuperUserCanChangeAnyHost() {
	h := s.rm.Methods[0].Handler().(*hostChangeRDPPasswordHandler)
	h.hostID = "host2"
	h.rdpPassword = "Hunter2!"

	ctx := context.Background()
	ctx = context.WithValue(ctx, evergreen.RequestUser, s.sc.MockUserConnector.CachedUsers["root"])

	data, err := h.Execute(ctx, s.sc)
	s.Empty(data.Result)
	s.Nil(err)
}

func (s *TestHostChangeRDPPasswordHandlerSuite) tryParseAndValidate(mod model.APISpawnHostModify) error {
	r, err := makeMockHostRequest(mod)
	s.NoError(err)

	h := s.rm.Methods[0].Handler().(*hostChangeRDPPasswordHandler)

	return h.ParseAndValidate(context.TODO(), r)
}

type TestHostExtendExpirationHandlerSuite struct {
	rm *RouteManager
	sc *data.MockConnector
	suite.Suite
}

func TestHostExtendExpirationHandler(t *testing.T) {
	s := &TestHostExtendExpirationHandlerSuite{}
	suite.Run(t, s)
}

func (s *TestHostExtendExpirationHandlerSuite) SetupTest() {
	s.rm = getHostExtendExpirationRouteManager("", 2)
	s.sc = getMockHostsConnector()
}

func (s *TestHostExtendExpirationHandlerSuite) TestHostExtendExpirationWithNoUserPanics() {
	s.PanicsWithValue("no user attached to request", func() {
		_, _ = s.rm.Methods[0].Execute(context.TODO(), s.sc)
	})
}

func (s *TestHostExtendExpirationHandlerSuite) TestExecuteWithInvalidHost() {
	h := s.rm.Methods[0].Handler().(*hostExtendExpirationHandler)
	h.hostID = "host-that-doesn't-exist"

	ctx := context.Background()
	ctx = context.WithValue(ctx, evergreen.RequestUser, s.sc.MockUserConnector.CachedUsers["user0"])
	data, err := h.Execute(ctx, s.sc)
	s.Empty(data.Result)
	s.Error(err)
}

func (s *TestHostExtendExpirationHandlerSuite) TestParseAndValidateRejectsInvalidExpirations() {
	invalidExpirations := []model.APIString{"not a number", "0", "9223372036854775807", ""}
	for _, extendBy := range invalidExpirations {
		mod := model.APISpawnHostModify{
			HostID:   "host1",
			RDPPwd:   "",
			AddHours: extendBy,
		}

		err := s.tryParseAndValidate(mod)
		s.Error(err)
		s.IsType(new(rest.APIError), err)
		apiErr := err.(*rest.APIError)
		s.Equal(http.StatusBadRequest, apiErr.StatusCode)
	}
}

func (s *TestHostExtendExpirationHandlerSuite) TestExecuteWithLargeExpirationFails() {
	h := s.rm.Methods[0].Handler().(*hostExtendExpirationHandler)
	h.hostID = "host2"
	h.addHours = 9001

	ctx := context.Background()
	ctx = context.WithValue(ctx, evergreen.RequestUser, s.sc.MockUserConnector.CachedUsers["user0"])
	data, err := h.Execute(ctx, s.sc)
	s.Empty(data.Result)
	s.Error(err)
	s.IsType(new(rest.APIError), err)
	apiErr := err.(*rest.APIError)
	s.Equal(http.StatusBadRequest, apiErr.StatusCode)
}

func (s *TestHostExtendExpirationHandlerSuite) TestExecute() {
	expectedTime := s.sc.CachedHosts[1].ExpirationTime.Add(8 * time.Hour)

	h := s.rm.Methods[0].Handler().(*hostExtendExpirationHandler)
	h.hostID = "host2"
	h.addHours = 8

	ctx := context.Background()
	ctx = context.WithValue(ctx, evergreen.RequestUser, s.sc.MockUserConnector.CachedUsers["user0"])
	data, err := h.Execute(ctx, s.sc)
	s.Empty(data.Result)
	s.NoError(err)
	s.Equal(expectedTime, s.sc.CachedHosts[1].ExpirationTime)
}

func (s *TestHostExtendExpirationHandlerSuite) TestSuperUserCanExtendAnyHost() {
	expectedTime := s.sc.CachedHosts[1].ExpirationTime.Add(8 * time.Hour)

	h := s.rm.Methods[0].Handler().(*hostExtendExpirationHandler)
	h.hostID = "host2"
	h.addHours = 8

	ctx := context.Background()
	ctx = context.WithValue(ctx, evergreen.RequestUser, s.sc.MockUserConnector.CachedUsers["root"])
	data, err := h.Execute(ctx, s.sc)
	s.Empty(data.Result)
	s.NoError(err)
	s.Equal(expectedTime, s.sc.CachedHosts[1].ExpirationTime)
}

func (s *TestHostExtendExpirationHandlerSuite) tryParseAndValidate(mod model.APISpawnHostModify) error {
	r, err := makeMockHostRequest(mod)
	s.NoError(err)

	h := s.rm.Methods[0].Handler().(*hostExtendExpirationHandler)

	return h.ParseAndValidate(context.TODO(), r)
}

func makeMockHostRequest(mod model.APISpawnHostModify) (*http.Request, error) {
	data, err := json.Marshal(mod)
	if err != nil {
		return nil, err
	}

	var r *http.Request
	r, err = http.NewRequest("POST", fmt.Sprintf("https://example.com/hosts/%s", string(mod.HostID)), bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	return r, nil
}

func getMockHostsConnector() *data.MockConnector {
	windowsDistro := distro.Distro{
		Id: "windows",
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
						Id: "linux",
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
