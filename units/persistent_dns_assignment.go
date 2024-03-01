package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
)

const (
	persistentDNSAssignmentJobName   = "persistent-dns-assignment"
	persistentDNSAssignmentBatchSize = 10
)

func init() {
	registry.AddJobType(persistentDNSAssignmentJobName, func() amboy.Job {
		return makePersistentDNSAssignment()
	})
}

type persistentDNSAssignmentJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`

	env evergreen.Environment
}

func makePersistentDNSAssignment() *persistentDNSAssignmentJob {
	return &persistentDNSAssignmentJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    persistentDNSAssignmentJobName,
				Version: 0,
			},
		},
	}
}

func NewPersistentDNSAssignmentJob(ts string) amboy.Job {
	j := makePersistentDNSAssignment()
	j.SetID(fmt.Sprintf("%s.%s", persistentDNSAssignmentJobName, ts))
	j.SetScopes([]string{persistentDNSAssignmentJobName})
	return j
}

func (j *persistentDNSAssignmentJob) Run(ctx context.Context) {
	defer j.MarkComplete()
}
