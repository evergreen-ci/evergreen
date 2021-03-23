package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type fakeRetryableJob struct {
	job.Base `bson:"base" json:"bas" yaml:"base"`
}

func init() {
	registry.AddJobType("fake-retryable", func() amboy.Job {
		return makeFakeRetryableJob()
	})
}

func makeFakeRetryableJob() *fakeRetryableJob {
	j := &fakeRetryableJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    "fake-retryable",
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

func NewFakeRetryableJob(scope string, ts time.Time) amboy.Job {
	j := makeFakeRetryableJob()
	j.SetID(fmt.Sprintf("%s.%s", "fake-retryable", ts.Format(TSFormat)))
	j.SetScopes([]string{scope})
	j.SetShouldApplyScopesOnEnqueue(true)
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(5),
		WaitUntil:   utility.ToTimeDurationPtr(30 * time.Second),
	})
	return j
}

func (j *fakeRetryableJob) Run(ctx context.Context) {
	grip.Info(message.Fields{
		"message":    "kim: running fake retryable job",
		"job_id":     j.ID(),
		"retry_info": j.RetryInfo(),
	})
	j.AddRetryableError(errors.Errorf("retryable error #%d", j.RetryInfo().CurrentAttempt))
}
