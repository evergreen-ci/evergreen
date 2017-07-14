package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type DistroCostSuite struct {
	dc *task.DistroCost
	ps map[string]interface{}

	sumTimeTaken time.Duration
	distroId     string

	suite.Suite
}

func TestDistroCostSuite(t *testing.T) {
	suite.Run(t, new(DistroCostSuite))
}

func (s *DistroCostSuite) SetupSuite() {
	s.dc = &task.DistroCost{DistroId: s.distroId, SumTimeTaken: s.sumTimeTaken}
	s.ps = make(map[string]interface{})
}

// The following three TestDistroCostBuildFromService functions test
// the BuildFromService function.
func (s *DistroCostSuite) TestDistroCostBuildFromServiceSuccess1() {
	// Case 1: The DistroCost model has ec2 as provider,
	// and instance_type in providersettings
	s.dc.Provider = evergreen.ProviderNameEc2OnDemand
	s.ps["instance_type"] = "value"
	s.dc.ProviderSettings = s.ps

	apiDC := &APIDistroCost{}
	err := apiDC.BuildFromService(s.dc)
	s.NoError(err)
	s.Equal(apiDC.DistroId, APIString(s.distroId))
	s.Equal(apiDC.SumTimeTaken, s.sumTimeTaken)
	s.Equal(apiDC.Provider, APIString(evergreen.ProviderNameEc2OnDemand))
	s.Equal(apiDC.InstanceType, APIString("value"))
}

func (s *DistroCostSuite) TestDistroCostBuildFromServiceSuccess2() {
	// Case 2: The DistroCost model has ec2-spot as provider,
	// and instance_type in providersettings
	s.dc.Provider = evergreen.ProviderNameEc2Spot
	s.ps["instance_type"] = "value"
	s.dc.ProviderSettings = s.ps

	apiDC := &APIDistroCost{}
	err := apiDC.BuildFromService(s.dc)
	s.NoError(err)
	s.Equal(apiDC.DistroId, APIString(s.distroId))
	s.Equal(apiDC.SumTimeTaken, s.sumTimeTaken)
	s.Equal(apiDC.Provider, APIString(evergreen.ProviderNameEc2Spot))
	s.Equal(apiDC.InstanceType, APIString("value"))
}

func (s *DistroCostSuite) TestDistroCostBuildFromServiceSuccess3() {
	// Case 3: The DistroCost model has gce as provider,
	// and instance_type in providersettings
	s.dc.Provider = evergreen.ProviderNameGce
	s.ps["instance_type"] = "value" // ?
	s.dc.ProviderSettings = s.ps

	apiDC := &APIDistroCost{}
	err := apiDC.BuildFromService(s.dc)
	s.NoError(err)
	s.Equal(apiDC.DistroId, APIString(s.distroId))
	s.Equal(apiDC.SumTimeTaken, s.sumTimeTaken)
	s.Equal(apiDC.Provider, APIString(evergreen.ProviderNameGce))
	s.Equal(apiDC.InstanceType, APIString(""))
}

func (s *DistroCostSuite) TestDistroCostBuildFromServiceFail1() {
	// Case 1: The DistroCost model has ec2-spot as provider,
	// and an invalid value for instance_type in providersettings
	s.dc.Provider = evergreen.ProviderNameEc2Spot
	s.ps["instance_type"] = time.Now()
	s.dc.ProviderSettings = s.ps

	apiDC := &APIDistroCost{}
	err := apiDC.BuildFromService(s.dc)
	s.Error(err)
}

func (s *DistroCostSuite) TestDistroCostBuildFromServiceFail2() {
	// Case 2: The DistroCost model has ec2-spot as provider,
	// and missing instance_type in providersettings
	s.dc.Provider = evergreen.ProviderNameEc2Spot
	s.ps = make(map[string]interface{})
	s.dc.ProviderSettings = s.ps

	apiDC := &APIDistroCost{}
	err := apiDC.BuildFromService(s.dc)
	s.Error(err)
}

func (s *DistroCostSuite) TestDistroCostBuildFromServiceFail3() {
	// Case 3: The DistroCost model has ec2-spot as provider,
	// and an invalid value (empty string) for instance_type in providersettings
	s.dc.Provider = evergreen.ProviderNameEc2Spot
	s.ps["instance_type"] = ""
	s.dc.ProviderSettings = s.ps

	apiDC := &APIDistroCost{}
	err := apiDC.BuildFromService(s.dc)
	s.Error(err)
}

// TestDistroCostToService tests that ToService() correctly returns an error.
func TestDistroCostToService(t *testing.T) {
	assert := assert.New(t)
	apiDC := &APIDistroCost{}
	dc, err := apiDC.ToService()
	assert.Nil(dc)
	assert.Error(err)
}
