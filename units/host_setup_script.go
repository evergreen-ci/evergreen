package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	hostSetupScriptJobName = "host-setup-script"
	setupScriptRetryLimit  = 5
)

func init() {
	registry.AddJobType(hostSetupScriptJobName, func() amboy.Job { return makeHostSetupScriptJob() })
}

type hostSetupScriptJob struct {
	HostID         string `bson:"host_id" json:"host_id" yaml:"host_id"`
	job.Base       `bson:"job_base" json:"job_base" yaml:"job_base"`
	CurrentAttempt int `bson:"current_attempt" json:"current_attempt" yaml:"current_attempt"`

	host *host.Host
	env  evergreen.Environment
}

func makeHostSetupScriptJob() *hostSetupScriptJob {
	j := &hostSetupScriptJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    hostSetupScriptJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

// NewHostSetupScriptJob creates a job that executes the setup script after task data is loaded onto the host.
func NewHostSetupScriptJob(env evergreen.Environment, attempt int, h host.Host) amboy.Job {
	j := makeHostSetupScriptJob()
	j.env = env
	j.host = &h
	j.HostID = h.Id
	j.CurrentAttempt = attempt
	j.SetPriority(1)
	ts := utility.RoundPartOfHour(2).Format(TSFormat)
	j.SetID(fmt.Sprintf("%s.%s.%s", hostSetupScriptJobName, j.HostID, ts))
	return j
}

func (j *hostSetupScriptJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	defer func() {
		if j.HasErrors() {
			if err := j.tryRequeue(ctx); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message": "could not enqueue spawn host setup script job",
					"host_id": j.host.Id,
					"distro":  j.host.Distro.Id,
					"job":     j.ID(),
				}))
				j.AddError(err)
			}
		}
	}()
	if j.host == nil {
		var err error
		j.host, err = host.FindOneByIdOrTag(j.HostID)
		if err != nil {
			j.AddError(err)
			return
		}
		if j.host == nil {
			j.AddError(fmt.Errorf("could not find host %s for job %s", j.HostID, j.ID()))
			return
		}
		if j.host.ProvisionOptions == nil || j.host.ProvisionOptions.SetupScript == "" {
			j.AddError(fmt.Errorf("host doesn't contain a setup script"))
			return
		}
	}

	if j.host.ProvisionOptions.TaskId != "" {
		if err := j.host.CheckTaskDataFetched(ctx, j.env); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "failed to check for task data",
				"task":    j.host.ProvisionOptions.TaskId,
				"user":    j.host.StartedBy,
				"host_id": j.host.Id,
				"job":     j.ID(),
			}))
			j.AddError(err)
			return
		}
	}
	logs := ""
	if err := runSpawnHostSetupScript(ctx, j.env, *j.host); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":      "failed to run setup script",
			"task":         j.host.ProvisionOptions.TaskId,
			"setup_script": j.host.ProvisionOptions.SetupScript,
			"user":         j.host.StartedBy,
			"host_id":      j.host.Id,
			"job":          j.ID(),
		}))
		j.AddError(err)
		logs = err.Error()
	}

	event.LogHostSetupScriptFinished(j.HostID, logs)
}

func (j *hostSetupScriptJob) tryRequeue(ctx context.Context) error {
	if j.CurrentAttempt >= setupScriptRetryLimit {
		return errors.Errorf("exceeded max retries for setup script (%d)", setupScriptRetryLimit)
	}
	return j.env.RemoteQueue().Put(ctx, NewHostSetupScriptJob(j.env, j.CurrentAttempt+1, *j.host))
}
