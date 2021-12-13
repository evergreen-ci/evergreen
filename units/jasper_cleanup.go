package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
)

const jasperManagerCleanupJobName = "jasper-manager-cleanup"

func init() {
	registry.AddJobType(jasperManagerCleanupJobName,
		func() amboy.Job { return makeJasperManagerCleanup() })
}

type jasperManagerCleanup struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment
}

// NewJasperManagerCleanup reports basic system information and a
// report of the go runtime information, as provided by grip.
func NewJasperManagerCleanup(id string, env evergreen.Environment) amboy.Job {
	j := makeJasperManagerCleanup()
	j.env = env
	j.SetID(fmt.Sprintf("%s.%s", jasperManagerCleanupJobName, id))
	ti := j.TimeInfo()
	ti.MaxTime = time.Second
	j.UpdateTimeInfo(ti)
	return j
}

func makeJasperManagerCleanup() *jasperManagerCleanup {
	j := &jasperManagerCleanup{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    jasperManagerCleanupJobName,
				Version: 0,
			},
		},
	}
	return j
}

func (j *jasperManagerCleanup) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	j.env.JasperManager().Clear(ctx)
}
