package registry

import (
	"fmt"
	"sync"
	"testing"

	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/stretchr/testify/suite"
)

// AmboyJobRegustrySuite tests the amboy job registry resource, which is
// internal to the amboy package, but has an isolated interface for
// client code that writes amboy jobs to support serialization and
// de-serialization of jobs.
type AmboyJobRegustrySuite struct {
	registry *typeRegistry
	suite.Suite
}

func TestAmboyJobRegistryResources(t *testing.T) {
	suite.Run(t, new(AmboyJobRegustrySuite))
}

func (s *AmboyJobRegustrySuite) SetupSuite() {
	lvl := grip.GetSender().Level()
	lvl.Threshold = level.Emergency
	s.NoError(grip.GetSender().SetLevel(lvl))
}

func (s *AmboyJobRegustrySuite) SetupTest() {
	s.registry = newTypeRegistry()
	s.Len(s.registry.job.m, 0)
}

func exampleJobFactory() amboy.Job {
	return (amboy.Job)(nil)
}

func groupJobFactory() amboy.Job {
	return (amboy.Job)(nil)
}

func (s *AmboyJobRegustrySuite) TestRegisterNewJobTypePersists() {
	s.registry.registerJobType("group_one", exampleJobFactory)
	s.Len(s.registry.job.m, 1)

	_, err := s.registry.getJobFactory("group_one")
	s.NoError(err)

	_, err = s.registry.getJobFactory("group")
	s.Error(err)

	s.Len(s.registry.job.m, 1)
}

func (s *AmboyJobRegustrySuite) TestJobsHaveUniqueNames() {
	s.registry.registerJobType("group", groupJobFactory)
	s.registry.registerJobType("group", groupJobFactory)
	s.registry.registerJobType("group", groupJobFactory)
	s.Len(s.registry.job.m, 1)
}

func (s *AmboyJobRegustrySuite) TestConcurrentAccess() {
	// this is a little simulation to test a moderate number
	// threads doing read and write access on a registry object.
	wg := &sync.WaitGroup{}

	num := 128
	for i := 0; i < num; i++ {
		wg.Add(1)
		go func(n int) {
			name := fmt.Sprintf("worker-%d", n)

			for f := 1; f < 12; f++ {
				s.registry.registerJobType(name, groupJobFactory)
				s.registry.registerJobType(name, groupJobFactory)
			}

			var err error

			for l := 1; l < 12; l++ {
				_, err = s.registry.getJobFactory(name)
				s.NoError(err)
				_, err = s.registry.getJobFactory(fmt.Sprintf("%s-%d-%d", name, n, l))
				s.Error(err)
				// throw in another option to contend on the lock
				s.registry.registerJobType(name, groupJobFactory)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()

	s.registry.job.l.RLock()
	defer s.registry.job.l.RUnlock()

	s.Len(s.registry.job.m, num,
		fmt.Sprintf("%d jobs in registry, %d expected", len(s.registry.job.m), num))
}

func (s *AmboyJobRegustrySuite) TestRegistryFactoriesProduceTypesWithMatchingNames() {
	amboyRegistry.job.l.RLock()
	defer amboyRegistry.job.l.RUnlock()

	for name, factory := range amboyRegistry.job.m {
		job := factory()
		s.Equal(name, job.Type().Name)
	}
}
