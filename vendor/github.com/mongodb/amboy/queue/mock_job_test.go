package queue

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
)

var mockJobCounters *mockJobRunEnv

type mockJobRunEnv struct {
	runCount int
	mu       sync.Mutex
}

func (e *mockJobRunEnv) Inc() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.runCount++
}

func (e *mockJobRunEnv) Count() int {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.runCount
}

func (e *mockJobRunEnv) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.runCount = 0
}

func init() {
	mockJobCounters = &mockJobRunEnv{}
	registry.AddJobType("mock", func() amboy.Job { return newMockJob() })
	registry.AddJobType("sleep", func() amboy.Job { return newSleepJob() })
}

//
type mockJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
}

func newMockJob() *mockJob {
	j := &mockJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    "mock",
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

func (j *mockJob) Run(_ context.Context) {
	defer j.MarkComplete()

	mockJobCounters.Inc()
}

type sleepJob struct {
	Sleep time.Duration
	job.Base
}

func newSleepJob() *sleepJob {
	j := &sleepJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    "sleep",
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	j.SetID(uuid.New().String())
	return j
}

func (j *sleepJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.Sleep == 0 {
		return
	}

	timer := time.NewTimer(j.Sleep)
	defer timer.Stop()

	select {
	case <-timer.C:
		return
	case <-ctx.Done():
		return
	}
}
