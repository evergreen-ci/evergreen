package operations

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"time"

	"github.com/evergreen-ci/evergreen/agent/globals"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	// microVMConfigPath is the PoC opt-in switch for running the agent inside
	// a Firecracker microVM. When the file exists, the monitor routes the
	// agent through the microVM launcher instead of starting it as a host
	// process. Production would replace this with a distro-level setting.
	microVMConfigPath = "/etc/evergreen/microvm.json"

	// defaultMicroVMLauncherPath is the launcher used when the config file
	// does not override it.
	defaultMicroVMLauncherPath = "/usr/local/bin/evg-microvm"
)

// microVMSettings configures microVM-backed agent execution.
type microVMSettings struct {
	// LauncherPath is the path to the root-owned launcher binary.
	LauncherPath string `json:"launcher_path"`
}

// loadMicroVMSettings returns microVM settings when the distro opts into
// microVM-backed agent execution via the config file, and nil otherwise. A
// missing file is the normal host-mode case, not an error. A malformed file
// falls back to host mode with a warning, consistent with the fail-open
// behavior of container isolation.
func loadMicroVMSettings(ctx context.Context, path string) *microVMSettings {
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			grip.Error(ctx, errors.Wrapf(err, "reading microVM config '%s', falling back to host mode", path))
		}
		return nil
	}
	settings := &microVMSettings{LauncherPath: defaultMicroVMLauncherPath}
	if err := json.Unmarshal(data, settings); err != nil {
		grip.Error(ctx, errors.Wrapf(err, "parsing microVM config '%s', falling back to host mode", path))
		return nil
	}
	return settings
}

// agentCmdArgsForMicroVM returns the command for starting the agent inside a
// fresh microVM instead of as a host process. The agent exits at the
// task-group boundary, which terminates the guest, and the monitor then boots
// a fresh one for the next task. The launcher is root-owned because it needs
// /dev/kvm and network administration privileges to boot the VM; sudo -E
// forwards the host ID and setup secret environment variables to it.
func (m *monitor) agentCmdArgsForMicroVM() []string {
	agentCmdArgs := m.agentCmdArgs()

	// The agent's file logs die with the microVM, so log to stdout instead;
	// the launcher captures the VM console on the host.
	args := make([]string, 0, len(agentCmdArgs)+2)
	logOutputArg := "--log_output=" + string(globals.LogOutputFile)
	replacedLogOutput := false
	for _, arg := range agentCmdArgs {
		if arg == logOutputArg {
			args = append(args, "--log_output="+string(globals.LogOutputStdout))
			replacedLogOutput = true
			continue
		}
		args = append(args, arg)
	}
	if !replacedLogOutput {
		args = append(args, "--log_output="+string(globals.LogOutputStdout))
	}
	args = append(args, "--"+exitAfterTaskGroupFlagName)

	return append([]string{"sudo", "-E", m.microvm.LauncherPath, "run", "--client", m.clientPath, "--"}, args...)
}

// cleanupOrphanMicroVMs removes leftover microVM state (disk clones, TAP
// devices, Firecracker processes) from a previous monitor run before normal
// agent dispatch resumes.
func (m *monitor) cleanupOrphanMicroVMs(ctx context.Context) {
	cleanupCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	cmd := exec.CommandContext(cleanupCtx, "sudo", "-E", m.microvm.LauncherPath, "cleanup")
	output, err := cmd.CombinedOutput()
	if err != nil {
		grip.Error(ctx, errors.Wrapf(err, "cleaning up orphaned microVMs: %s", output))
	}
}
