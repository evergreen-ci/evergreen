package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/hoststat"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/pkg/errors"
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
	distro   distro.Distro
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

	if err := j.populate(ctx); err != nil {
		j.AddError(errors.Wrapf(err, "populating job for distro '%s'", j.DistroID))
		return
	}

	// kim: TODO: needs distro feature flag merged.
	if !j.distro.HostAllocatorSettings.AutoTuneMaximumHosts {
		return
	}

	const recentStatsWindow = 7 * utility.Day
	stats, err := hoststat.FindByDistroSince(ctx, j.DistroID, time.Now().Add(-recentStatsWindow))
	if err != nil {
		j.AddError(errors.Wrapf(err, "getting host stats for distro '%s'", j.DistroID))
		return
	}

	// kim: TODO: use stats to decide whether to adjust up/down.
	fmt.Println(stats)
}

func (j *distroAutoTuneJob) populate(ctx context.Context) error {
	d, err := distro.FindOneId(j.DistroID)
	if err != nil {
		return errors.Wrapf(err, "finding distro '%s'", j.DistroID)
	}
	if d == nil {
		return errors.Errorf("distro '%s' not found", j.DistroID)
	}
	j.distro = d
}
