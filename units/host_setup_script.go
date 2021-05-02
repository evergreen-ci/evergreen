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

// NewHostSetupScriptJob creates a job that executes the spawn host's setup
// script after task data is loaded onto the host.
func NewHostSetupScriptJob(env evergreen.Environment, h *host.Host) amboy.Job {
	j := makeHostSetupScriptJob()
	j.env = env
	j.host = h
	j.HostID = h.Id
	j.SetPriority(1)
	j.SetScopes([]string{fmt.Sprintf("%s.%s", hostSetupScriptJobName, h.Id)})
	j.SetShouldApplyScopesOnEnqueue(true)
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(setupScriptRetryLimit),
	})
	j.SetID(fmt.Sprintf("%s.%s", hostSetupScriptJobName, j.HostID))
	return j
}

func (j *hostSetupScriptJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	defer func() {
		if j.HasErrors() && (j.RetryInfo().GetRemainingAttempts() == 0 || !j.RetryInfo().ShouldRetry()) {
			event.LogHostScriptExecuteFailed(j.HostID, j.Error())
		}
	}()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if j.host == nil {
		var err error
		j.host, err = host.FindOneByIdOrTag(j.HostID)
		if err != nil {
			j.AddRetryableError(err)
			return
		}
		if j.host == nil {
			j.AddRetryableError(fmt.Errorf("could not find host '%s' for job '%s'", j.HostID, j.ID()))
			return
		}
		if j.host.ProvisionOptions == nil || j.host.ProvisionOptions.SetupScript == "" {
			j.AddError(fmt.Errorf("host doesn't contain a setup script"))
			return
		}
	}

	if j.host.ProvisionOptions.TaskId != "" {
		if err := j.host.CheckTaskDataFetched(ctx, j.env); err != nil {
			j.AddRetryableError(errors.Wrap(err, "checking if task data is fetched yet"))
			return
		}
	}

	// Do not retry after the setup script executes and fails because there's no
	// guarantee that the script is idempotent.
	j.AddError(errors.Wrap(runSpawnHostSetupScript(ctx, j.env, j.host), "executing spawn host setup script"))
}

func runSpawnHostSetupScript(ctx context.Context, env evergreen.Environment, h *host.Host) error {
	script := fmt.Sprintf("cd %s\n%s", h.Distro.HomeDir(), h.ProvisionOptions.SetupScript)
	ts := utility.RoundPartOfMinute(0).Format(TSFormat)
	j := NewHostExecuteJob(env, *h, script, false, "", ts)
	j.Run(ctx)

	return errors.Wrapf(j.Error(), "error running setup script for spawn host")
}
