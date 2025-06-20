package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
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
	distro   *distro.Distro
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

	if !evergreen.IsEc2Provider(j.distro.Provider) {
		return
	}

	// kim: TODO: needs distro feature flag merged.
	// if !j.distro.HostAllocatorSettings.AutoTuneMaximumHosts {
	//     return
	// }

	const recentStatsWindow = 7 * utility.Day
	stats, err := hoststat.FindByDistroSince(ctx, j.DistroID, time.Now().Add(-recentStatsWindow))
	if err != nil {
		j.AddError(errors.Wrapf(err, "getting host stats for distro '%s'", j.DistroID))
		return
	}

	// kim: TODO: use stats to decide whether to adjust up/down.
	if len(stats) == 0 {
		return
	}

	summary := j.summarizeStatsUsage(stats)

	const (
		thresholdFractionToReduceHosts = 0.5
		maxFractionalHostReduction     = 0.1
	)
	maxHostUtilization := float64(summary.maxHostUsage) / float64(j.distro.HostAllocatorSettings.MaximumHosts)
	if maxHostUtilization < thresholdFractionToReduceHosts {
		// Reduce max hosts a bit due to low usage (based on % above).
		return
	}

	const (
		thresholdFractionToIncreaseHosts = 0.02
		maxFractionalHostIncrease        = 0.5
	)
	fractionOfTimeAtMaxHosts := float64(summary.numTimesMaxHostsHit) / float64(len(stats))
	if fractionOfTimeAtMaxHosts > thresholdFractionToIncreaseHosts {
		// Increase max hosts a bit (based on % of times hitting max hosts).
		// kim: NOTE: increasing max hosts would cause later days to not hit
		// max hosts, so autotune would not kick in.
	}

	// kim: TODO: ensure it's different from below + above min hosts before
	// saving.
}

func (j *distroAutoTuneJob) populate(ctx context.Context) error {
	d, err := distro.FindOneId(ctx, j.DistroID)
	if err != nil {
		return errors.Wrapf(err, "finding distro '%s'", j.DistroID)
	}
	if d == nil {
		return errors.Errorf("distro '%s' not found", j.DistroID)
	}
	j.distro = d

	return nil
}

type hostStatsSummary struct {
	// NumTimesMaxHostsHit is how many times max hosts was hit or exceeded.
	numTimesMaxHostsHit int
	// MaxHostUsage is the max amount of hosts that were actually used by the
	// distro.
	maxHostUsage int
}

func (j *distroAutoTuneJob) summarizeStatsUsage(stats []hoststat.HostStat) hostStatsSummary {
	summary := hostStatsSummary{}
	distroMaxHosts := j.distro.HostAllocatorSettings.MaximumHosts
	for _, stat := range stats {
		if stat.NumHosts >= distroMaxHosts {
			summary.numTimesMaxHostsHit++
		}
		if stat.NumHosts > summary.maxHostUsage {
			summary.maxHostUsage = stat.NumHosts
		}
	}
	return summary
}
