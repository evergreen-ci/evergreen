package units

import (
	"context"
	"fmt"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/jasper/options"
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
	Sudo     bool   `bson:"sudo" json:"sudo" yaml:"sudo"`
	SudoUser string `bson:"sudo_user" json:"sudo_user" yaml:"sudo_user"`
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
	return j
}

// NewHostExecuteJob creates a job that executes a script on the host.
func NewHostExecuteJob(env evergreen.Environment, h host.Host, script string, sudo bool, sudoUser string, id string) amboy.Job {
	j := makeHostExecuteJob()
	j.env = env
	j.host = &h
	j.HostID = h.Id
	j.Script = script
	j.Sudo = sudo
	j.SudoUser = sudoUser
	j.SetID(fmt.Sprintf("%s.%s.%s", hostExecuteJobName, j.HostID, id))
	return j
}

func (j *hostExecuteJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if err := j.populateIfUnset(ctx); err != nil {
		j.AddError(err)
		return
	}

	if j.host.Status != evergreen.HostRunning {
		grip.Debug(message.Fields{
			"message": "host is down, not attempting to run script",
			"host_id": j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		})
		return
	}

	var logs string
	if !j.host.Distro.LegacyBootstrap() {
		var args []string
		if !j.host.Distro.IsWindows() && j.Sudo {
			args = append(args, "sudo")
			if j.SudoUser != "" {
				args = append(args, fmt.Sprintf("--user=%s", j.SudoUser))
			}
		}
		// We read the shell script verbatim from stdin  (i.e. with "bash -s"
		// instead of "bash -c") to avoid a Windows limitation on exec string length.
		args = append(args, j.host.Distro.ShellBinary(), "-s", "-l")
		var output []string
		output, err := j.host.RunJasperProcess(ctx, j.env, &options.Create{
			Args:               args,
			StandardInputBytes: []byte(j.Script),
		})
		logs = strings.Join(output, "\n")
		if err != nil {
			event.LogHostScriptExecuteFailed(j.host.Id, logs, err)
			grip.Error(message.WrapError(err, message.Fields{
				"message":          "script failed during execution",
				"legacy_bootstrap": j.host.Distro.LegacyBootstrap(),
				"host_id":          j.host.Id,
				"distro":           j.host.Distro.Id,
				"logs":             logs,
				"job":              j.ID(),
			}))
			j.AddError(err)
			return
		}
	} else {
		var err error
		logs, err = j.host.RunSSHShellScript(ctx, j.Script, j.Sudo, j.SudoUser)
		if err != nil {
			event.LogHostScriptExecuteFailed(j.host.Id, logs, err)
			grip.Error(message.WrapError(err, message.Fields{
				"message": "script failed during execution",
				"host_id": j.host.Id,
				"distro":  j.host.Distro.Id,
				"logs":    logs,
				"job":     j.ID(),
			}))
			j.AddError(err)
			return
		}
	}

	event.LogHostScriptExecuted(j.host.Id, logs)

	grip.Info(message.Fields{
		"message": "host executed script successfully",
		"host_id": j.host.Id,
		"distro":  j.host.Distro.Id,
		"logs":    logs,
		"job":     j.ID(),
	})
}

// populateIfUnset populates the unset job fields.
func (j *hostExecuteJob) populateIfUnset(ctx context.Context) error {
	if j.host == nil {
		h, err := host.FindOneId(ctx, j.HostID)
		if err != nil {
			return errors.Wrapf(err, "finding host '%s'", j.HostID)
		}
		if h == nil {
			return errors.Errorf("host '%s' not found", j.HostID)
		}
		j.host = h
	}

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	return nil
}
