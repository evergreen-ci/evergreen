package jasper

import (
	"syscall"

	"github.com/containerd/cgroups"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

const (
	// defaultSubsystem is the default subsystem where all tracked processes are
	// added. There is no significance behind using the freezer subsystem
	// over any other subsystem for this purpose; its purpose is to ensure all
	// processes can be tracked in a single subsystem for cleanup.
	defaultSubsystem = cgroups.Freezer
)

// linuxProcessTracker uses cgroups to track processes. If cgroups is not
// available, it kills processes by checking the process' environment variables
// for the marker ManagerEnvironID.
type linuxProcessTracker struct {
	*processTrackerBase
	cgroup cgroups.Cgroup
	infos  []ProcessInfo
}

// NewProcessTracker creates a cgroup for all tracked processes if supported.
// Cgroups functionality requires admin privileges. It also tracks the
// ProcessInfo for all added processes so that it can find processes to
// terminate in Cleanup() based on their environment variables.
func NewProcessTracker(name string) (ProcessTracker, error) {
	tracker := &linuxProcessTracker{
		processTrackerBase: &processTrackerBase{Name: name},
		infos:              []ProcessInfo{},
	}
	if err := tracker.setDefaultCgroupIfInvalid(); err != nil {
		grip.Debug(message.WrapErrorf(err, "could not initialize process tracker named '%s' with cgroup", name))
	}

	return tracker, nil
}

// validCgroup returns true if the cgroup is non-nil and not deleted.
func (t *linuxProcessTracker) validCgroup() bool {
	return t.cgroup != nil && t.cgroup.State() != cgroups.Deleted
}

// setDefaultCgroupIfInvalid attempts to set the tracker's cgroup if it is
// invalid. This can fail if cgroups is not a supported feature on this
// platform.
func (t *linuxProcessTracker) setDefaultCgroupIfInvalid() error {
	if t.validCgroup() {
		return nil
	}

	cgroup, err := cgroups.New(cgroups.V1, cgroups.StaticPath("/"+t.Name), &specs.LinuxResources{})
	if err != nil {
		return errors.Wrap(err, "could not create default cgroup")
	}
	t.cgroup = cgroup

	return nil
}

// Add adds this PID to the cgroup if cgroups is available. It also keeps track
// of this process' ProcessInfo.
func (t *linuxProcessTracker) Add(info ProcessInfo) error {
	t.infos = append(t.infos, info)

	if err := t.setDefaultCgroupIfInvalid(); err != nil {
		return nil
	}

	proc := cgroups.Process{Subsystem: defaultSubsystem, Pid: info.PID}
	if err := t.cgroup.Add(proc); err != nil {
		return errors.Wrap(err, "failed to add process with pid '%d' to cgroup")
	}
	return nil
}

// listCgroupPIDs lists all PIDs in the cgroup. If no cgroup is available, this
// returns a nil slice.
func (t *linuxProcessTracker) listCgroupPIDs() ([]int, error) {
	if !t.validCgroup() {
		return nil, nil
	}

	procs, err := t.cgroup.Processes(defaultSubsystem, false)
	if err != nil {
		return nil, errors.Wrap(err, "could not list tracked PIDs")
	}

	pids := make([]int, 0, len(procs))
	for _, proc := range procs {
		pids = append(pids, proc.Pid)
	}
	return pids, nil
}

// doCleanupByCgroup terminates running processes in this process tracker's
// cgroup.
func (t *linuxProcessTracker) doCleanupByCgroup() error {
	if !t.validCgroup() {
		return errors.New("cgroup is invalid so cannot cleanup by cgroup")
	}

	pids, err := t.listCgroupPIDs()
	if err != nil {
		return errors.Wrap(err, "could not find tracked processes")
	}

	catcher := grip.NewBasicCatcher()
	for _, pid := range pids {
		catcher.Add(errors.Wrapf(cleanupProcess(pid), "error while cleaning up process with pid '%d'", pid))
	}

	// Delete the cgroup. If the process tracker is still used, the cgroup must
	// be re-initialized.
	catcher.Add(t.cgroup.Delete())
	return catcher.Resolve()
}

// doCleanupByEnvironmentVariable terminates running processes whose
// value for environment variable ManagerEnvironID equals this process
// tracker's name.
func (t *linuxProcessTracker) doCleanupByEnvironmentVariable() error {
	catcher := grip.NewBasicCatcher()
	for _, info := range t.infos {
		if value, ok := info.Options.Environment[ManagerEnvironID]; ok && value == t.Name {
			catcher.Add(cleanupProcess(info.PID))
		}
	}
	t.infos = []ProcessInfo{}
	return catcher.Resolve()
}

// cleanupProcess terminates the process given by its PID. If the process has
// already terminated, this will not return an error.
func cleanupProcess(pid int) error {
	catcher := grip.NewBasicCatcher()
	// A process returns syscall.ESRCH if it already terminated.
	if err := syscall.Kill(pid, syscall.SIGTERM); err != syscall.ESRCH {
		catcher.Add(errors.Wrapf(err, "failed to send sigterm to process with pid '%d'", pid))
		catcher.Add(errors.Wrapf(syscall.Kill(pid, syscall.SIGKILL), "failed to send sigkill to process with pid '%d'", pid))
	}
	return catcher.Resolve()
}

// Cleanup kills all tracked processes. If cgroups is available, it kills all
// processes in the cgroup. Otherwise, it kills processes based on the expected
// environment variable that should be set in all managed processes. This means
// that there should be an environment variable ManagerEnvironID that has a
// value equal to this process tracker's name.
func (t *linuxProcessTracker) Cleanup() error {
	catcher := grip.NewBasicCatcher()
	if t.validCgroup() {
		catcher.Add(errors.Wrap(t.doCleanupByCgroup(), "error occurred while cleaning up processes tracked by cgroup"))
	}
	catcher.Add(errors.Wrap(t.doCleanupByEnvironmentVariable(), "error occurred while cleaning up processes tracked by environment variable"))

	return catcher.Resolve()
}
