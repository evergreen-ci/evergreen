package queue

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type InternalSuite struct {
	driver  *driverInternal
	require *require.Assertions
	suite.Suite
}

func TestInternalSuite(t *testing.T) {
	suite.Run(t, new(InternalSuite))
}

func (s *InternalSuite) SetupSuite() {
	s.require = s.Require()
}

func (s *InternalSuite) SetupTest() {
	s.driver = NewInternalDriver().(*driverInternal)
}

func (s *InternalSuite) TestInternalImplementsDriverInterface() {
	s.Implements((*Driver)(nil), s.driver)
}

func (s *InternalSuite) TestInternalInitialValues() {
	stats := s.driver.Stats(context.TODO())
	s.Equal(0, stats.Completed)
	s.Equal(0, stats.Blocked)
	s.Equal(0, stats.Pending)
	s.Equal(0, stats.Total)

	s.Len(s.driver.jobs.m, 0)
	s.Len(s.driver.jobs.dispatched, 0)
}

func (s *InternalSuite) TestOpenShouldReturnNil() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.NoError(s.driver.Open(ctx))
}

func (s *InternalSuite) TestOpenShouldReturnNilOnSuccessiveCalls() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	before := s.driver.Stats(ctx)
	for i := 0; i < 200; i++ {
		s.NoError(s.driver.Open(ctx))
	}
	after := s.driver.Stats(ctx)

	s.Equal(before, after)
}

func (s *InternalSuite) TestCloseShouldBeANoop() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	before := s.driver.Stats(ctx)
	for i := 0; i < 200; i++ {
		s.driver.Close()
	}
	after := s.driver.Stats(ctx)

	s.Equal(before, after)
}
