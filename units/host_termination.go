package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	adb "github.com/mongodb/anser/db"
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
	HostID          string `bson:"host_id" json:"host_id"`
	Reason          string `bson:"reason,omitempty" json:"reason,omitempty"`
	TerminateIfBusy bool   `bson:"terminate_if_busy" json:"terminate_if_busy"`
	job.Base        `bson:"metadata" json:"metadata"`

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

func NewHostTerminationJob(env evergreen.Environment, h host.Host, terminateIfBusy bool, reason string) amboy.Job {
	j := makeHostTerminationJob()
	j.host = &h
	j.HostID = h.Id
	j.env = env
	j.TerminateIfBusy = terminateIfBusy
	j.Reason = reason
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
			j.AddError(errors.Wrapf(err, "error finding host '%s'", j.HostID))
			return
		}
		if j.host == nil {
			j.AddError(fmt.Errorf("could not find host %s for job %s", j.HostID, j.TaskID))
			return
		}
	}
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
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

	if err = j.host.DeleteJasperCredentials(ctx, j.env); err != nil {
		j.AddError(err)
		grip.Error(message.WrapError(err, message.Fields{
			"message":  "problem deleting Jasper credentials",
			"host":     j.host.Id,
			"provider": j.host.Distro.Provider,
			"job":      j.ID(),
		}))
		return
	}

	// we may be running these jobs on hosts that are already
	// terminated.
	grip.InfoWhen(j.host.Status == evergreen.HostTerminated,
		message.Fields{
			"host":     j.host.Id,
			"provider": j.host.Distro.Provider,
			"job_type": j.Type().Name,
			"job":      j.ID(),
			"message":  "terminating host already marked terminated in the db",
			"theory":   "job collision",
			"outcome":  "investigate-spurious-host-termination",
		})

	// host may still be an intent host
	if j.host.Status == evergreen.HostUninitialized {
		if err = j.host.Terminate(evergreen.User, j.Reason); err != nil {
			j.AddError(errors.Wrap(err, "problem terminating intent host in db"))
			grip.Error(message.WrapError(err, message.Fields{
				"host":     j.host.Id,
				"provider": j.host.Distro.Provider,
				"job_type": j.Type().Name,
				"job":      j.ID(),
				"message":  "problem terminating intent host in db",
			}))
		}
		return
	}

	// clear the running task of the host in case one has been assigned.
	if j.host.RunningTask != "" {
		if j.TerminateIfBusy {
			grip.Warning(message.Fields{
				"message":  "Host has running task; clearing before terminating",
				"job":      j.ID(),
				"job_type": j.Type().Name,
				"host":     j.host.Id,
				"provider": j.host.Distro.Provider,
				"task":     j.host.RunningTask,
			})

			j.AddError(model.ClearAndResetStrandedTask(j.host))
		} else {
			return
		}
	}
	// set host as decommissioned in DB so no new task will be assigned
	prevStatus := j.host.Status
	if err = j.host.SetDecommissioned(evergreen.User, ""); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"host":     j.host.Id,
			"provider": j.host.Distro.Provider,
			"job_type": j.Type().Name,
			"job":      j.ID(),
			"message":  "problem decommissioning host",
		}))
	}

	j.host, err = host.FindOneId(j.HostID)
	if err != nil {
		j.AddError(errors.Wrapf(err, "error finding host '%s'", j.HostID))
		return
	}
	if j.host == nil {
		j.AddError(fmt.Errorf("could not find host %s for job %s", j.HostID, j.TaskID))
		return
	}

	// check if running task has been assigned since status changed
	if j.host.RunningTask != "" {
		if j.TerminateIfBusy {
			grip.Warning(message.Fields{
				"message":  "Host has running task; clearing before terminating",
				"job":      j.ID(),
				"job_type": j.Type().Name,
				"host":     j.host.Id,
				"provider": j.host.Distro.Provider,
				"task":     j.host.RunningTask,
			})

			j.AddError(model.ClearAndResetStrandedTask(j.host))
		} else {
			return
		}
	}

	// terminate containers in DB if parent already terminated
	if j.host.ParentID != "" {
		var parent *host.Host
		parent, err = j.host.GetParent()
		if err != nil {
			j.AddError(errors.Wrapf(err, "problem finding parent of '%s'", j.host.Id))
			return
		}
		if parent.Status == evergreen.HostTerminated {
			if err = j.host.Terminate(evergreen.User, "parent was already terminated"); err != nil {
				j.AddError(errors.Wrap(err, "problem terminating container in db"))
				grip.Error(message.WrapError(err, message.Fields{
					"host":     j.host.Id,
					"provider": j.host.Distro.Provider,
					"job_type": j.Type().Name,
					"job":      j.ID(),
					"message":  "problem terminating container in db",
				}))
			}
			return
		}
	} else if prevStatus == evergreen.HostBuilding {
		// If the host is not a container and is building, this means the host is an intent
		// host, and should be terminated in the database, and not in the cloud manager.
		if err = j.host.Terminate(evergreen.User, "host was never started"); err != nil {
			// It is possible that the provisioning-create-host job has removed the
			// intent host from the database before this job got to it. If so, there is
			// nothing to terminate with a cloud manager, since if there is a
			// cloud-managed host, it has a different ID.
			if adb.ResultsNotFound(err) {
				return
			}
			j.AddError(errors.Wrap(err, "problem terminating intent host in db"))
			grip.Error(message.WrapError(err, message.Fields{
				"host":     j.host.Id,
				"provider": j.host.Distro.Provider,
				"job_type": j.Type().Name,
				"job":      j.ID(),
				"message":  "problem terminating intent host in db",
			}))
		}
		return
	}

	settings := j.env.Settings()

	idleTimeStartsAt := j.host.LastTaskCompletedTime
	if idleTimeStartsAt.IsZero() || idleTimeStartsAt == util.ZeroTime {
		idleTimeStartsAt = j.host.StartTime
	}

	// convert the host to a cloud host
	cloudHost, err := cloud.GetCloudHost(ctx, j.host, j.env)
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
		// other problem getting cloud status
		j.AddError(err)
		grip.Error(message.WrapError(err, message.Fields{
			"host":     j.host.Id,
			"provider": j.host.Distro.Provider,
			"job_type": j.Type().Name,
			"job":      j.ID(),
			"message":  "problem getting cloud host instance status",
		}))

		if !util.StringSliceContains(evergreen.UpHostStatus, prevStatus) {
			if err := j.host.Terminate(evergreen.User, "unable to get cloud status for host"); err != nil {
				j.AddError(err)
			}
			return
		}

	}

	if cloudStatus == cloud.StatusTerminated {
		j.AddError(errors.New("host is already terminated"))
		grip.Warning(message.Fields{
			"host":     j.host.Id,
			"provider": j.host.Distro.Provider,
			"job_type": j.Type().Name,
			"job":      j.ID(),
			"message":  "attempted to terminated an already terminated host",
			"theory":   "external termination",
		})
		if err := j.host.Terminate(evergreen.User, "cloud provider indicated host was terminated"); err != nil {
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

	if err := j.runHostTeardown(ctx, settings); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"job_type": j.Type().Name,
			"message":  "Error running teardown script",
			"host":     j.host.Id,
		}))
	}

	if err := cloudHost.TerminateInstance(ctx, evergreen.User, j.Reason); err != nil {
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

	if j.host.Distro.IsEphemeral() {
		idleJob := newHostIdleJobForTermination(j.env, settings, cloudHost.CloudMgr, j.host, idleTimeStartsAt, hostBillingEnds)
		idleJob.Run(ctx)
		j.AddError(idleJob.Error())

		if j.host.SpawnOptions.SpawnedByTask {
			mgrOpts := cloud.ManagerOpts{
				Provider: j.host.Provider,
				Region:   cloud.GetRegion(j.host.Distro),
			}
			manager, err := cloud.GetManager(ctx, j.env, mgrOpts)
			if err != nil {
				j.AddError(err)
				return
			}
			if calc, ok := manager.(cloud.CostCalculator); ok {
				cost, err := calc.CostForDuration(ctx, j.host, j.host.StartTime, hostBillingEnds)
				if err != nil {
					j.AddError(err)
					return
				}
				j.AddError(task.IncSpawnedHostCost(j.host.StartedBy, cost))
			}
		}
	}
}

func (j *hostTerminationJob) runHostTeardown(ctx context.Context, settings *evergreen.Settings) error {
	if j.host.Distro.Teardown == "" ||
		j.host.Status == evergreen.HostProvisionFailed ||
		j.host.SpawnOptions.SpawnedByTask {
		return nil
	}

	sshOptions, err := j.host.GetSSHOptions(settings)
	if err != nil {
		return errors.Wrapf(err, "error getting ssh options for host %s", j.host.Id)
	}
	startTime := time.Now()
	// run the teardown script with the agent
	logs, err := j.host.RunSSHCommand(ctx, j.host.TearDownCommand(), sshOptions)
	if err != nil {
		event.LogHostTeardown(j.host.Id, logs, false, time.Since(startTime))
		// Try again, this time without the agent, just in case.
		logs, err = j.host.RunSSHCommand(ctx, host.TearDownCommandOverSSH(), sshOptions)
		if err != nil {
			event.LogHostTeardown(j.host.Id, logs, false, time.Since(startTime))
			return errors.Wrapf(err, "error running teardown script on remote host: %s", logs)
		}
	}
	event.LogHostTeardown(j.host.Id, logs, true, time.Since(startTime))
	return nil
}
