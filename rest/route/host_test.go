package route

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model/host"
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
	res, err := handler.Execute(nil, s.sc)
	s.NoError(err)
	s.NotNil(res)
	s.Equal(1, len(res.Result))

	h, ok := (res.Result[0]).(*model.APIHost)
	s.True(ok)
	s.Equal(model.APIString("host1"), h.Id)
}

func (s *HostSuite) TestFindByIdLast() {
	handler := &hostIDGetHandler{hostId: "host2"}
	res, err := handler.Execute(nil, s.sc)
	s.NoError(err)
	s.NotNil(res)
	s.Equal(1, len(res.Result))

	h, ok := (res.Result[0]).(*model.APIHost)
	s.True(ok)
	s.Equal(model.APIString("host2"), h.Id)
}

func (s *HostSuite) TestFindByIdFail() {
	handler := &hostIDGetHandler{hostId: "host3"}
	_, ok := handler.Execute(nil, s.sc)
	s.Error(ok)
}
