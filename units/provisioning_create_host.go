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

const createHostJobName = "provisioning-create-host"

func init() {
	registry.AddJobType(createHostJobName, func() amboy.Job {
		return makeCreateHostJob()
	})
}

type createHostJob struct {
	HostID         string `bson:"host_id" json:"host_id" yaml:"host_id"`
	CurrentAttempt int    `bson:"current_attempt" json:"current_attempt" yaml:"current_attempt"`
	MaxAttempts    int    `bson:"max_attempts" json:"max_attempts" yaml:"max_attempts"`
	job.Base       `bson:"metadata" json:"metadata" yaml:"metadata"`

	start time.Time
	host  *host.Host
	env   evergreen.Environment
}

func makeCreateHostJob() *createHostJob {
	j := &createHostJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    createHostJobName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

func NewHostCreateJob(env evergreen.Environment, h host.Host, id string, CurrentAttempt int, MaxAttempts int) amboy.Job {
	j := makeCreateHostJob()
	j.host = &h
	j.HostID = h.Id
	j.env = env
	j.SetPriority(1)
	j.SetID(fmt.Sprintf("%s.%s.%s", createHostJobName, j.HostID, id))
	j.CurrentAttempt = CurrentAttempt
	if MaxAttempts > 0 {
		j.MaxAttempts = MaxAttempts
	} else if j.host.SpawnOptions.Retries > 0 {
		j.MaxAttempts = j.host.SpawnOptions.Retries
	} else {
		j.MaxAttempts = 1
	}
	return j
}

func (j *createHostJob) Run(ctx context.Context) {
	var err error
	defer j.MarkComplete()

	j.start = time.Now()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if j.host == nil {
		j.host, err = host.FindOneId(j.HostID)
		if err != nil {
			j.AddError(err)
			return
		}
		if j.host == nil {
			j.AddError(fmt.Errorf("could not find host %s for job %s", j.HostID, j.TaskID))
			return
		}
	}

	if j.TimeInfo().MaxTime == 0 && !j.host.SpawnOptions.TimeoutSetup.IsZero() {
		j.UpdateTimeInfo(amboy.JobTimeInfo{
			MaxTime: j.host.SpawnOptions.TimeoutSetup.Sub(j.start),
		})
	}

	j.AddError(j.createHost(ctx))
}

func (j *createHostJob) createHost(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return errors.Wrap(err, "canceling create host because context is canceled")
	}
	hostStartTime := j.start
	grip.Info(message.Fields{
		"message":      "attempting to start host",
		"hostid":       j.host.Id,
		"job":          j.ID(),
		"attempt":      j.CurrentAttempt,
		"max_attempts": j.MaxAttempts,
	})

	cloudManager, err := cloud.GetManager(ctx, j.host.Provider, j.env.Settings())
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "problem getting cloud provider for host",
			"host":    j.host.Id,
			"job":     j.ID(),
		}))
		return errors.Wrapf(errIgnorableCreateHost, "problem getting cloud provider for host '%s' [%s]", j.host.Id, err.Error())
	}

	// On the first attempt, remove the intent document so no other create host job tries to create this intent.
	if j.CurrentAttempt == 1 {
		if err := j.host.Remove(); err != nil {
			grip.Notice(message.WrapError(err, message.Fields{
				"message": "problem removing intent host",
				"job":     j.ID(),
				"host":    j.host.Id,
			}))
			return errors.Wrapf(errIgnorableCreateHost, "problem removing intent host '%s' [%s]", j.host.Id, err.Error())
		}
	}

	defer j.tryRequeue(ctx)
	if _, err = cloudManager.SpawnHost(ctx, j.host); err != nil {
		return errors.Wrapf(err, "error spawning host %s", j.host.Id)
	}

	j.host.Status = evergreen.HostStarting

	// Provisionally set j.host.StartTime to now. Cloud providers may override
	// this value with the time the host was created.
	j.host.StartTime = j.start

	if err := j.host.Insert(); err != nil {
		return errors.Wrapf(err, "error updating host %v", j.host.Id)
	}

	grip.Info(message.Fields{
		"message": "successfully started host",
		"hostid":  j.host.Id,
		"job":     j.ID(),
		"DNS":     j.host.Host,
		"runtime": time.Since(hostStartTime),
	})

	return nil
}

func (j *createHostJob) tryRequeue(ctx context.Context) {
	if j.shouldRetryCreateHost(ctx) && j.env.RemoteQueue().Started() {
		job := NewHostCreateJob(j.env, *j.host, fmt.Sprintf("attempt-%d", j.CurrentAttempt+1), j.CurrentAttempt+1, j.MaxAttempts)
		job.UpdateTimeInfo(amboy.JobTimeInfo{
			WaitUntil: j.start.Add(time.Minute),
			MaxTime:   j.TimeInfo().MaxTime - (time.Now().Sub(j.start)) - time.Minute,
		})
		err := j.env.RemoteQueue().Put(job)
		grip.Critical(message.WrapError(err, message.Fields{
			"message":  "failed to requeue setup job",
			"host":     j.host.Id,
			"job":      j.ID(),
			"distro":   j.host.Distro.Id,
			"attempts": j.host.ProvisionAttempts,
		}))
		j.AddError(err)
	}
}

func (j *createHostJob) shouldRetryCreateHost(ctx context.Context) bool {
	return j.CurrentAttempt < j.MaxAttempts &&
		j.host.Status != evergreen.HostStarting &&
		ctx.Err() == nil
}
