package route

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model/distro"
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
	testTask1 := task.Task{Id: "task1", Version: "version1", TimeTaken: time.Millisecond}
	testTask2 := task.Task{Id: "task2", Version: "version2", TimeTaken: time.Millisecond}
	testTask3 := task.Task{Id: "task3", Version: "version2", TimeTaken: time.Millisecond}
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
	handler := &costByVersionHandler{versionId: "version1", sc: s.sc}
	res := handler.Run(context.TODO())
	s.NotNil(res)
	s.Equal(http.StatusOK, res.Status())

	// Test that the handler returns the result with correct properties, i.e. that
	// it is the right type (model.APIVersionCost) and has correct versionId and SumTimeTaken
	h, ok := (res.Data()).(*model.APIVersionCost)
	s.True(ok)
	s.Equal(model.ToStringPtr("version1"), h.VersionId)
	s.Equal(1*time.Millisecond, h.SumTimeTaken)
}

// TestFindCostByVersionIdMany tests the handler where information is aggregated on
// multiple tasks of the same version id
func (s *VersionCostSuite) TestFindCostByVersionIdMany() {
	// Test that the handler executes properly
	handler := &costByVersionHandler{versionId: "version2", sc: s.sc}
	res := handler.Run(context.TODO())
	s.NotNil(res)
	s.Equal(http.StatusOK, res.Status())

	h, ok := (res.Data()).(*model.APIVersionCost)
	s.True(ok)
	s.Equal(model.ToStringPtr("version2"), h.VersionId)
	s.Equal(2*time.Millisecond, h.SumTimeTaken)
}

// TestFindCostByVersionFail tests that the handler correctly returns error when
// incorrect query is passed in
func (s *VersionCostSuite) TestFindCostByVersionIdFail() {
	handler := &costByVersionHandler{versionId: "fake_version", sc: s.sc}
	res := handler.Run(context.TODO())
	s.NotEqual(http.StatusOK, res.Status())
}

type DistroCostSuite struct {
	sc        *data.MockConnector
	data      data.MockDistroConnector
	starttime time.Time

	suite.Suite
}

func TestDistroCostSuite(t *testing.T) {
	suite.Run(t, new(DistroCostSuite))
}

func (s *DistroCostSuite) SetupSuite() {
	s.starttime = time.Now()

	testTask1 := task.Task{Id: "task1", DistroId: "distro1",
		TimeTaken: time.Millisecond, StartTime: s.starttime,
		FinishTime: s.starttime.Add(time.Millisecond)}
	testTask2 := task.Task{Id: "task2", DistroId: "distro2",
		TimeTaken: time.Millisecond, StartTime: s.starttime,
		FinishTime: s.starttime.Add(time.Millisecond)}
	testTask3 := task.Task{Id: "task3", DistroId: "distro2",
		TimeTaken: time.Millisecond, StartTime: s.starttime,
		FinishTime: s.starttime.Add(time.Millisecond)}

	var settings1 = make(map[string]interface{})
	var settings2 = make(map[string]interface{})
	settings1["instance_type"] = "type"
	testDistro1 := distro.Distro{
		Id:               "distro1",
		Provider:         "ec2-ondemand",
		ProviderSettings: &settings1,
	}
	testDistro2 := distro.Distro{
		Id:               "distro2",
		Provider:         "gce",
		ProviderSettings: &settings2,
	}

	s.data = data.MockDistroConnector{
		CachedTasks:   []task.Task{testTask1, testTask2, testTask3},
		CachedDistros: []*distro.Distro{&testDistro1, &testDistro2},
	}
	s.sc = &data.MockConnector{
		MockDistroConnector: s.data,
	}
}

