package units

import (
	"bytes"
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

	// setupScriptOutputBufferSize is the maximum number of log lines captured
	// from a host execute script. It is much larger than the default
	// host.OutputBufferSize so that engineers can see the full setup script
	// output in the host event logs, rather than only the final lines.
	setupScriptOutputBufferSize = 100 * 1000

	// maxScriptLogsSize bounds the script output stored in the host event log
	// and sent to Splunk, matching the cap on the SSH execution path
	// (util.NewMBCappedWriter).
	maxScriptLogsSize = 1024 * 1024
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
		grip.Debug(ctx, message.Fields{
			"message": "host is down, not attempting to run script",
			"host_id": j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		})
		return
	}

	var logs string
	if !j.host.Distro.LegacyBootstrap() {
		scriptFilename := fmt.Sprintf("evg-execute-%s-%s.sh", j.host.Id, j.ID())
		// Jasper's WriteFile RPC needs a windows-native path
		nativeScriptPath := j.host.Distro.AbsPathNotCygwinCompatible("/tmp", scriptFilename)
		shellScriptPath := "/tmp/" + scriptFilename

		if err := j.host.WriteJasperFile(ctx, j.env, options.WriteFile{
			Path:   nativeScriptPath,
			Reader: bytes.NewReader([]byte(j.Script)),
			Perm:   0700,
		}); err != nil {
			j.AddError(errors.Wrap(err, "writing script to temp file on host"))
			return
		}

		defer func() {
			if _, cleanupErr := j.host.RunJasperProcess(ctx, j.env, &options.Create{
				Args: []string{j.host.Distro.ShellBinary(), "-c", fmt.Sprintf("rm -f %s", shellScriptPath)},
			}); cleanupErr != nil {
				grip.Warning(ctx, message.WrapError(cleanupErr, message.Fields{
					"message":     "could not clean up temp script file",
					"script_path": shellScriptPath,
					"host_id":     j.host.Id,
					"job":         j.ID(),
				}))
			}
		}()

		var args []string
		if !j.host.Distro.IsWindows() && j.Sudo {
			args = append(args, "sudo")
			if j.SudoUser != "" {
				args = append(args, fmt.Sprintf("--user=%s", j.SudoUser))
			}
		}
		args = append(args, j.host.Distro.ShellBinary(), "-l", shellScriptPath)

		output, err := j.host.RunJasperProcessWithOutputSize(ctx, j.env, &options.Create{
			Args: args,
		}, setupScriptOutputBufferSize)
		logs = truncateScriptLogs(strings.Join(output, "\n"))
		if err != nil {
			event.LogHostScriptExecuteFailed(ctx, j.host.Id, logs, err)
			grip.Error(ctx, message.WrapError(err, message.Fields{
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
			event.LogHostScriptExecuteFailed(ctx, j.host.Id, logs, err)
			grip.Error(ctx, message.WrapError(err, message.Fields{
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

	event.LogHostScriptExecuted(ctx, j.host.Id, logs)

	grip.Info(ctx, message.Fields{
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

// truncateScriptLogs bounds the script output stored in the host event log and
// sent to Splunk. If the output exceeds maxScriptLogsSize, it keeps the
// beginning and end (where the initial setup steps and any final error
// typically appear) and elides the middle.
func truncateScriptLogs(logs string) string {
	if len(logs) <= maxScriptLogsSize {
		return logs
	}
	const marker = "\n...[log truncated]...\n"
	keep := (maxScriptLogsSize - len(marker)) / 2
	return logs[:keep] + marker + logs[len(logs)-keep:]
}
