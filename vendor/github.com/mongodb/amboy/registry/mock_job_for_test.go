package registry

// This file has a mock implementation of a job. Used in other tests.

import (
	"context"
	"errors"
	"fmt"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
)

func init() {
	AddJobType("test", jobTestFactory)
}

type JobTest struct {
	Name       string `bson:"name" json:"name" yaml:"name"`
	Content    string `bson:"content" json:"content" yaml:"content"`
	ShouldFail bool   `bson:"should_fail" json:"should_fail" yaml:"should_fail"`
	HadError   bool   `bson:"has_error" json:"has_error" yaml:"has_error"`

	JobPriority int `bson:"priority" json:"priority" yaml:"priority"`

	T          amboy.JobType       `bson:"type" json:"type" yaml:"type"`
	Stat       amboy.JobStatusInfo `bson:"status" json:"status" yaml:"status"`
	TimingInfo amboy.JobTimeInfo   `bson:"time_info" json:"time_info" yaml:"time_info"`

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

func (j *JobTest) Run(_ context.Context) {
	j.Stat.Completed = true
}

func (j *JobTest) Error() error {
	if j.ShouldFail {
		return errors.New("poisoned task")
	}

	return nil
}

func (j *JobTest) AddError(err error) {
	if err != nil {
		j.HadError = true
	}
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
