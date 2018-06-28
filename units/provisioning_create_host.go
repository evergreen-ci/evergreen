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
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`

	host *host.Host
	env  evergreen.Environment
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

func NewHostCreateJob(env evergreen.Environment, h host.Host, id string) amboy.Job {
	j := makeCreateHostJob()
	j.host = &h
	j.HostID = h.Id
	j.env = env
	j.SetPriority(1)
	j.SetID(fmt.Sprintf("%s.%s.%s", createHostJobName, j.HostID, id))
	return j
}

func (j *createHostJob) Run(ctx context.Context) {
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
			return
		}
	}

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	settings := j.env.Settings()

	err = j.createHost(ctx, j.host, settings)
	j.AddError(err)
}

func (j *createHostJob) createHost(ctx context.Context, h *host.Host, settings *evergreen.Settings) error {
	hostStartTime := time.Now()
	grip.Info(message.Fields{
		"message": "attempting to start host",
		"hostid":  h.Id,
		"job":     j.ID(),
	})

	cloudManager, err := cloud.GetManager(ctx, h.Provider, settings)
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "problem getting cloud provider for host",
			"host":    h.Id,
			"job":     j.ID(),
		}))
		return errors.Wrapf(errIgnorableCreateHost, "problem getting cloud provider for host '%s' [%s]", h.Id, err.Error())
	}

	if err = h.Remove(); err != nil {
		grip.Notice(message.WrapError(err, message.Fields{
			"message": "problem removing intent host",
			"job":     j.ID(),
			"host":    h.Id,
		}))
		return errors.Wrapf(errIgnorableCreateHost, "problem getting cloud provider for host '%s' [%s]", h.Id, err.Error())
	}

	_, err = cloudManager.SpawnHost(ctx, h)
	if err != nil {
		return errors.Wrapf(err, "error spawning host %s", h.Id)
	}

	h.Status = evergreen.HostStarting

	// Provisionally set h.StartTime to now. Cloud providers may override
	// this value with the time the host was created.
	h.StartTime = time.Now()

	_, err = h.Upsert()
	if err != nil {
		return errors.Wrapf(err, "error updating host %v", h.Id)
	}

	grip.Info(message.Fields{
		"message": "successfully started host",
		"hostid":  h.Id,
		"job":     j.ID(),
		"DNS":     h.Host,
		"runtime": time.Since(hostStartTime),
	})

	return nil
}
