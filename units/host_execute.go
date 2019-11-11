package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
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
	hostExecuteJobName = "host-execute"
)

func init() {
	registry.AddJobType(hostExecuteJobName, func() amboy.Job { return makeHostExecuteJob() })
}

type hostExecuteJob struct {
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`
	Script   string `bson:"script" json:"script" yaml:"script"`
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`

	host *host.Host
	env  evergreen.Environment
}

func makeHostExecuteJob() *hostExecuteJob {
	j := &hostExecuteJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    hostExecuteJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

// NewHostExecuteJob creates a job that executes a script on the host.
func NewHostExecuteJob(env evergreen.Environment, h host.Host, script string, id string) amboy.Job {
	j := makeHostExecuteJob()
	j.env = env
	j.host = &h
	j.HostID = h.Id
	j.Script = script
	j.SetPriority(1)
	j.SetID(fmt.Sprintf("%s.%s.%s", hostExecuteJobName, j.HostID, id))
	return j
}

func (j *hostExecuteJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if err := j.populateIfUnset(); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not populate fields",
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
		j.AddError(err)
		return
	}

	if j.host.Status != evergreen.HostRunning {
		grip.Debug(message.Fields{
			"message": "host is down, not attempting to run script",
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		})
		return
	}

	sshOptions, err := j.host.GetSSHOptions(j.env.Settings())
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not get ssh options",
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
		j.AddError(err)
		return
	}

	logs, err := j.host.RunSSHShellScript(ctx, j.Script, sshOptions)
	if err != nil {
		event.LogHostScriptExecuteFailed(j.host.Id, err)
		grip.Error(message.WrapError(err, message.Fields{
			"message": "script failed during execution",
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"logs":    logs,
			"job":     j.ID(),
		}))
		j.AddError(err)
		return
	}

	event.LogHostScriptExecuted(j.host.Id, logs)

	grip.Info(message.Fields{
		"message": "host executed script successfully",
		"host":    j.host.Id,
		"distro":  j.host.Distro.Id,
		"logs":    logs,
		"job":     j.ID(),
	})
}

// populateIfUnset populates the unset job fields.
func (j *hostExecuteJob) populateIfUnset() error {
	if j.host == nil {
		h, err := host.FindOneId(j.HostID)
		if err != nil {
			return errors.Wrapf(err, "could not find host %s for job %s", j.HostID, j.ID())
		}
		if h == nil {
			return errors.Errorf("could not find host %s for job %s", j.HostID, j.ID())
		}
		j.host = h
	}

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	return nil
}
