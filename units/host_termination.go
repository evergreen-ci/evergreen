package units

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
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
	"github.com/mongodb/jasper/options"
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
	ts := util.RoundPartOfHour(2).Format(TSFormat)
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
			"host_id":  j.HostID,
			"job_type": j.Type().Name,
			"status":   j.host.Status,
			"provider": j.host.Distro.Provider,
			"message":  "host termination for a non-spawnable distro",
			"cause":    "programmer error",
		})
		return
	}

	if j.host.HasContainers {
		var idle bool
		idle, err = j.host.IsIdleParent()
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "problem checking if host is an idle parent",
				"host_id": j.host.Id,
				"job":     j.ID(),
			}))
		}
		if !idle {
			return
		}
	}

	if err = j.host.DeleteJasperCredentials(ctx, j.env); err != nil {
		j.AddError(err)
		grip.Error(message.WrapError(err, message.Fields{
			"message":  "problem deleting Jasper credentials",
			"host_id":  j.host.Id,
			"provider": j.host.Distro.Provider,
			"job":      j.ID(),
		}))
		return
	}

	// we may be running these jobs on hosts that are already
	// terminated.
	grip.InfoWhen(j.host.Status == evergreen.HostTerminated,
		message.Fields{
			"host_id":  j.host.Id,
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
				"host_id":  j.host.Id,
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
				"host_id":  j.host.Id,
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
	if err = j.host.SetDecommissioned(evergreen.User, "host will be terminated shortly, preventing task dispatch"); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"host_id":  j.host.Id,
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
				"host_id":  j.host.Id,
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
			if err.Error() != host.ErrorParentNotFound {
				j.AddError(errors.Wrapf(err, "problem finding parent of '%s'", j.host.Id))
				return
			}
		}
		if parent == nil || parent.Status == evergreen.HostTerminated {
			if err = j.host.Terminate(evergreen.User, "parent was already terminated"); err != nil {
				j.AddError(errors.Wrap(err, "problem terminating container in db"))
				grip.Error(message.WrapError(err, message.Fields{
					"host_id":  j.host.Id,
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
				"host_id":  j.host.Id,
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
			"host_id":  j.host.Id,
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
			"host_id":  j.host.Id,
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
			"host_id":  j.host.Id,
			"provider": j.host.Distro.Provider,
			"job_type": j.Type().Name,
			"job":      j.ID(),
			"message":  "attempted to terminated an already terminated host",
			"theory":   "external termination",
		})
		if err := j.host.Terminate(evergreen.User, "cloud provider indicated host was terminated"); err != nil {
			j.AddError(errors.Wrap(err, "problem terminating host in db"))
			grip.Error(message.WrapError(err, message.Fields{
				"host_id":  j.host.Id,
				"provider": j.host.Distro.Provider,
				"job_type": j.Type().Name,
				"job":      j.ID(),
				"message":  "problem terminating host in db",
			}))
		}

		return
	}

	if output, err := j.runHostTeardown(ctx, settings); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"job_type": j.Type().Name,
			"message":  "Error running teardown script",
			"host_id":  j.host.Id,
			"logs":     output,
		}))
	}

	if err := cloudHost.TerminateInstance(ctx, evergreen.User, j.Reason); err != nil {
		j.AddError(err)
		grip.Error(message.WrapError(err, message.Fields{
			"message":  "problem terminating host",
			"host_id":  j.host.Id,
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
			mgrOpts, err := cloud.GetManagerOptions(j.host.Distro)
			if err != nil {
				j.AddError(errors.Wrapf(err, "can't get ManagerOpts for '%s'", j.host.Id))
				return
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

func (j *hostTerminationJob) runHostTeardown(ctx context.Context, settings *evergreen.Settings) (string, error) {
	if j.host.Distro.Teardown == "" ||
		j.host.Status == evergreen.HostProvisionFailed ||
		j.host.SpawnOptions.SpawnedByTask {
		return "", nil
	}

	if !j.host.Distro.LegacyBootstrap() {
		if j.host.Distro.BootstrapSettings.Method == distro.BootstrapMethodUserData {
			if output, err := j.writeTeardownScript(ctx, settings); err != nil {
				return output, errors.Wrap(err, "could not put teardown script on host")
			}
		}

		// Attempt to run the teardown command through Jasper.
		output, err := j.tryRunTeardownScript(ctx, settings, func(runScript string) (string, error) {
			output, err := j.host.RunJasperProcess(ctx, j.env, &options.Create{
				Args: []string{j.host.Distro.ShellBinary(), "-l", "-c", runScript},
			})
			return strings.Join(output, "\n"), err
		})
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "could not run teardown script using Jasper, will attempt to run teardown using SSH",
				"host_id": j.host.Id,
				"distro":  j.host.Distro.Id,
				"logs":    output,
				"job":     j.ID(),
			}))
		} else {
			return output, nil
		}
	}

	return j.tryRunTeardownScript(ctx, settings, func(runScript string) (string, error) {
		return j.host.RunSSHCommand(ctx, runScript)
	})
}

// tryRunTeardownScript attempts to run the teardown script using the given
// runCmd to execute the teardown command.
func (j *hostTerminationJob) tryRunTeardownScript(ctx context.Context, settings *evergreen.Settings, runCmd func(runScript string) (string, error)) (string, error) {
	startTime := time.Now()
	output, err := runCmd(j.host.TearDownCommand())
	if err != nil {
		event.LogHostTeardown(j.host.Id, output, false, time.Since(startTime))

		output, err = runCmd(host.TearDownDirectlyCommand())
		if err != nil {
			event.LogHostTeardown(j.host.Id, output, false, time.Since(startTime))
		}
	}
	event.LogHostTeardown(j.host.Id, output, true, time.Since(startTime))
	return output, nil
}

// writeTeardownScript writes the teardown script to the host for hosts
// provisioned with user data User data hosts do not write the teardown script
// in the user data script because the user data script is subject to a 16kB
// text limit, which is easy to exceed.
func (j *hostTerminationJob) writeTeardownScript(ctx context.Context, settings *evergreen.Settings) (string, error) {
	if j.host.Distro.BootstrapSettings.Method != distro.BootstrapMethodUserData {
		return "", nil
	}

	script, err := expandScript(j.host.Distro.Teardown, settings)
	if err != nil {
		return "", errors.Wrap(err, "error expanding teardown script")
	}

	args := []string{j.host.Distro.ShellBinary(), "-c",
		fmt.Sprintf("tee %s", filepath.Join(j.host.Distro.HomeDir(), evergreen.TeardownScriptName))}
	output, err := j.host.RunJasperProcess(ctx, j.env, &options.Create{
		Args:               args,
		StandardInputBytes: []byte(script),
	})
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not write teardown script to host through Jasper",
			"host_id": j.host.Id,
			"distro":  j.host.Distro.Id,
			"logs":    strings.Join(output, "\n"),
			"job":     j.ID(),
		}))

		// If Jasper fails to write the file, fall back to SCPing it
		// onto the host.
		var scpOutput string
		scpOutput, err = copyScript(ctx, j.env, settings, j.host, evergreen.TeardownScriptName, script)
		return scpOutput, errors.Wrap(err, "failed to SCP teardown script to host")
	}
	return strings.Join(output, "\n"), nil
}
