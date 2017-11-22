package route

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/stretchr/testify/assert"
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

	s.sc = &data.MockConnector{
		MockHostConnector: data.MockHostConnector{
			CachedHosts: []host.Host{
				{
					Id:             "host1",
					Host:           "host1",
					Status:         evergreen.HostTerminated,
					ExpirationTime: time.Now().Add(time.Hour),
				},
				{
					Id:             "host2",
					Host:           "host2",
					Status:         evergreen.HostRunning,
					ExpirationTime: time.Now().Add(time.Hour),
				},
				{
					Id:             "host3",
					Host:           "host3",
					Status:         evergreen.HostUninitialized,
					ExpirationTime: time.Now().Add(time.Hour),
				},
			},
		}}
}

func (s *TestTerminateHostHandlerSuite) TestExecuteWithInvalidHost() {
	h := s.rm.Methods[0].Handler().(*hostTerminateHandler)
	h.hostID = "host-that-doesn't-exist"

	data, err := h.Execute(context.TODO(), s.sc)
	s.Empty(data.Result)
	s.Error(err)
}

func (s *TestTerminateHostHandlerSuite) TestExecuteWithTerminatedHost() {
	h := s.rm.Methods[0].Handler().(*hostTerminateHandler)
	h.hostID = "host1"

	data, err := h.Execute(context.TODO(), s.sc)
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
	data, err := h.Execute(context.TODO(), s.sc)
	s.Empty(data.Result)
	s.Nil(err)
	s.Equal(evergreen.HostTerminated, s.sc.CachedHosts[2].Status)

}

func (s *TestTerminateHostHandlerSuite) TestExecuteWithRunningHost() {
	h := s.rm.Methods[0].Handler().(*hostTerminateHandler)
	h.hostID = "host2"

	s.Equal(evergreen.HostRunning, s.sc.CachedHosts[1].Status)
	data, err := h.Execute(context.TODO(), s.sc)
	s.Empty(data.Result)
	s.Nil(err)
	s.Equal(evergreen.HostRunning, s.sc.CachedHosts[1].Status)

}

type TestHostChangeRDPPasswordHandlerSuite struct {
	hostID      string
	rdpPassword string

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
	s.sc = &data.MockConnector{
		MockHostConnector: data.MockHostConnector{
			CachedHosts: []host.Host{
				{
					Id:             "host1",
					Host:           "host1",
					Status:         evergreen.HostTerminated,
					ExpirationTime: time.Now().Add(time.Hour),
				},
				{
					Id:             "host2",
					Host:           "host2",
					Status:         evergreen.HostRunning,
					ExpirationTime: time.Now().Add(time.Hour),
				},
				{
					Id:             "host3",
					Host:           "host3",
					Status:         evergreen.HostUninitialized,
					ExpirationTime: time.Now().Add(time.Hour),
				},
			},
		}}
}

func (s *TestHostChangeRDPPasswordHandlerSuite) TestExecuteWithInvalidHost() {
	h := s.rm.Methods[0].Handler().(*hostChangeRDPPasswordHandler)
	h.hostID = "host-that-doesn't-exist"

	data, err := h.Execute(context.TODO(), s.sc)
	s.Empty(data.Result)
	s.Error(err)
}

