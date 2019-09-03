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
	idleHostJobName = "idle-host-termination"

	// idleTimeCutoff is the amount of time we wait for an idle host to be marked as idle.
	idleTimeCutoff            = 4 * time.Minute
	idleWaitingForAgentCutoff = 10 * time.Minute
	idleTaskGroupHostCutoff   = 10 * time.Minute

	// MaxTimeNextPayment is the amount of time we wait to have left before marking a host as idle
	maxTimeTilNextPayment = 5 * time.Minute
)

func init() {
	registry.AddJobType(idleHostJobName, func() amboy.Job {
		return makeIdleHostJob()
	})
}

type idleHostJob struct {
	HostID     string `bson:"host" json:"host" yaml:"host"`
	job.Base   `bson:"metadata" json:"metadata" yaml:"metadata"`
	Terminated bool `bson:"terminated" json:"terminated" yaml:"terminated"`

	env      evergreen.Environment
	settings *evergreen.Settings
	host     *host.Host
}

func makeIdleHostJob() *idleHostJob {
	j := &idleHostJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    idleHostJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	j.SetPriority(2)

	return j
}

func NewIdleHostTerminationJob(env evergreen.Environment, h host.Host, id string) amboy.Job {
	j := makeIdleHostJob()
	j.host = &h
	j.HostID = h.Id
	j.SetID(fmt.Sprintf("%s.%s.%s", idleHostJobName, j.HostID, id))
	return j
}

func (j *idleHostJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	var err error
	if j.host == nil {
		j.host, err = host.FindOneId(j.HostID)
		j.AddError(err)
		if err != nil {
			return
		} else if j.host == nil {
			j.AddError(errors.Errorf("unable to retrieve host %s", j.HostID))
			return
		}
	}
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if j.settings == nil {
		j.settings = j.env.Settings()
	}

	if j.HasErrors() {
		return
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
		j.AddError(errors.New("non-spawnable host"))
		return
	}

	// ask the host how long it has been idle
	idleTime := j.host.IdleTime()

	// if the communication time is > 10 mins then there may not be an agent on the host.
	communicationTime := j.host.GetElapsedCommunicationTime()

	// get a cloud manager for the host
	manager, err := cloud.GetManager(ctx, j.host.Provider, j.host.Distro.ProviderSettings, j.settings)
	if err != nil {
		j.AddError(errors.Wrapf(err, "error getting cloud manager for host %v", j.host.Id))
		return
	}

	// ask how long until the next payment for the host
	tilNextPayment := manager.TimeTilNextPayment(j.host)

	if tilNextPayment > maxTimeTilNextPayment {
		return
	}

	if j.host.IsWaitingForAgent() && (communicationTime < idleWaitingForAgentCutoff || idleTime < idleWaitingForAgentCutoff) {
		grip.Notice(message.Fields{
			"op":                j.Type().Name,
			"id":                j.ID(),
			"message":           "not flagging idle host, waiting for an agent",
			"host":              j.host.Id,
			"distro":            j.host.Distro.Id,
			"idle":              idleTime.String(),
			"last_communicated": communicationTime.String(),
		})
		return
	}

	idleThreshold := idleTimeCutoff
	if j.host.RunningTaskGroup != "" {
		idleThreshold = idleTaskGroupHostCutoff
	}

	// if we haven't heard from the host or it's been idle for longer than the cutoff, we should terminate
	if communicationTime >= idleThreshold || idleTime >= idleThreshold {
		j.Terminated = true
		tjob := NewHostTerminationJob(j.env, *j.host, false)
		queue := j.env.RemoteQueue()
		j.AddError(queue.Put(ctx, tjob))
	}
}
