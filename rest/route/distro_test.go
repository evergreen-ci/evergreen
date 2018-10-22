package route

import (
	"context"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/suite"
)

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch distro by id

type DistroByIdSuite struct {
	sc   *data.MockConnector
	data data.MockDistroConnector
	rm   gimlet.RouteHandler
	suite.Suite
}

func TestDistroSuite(t *testing.T) {
	suite.Run(t, new(DistroByIdSuite))
}

func (s *DistroByIdSuite) SetupSuite() {
	s.data = data.MockDistroConnector{
		CachedDistros: []distro.Distro{
			{Id: "distro1"},
			{Id: "distro2"},
		},
		CachedTasks: []task.Task{
			{Id: "task1"},
			{Id: "task2"},
		},
	}
	s.sc = &data.MockConnector{
		MockDistroConnector: s.data,
	}
}

func (s *DistroByIdSuite) SetupTest() {
	s.rm = makeGetDistroByID(s.sc)
}

func (s *DistroByIdSuite) TestFindByIdFound() {
	s.rm.(*distroIDGetHandler).distroId = "distro1"
	resp := s.rm.Run(context.TODO())
	s.Equal(resp.Status(), http.StatusOK)
	s.NotNil(resp.Data())

	d, ok := (resp.Data()).(*model.APIDistro)
	s.True(ok)
	s.Equal(model.ToAPIString("distro1"), d.Name)
}

func (s *DistroByIdSuite) TestFindByIdFail() {
	s.rm.(*distroIDGetHandler).distroId = "distro3"
	resp := s.rm.Run(context.TODO())
	s.NotEqual(resp.Status(), http.StatusOK)
}
