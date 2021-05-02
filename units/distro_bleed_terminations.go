package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	hostBleedJobName = "host-bleedoff"

	// if we need to bleed hosts, we want to catch as many hosts as we can that are between jobs
	idleTimeBleedCutoff = 5 * time.Second
)

func init() {
	registry.AddJobType(hostBleedJobName, func() amboy.Job {
		return makeHostBleedJob()
	})
}

type BleedInfo struct {
	DistroID     string
	NewCapTarget int
}

type hostBleedJob struct {
	job.Base        `bson:"metadata" json:"metadata" yaml:"metadata"`
	Terminated      int      `bson:"terminated" json:"terminated" yaml:"terminated"`
	TerminatedHosts []string `bson:"terminated_hosts" json:"terminated_hosts" yaml:"terminated_hosts"`

	env      evergreen.Environment
	settings *evergreen.Settings
	host     *host.Host

	BleedInfo BleedInfo
}

func makeHostBleedJob() *hostBleedJob {
	j := &hostBleedJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    hostBleedJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	j.SetPriority(2)

	return j
}

func NewHostBleedJob(env evergreen.Environment, bleedInfo BleedInfo, id string) amboy.Job {
	j := makeHostBleedJob()
	j.BleedInfo = bleedInfo
	j.env = env
	j.SetID(fmt.Sprintf("%s.%s", hostBleedJobName, id))
	return j
}

func (j *hostBleedJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if j.settings == nil {
		j.settings = j.env.Settings()
	}

	if j.HasErrors() {
		return
	}

	// get currently existing hosts, in case some hosts have already been terminated elsewhere
	existingHostCount, err := host.CountRunningHosts(j.BleedInfo.DistroID)
	if err != nil {
		j.AddError(errors.Wrap(err, "database error getting existing hosts by Distro.Id"))
		return
	}
	idleHosts, err := host.IdleHostsWithDistroID(j.BleedInfo.DistroID)
	if err != nil {
		j.AddError(errors.Wrap(err, "database error getting idle hosts by Distro.Id"))
		return
	}
	bleedTarget := existingHostCount - j.BleedInfo.NewCapTarget

	for i := 0; i < len(idleHosts); i++ {
		j.AddError(j.checkAndTerminateHost(ctx, &idleHosts[i], &bleedTarget))
		if bleedTarget == 0 {
			break
		}
	}

	grip.Info(message.Fields{
		"id":                   j.ID(),
		"job_type":             hostBleedJobName,
		"distro_id":            j.BleedInfo.DistroID,
		"num_idle_hosts":       len(idleHosts),
		"num_terminated_hosts": j.Terminated,
		"terminated_hosts":     j.TerminatedHosts,
	})
}

func (j *hostBleedJob) checkAndTerminateHost(ctx context.Context, h *host.Host, bleedTarget *int) error {
	if !h.IsEphemeral() {
		grip.Notice(message.Fields{
			"job":      j.ID(),
			"host_id":  h.Id,
			"job_type": j.Type().Name,
			"status":   h.Status,
			"provider": h.Distro.Provider,
			"message":  "host termination for a non-ephemeral distro",
			"cause":    "programmer error",
		})
		return errors.New("attempted to terminate non-ephemeral host")
	}

	// ask the host how long it has been idle
	idleTime := h.IdleTime()

	// if the communication time is > 10 mins then there may not be an agent on the host.
	communicationTime := h.GetElapsedCommunicationTime()

	// get a cloud manager for the host
	mgrOpts, err := cloud.GetManagerOptions(h.Distro)
	if err != nil {
		return errors.Wrapf(err, "can't get ManagerOpts for host '%s'", h.Id)
	}
	manager, err := cloud.GetManager(ctx, j.env, mgrOpts)
	if err != nil {
		return errors.Wrapf(err, "error getting cloud manager for host %v", h.Id)
	}

	// ask how long until the next payment for the host
	tilNextPayment := manager.TimeTilNextPayment(h)

	if tilNextPayment > maxTimeTilNextPayment {
		return nil
	}

	if h.IsWaitingForAgent() && (communicationTime < idleWaitingForAgentCutoff || idleTime < idleWaitingForAgentCutoff) {
		grip.Notice(message.Fields{
			"op":                j.Type().Name,
			"id":                j.ID(),
			"message":           "not flagging idle host, waiting for an agent",
			"host_id":           h.Id,
			"distro":            h.Distro.Id,
			"idle":              idleTime.String(),
			"last_communicated": communicationTime.String(),
		})
		return nil
	}

	idleThreshold := idleTimeBleedCutoff
	if h.RunningTaskGroup != "" {
		idleThreshold = idleTaskGroupHostCutoff
	}

	if idleTime > idleThreshold {
		terminateReason := fmt.Sprintf("host is being terminated due to temporary overallocation of hosts")
		(*bleedTarget)--
		j.Terminated++
		j.TerminatedHosts = append(j.TerminatedHosts, h.Id)
		return j.env.RemoteQueue().Put(ctx, NewHostTerminationJob(j.env, h, false, terminateReason))
	}
	return nil
}
