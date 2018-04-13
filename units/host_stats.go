package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const hostStatsName = "host-status-alerting"

func init() {
	registry.AddJobType(hostStatsName, func() amboy.Job {
		return makeHostStats()
	})
}

type hostStatsJob struct {
	job.Base `bson:"base" json:"base" yaml:"base"`
}

func makeHostStats() *hostStatsJob {
	j := &hostStatsJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    hostStatsName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

func NewHostStatsJob(ts string) amboy.Job {
	job := makeHostStats()
	job.SetID(fmt.Sprintf("%s.%s", hostAlertingName, ts))
	return job
}

func (j *hostStatsJob) Run(ctx context.Context) {
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()
	defer j.MarkComplete()

	var counts []host.InactiveHostCounts
	var static int
	var dynamic int
	err := db.Aggregate(host.Collection, host.InactiveHostCountPipeline(), &counts)
	if err != nil {
		j.AddError(errors.Wrap(err, "error aggregating inactive hosts"))
		return
	}
	for _, count := range counts {
		if count.HostType == evergreen.HostTypeStatic {
			static = count.Count
		} else {
			dynamic++
		}
	}

	grip.Info(message.Fields{
		"message": "count of decommissioned/quarantined hosts",
		"total":   static + dynamic,
		"static":  static,
		"dynamic": dynamic,
	})
}
