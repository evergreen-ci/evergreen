package units

import (
	"context"
	"fmt"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
)

const distroAutoTuneJobName = "distro-auto-tune"

func init() {
	registry.AddJobType(distroAutoTuneJobName, func() amboy.Job {
		return makeDistroAutoTuneJob()
	})
}

type distroAutoTuneJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	DistroID string `bson:"distro_id" json:"distro_id" yaml:"distro_id"`
}

func makeDistroAutoTuneJob() *distroAutoTuneJob {
	j := &distroAutoTuneJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    distroAutoTuneJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewDistroAutoTuneJob returns a job to automatically adjust a distro's maximum
// hosts.
func NewDistroAutoTuneJob(distroID, ts string) amboy.Job {
	j := makeDistroAutoTuneJob()
	j.SetID(fmt.Sprintf("%s.%s.%s", distroAutoTuneJobName, distroID, ts))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", distroAutoTuneJobName, distroID)})
	j.SetEnqueueAllScopes(true)
	j.DistroID = distroID
	return j
}

// kim: TODO: implement
func (j *distroAutoTuneJob) Run(ctx context.Context) {
	defer j.MarkComplete()

}
