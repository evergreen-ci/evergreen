package job

import (
	"context"
	"strings"
	"testing"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/registry"
	"github.com/stretchr/testify/suite"
)

func init() {
	RegisterDefaultJobs()
}

// JobGroupSuite exercises the Job implementation that allows you to
// run multiple tasks in a worker pool as part of a single isolated
// task. This is good exercise for the JobInterchange code and
// requires some type fidelity of the interchange system.
type JobGroupSuite struct {
	job *Group
	suite.Suite
}

func TestJobGroupSuite(t *testing.T) {
	// t.Skip("problems with interchange conversion in the context of group jobs")
	suite.Run(t, new(JobGroupSuite))
}

func (s *JobGroupSuite) SetupTest() {
	s.job = NewGroup("group")
}

func (s *JobGroupSuite) TestJobFactoryAndConstructorHaveIdenticalTypeInformation() {
	fj, err := registry.GetJobFactory("group")
	s.NoError(err)

	j := fj()
	s.Equal(s.job.Type().Name, j.Type().Name)
	s.Equal(s.job.Type().Version, j.Type().Version)

	s.IsType(s.job, j)
}

func (s *JobGroupSuite) TestGroupAddMethodRequiresUniqueNames() {
	job := NewShellJob("touch foo", "foo")

	s.NoError(s.job.Add(job))
	s.Error(s.job.Add(job))

	job = NewShellJob("touch bar", "bar")
	s.NoError(s.job.Add(job))

	s.Len(s.job.Jobs, 2)
}

func (s *JobGroupSuite) TestAllJobsAreCompleteAfterRunningGroup() {
	names := []string{"a", "b", "c", "d", "e", "f"}
	for _, name := range names {
		s.NoError(s.job.Add(NewShellJob("echo "+name, "")))
	}
	s.Len(s.job.Jobs, len(names))

	s.job.Run(context.Background())
	s.True(s.job.Status().Completed)
	s.NoError(s.job.Error())

	for _, interchange := range s.job.Jobs {
		s.True(interchange.Status.Completed)

		job, err := interchange.Resolve(amboy.JSON)
		s.NoError(err)
		s.True(job.Status().Completed)
		s.IsType(&ShellJob{}, job)
	}
}

func (s *JobGroupSuite) TestJobResultsPersistAfterGroupRuns() {
	// this tests runs two jobs, one that will fail and produce
	// errors, and one that runs normally, and we want to be able
	// to see that we can retrieve the results reasonably.

	s.NoError(s.job.Add(NewShellJob("true", "")))
	fail := NewShellJob("false", "")
	fail.Env["name"] = "fail"

	s.NoError(s.job.Add(fail))
	s.Len(s.job.Jobs, 2)

	s.job.Run(context.Background())
	s.True(s.job.Status().Completed)
	s.Error(s.job.Error())

	s.False(fail.Status().Completed)
	failEnvName, ok := fail.Env["name"]
	s.True(ok)
	s.Equal("fail", failEnvName)

	interchange, exists := s.job.Jobs[fail.ID()]
	s.True(exists)

	job, err := interchange.Resolve(amboy.JSON)
	s.NoError(err)
	s.False(job.Status().Completed)
	s.IsType(&ShellJob{}, job)
}

func (s *JobGroupSuite) TestJobIdReturnsUniqueString() {
	name := "foo"
	for i := 0; i < 20; i++ {
		job := NewGroup(name)

		id := job.ID()
		s.True(strings.HasPrefix(id, name), id)
	}
}

func (s *JobGroupSuite) TestJobGroupReturnsAlwaysDependency() {
	s.Equal(s.job.Dependency().Type().Name, "always")
	s.Equal(s.job.Dependency(), dependency.NewAlways())
}

func (s *JobGroupSuite) TestJobGroupSetIsANoOp() {
	s.Equal(s.job.Dependency().Type().Name, "always")
	s.job.SetDependency(dependency.MakeLocalFile())
	s.Equal(s.job.Dependency().Type().Name, "always")
}
