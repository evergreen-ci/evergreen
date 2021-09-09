package units

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const cloudHostReadyJobName = "set-cloud-hosts-ready"

func init() {
	registry.AddJobType(cloudHostReadyJobName,
		func() amboy.Job { return makeCloudHostReadyJob() })
}

type cloudHostReadyJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment
}

// NewCloudHostReadyJob checks the cloud instance status for all hosts created
// by cloud providers when the instance is not yet ready to be used (e.g. the
// instance is still booting up). Once the cloud instance status is resolved,
// the job can either transition the host into the next step in the host
// lifecycle or be appropriately handled if it is in an unrecoverable state.
func NewCloudHostReadyJob(env evergreen.Environment, id string) amboy.Job {
	j := makeCloudHostReadyJob()
	j.SetID(fmt.Sprintf("%s.%s", cloudHostReadyJobName, id))
	j.env = env
	j.SetPriority(1)
	j.SetScopes([]string{cloudHostReadyJobName})
	// Jobs never appear to exceed 1 minute, but add a bunch of padding.
	j.UpdateTimeInfo(amboy.JobTimeInfo{MaxTime: 10 * time.Minute})
	return j
}

func makeCloudHostReadyJob() *cloudHostReadyJob {
	j := &cloudHostReadyJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    cloudHostReadyJobName,
				Version: 1,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

func (j *cloudHostReadyJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	// Collect hosts by provider and region
	settings, err := evergreen.GetConfig()
	if err != nil {
		j.AddError(errors.Wrap(err, "unable to get evergreen settings"))
		return
	}
	startingHostsByClient, err := host.StartingHostsByClient(settings.HostInit.CloudStatusBatchSize)
	if err != nil {
		j.AddError(errors.Wrap(err, "can't get starting hosts"))
		return
	}
clientsLoop:
	for clientOpts, hosts := range startingHostsByClient {
		if ctx.Err() != nil {
			j.AddError(ctx.Err())
			return
		}
		if len(hosts) == 0 {
			continue
		}
		mgrOpts := cloud.ManagerOpts{
			Provider:       clientOpts.Provider,
			Region:         clientOpts.Region,
			ProviderKey:    clientOpts.Key,
			ProviderSecret: clientOpts.Secret,
		}
		m, err := cloud.GetManager(ctx, j.env, mgrOpts)
		if err != nil {
			j.AddError(errors.Wrap(err, "error getting cloud manager"))
			return
		}
		if batch, ok := m.(cloud.BatchManager); ok {
			statusesCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()

			startAt := time.Now()
			statuses, err := batch.GetInstanceStatuses(statusesCtx, hosts)
			grip.Debug(message.Fields{
				"message":       "finished getting instance statuses",
				"num_hosts":     len(hosts),
				"provider":      clientOpts.Provider,
				"duration_secs": time.Since(startAt).Seconds(),
			})
			if err != nil {
				if strings.Contains(err.Error(), "InvalidInstanceID.NotFound") {
					j.AddError(j.terminateUnknownHosts(ctx, err.Error()))
					continue clientsLoop
				}
				j.AddError(errors.Wrap(err, "error getting host statuses for providers"))
				continue clientsLoop
			}
			if len(statuses) != len(hosts) {
				j.AddError(errors.Errorf("programmer error: length of statuses != length of hosts"))
				continue clientsLoop
			}
			for i := range hosts {
				j.AddError(errors.Wrapf(j.setCloudHostStatus(ctx, m, hosts[i], statuses[i]), "setting instance status for host '%s'", hosts[i].Id))
			}
			continue clientsLoop
		}
		for _, h := range hosts {
			statusCtx, cancel := context.WithTimeout(ctx, time.Minute)
			startAt := time.Now()
			cloudStatus, err := m.GetInstanceStatus(statusCtx, &h)
			cancel()
			grip.Debug(message.Fields{
				"message":       "finished getting instance status",
				"host_id":       h.Id,
				"provider":      clientOpts.Provider,
				"duration_secs": time.Since(startAt).Seconds(),
			})
			if err != nil {
				j.AddError(errors.Wrapf(err, "error checking instance status of host %s", h.Id))
				continue clientsLoop
			}
			j.AddError(errors.Wrapf(j.setCloudHostStatus(ctx, m, h, cloudStatus), "setting instance status for host '%s'", h.Id))
		}
	}
}

func (j *cloudHostReadyJob) terminateUnknownHosts(ctx context.Context, awsErr string) error {
	pieces := strings.Split(awsErr, "'")
	if len(pieces) != 3 {
		return errors.Errorf("unexpected format of AWS error: %s", awsErr)
	}
	instanceIDs := strings.Split(pieces[1], ",")
	grip.Warning(message.Fields{
		"message": "host IDs not found in AWS, will terminate",
		"hosts":   instanceIDs,
	})
	catcher := grip.NewBasicCatcher()
	for _, hostID := range instanceIDs {
		h, err := host.FindOneId(hostID)
		if err != nil {
			catcher.Add(err)
			continue
		}
		if h == nil {
			continue
		}
		catcher.Add(amboy.EnqueueUniqueJob(ctx, j.env.RemoteQueue(), NewHostTerminationJob(j.env, h, true, "instance ID not found")))
	}
	return catcher.Resolve()
}

// setCloudHostStatus checks the status of the host's cloud instance to
// determine the next step in the host lifecycle. Hosts that are running
// in the cloud can successfully transition to the next step in the lifecycle.
// Hosts found in an unrecoverable state are terminated.
func (j *cloudHostReadyJob) setCloudHostStatus(ctx context.Context, m cloud.Manager, h host.Host, cloudStatus cloud.CloudStatus) error {
	switch cloudStatus {
	case cloud.StatusFailed, cloud.StatusTerminated, cloud.StatusStopped, cloud.StatusStopping:
		j.logHostStatusMessage(&h, cloudStatus)

		event.LogHostTerminatedExternally(h.Id, h.Status)

		catcher := grip.NewBasicCatcher()
		if h.SpawnOptions.SpawnedByTask {
			if err := task.AddHostCreateDetails(h.SpawnOptions.TaskID, h.Id, h.SpawnOptions.TaskExecutionNumber, errors.New("host was externally terminated")); err != nil {
				catcher.Wrap(err, "error adding host create error details")
			}
		}
		catcher.Wrap(h.SetUnprovisioned(), "marking host as failed provisioning")
		catcher.Wrap(amboy.EnqueueUniqueJob(ctx, j.env.RemoteQueue(), NewHostTerminationJob(j.env, &h, true, "instance was found in stopped state")), "enqueueing job to terminate host")

		return catcher.Resolve()
	case cloud.StatusRunning:
		if err := j.initialSetup(ctx, m, &h); err != nil {
			return errors.Wrap(err, "problem doing initial setup")
		}
		catcher := grip.NewBasicCatcher()
		catcher.Wrapf(j.setNextState(ctx, &h), "transitioning host state")
		if h.UserHost && h.Distro.BootstrapSettings.Method == distro.BootstrapMethodUserData {
			catcher.Wrap(amboy.EnqueueUniqueJob(ctx, j.env.RemoteQueue(), NewUserDataDoneJob(j.env, h.Id, utility.RoundPartOfHour(1))), "enqueueing job to check when user data is done")
		}
		j.logHostStatusMessage(&h, cloudStatus)
		return catcher.Resolve()
	}

	grip.Info(message.Fields{
		"message":      "host not ready for setup",
		"host_id":      h.Id,
		"distro":       h.Distro.Id,
		"runner":       "hostinit",
		"cloud_status": cloudStatus.String(),
		"job":          j.ID(),
	})
	return nil
}

func (j *cloudHostReadyJob) setNextState(ctx context.Context, h *host.Host) error {
	switch h.Distro.BootstrapSettings.Method {
	case distro.BootstrapMethodUserData:
		// From the app server's perspective, it is done provisioning a user
		// data host once the instance is running. The user data script will
		// handle the rest of host provisioning.
		return errors.Wrap(h.SetProvisionedNotRunning(), "marking host as provisioned but not yet running")
	case distro.BootstrapMethodNone:
		// A host created by a task goes through no further provisioning, so we
		// can just set it as running.
		return errors.Wrap(h.MarkAsProvisioned(), "marking host as running")
	default:
		// All other host types must be manually provisioned by the app server.
		if err := h.SetProvisioning(); err != nil {
			return errors.Wrap(err, "marking host as provisioning")
		}

		setupJob := NewSetupHostJob(j.env, h, utility.RoundPartOfMinute(0).Format(TSFormat))
		if err := amboy.EnqueueUniqueJob(ctx, j.env.RemoteQueue(), setupJob); err != nil {
			grip.Warning(message.WrapError(err, message.Fields{
				"message":          "could not enqueue host setup job",
				"host_id":          h.Id,
				"job_id":           j.ID(),
				"enqueue_job_type": setupJob.Type(),
			}))
		}

		return nil
	}
}

func (j *cloudHostReadyJob) initialSetup(ctx context.Context, cloudMgr cloud.Manager, h *host.Host) error {
	if err := cloudMgr.OnUp(ctx, h); err != nil {
		return errors.Wrapf(err, "OnUp callback failed for host %s", h.Id)
	}
	return j.setDNSName(ctx, cloudMgr, h)
}

func (j *cloudHostReadyJob) setDNSName(ctx context.Context, cloudMgr cloud.Manager, h *host.Host) error {
	if h.Host != "" {
		return nil
	}

	hostDNS, err := cloudMgr.GetDNSName(ctx, h)
	if err != nil {
		return errors.Wrapf(err, "error checking DNS name for host %s", h.Id)
	}

	// sanity check for the host DNS name
	if hostDNS == "" {
		// DNS name not required if IP address set
		if h.IP != "" {
			return nil
		}
		return errors.Errorf("instance %s is running but not returning a DNS name or IP address", h.Id)
	}

	// update the host's DNS name
	if err = h.SetDNSName(hostDNS); err != nil {
		return errors.Wrapf(err, "error setting DNS name for host %s", h.Id)
	}

	return nil
}

// logHostStatusMessage logs the appropriate message once the status of a host's
// cloud instance is known and the host can transition to the next step in
// provisioning.
func (j *cloudHostReadyJob) logHostStatusMessage(h *host.Host, cloudStatus cloud.CloudStatus) {
	switch cloudStatus {
	case cloud.StatusStopped, cloud.StatusStopping:
		grip.Warning(message.Fields{
			"message":      "host was found in stopped state before it could transition to ready, which should not occur",
			"hypothesis":   "stopped by the AWS reaper",
			"host_id":      h.Id,
			"distro":       h.Distro.Id,
			"cloud_status": cloudStatus.String(),
			"job":          j.ID(),
		})
	case cloud.StatusTerminated:
		grip.Warning(message.Fields{
			"message":      "host's instance was terminated before it could transition to ready",
			"host_id":      h.Id,
			"distro":       h.Distro.Id,
			"cloud_status": cloudStatus.String(),
			"job":          j.ID(),
		})
	case cloud.StatusFailed:
		grip.Warning(message.Fields{
			"message":      "host's instance failed to start",
			"host_id":      h.Id,
			"distro":       h.Distro.Id,
			"cloud_status": cloudStatus.String(),
			"job":          j.ID(),
		})
	case cloud.StatusRunning:
		grip.Info(message.Fields{
			"message":      "host's instance was successfully found up and running",
			"host_id":      h.Id,
			"distro":       h.Distro.Id,
			"cloud_status": cloudStatus.String(),
			"job":          j.ID(),
		})
	default:
		grip.Error(message.Fields{
			"message":      "host's instance is in a state that the system does not know how to handle",
			"host_id":      h.Id,
			"distro":       h.Distro.Id,
			"cloud_status": cloudStatus.String(),
			"job":          j.ID(),
		})
	}
}
