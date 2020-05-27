package util

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/suite"
)

func TestBasicCatcherSuite(t *testing.T) {
	s := new(CatcherSuite)
	s.reset = func() Catcher { return NewCatcher() }
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

func (s *CatcherSuite) TestErrorsAndExtendMethods() {
	for i := 1; i <= 10; i++ {
		s.catcher.Add(errors.New(strconv.Itoa(i)))
		s.True(s.catcher.HasErrors())
	}

	errs := s.catcher.Errors()

	s.Equal(s.catcher.Len(), 10)
	s.Equal(s.catcher.Len(), len(errs))

	s.catcher.Extend(errs)
	s.Equal(s.catcher.Len(), 20)
}

func (s *CatcherSuite) TestExtendWithEmptySet() {
	s.Equal(s.catcher.Len(), 0)
	s.catcher.Extend(s.catcher.Errors())
	s.Equal(s.catcher.Len(), 0)
}

func (s *CatcherSuite) TestExtendWithNilErrors() {
	errs := []error{nil, errors.New("what"), nil}
	s.Len(errs, 3)
	s.catcher.Extend(errs)
	s.Equal(s.catcher.Len(), 1)

}

func (s *CatcherSuite) TestAddWhenNilError() {
	s.catcher.AddWhen(true, nil)
	s.Equal(s.catcher.Len(), 0)
	s.catcher.AddWhen(false, nil)
	s.Equal(s.catcher.Len(), 0)
}

func (s *CatcherSuite) TestAddWithErrorAndFalse() {
	s.catcher.AddWhen(false, errors.New("f"))
	s.Equal(s.catcher.Len(), 0)
	s.catcher.AddWhen(true, errors.New("f"))
	s.Equal(s.catcher.Len(), 1)
	s.catcher.AddWhen(false, errors.New("f"))
	s.Equal(s.catcher.Len(), 1)
}

func (s *CatcherSuite) TestExtendWhenNilError() {
	s.catcher.ExtendWhen(true, []error{})
	s.Equal(s.catcher.Len(), 0)
	s.catcher.ExtendWhen(false, []error{})
	s.Equal(s.catcher.Len(), 0)
	s.catcher.ExtendWhen(true, nil)
	s.Equal(s.catcher.Len(), 0)
	s.catcher.ExtendWhen(false, nil)
	s.Equal(s.catcher.Len(), 0)
}

func (s *CatcherSuite) TestExtendWithErrorAndFalse() {
	s.catcher.ExtendWhen(false, []error{errors.New("f")})
	s.Equal(s.catcher.Len(), 0)
	s.catcher.ExtendWhen(true, []error{errors.New("f")})
	s.Equal(s.catcher.Len(), 1)
	s.catcher.ExtendWhen(false, []error{errors.New("f")})
	s.Equal(s.catcher.Len(), 1)

	s.catcher.ExtendWhen(false, []error{errors.New("f"), errors.New("f")})
	s.Equal(s.catcher.Len(), 1)
	s.catcher.ExtendWhen(true, []error{errors.New("f"), errors.New("f")})
	s.Equal(s.catcher.Len(), 3)
}

func (s *CatcherSuite) TestNewEmpty() {
	s.catcher.New("")
	s.Equal(s.catcher.Len(), 0)
}

func (s *CatcherSuite) TestNewAdd() {
	s.catcher.New("one")
	s.Equal(s.catcher.Len(), 1)
	s.Contains(s.catcher.Errors()[0].Error(), "one")
}

func (s *CatcherSuite) TestWrapEmpty() {
	s.catcher.Wrap(nil, "foo")
	s.Equal(s.catcher.Len(), 0)
}

func (s *CatcherSuite) TestWrapFEmpty() {
	s.catcher.Wrapf(nil, "foo:%t", true)
	s.Equal(s.catcher.Len(), 0)
}

func (s *CatcherSuite) TestWrapPopulated() {
	s.catcher.Wrap(errors.New("foo"), "bar")
	s.Equal(s.catcher.Len(), 1)
	s.Contains(s.catcher.Errors()[0].Error(), "foo")
	s.Contains(s.catcher.Errors()[0].Error(), "bar")
}

func (s *CatcherSuite) TestWrapfPopulated() {
	s.catcher.Wrapf(errors.New("foo"), "bar: %s", "this")
	s.Equal(s.catcher.Len(), 1)
	s.Contains(s.catcher.Errors()[0].Error(), "foo")
	s.Contains(s.catcher.Errors()[0].Error(), "bar: this")
}

func (s *CatcherSuite) TestWrapPopulatedNillAnnotation() {
	s.catcher.Wrap(errors.New("foo"), "")
	s.Equal(s.catcher.Len(), 1)
	s.Contains(s.catcher.Errors()[0].Error(), "foo")
}

func (s *CatcherSuite) TestWrapfPopulatedNillAnnotation() {
	s.catcher.Wrapf(errors.New("foo"), "")
	s.Equal(s.catcher.Len(), 1)
	s.Contains(s.catcher.Errors()[0].Error(), "foo")
}

func (s *CatcherSuite) TestNewWhen() {
	s.catcher.NewWhen(false, "")
	s.Equal(s.catcher.Len(), 0)
	s.catcher.NewWhen(false, "one")
	s.Equal(s.catcher.Len(), 0)
	s.catcher.NewWhen(true, "one")
	s.Equal(s.catcher.Len(), 1)
	s.catcher.NewWhen(true, "")
	s.Equal(s.catcher.Len(), 1)
	s.catcher.NewWhen(false, "one")
	s.Equal(s.catcher.Len(), 1)
}

func (s *CatcherSuite) TestErrorfNilCases() {
	s.catcher.Errorf("")
	s.Equal(s.catcher.Len(), 0)
	s.catcher.Errorf("", true, false)
	s.Equal(s.catcher.Len(), 0)
}

func (s *CatcherSuite) TestErrorfWhenNilCases() {
	s.catcher.ErrorfWhen(true, "")
	s.Equal(s.catcher.Len(), 0)
	s.catcher.ErrorfWhen(true, "", true, false)
	s.Equal(s.catcher.Len(), 0)

	s.catcher.ErrorfWhen(false, "")
	s.Equal(s.catcher.Len(), 0)
	s.catcher.ErrorfWhen(false, "", true, false)
	s.Equal(s.catcher.Len(), 0)
}

func (s *CatcherSuite) TestErrorfNoArgs() {
	s.catcher.Errorf("%s what")
	s.Equal(s.catcher.Len(), 1)
	s.Contains(s.catcher.Errors()[0].Error(), "%s what")
}

func (s *CatcherSuite) TestErrorfWhenNoArgs() {
	s.catcher.ErrorfWhen(false, "%s what")
	s.Equal(s.catcher.Len(), 0)

	s.catcher.ErrorfWhen(true, "%s what")
	s.Equal(s.catcher.Len(), 1)
	s.Contains(s.catcher.Errors()[0].Error(), "%s what")
}

func (s *CatcherSuite) TestErrorfFull() {
	s.catcher.Errorf("%s what", "this")
	s.Equal(s.catcher.Len(), 1)
	s.Contains(s.catcher.Errors()[0].Error(), "this what")
}

func (s *CatcherSuite) TestWhenErrorfFull() {
	s.catcher.ErrorfWhen(false, "%s what", "this")
	s.Equal(s.catcher.Len(), 0)

	s.catcher.ErrorfWhen(true, "%s what", "this")
	s.Equal(s.catcher.Len(), 1)
	s.Contains(s.catcher.Errors()[0].Error(), "this what")
}

func (s *CatcherSuite) TestCheckWhenError() {
	fn := func() error { return errors.New("hi") }
	s.Error(fn())
	s.catcher.CheckWhen(false, fn)
	s.Equal(s.catcher.Len(), 0)
	s.catcher.CheckWhen(true, fn)
	s.Equal(s.catcher.Len(), 1)
	s.catcher.Check(fn)
	s.Equal(s.catcher.Len(), 2)
}

func (s *CatcherSuite) TestCheckWhenNoError() {
	fn := func() error { return nil }
	s.NoError(fn())
	s.catcher.CheckWhen(false, fn)
	s.Equal(s.catcher.Len(), 0)
	s.catcher.CheckWhen(true, fn)
	s.Equal(s.catcher.Len(), 0)
	s.catcher.Check(fn)
	s.Equal(s.catcher.Len(), 0)
}
