package route

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
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
