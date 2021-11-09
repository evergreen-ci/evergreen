package dependency

import (
	"fmt"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/suite"
)

func GetDirectoryOfFile() string {
	_, file, _, _ := runtime.Caller(1)

	return filepath.Dir(file)
}

// CreatesFileSuite tests the dependency.Manager
// implementation that checks for the existence of a file. If the file
// exist the dependency becomes a noop.
type CreatesFileSuite struct {
	dep      *createsFile
	packages []string
	suite.Suite
}

func TestCreatesFileSuite(t *testing.T) {
	suite.Run(t, new(CreatesFileSuite))
}

func (s *CreatesFileSuite) SetupSuite() {
	s.packages = []string{"job", "dependency", "queue", "pool", "build", "registry"}
}

func (s *CreatesFileSuite) SetupTest() {
	s.dep = makeCreatesFile()
}

func (s *CreatesFileSuite) TestInstanceImplementsManagerInterface() {
	s.Implements((*Manager)(nil), s.dep)
}

func (s *CreatesFileSuite) TestConstructorCreatesObjectWithFileNameSet() {
	for _, dir := range s.packages {
		dep, ok := NewCreatesFile(dir).(*createsFile)
		s.True(ok)
		s.Equal(dir, dep.FileName)
	}
}

func (s *CreatesFileSuite) TestDependencyWithoutFileSetReportsReady() {
	s.Equal(s.dep.FileName, "")
	s.Equal(s.dep.State(), Ready)

	s.dep.FileName = " \\[  ]"
	s.Equal(s.dep.State(), Ready)

	s.dep.FileName = "foo"
	s.Equal(s.dep.State(), Ready)

	s.dep.FileName = " "
	s.Equal(s.dep.State(), Ready)

	s.dep.FileName = ""
	s.Equal(s.dep.State(), Ready)
}

func (s *CreatesFileSuite) TestAmboyPackageDirectoriesExistAndReportPassedState() {
	cwd := filepath.Dir(GetDirectoryOfFile())

	for _, dir := range s.packages {
		p := filepath.Join(cwd, dir)
		dep := NewCreatesFile(p)
		s.Equal(dep.State(), Passed, fmt.Sprintln(dep.State(), p))
	}
}

func (s *CreatesFileSuite) TestCreatesDependencyTestReportsExpectedType() {
	t := s.dep.Type()
	s.Equal(t.Name, "create-file")
	s.Equal(t.Version, 0)
}
