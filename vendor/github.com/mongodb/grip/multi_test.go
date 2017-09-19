package grip

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/suite"
)

func TestDeprecatedLegacyCatcherSuite(t *testing.T) {
	s := new(CatcherSuite)
	s.reset = func() Catcher { return NewCatcher() }
	suite.Run(t, s)
}

func TestExtendedCatcherSuite(t *testing.T) {
	s := new(CatcherSuite)
	s.reset = func() Catcher { return NewExtendedCatcher() }
	suite.Run(t, s)
}

func TestBasicCatcherSuite(t *testing.T) {
	s := new(CatcherSuite)
	s.reset = func() Catcher { return NewBasicCatcher() }
	suite.Run(t, s)
}

func TestSimpleCatcherSuite(t *testing.T) {
	s := new(CatcherSuite)
	s.reset = func() Catcher { return NewSimpleCatcher() }
	suite.Run(t, s)
}

// CatcherSuite provides
type CatcherSuite struct {
	catcher Catcher
	reset   func() Catcher
	suite.Suite
}

func (s *CatcherSuite) SetupTest() {
	s.catcher = s.reset()
}

func (s *CatcherSuite) TestInitialValuesOfCatcherInterface() {
	s.False(s.catcher.HasErrors())
	s.Equal(0, s.catcher.Len())
	s.Equal("", s.catcher.String())
}

func (s *CatcherSuite) TestAddMethodImpactsState() {
	err := errors.New("foo")

	s.False(s.catcher.HasErrors())
	s.Equal(0, s.catcher.Len())

	s.catcher.Add(err)

	s.True(s.catcher.HasErrors())
	s.Equal(1, s.catcher.Len())
}

func (s *CatcherSuite) TestAddingNilMethodDoesNotImpactCatcherState() {
	s.False(s.catcher.HasErrors())
	s.Equal(0, s.catcher.Len())

	for i := 0; i < 100; i++ {
		s.catcher.Add(nil)
	}

	s.False(s.catcher.HasErrors())
	s.Equal(0, s.catcher.Len())
}

func (s *CatcherSuite) TestAddingManyErrorsIsCaptured() {
	s.False(s.catcher.HasErrors())
	s.Equal(0, s.catcher.Len())

	for i := 1; i <= 100; i++ {
		s.catcher.Add(errors.New(strconv.Itoa(i)))
		s.True(s.catcher.HasErrors())
		s.Equal(i, s.catcher.Len())
	}

	s.True(s.catcher.HasErrors())
	s.Equal(100, s.catcher.Len())
}

func (s *CatcherSuite) TestResolveMethodIsNilIfNotHasErrors() {
	s.False(s.catcher.HasErrors())
	s.Equal(0, s.catcher.Len())

	s.NoError(s.catcher.Resolve())

	for i := 0; i < 100; i++ {
		s.catcher.Add(nil)
		s.NoError(s.catcher.Resolve())
	}

	s.False(s.catcher.HasErrors())
	s.Equal(0, s.catcher.Len())
}

func (s *CatcherSuite) TestResolveMethodDoesNotClearStateOfCatcher() {
	s.False(s.catcher.HasErrors())
	s.Equal(0, s.catcher.Len())

	for i := 1; i <= 10; i++ {
		s.catcher.Add(errors.New(strconv.Itoa(i)))
		s.True(s.catcher.HasErrors())
	}
	s.Equal(10, s.catcher.Len())

	s.Error(s.catcher.Resolve())

	s.True(s.catcher.HasErrors())
	s.Equal(10, s.catcher.Len())
}

func (s *CatcherSuite) TestConcurrentAddingOfErrors() {
	wg := &sync.WaitGroup{}
	s.Equal(s.catcher.Len(), 0)
	for i := 0; i < 256; i++ {
		wg.Add(1)
		func(num int) {
			s.catcher.Add(fmt.Errorf("adding err #%d", num))
			wg.Done()
		}(i)
	}
	wg.Wait()
	s.Equal(s.catcher.Len(), 256)
}
