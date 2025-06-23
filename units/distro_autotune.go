package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/hoststat"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	distroAutoTuneJobName = "distro-auto-tune"
	distroAutoTuneUser    = "distro_auto_tune"
)

func init() {
	registry.AddJobType(distroAutoTuneJobName, func() amboy.Job {
		return makeDistroAutoTuneJob()
	})
}

type distroAutoTuneJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	DistroID string `bson:"distro_id" json:"distro_id" yaml:"distro_id"`
	settings *evergreen.Settings
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
// kim: TODO: test job in staging
func NewDistroAutoTuneJob(distroID, ts string) amboy.Job {
	j := makeDistroAutoTuneJob()
	j.SetID(fmt.Sprintf("%s.%s.%s", distroAutoTuneJobName, distroID, ts))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", distroAutoTuneJobName, distroID)})
	j.SetEnqueueAllScopes(true)
	j.DistroID = distroID
	return j
}

func (j *distroAutoTuneJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if err := j.populate(ctx); err != nil {
		j.AddError(errors.Wrapf(err, "populating job for distro '%s'", j.DistroID))
		return
	}

	if !evergreen.IsEc2Provider(j.distro.Provider) {
		return
	}

	if !j.distro.HostAllocatorSettings.AutoTuneMaximumHosts {
		return
	}

	const recentStatsWindow = 7 * utility.Day
	stats, err := hoststat.FindByDistroSince(ctx, j.DistroID, time.Now().Add(-recentStatsWindow))
	if err != nil {
		j.AddError(errors.Wrapf(err, "getting host stats for distro '%s'", j.DistroID))
		return
	}

	if len(stats) == 0 {
		return
	}

	summary := j.summarizeStatsUsage(stats)

	// Avoid tuning rarely-used distros because they may not have enough data to
	// make a reasonable decision about the distro's host usage.
	const minFractionOfTimeUsingHostsToAutoTune = 0.01
	if summary.fractionOfTimeUsingHosts < minFractionOfTimeUsingHostsToAutoTune {
		grip.Info(message.Fields{
			"message":                      "skipping auto-tuning maximum hosts for rarely-used distro",
			"distro":                       j.DistroID,
			"fraction_of_time_using_hosts": summary.fractionOfTimeUsingHosts,
			"job":                          j.ID(),
		})
		return
	}

	const (
		thresholdFractionToDecreaseHosts = 0.5
		maxFractionalHostDecrease        = 0.1

		thresholdFractionToIncreaseHosts = 0.02
		maxFractionalHostIncrease        = 0.25
	)
	newMaxHosts := j.distro.HostAllocatorSettings.MaximumHosts
	if summary.fractionOfMaxHostsUsed < thresholdFractionToDecreaseHosts {
		// Decrease max hosts due to low usage based on percentage of distro
		// max hosts utilized.
		fractionToDecrease := min(thresholdFractionToDecreaseHosts-summary.fractionOfMaxHostsUsed, maxFractionalHostDecrease)
		newMaxHosts = int(float64(j.distro.HostAllocatorSettings.MaximumHosts) * (1 - fractionToDecrease))
	} else if summary.fractionOfTimeAtMaxHosts >= thresholdFractionToIncreaseHosts {
		// Increase max hosts based on % of times distro max hosts was hit.
		fractionToIncrease := min(summary.fractionOfTimeAtMaxHosts, maxFractionalHostIncrease)
		newMaxHosts = int(float64(j.distro.HostAllocatorSettings.MaximumHosts) * (1 + fractionToIncrease))
		// kim: NOTE: increasing max hosts would cause later days to not hit
		// max hosts, so autotune would not kick in for a few days unless it was
		// consistently hitting it.
	}

	// Put reasonable bounds on hosts so that it's not increased extremely high
	// or decreased extremely low relative to global max hosts.
	const (
		lowerBoundMaxHostsFraction = 0.005
		lowerBoundMaxHostsNum      = 5
		upperBoundMaxHostsFraction = 0.4
	)
	lowerBoundMaxHosts := int(float64(j.settings.HostInit.MaxTotalDynamicHosts) * lowerBoundMaxHostsFraction)
	lowerBoundMaxHosts = max(lowerBoundMaxHosts, lowerBoundMaxHostsNum)
	upperBoundMaxHosts := int(float64(j.settings.HostInit.MaxTotalDynamicHosts) * upperBoundMaxHostsFraction)
	newMaxHosts = min(newMaxHosts, upperBoundMaxHosts)
	newMaxHosts = max(newMaxHosts, lowerBoundMaxHosts)
	newMaxHosts = max(newMaxHosts, j.distro.HostAllocatorSettings.MinimumHosts)

	if newMaxHosts == j.distro.HostAllocatorSettings.MaximumHosts {
		grip.Info(message.Fields{
			"message":                      "did not change maximum hosts during auto-tuning",
			"distro":                       j.DistroID,
			"fraction_of_time_using_hosts": summary.fractionOfTimeUsingHosts,
			"job":                          j.ID(),
		})
		return
	}

	if err := j.updateMaxHosts(ctx, newMaxHosts); err != nil {
		j.AddError(err)
	}
}

