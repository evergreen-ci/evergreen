package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

const hostStatsName = "host-stats"

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

	statuses := []string{evergreen.HostDecommissioned, evergreen.HostQuarantined}
	hosts, err := host.Find(host.ByStatuses(statuses))
	if err != nil {
		j.AddError(err)
		return
	}
	dynamic := 0
	static := 0
	for _, h := range hosts {
		if h.Provider == evergreen.HostTypeStatic {
			static++
		} else {
			dynamic++
		}
	}
	if hosts != nil {
		grip.Info(message.Fields{
			"message": "count of decommissioned/quarantined hosts",
			"total":   len(hosts),
			"static":  static,
			"dynamic": dynamic,
		})
	}
}
