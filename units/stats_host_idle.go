package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

const collectHostIdleDataJobName = "collect-host-idle-data"

type collecHostIdleDataJob struct {
	HostID     string    `bson:"host_id" json:"host_id" yaml:"host_id"`
	StartTime  time.Time `bson:"start_time" json:"start_time" yaml:"start_time"`
	FinishTime time.Time `bson:"finish_time" json:"finish_time" yaml:"finish_time"`
	job.Base   `bson:"metadata" json:"metadata" yaml:"metadata"`

	// internal cache
	host *host.Host
	env  evergreen.Environment
}

func newHostIdleJob() *collecHostIdleDataJob {
	j := &collecHostIdleDataJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    collectHostIdleDataJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

func NewCollectHostIdleDataJob(h *host.Host, startTime, finishTime time.Time) amboy.Job {
	j := newHostIdleJob()
	j.host = h
	j.HostID = h.Id
	j.StartTime = startTime
	j.FinishTime = finishTime
	j.SetID(fmt.Sprintf("%s.%s.%d", collectHostIdleDataJobName, j.HostID, job.GetNumber()))
	return j
}

func (j *collecHostIdleDataJob) Run() {
	var err error

	if j.host == nil {
		j.host, err = host.FindOneId(j.HostID)
		j.AddError(err)
	}

	if j.HasErrors() {
		return
	}

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if j.StartTime.IsZero() || j.StartTime == util.ZeroTime {
		j.StartTime = j.host.StartTime
	}

	settings := j.env.Settings()

	var cost float64
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager, err := cloud.GetCloudManager(ctx, j.host.Provider, settings)
	if err != nil {
		j.AddError(err)
		grip.Error(message.WrapErrorf(err, "Error loading provider for host %s cost calculation", j.HostID))
	} else {
		if calc, ok := manager.(cloud.CloudCostCalculator); ok {
			cost, err = calc.CostForDuration(ctx, j.host, j.StartTime, j.FinishTime)
			if err != nil {
				j.AddError(err)
			}
			if err = j.host.IncCost(cost); err != nil {
				j.AddError(err)
			}
		}
	}

	idleTime := j.FinishTime.Sub(j.StartTime)

	if err = j.Host.IncIdleTime(idleTime); err != nil {
		j.AddError(err)
	}

	msg := message.Fields{
		"stat":      "host-idle-stat",
		"distro":    j.host.Distro.Id,
		"provider":  j.host.Distro.Provider,
		"host":      j.host.Id,
		"status":    j.host.Status,
		"idle_secs": idleTime.Seconds(),
	}

	if cost != 0 {
		msg["cost"] = cost
	}

	grip.Info(msg)
}
