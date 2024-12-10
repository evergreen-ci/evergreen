package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	hostSetupScriptJobName = "host-setup-script"
	setupScriptRetryLimit  = 5

	// maxSpawnHostSetupScriptCheckDuration is the total amount of time that the
	// spawn host setup script job can poll to see if the task data is loaded.
	maxSpawnHostSetupScriptCheckDuration = 10 * time.Minute
	// maxSpawnHostSetupScriptDuration is the total amount of time that the
	// spawn host setup script can run after task data is loaded.
	maxSpawnHostSetupScriptDuration = 30 * time.Minute
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
	return j
}

// NewHostSetupScriptJob creates a job that executes the spawn host's setup
// script after task data is loaded onto the host.
func NewHostSetupScriptJob(env evergreen.Environment, h *host.Host) amboy.Job {
	j := makeHostSetupScriptJob()
	j.env = env
	j.host = h
	j.HostID = h.Id
	j.SetScopes([]string{fmt.Sprintf("%s.%s", hostSetupScriptJobName, h.Id)})
	j.SetEnqueueAllScopes(true)
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(setupScriptRetryLimit),
	})
	j.SetTimeInfo(amboy.JobTimeInfo{
		MaxTime: maxSpawnHostSetupScriptCheckDuration + maxSpawnHostSetupScriptDuration,
	})
	j.SetID(fmt.Sprintf("%s.%s", hostSetupScriptJobName, j.HostID))
	return j
}

func (j *hostSetupScriptJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	defer func() {
		if j.HasErrors() && j.IsLastAttempt() {
			grip.Error(message.WrapError(j.Error(), message.Fields{
				"message": "setup script failed during execution",
				"host_id": j.host.Id,
				"distro":  j.host.Distro.Id,
				"job":     j.ID(),
			}))
			event.LogHostScriptExecuteFailed(j.HostID, "", j.Error())
		}
	}()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if j.host == nil {
		var err error
		j.host, err = host.FindOneByIdOrTag(ctx, j.HostID)
		if err != nil {
			j.AddRetryableError(errors.Wrapf(err, "finding host '%s'", j.HostID))
			return
		}
		if j.host == nil {
			j.AddRetryableError(errors.Errorf("host '%s' not found", j.HostID))
			return
		}
		if j.host.ProvisionOptions == nil || j.host.ProvisionOptions.SetupScript == "" {
			j.AddError(errors.Errorf("host '%s' does not have a setup script", j.host.Id))
			return
		}
	}

	if j.host.ProvisionOptions.TaskId != "" {
		checkCtx, checkCancel := context.WithTimeout(ctx, maxSpawnHostSetupScriptCheckDuration)
		defer checkCancel()
		if err := j.host.CheckTaskDataFetched(checkCtx, j.env); err != nil {
			j.AddRetryableError(errors.Wrap(err, "checking if task data is fetched yet"))
			return
		}
	}

	// Do not retry after the setup script executes and fails because there's no
	// guarantee that the script is idempotent.
	j.AddError(errors.Wrap(runSpawnHostSetupScript(ctx, j.env, j.host), "executing spawn host setup script"))
}

func runSpawnHostSetupScript(ctx context.Context, env evergreen.Environment, h *host.Host) error {
	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, maxSpawnHostSetupScriptDuration)
	defer timeoutCancel()

	script := fmt.Sprintf("cd %s\n%s", h.Distro.HomeDir(), h.ProvisionOptions.SetupScript)
	ts := utility.RoundPartOfMinute(0).Format(TSFormat)
	j := NewHostExecuteJob(env, *h, script, false, "", ts)
	j.Run(timeoutCtx)

	return errors.Wrapf(j.Error(), "running setup script for spawn host")
}
