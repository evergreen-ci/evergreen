package registry

import (
	"testing"

	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/stretchr/testify/suite"
)

type AmboyDependencyRegistrySuite struct {
	registry *typeRegistry
	suite.Suite
}

func TestAmboyDependencyRegistrySuite(t *testing.T) {
	suite.Run(t, new(AmboyDependencyRegistrySuite))
}

func (s *AmboyDependencyRegistrySuite) SetupSuite() {
	s.NoError(grip.SetThreshold(level.Emergency))
}

func (s *AmboyDependencyRegistrySuite) SetupTest() {
	s.registry = newTypeRegistry()
}

func (s *AmboyDependencyRegistrySuite) TestRegisteringNewJobTypeSavedToRegistry() {
	s.Len(s.registry.dep.m, 0)
	s.registry.registerDependencyType("always", alwaysDependencyFactory)
	s.Len(s.registry.dep.m, 1)

	s.registry.registerDependencyType("always", alwaysDependencyFactory)
	s.Len(s.registry.dep.m, 1)

	s.registry.registerDependencyType("local-file", localFileDependencyFactory)
	s.Len(s.registry.dep.m, 2)

	s.registry.registerDependencyType("local-file", localFileDependencyFactory)
	s.Len(s.registry.dep.m, 2)
}

func (s *AmboyDependencyRegistrySuite) TestRegisteringKnownJobProducesFactory() {
	s.Len(s.registry.dep.m, 0)
	s.registry.registerDependencyType("always", alwaysDependencyFactory)
	s.Len(s.registry.dep.m, 1)

	var factory DependencyFactory
	var err error

	factory, err = s.registry.getDependencyFactory("always")
	s.NoError(err)

	dep := factory()

	s.IsType(dep, &dependency.Always{})

	factory, err = s.registry.getDependencyFactory("never")
	s.Nil(factory)
	s.Error(err)
}

func (s *AmboyDependencyRegistrySuite) TestRegistryFactoriesProduceTypesWithMatchingNames() {
	amboyRegistry.dep.l.RLock()
	defer amboyRegistry.dep.l.RUnlock()

	for name, factory := range amboyRegistry.dep.m {
		dep := factory()
		s.Equal(name, dep.Type().Name)
	}
}
