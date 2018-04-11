package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const hostMonitorName = "host-monitoring"

func init() {
	registry.AddJobType(hostMonitorName, func() amboy.Job {
		return makeHostMonitor()
	})
}

type hostMonitorJob struct {
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`
	job.Base `bson:"base" json:"base" yaml:"base"`

	// cache
	host   *host.Host
	env    evergreen.Environment
	logger grip.Journaler
}

func makeHostMonitor() *hostMonitorJob {
	j := &hostMonitorJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    hostMonitorName,
				Version: 0,
			},
		},
		logger: logging.MakeGrip(grip.GetSender()),
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

func NewHostMonitorJob(env evergreen.Environment, h *host.Host, id string) amboy.Job {
	job := makeHostMonitor()

	job.host = h
	job.HostID = h.Id

	job.SetID(fmt.Sprintf("%s.%s.%s", hostMonitorName, job.HostID, id))

	return job
}

func (j *hostMonitorJob) Run(ctx context.Context) {
	var cancel context.CancelFunc

	ctx, cancel = context.WithCancel(ctx)
	defer cancel()
	defer j.MarkComplete()

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		j.AddError(errors.Wrap(err, "error retrieving admin settings"))
		return
	}
	if flags.MonitorDisabled {
		j.AddError(errors.New("monitor is disabled"))
		return
	}

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	j.host, err = host.FindOneId(j.HostID)
	if err != nil {
		j.AddError(err)
		return
	}
	if j.host == nil {
		j.AddError(fmt.Errorf("unable to retrieve host %s", j.HostID))
		return
	}

	j.monitorExternalTermination(ctx)
	j.monitorLongRunningTasks()
}

func (j *hostMonitorJob) monitorExternalTermination(ctx context.Context) {
	settings := j.env.Settings()

	cloudHost, err := cloud.GetCloudHost(ctx, j.host, settings)
	if err != nil {
		j.AddError(errors.Wrapf(err, "error getting cloud host for host %v: %v", j.host.Id))
		return
	}

	cloudStatus, err := cloudHost.GetInstanceStatus(ctx)
	if err != nil {
		j.AddError(errors.Wrapf(err, "error getting cloud status for host %s", j.HostID))
		return
	}

	switch cloudStatus {
	case cloud.StatusRunning:
		if j.host.Status != evergreen.HostRunning {
			j.logger.Info(message.Fields{
				"op":      hostMonitorName,
				"op_id":   j.ID(),
				"message": "found running host, with incorrect status ",
				"status":  j.host.Status,
				"host":    j.HostID,
				"distro":  j.host.Distro.Id,
			})

			j.AddError(errors.Wrapf(j.host.MarkReachable(), "error updating reachability for host %s", j.HostID))
		}
	case cloud.StatusTerminated:
		j.logger.Info(message.Fields{
			"op":      hostMonitorName,
			"op_id":   j.ID(),
			"message": "host terminated externally",
			"host":    j.HostID,
			"distro":  j.host.Distro.Id,
		})

		event.LogHostTerminatedExternally(j.HostID)

		// the instance was terminated from outside our control
		j.AddError(errors.Wrapf(j.host.SetTerminated("external"), "error setting host %s terminated", j.HostID))
	default:
		j.logger.Warning(message.Fields{
			"message":      "host found with unexpected status",
			"op":           hostMonitorName,
			"op_id":        j.ID(),
			"host":         j.HostID,
			"distro":       j.host.Distro.Id,
			"host_status":  j.host.Status,
			"cloud_status": cloudStatus,
		})
	}
}

func (j *hostMonitorJob) monitorLongRunningTasks() {
	const warnThreshold = 12 * time.Hour
	const criticalThreshold = 24 * time.Hour

	runningTask, err := task.FindOne(task.ById(j.host.RunningTask))
	if err != nil {
		j.AddError(err)
		return
	}
	if runningTask == nil {
		return
	}
	elapsed := time.Now().Sub(runningTask.StartTime)
	msg := message.Fields{
		"message":     "host running task for too long",
		"op":          hostMonitorName,
		"op_id":       j.ID(),
		"host":        j.HostID,
		"distro":      j.host.Distro.Id,
		"task":        runningTask.Id,
		"elapsed":     elapsed.String(),
		"elapsed_raw": elapsed,
	}
	j.logger.WarningWhen(elapsed > warnThreshold && elapsed <= criticalThreshold, msg)
	j.logger.CriticalWhen(elapsed > criticalThreshold, msg)
}