// TestParseAndValidate tests the logic of ParseAndValidate for costByDistroHandler
// works correctly. When Mux is updated for Go 1.7, this test could be re-written
// to test the ParseAndValidate() function directly.
func TestParseAndValidate(t *testing.T) {
	req := &http.Request{Method: "GET"}
	req.URL, _ = url.Parse("http://evergreen.mongodb.com/rest/v2/cost/distro/distroid?starttime=2012-11-01T22:08:00%2B00:00&duration=4h")

	st := req.FormValue("starttime")
	if st != "2012-11-01T22:08:00+00:00" {
		t.Errorf(`req.FormValue("starttime") = %q, want "2012-11-01T22:08:00+00:00"`, st)
	}

	d := req.FormValue("duration")
	if d != "4h" {
		t.Errorf(`req.FormValue("duration") = %q, want "4h"`, d)
	}

	_, err := time.Parse(time.RFC3339, st)
	if err != nil {
		t.Errorf("Error in parsing start time: %s", err)
	}

	_, err = time.ParseDuration(d)
	if err != nil {
		t.Errorf("Error in parsing duration: %s", err)
	}
}

// TestFindCostByDistroIdSingle tests the handler where information is aggregated on
// a single task of a distro id
func (s *DistroCostSuite) TestFindCostByDistroIdSingle() {
	// Test that the handler executes properly
	handler := &costByDistroHandler{
		distroId:  "distro1",
		startTime: s.starttime,
		duration:  time.Millisecond,
		sc:        s.sc,
	}
	res := handler.Run(context.TODO())
	s.NotNil(res)
	s.Equal(http.StatusOK, res.Status())

	// Test that the handler returns the result with correct properties, i.e. that
	// it is the right type (model.APIDistroCost) and has correct distroId and SumTimeTaken
	h, ok := (res.Data()).(*model.APIDistroCost)
	s.True(ok)
	s.Equal(model.ToStringPtr("distro1"), h.DistroId)
	s.Equal(1*time.Millisecond, h.SumTimeTaken)
	s.Equal(model.ToStringPtr("ec2-ondemand"), h.Provider)
	s.Equal(model.ToStringPtr("type"), h.InstanceType)
}

// TestFindCostByDistroIdMany tests the handler where information is aggregated on
// multiple tasks of the same distro id
func (s *DistroCostSuite) TestFindCostByDistroIdMany() {
	// Test that the handler executes properly
	handler := &costByDistroHandler{
		distroId:  "distro2",
		startTime: s.starttime,
		duration:  time.Millisecond,
		sc:        s.sc,
	}
	res := handler.Run(context.TODO())
	s.NotNil(res)
	s.Equal(http.StatusOK, res.Status())

	// Test that the handler returns the result with correct properties, i.e. that
	// it is the right type (model.APIDistroCost) and has correct distroId and SumTimeTaken
	h, ok := (res.Data()).(*model.APIDistroCost)
	s.True(ok)
	s.Equal(model.ToStringPtr("distro2"), h.DistroId)
	s.Equal(2*time.Millisecond, h.SumTimeTaken)
	s.Equal(model.ToStringPtr("gce"), h.Provider)
	s.Nil(h.InstanceType)
}

// TestFindCostByDistroIdNoResult tests that the handler correct returns
// no information when a valid distroId contains no tasks of the given time range.
func (s *DistroCostSuite) TestFindCostByDistroIdNoResult() {
	handler := &costByDistroHandler{
		distroId:  "distro2",
		startTime: time.Now().AddDate(0, -1, 0),
		duration:  time.Millisecond,
		sc:        s.sc,
	}
	res := handler.Run(context.TODO())
	s.NotNil(res)
	s.Equal(http.StatusOK, res.Status())

	h, ok := (res.Data()).(*model.APIDistroCost)
	s.True(ok)
	s.Equal(model.ToStringPtr("distro2"), h.DistroId)
	s.Equal(time.Duration(0), h.SumTimeTaken)
	s.Equal(model.ToStringPtr(""), h.Provider)
	s.Nil(h.InstanceType)
}

// TestFindCostByDistroFail tests that the handler correctly returns error when
// incorrect query is passed in
func (s *DistroCostSuite) TestFindCostByDistroIdFail() {
	handler := &costByDistroHandler{
		distroId:  "fake_distro",
		startTime: s.starttime,
		duration:  1,
		sc:        s.sc,
	}

	res := handler.Run(context.TODO())
	s.NotEqual(http.StatusOK, res.Status())
}
