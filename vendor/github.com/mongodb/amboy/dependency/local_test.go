package dependency

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
)

// LocalFileSuite contains a group of tests for the
// LocalFileDependency implementation of the dependency.Manager
// interface, which provides make-style dependency checking.
type LocalFileSuite struct {
	dep    *LocalFile
	tmpDir string
	suite.Suite
}

func TestLocalFileSuite(t *testing.T) {
	suite.Run(t, new(LocalFileSuite))
}

func (s *LocalFileSuite) SetupTest() {
	s.dep = MakeLocalFile()
}

func (s *LocalFileSuite) SetupSuite() {
	tmpDir, err := ioutil.TempDir("", uuid.New().String())
	s.NoError(err)
	s.tmpDir = tmpDir
}

func (s *LocalFileSuite) TearDownSuite() {
	// s.NoError(os.RemoveAll(s.tmpDir))
}

func (s *LocalFileSuite) TestTypeImplementsManagerInterface() {
	s.Implements((*Manager)(nil), s.dep)
}

func (s *LocalFileSuite) TestDefaultConstructorStoresExpectedValues() {
	s.dep = NewLocalFile("foo", "bar", "baz")
	s.Implements((*Manager)(nil), s.dep)

	s.Equal("foo", s.dep.Targets[0])
	s.Len(s.dep.Targets, 1)

	s.Equal("bar", s.dep.Dependencies[0])
	s.Equal("baz", s.dep.Dependencies[1])
	s.Len(s.dep.Dependencies, 2)
}

func (s *LocalFileSuite) TestTaskEdgeTracking() {
	// edge defaults to empty
	s.Len(s.dep.Edges(), 0)

	s.NoError(s.dep.AddEdge("foo"))
	s.Len(s.dep.Edges(), 1)

	// make sure the internals look like we expect.
	s.Len(s.dep.edgesSet, 1)
	exists, ok := s.dep.edgesSet["foo"]
	s.True(exists)
	s.True(ok)
}

func (s *LocalFileSuite) TestLocalDependencyTestReportsExpectedType() {
	t := s.dep.Type()
	s.Equal(t.Name, "local-file")
	s.Equal(t.Version, 0)
}

func (s *LocalFileSuite) TestDependencyStateIsReadyWhenThereAreNoTargetsOrDependencies() {
	s.Len(s.dep.Targets, 0)
	s.Len(s.dep.Dependencies, 0)

	s.Equal(s.dep.State(), Ready)

	s.dep.Targets = append(s.dep.Targets, "foo")
	s.Equal(s.dep.State(), Ready)

	s.dep.Targets = []string{}
	s.dep.Dependencies = append(s.dep.Dependencies, "foo")
	s.Equal(s.dep.State(), Ready)
}

func (s *LocalFileSuite) TestDependencyStateIsReadyWhenTargetsAreOlderThanDependencies() {
	// first create
	for i := 0; i < 20; i++ {
		name := filepath.Join(s.tmpDir, fmt.Sprintf("target-%d", i))
		s.dep.Targets = append(s.dep.Targets, name)
		file, err := os.Create(name)
		s.NoError(err)
		s.NoError(file.Close())
		_, err = os.Stat(name)
		s.False(os.IsNotExist(err))
	}

	// now sleep for a bit so the deps are newer
	time.Sleep(1 * time.Second)

	// then set things up to create dependencies
	for i := 0; i < 20; i++ {
		name := filepath.Join(s.tmpDir, fmt.Sprintf("dep-%d", i))
		s.dep.Dependencies = append(s.dep.Dependencies, name)
		file, err := os.Create(name)
		s.NoError(err)
		s.NoError(file.Close())
		_, err = os.Stat(name)
		s.False(os.IsNotExist(err))
	}

	s.Equal(s.dep.State(), Ready)
}

func (s *LocalFileSuite) TestDependencyStateIsPassedWhenDependenciesAreOlderThanTargets() {
	// first set things up to create dependencies
	for i := 0; i < 20; i++ {
		name := filepath.Join(s.tmpDir, fmt.Sprintf("dep-%d", i))
		s.dep.Dependencies = append(s.dep.Dependencies, name)
		file, err := os.Create(name)
		s.NoError(err)
		s.NoError(file.Close())
		_, err = os.Stat(name)
		s.False(os.IsNotExist(err), name)
	}

	// now sleep for a bit so the targets are newer
	time.Sleep(1 * time.Second)

	// then create a bunch of targets
	for i := 0; i < 20; i++ {
		name := filepath.Join(s.tmpDir, fmt.Sprintf("target-%d", i))
		s.dep.Targets = append(s.dep.Targets, name)
		file, err := os.Create(name)
		s.NoError(err)
		s.NoError(file.Close())
		_, err = os.Stat(name)
		s.False(os.IsNotExist(err))
	}

	s.Equal(s.dep.State(), Passed)
}

func (s *LocalFileSuite) TestDependencyStateIsReadyWhenOneDependencyIsNewerThanTargets() {
	// first set things up to create dependencies
	for i := 0; i < 20; i++ {
		name := filepath.Join(s.tmpDir, fmt.Sprintf("dep-%d", i))
		s.dep.Dependencies = append(s.dep.Dependencies, name)
		file, err := os.Create(name)
		s.NoError(err)
		s.NoError(file.Close())
		_, err = os.Stat(name)
		s.False(os.IsNotExist(err), name)
	}

	// now sleep for a bit so the targets are newer
	time.Sleep(1 * time.Second)

	// then create a bunch of targets
	for i := 0; i < 20; i++ {
		name := filepath.Join(s.tmpDir, fmt.Sprintf("target-%d", i))
		s.dep.Targets = append(s.dep.Targets, name)
		file, err := os.Create(name)
		s.NoError(err)
		s.NoError(file.Close())
		_, err = os.Stat(name)
		s.False(os.IsNotExist(err))
	}

	// now sleep for a bit so the last dependency is newer
	time.Sleep(1 * time.Second)

	name := filepath.Join(s.tmpDir, "dep-newer")
	s.dep.Dependencies = append(s.dep.Dependencies, name)
	file, err := os.Create(name)
	s.NoError(err)
	s.NoError(file.Close())
	_, err = os.Stat(name)
	s.False(os.IsNotExist(err), name)

	s.Equal(s.dep.State(), Ready)
}
