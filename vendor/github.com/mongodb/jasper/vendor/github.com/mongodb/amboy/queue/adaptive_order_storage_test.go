package queue

import (
	"context"
	"testing"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/stretchr/testify/suite"
)

type AdaptiveOrderItemsSuite struct {
	items *adaptiveOrderItems
	suite.Suite
}

func TestAdaptiveOrderItemsSuite(t *testing.T) {
	suite.Run(t, new(AdaptiveOrderItemsSuite))
}

func (s *AdaptiveOrderItemsSuite) SetupTest() {
	s.items = &adaptiveOrderItems{
		jobs: map[string]amboy.Job{},
	}
	s.Len(s.items.jobs, 0)
	s.Len(s.items.ready, 0)
	s.Len(s.items.completed, 0)
	s.Len(s.items.waiting, 0)
	s.Len(s.items.stalled, 0)
}

func (s *AdaptiveOrderItemsSuite) TestAddMethodPutsJobsInReadyQueue() {
	j := job.NewShellJob("echo true", "")
	s.NoError(s.items.add(j))
	s.Len(s.items.jobs, 1)
	s.Len(s.items.ready, 1)
}

func (s *AdaptiveOrderItemsSuite) TestAddMethodErrorsForDuplicateJobs() {
	j := job.NewShellJob("echo true", "")
	s.NoError(s.items.add(j))
	s.Error(s.items.add(j))
	s.Len(s.items.jobs, 1)
	s.Len(s.items.ready, 1)
}

func (s *AdaptiveOrderItemsSuite) TestAddMethodStoresCompletedJobs() {
	j := job.NewShellJob("echo false", "")
	j.Run(context.Background())
	s.True(j.Status().Completed)
	s.NoError(s.items.add(j))
	s.Len(s.items.jobs, 1)
	s.Len(s.items.ready, 0)
	s.Len(s.items.completed, 1)
}

func (s *AdaptiveOrderItemsSuite) TestAddMethodPutsJobInCorrectQueue() {
	j := job.NewShellJob("echo false", "makefile")

	s.NoError(s.items.add(j))
	s.Len(s.items.jobs, 1)
	s.Len(s.items.ready, 0)
	s.Len(s.items.passed, 1)

	j2 := job.NewShellJob("echo foo", "")
	blockedDep := dependency.NewMock()
	blockedDep.Response = dependency.Blocked
	j2.SetDependency(blockedDep)
	s.Len(s.items.waiting, 0)
	s.NoError(s.items.add(j2))
	s.Len(s.items.jobs, 2)
	s.Len(s.items.ready, 0)
	s.Len(s.items.passed, 1)
	s.Len(s.items.waiting, 1)

	j3 := job.NewShellJob("echo wat", "")
	unresolvedDep := dependency.NewMock()
	unresolvedDep.Response = dependency.Unresolved
	j3.SetDependency(unresolvedDep)
	s.Len(s.items.stalled, 0)
	s.NoError(s.items.add(j3))
	s.Len(s.items.jobs, 3)
	s.Len(s.items.ready, 0)
	s.Len(s.items.passed, 1)
	s.Len(s.items.waiting, 1)
	s.Len(s.items.stalled, 1)
}

func (s *AdaptiveOrderItemsSuite) TestRefilterRemovesNonExistantTests() {
	ctx := context.Background()
	s.items.waiting = []string{"foo"}
	s.items.stalled = []string{"foo"}
	s.items.refilter(ctx)
	s.Len(s.items.waiting, 0)
	s.Len(s.items.stalled, 0)
}

func (s *AdaptiveOrderItemsSuite) TestRefilterIgnoresWorkWithCanceledContext() {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.items.waiting = []string{"foo"}
	s.items.refilter(ctx)
	s.Len(s.items.waiting, 1)

	s.items.waiting = []string{}
	s.items.stalled = []string{"foo"}
	s.items.refilter(ctx)
	s.Len(s.items.stalled, 1)
}

func (s *AdaptiveOrderItemsSuite) TestRefilterReoRdersItemsInSuite() {
	ctx := context.Background()
	originalOrder := []string{"foo", "bar", "buzz", "what", "foo"}
	s.items.ready = []string{"foo", "bar", "buzz", "what", "foo"}
	s.items.refilter(ctx)
	s.NotEqual(originalOrder, s.items.ready)
}

func (s *AdaptiveOrderItemsSuite) TestReadyTasksOnOtherQueuesMoved() {
	ctx := context.Background()
	j := job.NewShellJob("echo one", "")
	jj := job.NewShellJob("echo two", "")

	s.items.jobs[j.ID()] = j
	s.items.jobs[jj.ID()] = jj
	s.Len(s.items.ready, 0)
	s.items.stalled = []string{j.ID()}
	s.items.waiting = []string{jj.ID()}
	s.items.refilter(ctx)
	s.Len(s.items.ready, 2)
}

func (s *AdaptiveOrderItemsSuite) TestsBlockedMovedToWaiting() {
	ctx := context.Background()
	j := job.NewShellJob("echo one", "")
	jj := job.NewShellJob("echo two", "")

	unresolvedDep := dependency.NewMock()
	unresolvedDep.Response = dependency.Blocked
	j.SetDependency(unresolvedDep)
	jj.SetDependency(unresolvedDep)

	s.items.jobs[j.ID()] = j
	s.items.jobs[jj.ID()] = jj
	s.items.stalled = []string{j.ID()}
	s.items.waiting = []string{jj.ID()}
	s.items.refilter(ctx)
	s.Len(s.items.waiting, 2)
	s.Len(s.items.ready, 0)
	s.Len(s.items.stalled, 0)
}

func (s *AdaptiveOrderItemsSuite) TestUnresolvedMovedToStalled() {
	ctx := context.Background()
	j := job.NewShellJob("echo one", "")
	jj := job.NewShellJob("echo two", "")

	unresolvedDep := dependency.NewMock()
	unresolvedDep.Response = dependency.Unresolved
	j.SetDependency(unresolvedDep)
	jj.SetDependency(unresolvedDep)

	s.items.jobs[j.ID()] = j
	s.items.jobs[jj.ID()] = jj
	s.items.stalled = []string{j.ID()}
	s.items.waiting = []string{jj.ID()}
	s.items.refilter(ctx)
	s.Len(s.items.waiting, 0)
	s.Len(s.items.ready, 0)
	s.Len(s.items.stalled, 2)
}

func (s *AdaptiveOrderItemsSuite) TestCompletedJobsAreFiltered() {
	ctx := context.Background()
	j := job.NewShellJob("echo one", "")
	jj := job.NewShellJob("echo two", "")
	j.MarkComplete()
	jj.SetStatus(amboy.JobStatusInfo{InProgress: true})

	s.items.jobs[j.ID()] = j
	s.items.jobs[jj.ID()] = jj
	s.items.stalled = []string{j.ID()}
	s.items.waiting = []string{jj.ID()}

	s.items.refilter(ctx)

	s.Len(s.items.waiting, 0)
	s.Len(s.items.ready, 0)
	s.Len(s.items.stalled, 0)
	s.Len(s.items.completed, 2)
}
