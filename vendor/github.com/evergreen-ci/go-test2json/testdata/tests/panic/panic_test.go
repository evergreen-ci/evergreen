package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

func TestWillPanic(t *testing.T) {
	t.Parallel()
	t.Log("This test will panic in 10 seconds")
	time.Sleep(10 * time.Second)
	panic("in the disco")
}

type mySuite struct {
	suite.Suite
}

func (s *mySuite) TestWillPanic() {
	s.T().Log("This test will panic in 10 seconds")
	time.Sleep(10 * time.Second)
	panic("in the disco")
}

func TestTestifySuitePanic(t *testing.T) {
	t.Parallel()
	suite.Run(t, &mySuite{})
}
