package scheduler

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type SchedulerConnectorSuite struct {
	suite.Suite
	scheduler *Scheduler
}

func TestSchedulerSuite(t *testing.T) {
	s := new(SchedulerConnectorSuite)
	s.scheduler = &Scheduler{}

	suite.Run(t, s)
}
