package job

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type BaseCheckSuite struct {
	base    *Base
	require *require.Assertions
	suite.Suite
}

func TestBaseCheckSuite(t *testing.T) {
	suite.Run(t, new(BaseCheckSuite))
}

func (s *BaseCheckSuite) SetupSuite() {
	s.require = s.Require()
}

func (s *BaseCheckSuite) SetupTest() {
	s.base = &Base{dep: dependency.NewAlways()}
}

func (s *BaseCheckSuite) TestInitialValuesOfBaseObject() {
	s.False(s.base.status.Completed)
	s.Len(s.base.status.Errors, 0)
}

func (s *BaseCheckSuite) TestAddErrorWithNilObjectDoesNotChangeErrorState() {
	for i := 0; i < 100; i++ {
		s.base.AddError(nil)
		s.NoError(s.base.Error())
		s.Len(s.base.status.Errors, 0)
		s.False(s.base.HasErrors())
	}
}

func (s *BaseCheckSuite) TestAddErrorsPersistsErrorsInJob() {
	for i := 1; i <= 100; i++ {
		s.base.AddError(errors.New("foo"))
		s.Error(s.base.Error())
		s.Len(s.base.status.Errors, i)
		s.True(s.base.HasErrors())
		s.Len(strings.Split(s.base.Error().Error(), "\n"), i)
	}
}

func (s *BaseCheckSuite) TestIdIsAccessorForTaskIDAttribute() {
	s.Equal(s.base.TaskID, s.base.ID())
	s.base.TaskID = "foo"
	s.Equal("foo", s.base.ID())
	s.Equal(s.base.TaskID, s.base.ID())
}

func (s *BaseCheckSuite) TestDependencyAccessorIsCorrect() {
	s.Equal(s.base.dep, s.base.Dependency())
	s.base.SetDependency(dependency.NewAlways())
	s.Equal("always", s.base.Dependency().Type().Name)
}

func (s *BaseCheckSuite) TestSetDependencyAcceptsAndPersistsChangesToDependencyType() {
	s.Equal("always", s.base.dep.Type().Name)
	localDep := dependency.MakeLocalFile()
	s.NotEqual(localDep.Type().Name, "always")
	s.base.SetDependency(localDep)
	s.Equal("local-file", s.base.dep.Type().Name)
}

func (s *BaseCheckSuite) TestMarkCompleteHelperSetsCompleteState() {
	s.False(s.base.status.Completed)
	s.False(s.base.Status().Completed)
	s.base.MarkComplete()

	s.True(s.base.status.Completed)
}

func (s *BaseCheckSuite) TestDefaultTimeInfoIsUnset() {
	ti := s.base.TimeInfo()
	s.Zero(ti.Start)
	s.Zero(ti)
	s.Zero(ti.End)
	s.Zero(ti.WaitUntil)
}

func (s *BaseCheckSuite) TestTimeInfoSetsValues() {
	ti := s.base.TimeInfo()
	ti.Start = time.Now()
	ti.End = ti.Start.Add(time.Hour)
	s.Zero(ti.WaitUntil)
	s.Equal(time.Hour, ti.Duration())

	new := amboy.JobTimeInfo{}
	s.base.UpdateTimeInfo(ti)
	s.NotEqual(new, s.base.TimeInfo())
	s.Zero(s.base.TimeInfo().WaitUntil)

	new.End = ti.Start.Add(time.Minute)
	s.base.UpdateTimeInfo(new)
	result := s.base.TimeInfo()
	s.Equal(ti.Start, result.Start)
	s.Equal(time.Minute, result.Duration())
	s.Equal(new.End, result.End)
	s.NotEqual(ti.End, result.End)
	s.Zero(s.base.TimeInfo().WaitUntil)

	new = amboy.JobTimeInfo{WaitUntil: time.Now()}
	s.base.UpdateTimeInfo(new)
	last := s.base.TimeInfo()
	s.Equal(new.WaitUntil, last.WaitUntil)
	s.NotEqual(new.Start, last.Start)
	s.NotEqual(new.End, last.End)
	s.Equal(result.Start, last.Start)
	s.Equal(result.End, last.End)
}
