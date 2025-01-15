package units

import (
	"context"
	"fmt"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
)

const (
	parameterStoreSyncJobName = "parameter-store-sync"
)

func init() {
	registry.AddJobType(parameterStoreSyncJobName, func() amboy.Job { return makeParameterStoreSyncJob() })
}

type parameterStoreSyncJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
}

func makeParameterStoreSyncJob() *parameterStoreSyncJob {
	j := &parameterStoreSyncJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    parameterStoreSyncJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewParameterStoreSyncJob creates a job that syncs project variables to SSM
// Parameter Store for any branch project or repo ref that has Parameter Store
// enabled but whose vars are not already in sync.
// TODO (DEVPROD-11882): remove this job once the rollout is stable.
func NewParameterStoreSyncJob(ts string) amboy.Job {
	j := makeParameterStoreSyncJob()
	j.SetID(fmt.Sprintf("%s.%s", parameterStoreSyncJobName, ts))
	j.SetScopes([]string{parameterStoreSyncJobName})
	j.SetEnqueueAllScopes(true)
	return j
}

func (j *parameterStoreSyncJob) Run(ctx context.Context) {
	defer j.MarkComplete()
}