func (j *distroAutoTuneJob) populate(ctx context.Context) error {
	if j.settings == nil {
		settings, err := evergreen.GetConfig(ctx)
		if err != nil {
			return errors.Wrap(err, "getting admin settings")
		}
		j.settings = settings
	}

	if j.distro == nil {
		d, err := distro.FindOneId(ctx, j.DistroID)
		if err != nil {
			return errors.Wrapf(err, "finding distro '%s'", j.DistroID)
		}
		if d == nil {
			return errors.Errorf("distro '%s' not found", j.DistroID)
		}
		j.distro = d
	}

	return nil
}

type hostStatsSummary struct {
	// fractionOfTimeAtMaxHosts is the fraction of time that the distro was at
	// or above max hosts.
	fractionOfTimeAtMaxHosts float64
	// fractionOfMaxHostsUsed is the maximum number of hosts used as a fraction
	// of distro max hosts.
	fractionOfMaxHostsUsed float64
	// fractionOfTimeUsingHosts is the fraction of time that the distro used any
	// non-zero number of hosts.
	fractionOfTimeUsingHosts float64
}

func (j *distroAutoTuneJob) summarizeStatsUsage(stats []hoststat.HostStat) hostStatsSummary {
	summary := hostStatsSummary{}
	var (
		numTimesMaxHostsHit int
		numTimesHostsUsed   int
		maxHostsUsed        int
	)
	for _, stat := range stats {
		if stat.NumHosts > 0 {
			numTimesHostsUsed++
		}
		if stat.NumHosts >= j.distro.HostAllocatorSettings.MaximumHosts {
			numTimesMaxHostsHit++
		}
		maxHostsUsed = max(maxHostsUsed, stat.NumHosts)
	}
	summary.fractionOfTimeUsingHosts = float64(numTimesHostsUsed) / float64(len(stats))
	summary.fractionOfTimeAtMaxHosts = float64(numTimesMaxHostsHit) / float64(len(stats))
	summary.fractionOfMaxHostsUsed = float64(maxHostsUsed) / float64(j.distro.HostAllocatorSettings.MaximumHosts)
	return summary
}

func (j *distroAutoTuneJob) updateMaxHosts(ctx context.Context, newMaxHosts int) error {
	updatedDistro := *j.distro
	updatedDistro.HostAllocatorSettings.MaximumHosts = newMaxHosts

	if err := updatedDistro.ReplaceOne(ctx); err != nil {
		return errors.Wrapf(err, "updating maximum hosts for distro '%s' from %d to %d", j.DistroID, j.distro.HostAllocatorSettings.MaximumHosts, newMaxHosts)
	}

	event.LogDistroModified(ctx, j.DistroID, distroAutoTuneUser, j.distro.DistroData(), updatedDistro.DistroData())

	maxHostsDiff := newMaxHosts - j.distro.HostAllocatorSettings.MaximumHosts
	grip.Info(message.Fields{
		"message":                 "auto-tuned distro maximum hosts",
		"old_max_hosts":           j.distro.HostAllocatorSettings.MaximumHosts,
		"new_max_hosts":           newMaxHosts,
		"max_hosts_diff_absolute": maxHostsDiff,
		"max_hosts_diff_fraction": float64(maxHostsDiff) / float64(j.distro.HostAllocatorSettings.MaximumHosts),
		"distro":                  j.DistroID,
		"job":                     j.ID(),
	})

	return nil
}
