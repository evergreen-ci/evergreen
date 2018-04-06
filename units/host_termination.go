package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/notify"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const hostTerminationJobName = "host-termination-job"

func init() {
	registry.AddJobType(hostTerminationJobName, func() amboy.Job {
		return makeHostTerminationJob()
	})
}

type hostTerminationJob struct {
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`

	host *host.Host
	env  evergreen.Environment
}

func makeHostTerminationJob() *hostTerminationJob {
	j := &hostTerminationJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    hostTerminationJobName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

func NewHostTerminationJob(env evergreen.Environment, h host.Host) amboy.Job {
	j := makeHostTerminationJob()
	j.host = &h
	j.HostID = h.Id
	j.env = env
	j.SetPriority(2)
	ts := util.RoundPartOfHour(2).Format(tsFormat)
	j.SetID(fmt.Sprintf("%s.%s.%s", hostTerminationJobName, j.HostID, ts))

	return j
}

func (j *hostTerminationJob) Run(ctx context.Context) {
	var err error
	defer j.MarkComplete()

	if j.host == nil {
		j.host, err = host.FindOneId(j.HostID)
		if err != nil {
			j.AddError(err)
			return
		}
		if j.host == nil {
			j.AddError(fmt.Errorf("could not find host %s for job %s", j.HostID, j.TaskID))
		}
	}

	if !j.host.IsEphemeral() {
		grip.Notice(message.Fields{
			"job":      j.ID(),
			"host":     j.HostID,
			"job_type": j.Type().Name,
			"status":   j.host.Status,
			"provider": j.host.Distro.Provider,
			"message":  "host termination for a non-spawnable distro",
			"cause":    "programmer error",
		})
		return
	}

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	settings := j.env.Settings()

	idleTimeStartsAt := j.host.LastTaskCompletedTime
	if idleTimeStartsAt.IsZero() || idleTimeStartsAt == util.ZeroTime {
		idleTimeStartsAt = j.host.StartTime
	}

	// clear the running task of the host in case one has been assigned.
	if j.host.RunningTask != "" {
		grip.Warning(message.Fields{
			"message":  "Host has running task; clearing before terminating",
			"job":      j.ID(),
			"job_type": j.Type().Name,
			"host":     j.host.Id,
			"provider": j.host.Distro.Provider,
			"task":     j.host.RunningTask,
		})
		if err = j.host.ClearRunningTask(); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"job_type": j.Type().Name,
				"message":  "Error clearing running task for host",
				"provider": j.host.Distro.Provider,
				"host":     j.host.Id,
				"job":      j.ID(),
				"task":     j.host.RunningTask,
			}))
		}
	}

	// convert the host to a cloud host
	cloudHost, err := cloud.GetCloudHost(ctx, j.host, settings)
	if err != nil {
		err = errors.Wrapf(err, "error getting cloud host for %s", j.HostID)
		j.AddError(err)
		grip.Error(message.WrapError(err, message.Fields{
			"host":     j.host.Id,
			"provider": j.host.Distro.Provider,
			"job_type": j.Type().Name,
			"job":      j.ID(),
			"message":  "problem getting cloud host instance, aborting termination",
		}))
		return
	}

	cloudStatus, err := cloudHost.GetInstanceStatus(ctx)
	if err != nil {
		j.AddError(err)
		grip.Error(message.WrapError(err, message.Fields{
			"host":     j.host.Id,
			"provider": j.host.Distro.Provider,
			"job_type": j.Type().Name,
			"job":      j.ID(),
			"message":  "problem getting cloud host instance status",
		}))
	}

	if cloudStatus == cloud.StatusTerminated {
		j.AddError(errors.New("host is already terminated"))
		grip.Error(message.Fields{
			"host":     j.host.Id,
			"provider": j.host.Distro.Provider,
			"job_type": j.Type().Name,
			"job":      j.ID(),
			"message":  "attempted to terminated an already terminated host",
		})
		if err := j.host.Terminate(evergreen.User); err != nil {
			j.AddError(errors.Wrap(err, "problem terminating host in db"))
			grip.Error(message.WrapError(err, message.Fields{
				"host":     j.host.Id,
				"provider": j.host.Distro.Provider,
				"job_type": j.Type().Name,
				"job":      j.ID(),
				"message":  "problem terminating host in db",
			}))
		}

		return
	}

	if err := runHostTeardown(ctx, j.host, cloudHost); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"job_type": j.Type().Name,
			"message":  "Error running teardown script",
			"host":     j.host.Id,
		}))

		subj := fmt.Sprintf("%v Error running teardown for host %v",
			notify.TeardownFailurePreface, j.host.Id)

		grip.Error(message.WrapError(notify.NotifyAdmins(subj, err.Error(), settings),
			message.Fields{
				"message":  "problem sending email",
				"host":     j.host.Id,
				"subject":  subj,
				"error":    err.Error(),
				"job_type": j.Type().Name,
				"job":      j.ID(),
			}))
	}

	if err := cloudHost.TerminateInstance(ctx, evergreen.User); err != nil {
		j.AddError(err)
		grip.Error(message.WrapError(err, message.Fields{
			"message":  "problem terminating host",
			"host":     j.host.Id,
			"job":      j.ID(),
			"job_type": j.Type().Name,
		}))
		return
	}

	hostBillingEnds := j.host.TerminationTime

	pad := cloudHost.CloudMgr.TimeTilNextPayment(j.host)
	if pad > time.Second {
		hostBillingEnds = hostBillingEnds.Add(pad)
	}

	idleJob := newHostIdleJobForTermination(j.env, settings, cloudHost.CloudMgr, j.host, idleTimeStartsAt, hostBillingEnds)
	idleJob.Run(ctx)
	j.AddError(idleJob.Error())
}

func runHostTeardown(ctx context.Context, h *host.Host, cloudHost *cloud.CloudHost) error {
	sshOptions, err := cloudHost.GetSSHOptions()
	if err != nil {
		return errors.Wrapf(err, "error getting ssh options for host %s", h.Id)
	}
	startTime := time.Now()
	// run the teardown script with the agent
	logs, err := h.RunSSHCommand(ctx, h.TearDownCommand(), sshOptions)
	if err != nil {
		event.LogHostTeardown(h.Id, logs, false, time.Since(startTime))
		return errors.Wrapf(err, "error running teardown script on remote host: %s", logs)
	}
	event.LogHostTeardown(h.Id, logs, true, time.Since(startTime))
	return nil
}
