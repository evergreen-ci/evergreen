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
)

const (
	hostSetupScriptJobName = "host-setup-script"
)

func init() {
	registry.AddJobType(hostSetupScriptJobName, func() amboy.Job { return makeHostSetupScriptJob() })
}

type hostSetupScriptJob struct {
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`

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
func NewHostSetupScriptJob(env evergreen.Environment, h host.Host) amboy.Job {
	j := makeHostSetupScriptJob()
	j.env = env
	j.host = &h
	j.HostID = h.Id
	j.SetPriority(1)
	ts := utility.RoundPartOfHour(2).Format(TSFormat)
	j.SetID(fmt.Sprintf("%s.%s.%s", hostSetupScriptJobName, j.HostID, ts))
	return j
}

func (j *hostSetupScriptJob) Run(ctx context.Context) {
	defer j.MarkComplete()

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
		if err := j.host.CheckProvisioningHostFinished(ctx, j.env); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "failed to finish provisioning",
				"task":    j.host.ProvisionOptions.TaskId,
				"user":    j.host.StartedBy,
				"host_id": j.host.Id,
				"job":     j.ID(),
			}))
			j.AddError(err)
		}
	}
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
		return
	}
	logs := ""
	if j.HasErrors() {
		logs = j.Error().Error()
	}
	event.LogHostSetupScriptFinished(j.HostID, logs)
}