func (s *TestHostChangeRDPPasswordHandlerSuite) TestParseAndValidateRejectsInvalidPasswords() {
	invalidPasswords := []model.APIString{"", "weak", "stilltooweak1", "火a11"}
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

func (s *TestHostChangeRDPPasswordHandlerSuite) TestExecuteChangePassword() {
	h := s.rm.Methods[0].Handler().(*hostChangeRDPPasswordHandler)
	h.hostID = "host2"
	h.rdpPassword = "Hunter2!"

	data, err := h.Execute(context.TODO(), s.sc)
	s.Empty(data.Result)
	s.Nil(err)
}

func (s *TestHostChangeRDPPasswordHandlerSuite) tryParseAndValidate(mod model.APISpawnHostModify) error {
	data, err := json.Marshal(mod)
	s.NoError(err)
	s.NotEmpty(data)

	var r *http.Request
	r, err = http.NewRequest("POST", "http://example.com/spawn", bytes.NewReader(data))
	s.NoError(err)
	s.NotNil(r)

	h := s.rm.Methods[0].Handler().(*hostChangeRDPPasswordHandler)

	return h.ParseAndValidate(context.TODO(), r)
}

type TestHostExtendExpirationHandlerSuite struct {
	hostID  string
	addHour time.Duration

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
	s.sc = &data.MockConnector{
		MockHostConnector: data.MockHostConnector{
			CachedHosts: []host.Host{
				{
					Id:             "host1",
					Host:           "host1",
					Status:         evergreen.HostTerminated,
					ExpirationTime: time.Now().Add(time.Hour),
				},
				{
					Id:             "host2",
					Host:           "host2",
					Status:         evergreen.HostRunning,
					ExpirationTime: time.Now().Add(time.Hour),
				},
				{
					Id:             "host3",
					Host:           "host3",
					Status:         evergreen.HostUninitialized,
					ExpirationTime: time.Now().Add(time.Hour),
				},
			},
		}}
}

func (s *TestHostExtendExpirationHandlerSuite) TestExecuteWithInvalidHost() {
	h := s.rm.Methods[0].Handler().(*hostExtendExpirationHandler)
	h.hostID = "host-that-doesn't-exist"

	data, err := h.Execute(context.TODO(), s.sc)
	s.Empty(data.Result)
	s.Error(err)
}

func (s *TestHostExtendExpirationHandlerSuite) TestParseAndValidateRejectsInvalidExpirations() {
	invalidExpirations := []model.APIString{"not a number", "0", "9223372036854775807"}
	for _, extendBy := range invalidExpirations {
		mod := model.APISpawnHostModify{
			Action:   HostExpirationExtension,
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

func (s *TestHostExtendExpirationHandlerSuite) TestExecuteExtendExpirationWithLargeExpirationFails() {
	h := s.rm.Methods[0].Handler().(*hostExtendExpirationHandler)
	h.hostID = "host2"
	h.addHours = 9001

	data, err := h.Execute(context.TODO(), s.sc)
	s.Empty(data.Result)
	s.Error(err)
	s.IsType(new(rest.APIError), err)
	apiErr := err.(*rest.APIError)
	s.Equal(http.StatusBadRequest, apiErr.StatusCode)
}

func (s *TestHostExtendExpirationHandlerSuite) TestExecuteExtendExpiration() {
	expectedTime := s.sc.CachedHosts[1].ExpirationTime.Add(8 * time.Hour)

	h := s.rm.Methods[0].Handler().(*hostExtendExpirationHandler)
	h.hostID = "host2"
	h.addHours = 8

	data, err := h.Execute(context.TODO(), s.sc)
	s.Empty(data.Result)
	s.NoError(err)
	s.Equal(expectedTime, s.sc.CachedHosts[1].ExpirationTime)

}

func (s *TestHostExtendExpirationHandlerSuite) tryParseAndValidate(mod model.APISpawnHostModify) error {
	data, err := json.Marshal(mod)
	s.NoError(err)
	s.NotEmpty(data)

	var r *http.Request
	r, err = http.NewRequest("POST", "http://example.com/spawn", bytes.NewReader(data))
	s.NoError(err)
	s.NotNil(r)

	h := s.rm.Methods[0].Handler().(*hostExtendExpirationHandler)

	return h.ParseAndValidate(context.TODO(), r)
}

func TestMakeNewHostExpiration(t *testing.T) {
	assert := assert.New(t)

	host := host.Host{
		ExpirationTime: time.Now().Add(12 * time.Hour),
	}

	expTime, err := makeNewHostExpiration(&host, 1)
	assert.NotZero(expTime)
	assert.NoError(err, expTime.Format(time.RFC3339))
}

func TestMakeNewHostExpirationFailsBeyondOneWeek(t *testing.T) {
	assert := assert.New(t)

	host := host.Host{
		ExpirationTime: time.Now().Add(12 * time.Hour),
	}

	expTime, err := makeNewHostExpiration(&host, 24*7)
	assert.Zero(expTime)
	assert.Error(err, expTime.Format(time.RFC3339))
}
