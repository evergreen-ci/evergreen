package anser

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
)

type DependencyNetworkSuite struct {
	dep *dependencyNetwork
	suite.Suite
}

func TestDependencyNetworkSuite(t *testing.T) {
	suite.Run(t, new(DependencyNetworkSuite))
}

func (s *DependencyNetworkSuite) SetupSuite()    {}
func (s *DependencyNetworkSuite) TearDownSuite() {}
func (s *DependencyNetworkSuite) SetupTest() {
	var ok bool
	s.dep, ok = newDependencyNetwork().(*dependencyNetwork)
	s.Require().True(ok)
	s.Len(s.dep.network, 0)
	s.Len(s.dep.group, 0)
}
func (s *DependencyNetworkSuite) TearDownTest() {}

func (s *DependencyNetworkSuite) TestAddMethod() {
	s.dep.Add("foo", []string{"bar", "baz"})

	s.Len(s.dep.group, 0)
	s.Len(s.dep.network, 1)

	depSet, ok := s.dep.network["foo"]
	s.True(ok)

	s.Len(depSet, 2)
	count := 0
	for _, id := range []string{"bar", "baz"} {
		_, ok = depSet[id]
		s.True(ok)
		count++
	}

	s.Equal(len(depSet), count)

	// add an additional dependency
	s.dep.Add("foo", []string{"one", "two", "bar"})
	s.Len(s.dep.network, 1)
	depSet, ok = s.dep.network["foo"]
	s.True(ok)

	s.Len(depSet, 4)

	count = 0
	for _, id := range []string{"bar", "baz", "one", "two"} {
		_, ok = depSet[id]
		s.True(ok)
		count++
	}

	s.Equal(len(depSet), count)
}

func (s *DependencyNetworkSuite) TestDependencyNetworkResolution() {
	s.dep.Add("foo", []string{"one", "two", "three"})
	s.dep.Add("bar", []string{"one", "two", "three"})
	s.dep.Add("baz", []string{"three", "four", "five"})

	s.Len(s.dep.network, 3)

	for _, id := range []string{"foo", "bar", "baz"} {
		deps := s.dep.Resolve(id)
		s.Len(deps, 3)
	}

	foo := s.dep.Resolve("foo")
	s.Contains(foo, "one")
	s.Contains(foo, "two")
	s.Contains(foo, "three")

	bar := s.dep.Resolve("bar")
	s.Contains(bar, "one")
	s.Contains(bar, "two")
	s.Contains(bar, "three")

	baz := s.dep.Resolve("baz")
	s.Contains(baz, "three")
	s.Contains(baz, "four")
	s.Contains(baz, "five")

	s.Len(s.dep.Resolve("NULL"), 0)
	s.Len(s.dep.network, 3)

	all := s.dep.All()
	s.Len(all, 3)
	s.Contains(all, "foo")
	s.Contains(all, "bar")
	s.Contains(all, "baz")
}

func (s *DependencyNetworkSuite) TestNetworkResolution() {
	s.dep.Add("foo", []string{"one", "two", "three"})
	s.dep.Add("bar", []string{"one", "two", "three"})
	s.dep.Add("baz", []string{"three", "four", "five"})

	network := s.dep.Network()
	s.Len(network, 3)

	for _, id := range []string{"foo", "bar", "baz"} {
		edges, ok := network[id]
		s.True(ok)
		s.Len(edges, 3)
	}

	foo, ok := network["foo"]
	s.True(ok)
	s.Contains(foo, "one")
	s.Contains(foo, "two")
	s.Contains(foo, "three")

	bar, ok := network["bar"]
	s.True(ok)
	s.Contains(bar, "one")
	s.Contains(bar, "two")
	s.Contains(bar, "three")

	baz, ok := network["baz"]
	s.True(ok)
	s.Contains(baz, "three")
	s.Contains(baz, "four")
	s.Contains(baz, "five")
}

func (s *DependencyNetworkSuite) TestDependencyGroupResolution() {
	s.dep.AddGroup("foo", []string{"one", "two", "three"})
	s.dep.AddGroup("bar", []string{"one", "two", "three"})
	s.dep.AddGroup("baz", []string{"three", "four", "five"})

	s.Len(s.dep.group, 3)
	s.Len(s.dep.network, 0)

	for _, id := range []string{"foo", "bar", "baz"} {
		deps := s.dep.GetGroup(id)
		s.Len(deps, 3)
	}

	foo := s.dep.GetGroup("foo")
	s.Contains(foo, "one")
	s.Contains(foo, "two")
	s.Contains(foo, "three")

	bar := s.dep.GetGroup("bar")
	s.Contains(bar, "one")
	s.Contains(bar, "two")
	s.Contains(bar, "three")

	baz := s.dep.GetGroup("baz")
	s.Contains(baz, "three")
	s.Contains(baz, "four")
	s.Contains(baz, "five")

	s.Len(s.dep.GetGroup("NULL"), 0)
	s.Len(s.dep.group, 3)
}

func (s *DependencyNetworkSuite) TestOutputImplementations() {
	s.Implements((*fmt.Stringer)(nil), &dependencyNetwork{})
	s.Implements((*json.Marshaler)(nil), &dependencyNetwork{})

	s.dep.Add("bar", []string{"one", "two", "three"})

	s.True(len(s.dep.String()) > 1)
	jsonNet, err := s.dep.MarshalJSON()
	s.NoError(err)
	s.True(len(jsonNet) > 1)
}

func (s *DependencyNetworkSuite) TestValidationCyclesWithEmptyGraph() {
	s.NoError(s.dep.Validate())
}

func (s *DependencyNetworkSuite) TestNoErrorForAcylcicGraph() {
	s.dep.Add("foo", []string{"bar"})
	s.dep.Add("bar", []string{})

	s.NoError(s.dep.Validate())
}

func (s *DependencyNetworkSuite) TestErrorsForCycles() {
	s.dep.Add("foo", []string{"bar"})
	s.dep.Add("bar", []string{"baz"})
	s.dep.Add("baz", []string{"foo", "baz"})

	s.Error(s.dep.Validate())
}

func (s *DependencyNetworkSuite) TestErrorsForMissingDependency() {
	s.dep.Add("foo", []string{"bar"})
	s.dep.Add("bar", []string{"baz"})

	s.Error(s.dep.Validate())
}
