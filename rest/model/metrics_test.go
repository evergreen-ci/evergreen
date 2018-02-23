package model

import (
	"testing"

	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/suite"
)

type MetricsSuite struct {
	sysInfo *APISystemMetrics
	procs   APIProcessMetrics

	procData []*message.ProcessInfo
	suite.Suite
}

func TestMetricsSuite(t *testing.T) {
	suite.Run(t, new(MetricsSuite))
}

func (s *MetricsSuite) SetupTest() {
	s.sysInfo = &APISystemMetrics{}
	s.procs = APIProcessMetrics{}
}

func getCurrentProcStats() []*message.ProcessInfo {
	out := []*message.ProcessInfo{}

	for _, msg := range message.CollectProcessInfoSelfWithChildren() {
		proc := msg.(*message.ProcessInfo)
		out = append(out, proc)
	}

	return out
}

func (s *MetricsSuite) SetupSuite() {
	s.procData = getCurrentProcStats()
	s.NotEmpty(s.procData)
}

func (s *MetricsSuite) TestInvalidTypesProduceErrors() {
	for _, invalid := range []interface{}{1, true, nil, "string", 42.01, []string{"one"}, []bool{}} {
		s.Error(s.sysInfo.BuildFromService(invalid))
		s.Error(s.procs.BuildFromService(invalid))
	}
}

func (s *MetricsSuite) TestWriteMethidsAreNotImplemented() {
	out, err := s.sysInfo.ToService()
	s.Nil(out)
	s.Error(err)
	s.EqualError(err, "the api for metrics data is read only")

	out, err = s.procs.ToService()
	s.Nil(out)
	s.Error(err)
	s.EqualError(err, "the api for metrics data is read only")
}

func (s *MetricsSuite) TestBuildingFromServiceAffectsStateOfModel() {
	s.NoError(s.sysInfo.BuildFromService(message.CollectSystemInfo()))
	s.NotEqual(s.sysInfo, &APISystemMetrics{})

	s.Empty(s.procs)
	s.NoError(s.procs.BuildFromService(s.procData))
	s.Equal(len([]APIProcessStat(s.procs)), len(s.procData))
}

func (s *MetricsSuite) TestProcConverterOverwritesExistingData() {
	// first approach uses the existing data but changes the length.
	s.NoError(s.procs.BuildFromService(s.procData))
	s.Equal(len([]APIProcessStat(s.procs)), len(s.procData))
	s.NoError(s.procs.BuildFromService(s.procData[1:]))
	s.NotEqual(len([]APIProcessStat(s.procs)), len(s.procData))
	s.Equal(len([]APIProcessStat(s.procs)), len(s.procData)-1)

	// second approach generates new data and makes sure it overwrites
	existing := s.procs
	newProcs := getCurrentProcStats()
	s.NoError(s.procs.BuildFromService(s.procData))
	s.NotEqual(newProcs, s.procData)
	s.NoError(s.procs.BuildFromService(newProcs))
	s.NotEqual(s.procs, existing)
}

func (s *MetricsSuite) TestSysInfoConverterOverwritesExistingData() {
	first := message.CollectSystemInfo()
	second := message.CollectSystemInfo()

	s.NotEqual(first, second)

	s.NoError(s.sysInfo.BuildFromService(first))
	firstM := *s.sysInfo
	s.NoError(s.sysInfo.BuildFromService(second))
	s.NotEqual(firstM, s.sysInfo)
}

func (s *MetricsSuite) TestProcessInfoConversions() {
	validInputs := []interface{}{
		getCurrentProcStats(),
		message.CollectProcessInfoSelfWithChildren(),
		&message.ProcessInfo{},
		[]*message.ProcessInfo{},
		message.CollectProcessInfoSelf(),
	}

	for _, valid := range validInputs {
		s.NoError(s.procs.BuildFromService(valid), "for type %T", valid)
	}

	invalidInputCases := []interface{}{
		[]message.Composer{
			message.CollectSystemInfo(),
			message.CollectSystemInfo(),
			message.CollectSystemInfo(),
		},
		message.NewLine("hi"),
	}

	for _, invalid := range invalidInputCases {
		s.Error(s.procs.BuildFromService(invalid))
	}
}
