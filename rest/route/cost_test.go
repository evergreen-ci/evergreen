package route

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/stretchr/testify/suite"
)

type VersionCostSuite struct {
	sc   *data.MockConnector
	data data.MockVersionConnector

	suite.Suite
}

func TestVersionCostSuite(t *testing.T) {
	suite.Run(t, new(VersionCostSuite))
}

func (s *VersionCostSuite) SetupSuite() {
	testTask1 := task.Task{Id: "task1", Version: "version1", TimeTaken: time.Duration(1)}
	testTask2 := task.Task{Id: "task2", Version: "version2", TimeTaken: time.Duration(1)}
	testTask3 := task.Task{Id: "task3", Version: "version2", TimeTaken: time.Duration(1)}
	s.data = data.MockVersionConnector{
		CachedTasks: []task.Task{testTask1, testTask2, testTask3},
	}
	s.sc = &data.MockConnector{
		MockVersionConnector: s.data,
	}
}

// TestFindCostByVersionIdSingle tests the handler where information is aggregated on
// a single task of a version id
func (s *VersionCostSuite) TestFindCostByVersionIdSingle() {
	// Test that the handler executes properly
	handler := &costByVersionHandler{versionId: "version1"}
	res, err := handler.Execute(nil, s.sc)
	s.NoError(err)
	s.NotNil(res)
	s.Equal(1, len(res.Result))

	// Test that the handler returns the result with correct properties, i.e. that
	// it is the right type (model.APIVersionCost) and has correct versionId and SumTimeTaken
	versionCost := res.Result[0]
	h, ok := (versionCost).(*model.APIVersionCost)
	s.True(ok)
	s.Equal(model.APIString("version1"), h.VersionId)
	s.Equal(time.Duration(1), h.SumTimeTaken)
}

// TestFindCostByVersionIdMany tests the handler where information is aggregated on
// multiple tasks of the same version id
func (s *VersionCostSuite) TestFindCostByVersionIdMany() {
	// Test that the handler executes properly
	handler := &costByVersionHandler{versionId: "version2"}
	res, err := handler.Execute(nil, s.sc)
	s.NoError(err)
	s.NotNil(res)
	s.Equal(1, len(res.Result))

	// Test that the handler returns the result with correct properties, i.e. that
	// it is the right type (model.APIVersionCost) and has correct versionId and SumTimeTaken
	versionCost := res.Result[0]
	h, ok := (versionCost).(*model.APIVersionCost)
	s.True(ok)
	s.Equal(model.APIString("version2"), h.VersionId)
	s.Equal(time.Duration(2), h.SumTimeTaken)
}

// TestFindCostByVersionFail tests that the handler correctly returns error when
// incorrect query is passed in
func (s *VersionCostSuite) TestFindCostByVersionIdFail() {
	handler := &costByVersionHandler{versionId: "fake_version"}
	res, ok := handler.Execute(nil, s.sc)
	s.Nil(res.Result)
	s.Error(ok)
}

type DistroCostSuite struct {
	sc   *data.MockConnector
	data data.MockDistroCostConnector

	suite.Suite
}

func TestDistroCostSuite(t *testing.T) {
	suite.Run(t, new(DistroCostSuite))
}

func (s *DistroCostSuite) SetupSuite() {
	testTask1 := task.Task{Id: "task1", DistroId: "distro1", TimeTaken: time.Duration(1)}
	testTask2 := task.Task{Id: "task2", DistroId: "distro2", TimeTaken: time.Duration(1)}
	testTask3 := task.Task{Id: "task3", DistroId: "distro2", TimeTaken: time.Duration(1)}
	s.data = data.MockDistroCostConnector{
		CachedTasks: []task.Task{testTask1, testTask2, testTask3},
	}
	s.sc = &data.MockConnector{
		MockDistroCostConnector: s.data,
	}
}

// TestFindCostByDistroIdSingle tests the handler where information is aggregated on
// a single task of a distro id
func (s *DistroCostSuite) TestFindCostByDistroIdSingle() {
	// Test that the handler executes properly
	handler := &costByDistroHandler{distroId: "distro1"}
	res, err := handler.Execute(nil, s.sc)
	s.NoError(err)
	s.NotNil(res)
	s.Equal(1, len(res.Result))

	// Test that the handler returns the result with correct properties, i.e. that
	// it is the right type (model.APIDistroCost) and has correct distroId and SumTimeTaken
	distroCost := res.Result[0]
	h, ok := (distroCost).(*model.APIDistroCost)
	s.True(ok)
	s.Equal(model.APIString("distro1"), h.DistroId)
	s.Equal(time.Duration(1), h.SumTimeTaken)
}

// TestFindCostByDistroIdMany tests the handler where information is aggregated on
// multiple tasks of the same distro id
func (s *DistroCostSuite) TestFindCostByDistroIdMany() {
	// Test that the handler executes properly
	handler := &costByDistroHandler{distroId: "distro2"}
	res, err := handler.Execute(nil, s.sc)
	s.NoError(err)
	s.NotNil(res)
	s.Equal(1, len(res.Result))

	// Test that the handler returns the result with correct properties, i.e. that
	// it is the right type (model.APIDistroCost) and has correct distroId and SumTimeTaken
	distroCost := res.Result[0]
	h, ok := (distroCost).(*model.APIDistroCost)
	s.True(ok)
	s.Equal(model.APIString("distro2"), h.DistroId)
	s.Equal(time.Duration(2), h.SumTimeTaken)
}

// TestFindCostByDistroFail tests that the handler correctly returns error when
// incorrect query is passed in
func (s *DistroCostSuite) TestFindCostByDistroIdFail() {
	handler := &costByDistroHandler{distroId: "fake_distro"}
	res, ok := handler.Execute(nil, s.sc)
	s.Nil(res.Result)
	s.Error(ok)
}
