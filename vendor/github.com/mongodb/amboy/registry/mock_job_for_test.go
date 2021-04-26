package registry

// This file has a mock implementation of an amboy.Job. Used in other tests.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
)

func init() {
	AddJobType("test", jobTestFactory)
}

type JobTest struct {
	Name                 string              `bson:"name" json:"name" yaml:"name"`
	Content              string              `bson:"content" json:"content" yaml:"content"`
	ShouldFail           bool                `bson:"should_fail" json:"should_fail" yaml:"should_fail"`
	HadError             bool                `bson:"has_error" json:"has_error" yaml:"has_error"`
	IsLocked             bool                `bson:"is_locked" json:"is_locked" yaml:"is_locked"`
	JobPriority          int                 `bson:"priority" json:"priority" yaml:"priority"`
	T                    amboy.JobType       `bson:"type" json:"type" yaml:"type"`
	Stat                 amboy.JobStatusInfo `bson:"status" json:"status" yaml:"status"`
	TimingInfo           amboy.JobTimeInfo   `bson:"time_info" json:"time_info" yaml:"time_info"`
	LockScopes           []string            `bson:"scopes" json:"scopes" yaml:"scopes"`
	ApplyScopesOnEnqueue bool                `bson:"apply_scopes_on_enqueue" json:"apply_scopes_on_enqueue" yaml:"apply_scopes_on_enqueue"`

	Retry amboy.JobRetryInfo

	dep dependency.Manager
}

func NewTestJob(content string) *JobTest {
	id := fmt.Sprintf("%s-%s", content+"-job", content)

	return &JobTest{
		Name:    id,
		Content: content,
		dep:     dependency.NewAlways(),
		T: amboy.JobType{
			Name:    "test",
			Version: 0,
		},
	}
}

func jobTestFactory() amboy.Job {
	return &JobTest{
		T: amboy.JobType{
			Name:    "test",
			Version: 0,
		},
	}
}

func (j *JobTest) ID() string {
	return j.Name
}

func (j *JobTest) SetID(id string) {
	j.Name = id
}

func (j *JobTest) Run(_ context.Context) {
	j.Stat.Completed = true
}

func (j *JobTest) Error() error {
	if j.ShouldFail {
		return errors.New("poisoned task")
	}

	return nil
}

func (j *JobTest) Lock(id string, lockTimeout time.Duration) error {
	if j.IsLocked {
		return errors.New("Cannot lock locked job")
	}

	j.Stat.Owner = id
	j.Stat.ModificationCount++
	j.Stat.ModificationTime = time.Now()
	return nil
}

func (j *JobTest) Unlock(id string, lockTimeout time.Duration) {
	if !j.IsLocked {
		return
	}

	if j.Stat.Owner != id {
		return
	}
	j.IsLocked = false
	j.Stat.ModificationCount++
	j.Stat.ModificationTime = time.Now()
	j.Stat.Owner = ""
}

func (j *JobTest) AddError(err error) {
	if err != nil {
		j.HadError = true
	}
}

func (j *JobTest) AddRetryableError(err error) {
	if err == nil {
		return
	}
	j.HadError = true
	j.Retry.NeedsRetry = true
}

func (j *JobTest) Type() amboy.JobType {
	return j.T
}

func (j *JobTest) Dependency() dependency.Manager {
	return j.dep
}

func (j *JobTest) SetDependency(d dependency.Manager) {
	j.dep = d
}

func (j *JobTest) Priority() int {
	return j.JobPriority
}

func (j *JobTest) SetPriority(p int) {
	j.JobPriority = p
}

func (j *JobTest) Status() amboy.JobStatusInfo {
	return j.Stat
}

func (j *JobTest) SetStatus(s amboy.JobStatusInfo) {
	j.Stat = s
}

func (j *JobTest) TimeInfo() amboy.JobTimeInfo {
	return j.TimingInfo
}

func (j *JobTest) UpdateTimeInfo(i amboy.JobTimeInfo) {
	j.TimingInfo = i
}

func (j *JobTest) SetTimeInfo(i amboy.JobTimeInfo) {
	j.TimingInfo = i
}

func (j *JobTest) SetScopes(in []string) {
	j.LockScopes = in
}

func (j *JobTest) Scopes() []string {
	return j.LockScopes
}

func (j *JobTest) SetShouldApplyScopesOnEnqueue(val bool) {
	j.ApplyScopesOnEnqueue = val
}

func (j *JobTest) ShouldApplyScopesOnEnqueue() bool {
	return j.ApplyScopesOnEnqueue
}

func (j *JobTest) RetryInfo() amboy.JobRetryInfo {
	return j.Retry
}

func (j *JobTest) UpdateRetryInfo(opts amboy.JobRetryOptions) {
	if opts.Retryable != nil {
		j.Retry.Retryable = *opts.Retryable
	}
	if opts.NeedsRetry != nil {
		j.Retry.NeedsRetry = *opts.NeedsRetry
	}
	if opts.CurrentAttempt != nil {
		j.Retry.CurrentAttempt = *opts.CurrentAttempt
	}
	if opts.MaxAttempts != nil {
		j.Retry.MaxAttempts = *opts.MaxAttempts
	}
	if opts.DispatchBy != nil {
		j.Retry.DispatchBy = *opts.DispatchBy
	}
	if opts.WaitUntil != nil {
		j.Retry.WaitUntil = *opts.WaitUntil
	}
	if opts.Start != nil {
		j.Retry.Start = *opts.Start
	}
	if opts.End != nil {
		j.Retry.End = *opts.End
	}
}
