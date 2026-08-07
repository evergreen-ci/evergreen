package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/evergreen-ci/evergreen"
	agentcontainer "github.com/evergreen-ci/evergreen/agent/container"
	"github.com/evergreen-ci/evergreen/agent/globals"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// ContainerHandle is the interface through which the agent manages an active
// task isolation container. Using an interface lets unit tests inject a fake
// without requiring a live Docker daemon.
type ContainerHandle interface {
	GetID() string
	GetName() string
	GetEnvFileHostDir() string
	Destroy(ctx context.Context) error
}

// containerFactory creates and starts a new isolation container. The default
// implementation calls agentcontainer.CreateAndStart; tests replace it with a
// stub that returns a fake ContainerHandle without Docker.
type containerFactoryFunc func(ctx context.Context, cfg agentcontainer.Config) (ContainerHandle, error)

func defaultContainerFactory(ctx context.Context, cfg agentcontainer.Config) (ContainerHandle, error) {
	return agentcontainer.CreateAndStart(ctx, cfg)
}

// reaperTimeout bounds the total time the orphan-container reaper spends
// connecting to Docker, listing containers, and removing them.
const reaperTimeout = 30 * time.Second //nolint:unused

const (
	containerEnvFilePrefix  = ".evg-env-"
	hostEnvCaptureTimeout   = 30 * time.Second
	hostEnvCommandWaitDelay = time.Second
)

// tryReapOrphanContainers removes isolation containers left behind by a prior
// agent crash or host reboot. Called once at agent startup when the --cleanup
// flag is set, before any tasks are dispatched. Best-effort: if Docker is not
// running the function returns silently, and individual removal failures are
// warned but do not block startup.
//
// The reaper runs on a background-derived context so that a shutdown signal
// arriving during the --cleanup phase does not cancel every Docker call and
// silently skip the reap, leaving orphans on the host.
func (a *Agent) tryReapOrphanContainers(ctx context.Context) { //nolint:unused
	defer recovery.LogStackTraceAndContinue("reap orphan containers")

	reaperCtx, cancel := context.WithTimeout(context.Background(), reaperTimeout)
	defer cancel()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		grip.Infof(ctx, "Orphan container reaper: Docker client unavailable, skipping: %s", err)
		return
	}
	defer cli.Close()

	if _, err := cli.Info(reaperCtx); err != nil {
		grip.Infof(ctx, "Orphan container reaper: Docker daemon not reachable, skipping: %s", err)
		return
	}

	// Ownership is decided client-side by isAgentOwnedContainer, because
	// Docker's name filter cannot express an anchored prefix and a label
	// filter alone would strand containers created before labels existed.
	containers, err := cli.ContainerList(reaperCtx, dockercontainer.ListOptions{All: true})
	if err != nil {
		grip.Warningf(ctx, "Orphan container reaper: could not list containers: %s", err)
		return
	}

	for _, c := range containers {
		if !isAgentOwnedContainer(c.Labels, c.Names) {
			continue
		}
		shortID := c.ID
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}
		names := containerNames(c.Names)
		if err := cli.ContainerRemove(reaperCtx, c.ID, dockercontainer.RemoveOptions{Force: true}); err != nil {
			if ctx.Err() != nil {
				return
			}
			grip.Warningf(ctx, "Orphan container reaper: could not remove container '%s' (%s): %s", shortID, names, err)
		} else {
			grip.Infof(ctx, "Orphan container reaper: removed stale container '%s' (%s).", shortID, names)
		}
	}
}

// isAgentOwnedContainer reports whether the reaper may remove a container.
//
// An explicit owner label is authoritative in both directions: it claims
// containers this agent created and protects containers owned by anything
// else. Containers with no owner label at all are matched on an anchored name
// prefix, so containers created before ownership labels existed are still
// reaped rather than stranded on the host across an agent rollout.
//
// The prefix is checked here rather than in the Docker filter because Docker's
// name filter is an unanchored substring match, which would also match a
// container a human named something like "my-evergreen-task-debug".
func isAgentOwnedContainer(labels map[string]string, names []string) bool {
	if owner, ok := labels[agentcontainer.OwnerLabel]; ok {
		return owner == agentcontainer.OwnerLabelValue
	}
	for _, name := range names {
		if strings.HasPrefix(strings.TrimPrefix(name, "/"), agentcontainer.ContainerNamePrefix) {
			return true
		}
	}
	return false
}

// containerNames strips the leading '/' that the Docker API prepends to
// container names and joins them for display.
func containerNames(names []string) string {
	clean := make([]string, len(names))
	for i, n := range names {
		clean[i] = strings.TrimPrefix(n, "/")
	}
	return strings.Join(clean, ",")
}

// maybeStartContainer ensures a Docker container is running for task isolation
// when the distro has container isolation enabled. It sets conf.ContainerID so
// commands know to route execution through `docker exec`.
//
// Container lifecycle is task-group-scoped, not per-task. If a container is
// already running for this task group (a.currentContainer != nil), the existing
// container's identity is wired into conf without creating a new one. A new
// container is only created at the start of a task group (when a.currentContainer
// is nil). The container is destroyed in runTeardownGroupCommands.
func (a *Agent) maybeStartContainer(ctx context.Context, conf *internal.TaskConfig, log grip.Journaler) error {
	if conf.Distro == nil || conf.Distro.ContainerIsolation == nil {
		return nil
	}

	if a.currentContainer != nil {
		// Reuse the container that was started for the first task in this group.
		conf.ContainerID = a.currentContainer.GetID()
		conf.EnvFileHostDir = a.currentContainer.GetEnvFileHostDir()
		return nil
	}

	ci := conf.Distro.ContainerIsolation
	ctx, span := a.tracer.Start(ctx, "container.create_and_start")
	defer span.End()
	span.SetAttributes(
		attribute.String("container.image", ci.Image),
		attribute.String("container.task_id", conf.Task.Id),
		attribute.String("container.distro_id", conf.Task.DistroId),
	)

	// The task workdir and its tmp subdirectory are created by the agent and
	// are not writable by the container's exec user. Access is granted by
	// transferring ownership rather than by widening the mode, so the
	// directories never become world-writable on a host shared with other
	// local users.
	fellBack, err := secureContainerDirs(conf.WorkDir, conf.Distro.ExecUser)
	if err != nil {
		if ci.RequireIsolation {
			span.SetStatus(codes.Error, err.Error())
			return errors.Wrap(err, "preparing task directories for isolation container (fail-closed: require_isolation is set)")
		}
		grip.Warningf(ctx, "Could not prepare task directories for container isolation: %s", err)
	}
	if fellBack {
		span.SetAttributes(attribute.Bool("container.workdir_permissive_fallback", true))
		grip.Warningf(ctx, "Task directories for task '%s' are world-writable because ownership could not be transferred to exec user '%s'; the container can write to them, but so can any other local user on this host.",
			conf.Task.Id, conf.Distro.ExecUser)
	}

	extraMounts := toolchainMounts(ctx, conf.Task.Id)

	factory := a.containerFactory
	if factory == nil {
		factory = defaultContainerFactory
	}
	tc, err := factory(ctx, agentcontainer.Config{
		Image:       ci.Image,
		WorkDir:     conf.WorkDir,
		TaskID:      conf.Task.Id,
		MemoryMB:    ci.MemoryMB,
		CPUs:        ci.CPUs,
		ExtraMounts: extraMounts,
		Logger:      log,
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		if ci.RequireIsolation {
			return errors.Wrap(err, "starting isolation container (fail-closed: require_isolation is set)")
		}
		if log != nil {
			log.Warning(ctx, message.WrapError(err, message.Fields{
				"message": "container_isolation_degraded",
				"task_id": conf.Task.Id,
				"image":   ci.Image,
				"note":    "task will run without container isolation",
			}))
		}
		return nil
	}
	span.SetAttributes(
		attribute.String("container.id", tc.GetID()),
		attribute.String("container.name", tc.GetName()),
	)
	conf.ContainerID = tc.GetID()
	conf.EnvFileHostDir = tc.GetEnvFileHostDir()
	a.currentContainer = tc

	// Write a static host-env file in the env tmpfs containing PATH and
	// known toolchain env vars from the host. docker exec does not source
	// /etc/profile.d/*.sh, so toolchain paths set by those scripts (e.g.
	// /opt/golang/go1.24.13/bin added to PATH) are missing inside the
	// container. WrapWithContainer passes this as a second --env-file so
	// these vars reach every docker exec invocation. Per-command env vars
	// (from the command's own env map) are written to a separate env-file
	// and override these since Docker applies --env-file args in order.
	if conf.EnvFileHostDir != "" {
		hostEnvPath := filepath.Join(conf.EnvFileHostDir, ".evg-host-env")
		if err := writeHostEnvFile(ctx, hostEnvPath); err != nil {
			grip.Warningf(ctx, "Could not write host env file for container isolation: %s", err)
		}
	}

	grip.Infof(ctx, "Started isolation container '%s' (image=%s) for task group starting with task '%s'.", tc.GetName(), ci.Image, conf.Task.Id)
	return nil
}

const (
	// containerDirMode is applied to task directories once they are owned by
	// the container's exec user, so they are not world-writable.
	containerDirMode = 0755

	// containerDirPermissiveMode is the fallback applied when ownership cannot
	// be reconciled with the exec user. It is world-writable, which is unsafe
	// on a host shared with other local users, so it is only used to keep the
	// task runnable and is always reported to the caller.
	containerDirPermissiveMode = 0777
)

// containerToolchainDirs are the host toolchain directories bind-mounted
// read-only into the container. Toolchains are installed at AMI provisioning
// rather than baked into the image. Only these paths are mounted; the whole
// of /opt is not, because a read-only mount still lets a task read (and
// exfiltrate) anything beneath it.
var containerToolchainDirs = []string{
	"/opt/mongodbtoolchain",
	"/opt/golang",
	"/opt/java",
	"/opt/ruby",
	"/opt/node",
	"/opt/python",
}

// toolchainMounts returns read-only mounts for the toolchain directories that
// exist on the host. Nonexistent sources are skipped because Docker rejects
// bind mounts whose source is missing, which would fail container creation.
func toolchainMounts(ctx context.Context, taskID string) []agentcontainer.Mount {
	var mounts []agentcontainer.Mount
	for _, dir := range containerToolchainDirs {
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		mounts = append(mounts, agentcontainer.Mount{Source: dir, Target: dir, ReadOnly: true})
	}
	if len(mounts) == 0 {
		grip.Warningf(ctx, "No host toolchain directories found; container for task '%s' will only see image-provided toolchains.", taskID)
	}
	return mounts
}

// secureContainerDirs prepares the task working directory and its tmp
// subdirectory for use by the container's exec user, which must be able to
// write to them through the same-path bind mount.
//
// The preferred path transfers ownership to the exec user and applies a mode
// that is not world-writable. When ownership cannot be reconciled — the user
// does not resolve on the host, or the agent cannot chown — the directories
// fall back to a permissive mode so the task still runs, and fellBack reports
// the weaker posture so the caller can log it.
//
// Bind mounts are enforced by numeric uid, while `docker exec --user` resolves
// the exec user against the image's own passwd database. A host uid that
// disagrees with the image's uid for the same name is therefore not detectable
// here and surfaces as a write failure inside the container.
//
// When the distro has no exec user, docker exec runs as root inside the
// container and can already write to the agent-owned directories.
func secureContainerDirs(workDir, execUser string) (fellBack bool, err error) {
	if workDir == "" || execUser == "" {
		return false, nil
	}

	dirs := []string{workDir, filepath.Join(workDir, "tmp")}

	ownershipErr := transferDirOwnership(dirs, execUser)
	if ownershipErr == nil {
		return false, nil
	}

	// Windows has no POSIX ownership to fall back from, so surface the error
	// rather than implying a permissive mode resolved anything.
	if runtime.GOOS == "windows" {
		return false, ownershipErr
	}

	if fallbackErr := applyPermissiveDirMode(dirs); fallbackErr != nil {
		return false, errors.Wrapf(fallbackErr, "falling back to permissive mode after ownership transfer failed (%s)", ownershipErr)
	}
	return true, nil
}

// transferDirOwnership hands dirs to the exec user and applies containerDirMode.
func transferDirOwnership(dirs []string, execUser string) error {
	usr, err := user.Lookup(execUser)
	if err != nil {
		return errors.Wrapf(err, "looking up exec user '%s'", execUser)
	}
	if runtime.GOOS == "windows" {
		// Container isolation is Linux/Docker-only; Windows cannot express ownership as
		// a POSIX numeric UID/GID, and ACL-based equivalents are out of scope. The
		// lookup above still validates the exec user so isolation fails closed consistently.
		return nil
	}
	uid, err := strconv.Atoi(usr.Uid)
	if err != nil {
		return errors.Wrapf(err, "parsing uid for exec user '%s'", execUser)
	}
	gid, err := strconv.Atoi(usr.Gid)
	if err != nil {
		return errors.Wrapf(err, "parsing gid for exec user '%s'", execUser)
	}

	catcher := grip.NewBasicCatcher()
	for _, dir := range dirs {
		catcher.Wrapf(chownContainerDir(dir, uid, gid), "securing '%s'", dir)
	}
	return catcher.Resolve()
}

func applyPermissiveDirMode(dirs []string) error {
	catcher := grip.NewBasicCatcher()
	for _, dir := range dirs {
		catcher.Wrapf(setContainerDirMode(dir, containerDirPermissiveMode), "setting fallback mode on '%s'", dir)
	}
	return catcher.Resolve()
}

// chownContainerDir transfers ownership of dir to uid/gid and tightens its mode.
func chownContainerDir(dir string, uid, gid int) error {
	if err := verifyRealDir(dir); err != nil {
		return err
	}
	if err := os.Lchown(dir, uid, gid); err != nil {
		return errors.Wrap(err, "changing ownership")
	}
	return errors.Wrap(os.Chmod(dir, containerDirMode), "setting permissions")
}

func setContainerDirMode(dir string, mode os.FileMode) error {
	if err := verifyRealDir(dir); err != nil {
		return err
	}
	return errors.Wrap(os.Chmod(dir, mode), "setting permissions")
}

// verifyRealDir refuses symlinks so a local user cannot redirect a permission
// or ownership change at a path they do not own. This matters most on the
// permissive fallback, where following a symlink would grant world-writable
// access to the target.
func verifyRealDir(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return errors.Wrap(err, "stating directory")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("refusing to modify a symlink")
	}
	if !info.IsDir() {
		return errors.New("path is not a directory")
	}
	return nil
}

// destroyContainer tears down the isolation container and clears the agent's
// container reference. conf may be nil (e.g. when called from the loop-exit
// defer before any task has run), in which case conf fields are not cleared.
// The container is identified by its name in logs rather than the task ID,
// since the container was created for the first task in the group and its name
// does not change across task-group members.
//
// If a.retainContainerUntil is in the future, container ownership transfers to
// a bounded background cleanup. The container remains available for inspection
// until the retention deadline, or until the agent context is canceled during
// shutdown. The agent reference is cleared so subsequent dispatch is unaffected.
func (a *Agent) destroyContainer(ctx context.Context, conf *internal.TaskConfig) {
	if a.currentContainer == nil {
		a.retainContainerUntil = time.Time{}
		return
	}
	containerName := a.currentContainer.GetName()

	ctx, span := a.tracer.Start(ctx, "container.destroy")
	defer span.End()
	span.SetAttributes(
		attribute.String("container.id", a.currentContainer.GetID()),
		attribute.String("container.name", containerName),
	)

	if !a.retainContainerUntil.IsZero() && time.Now().Before(a.retainContainerUntil) {
		grip.Infof(ctx, "Retaining isolation container '%s' until %s for on-call inspection (retain_on_failure_secs).",
			containerName, a.retainContainerUntil.Format(time.RFC3339))
		retainedContainer := a.currentContainer
		retainedUntil := a.retainContainerUntil
		a.currentContainer = nil
		a.retainContainerUntil = time.Time{}
		if conf != nil {
			conf.ContainerID = ""
			conf.EnvFileHostDir = ""
		}
		go destroyRetainedContainer(ctx, a.tracer, retainedContainer, containerName, retainedUntil)
		return
	}

	if err := a.currentContainer.Destroy(ctx); err != nil {
		grip.Warningf(ctx, "Failed to destroy isolation container '%s': %s", containerName, err)
	}
	a.currentContainer = nil
	a.retainContainerUntil = time.Time{}
	if conf != nil {
		conf.ContainerID = ""
		conf.EnvFileHostDir = ""
	}
}

// destroyRetainedContainer waits for the retention deadline to pass or the
// agent to shut down, then removes the container. It runs detached from its
// caller, so it must recover from panics: an unrecovered panic in a goroutine
// terminates the entire agent process.
func destroyRetainedContainer(ctx context.Context, tracer trace.Tracer, container ContainerHandle, containerName string, retainUntil time.Time) {
	defer recovery.LogStackTraceAndContinue("destroy retained container")

	timer := time.NewTimer(time.Until(retainUntil))
	defer timer.Stop()

	select {
	case <-ctx.Done():
	case <-timer.C:
	}

	// Detach from the caller so shutdown cancellation cannot bypass removal.
	// The span starts here so its duration covers the removal itself rather
	// than the caller's already-completed teardown.
	destroyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), reaperTimeout)
	defer cancel()
	destroyCtx, span := tracer.Start(destroyCtx, "container.destroy_retained")
	defer span.End()
	span.SetAttributes(attribute.String("container.name", containerName))

	if err := container.Destroy(destroyCtx); err != nil {
		span.SetStatus(codes.Error, err.Error())
		grip.Warningf(destroyCtx, "Failed to destroy retained isolation container '%s': %s", containerName, err)
	}
}

// inspectContainerTimeout bounds a single docker inspect call.
const inspectContainerTimeout = 10 * time.Second //nolint:unused

// inspectContainer returns the Docker inspect document for a container.
func inspectContainer(ctx context.Context, containerID string) (dockercontainer.InspectResponse, error) { //nolint:unused
	inspectCtx, cancel := context.WithTimeout(ctx, inspectContainerTimeout)
	defer cancel()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return dockercontainer.InspectResponse{}, errors.Wrap(err, "creating Docker client")
	}
	defer cli.Close()

	info, err := cli.ContainerInspect(inspectCtx, containerID)
	return info, errors.Wrapf(err, "inspecting container '%s'", containerID)
}

// checkContainerOOM reads the container-native OOMKilled flag from docker
// inspect. Returns (true, nil) when the container was OOM-killed, (false, nil)
// when it was not, and (false, err) when inspect fails.
func checkContainerOOM(ctx context.Context, containerID string) (bool, error) { //nolint:unused
	info, err := inspectContainer(ctx, containerID)
	if err != nil {
		return false, err
	}
	if info.ContainerJSONBase == nil || info.State == nil {
		return false, nil
	}
	return info.State.OOMKilled, nil
}

// scheduleContainerRetention sets the retention window on the agent when a
// task fails with an active isolation container. The container is kept alive
// for ContainerRetainOnFailureSecs seconds after the task ends so on-call can
// docker exec into it for post-mortem inspection.
//
// Timing note: the window is measured from task failure time, not from when
// destroyContainer actually fires (which is at group teardown, potentially
// seconds to minutes later). For task groups where teardown fires within the
// window, the container is left running until the reaper; for teardown after
// the window, normal destroy runs. Both outcomes are acceptable for Phase 0
// on-call use: the container is either available or cleanly removed.
func (a *Agent) scheduleContainerRetention(ctx context.Context, containerName string) { //nolint:unused
	if a.opts.ContainerRetainOnFailureSecs <= 0 || a.currentContainer == nil {
		return
	}
	a.retainContainerUntil = time.Now().Add(time.Duration(a.opts.ContainerRetainOnFailureSecs) * time.Second)
	grip.Infof(ctx, "Task failed with isolation container '%s' active; scheduling retention until %s (retain_on_failure_secs=%d). Use `docker exec -it %s bash` for inspection.",
		containerName, a.retainContainerUntil.Format(time.RFC3339),
		a.opts.ContainerRetainOnFailureSecs, containerName)
}

// emitContainerFailureSnapshot collects post-mortem forensics from the active
// isolation container and emits them as a container.failure_snapshot OTel span.
// It is called on non-zero task exit while the container is still running
// (before destroyContainer fires at group teardown).
//
// Only allowlisted fields are exported, and every exported string is passed
// through redactForSnapshot, which fails closed. The raw inspect document and
// raw env-file values are never exported because both carry credentials.
func (a *Agent) emitContainerFailureSnapshot(ctx context.Context, tc *taskContext, detail *apimodels.TaskEndDetail) { //nolint:unused
	if a.currentContainer == nil || tc == nil {
		return
	}
	containerID := a.currentContainer.GetID()
	image := ""
	if tc.taskConfig != nil && tc.taskConfig.Distro != nil && tc.taskConfig.Distro.ContainerIsolation != nil {
		image = tc.taskConfig.Distro.ContainerIsolation.Image
	}

	ctx, span := a.tracer.Start(ctx, "container.failure_snapshot")
	defer span.End()
	span.SetAttributes(
		attribute.String("container.id", containerID),
		attribute.String("container.image", image),
		attribute.String("container.task_status", detail.Status),
		attribute.Bool("container.oom_killed", detail.OOMTracker != nil && detail.OOMTracker.Detected),
	)
	if detail.Status == evergreen.TaskFailed || detail.TimedOut {
		span.SetStatus(codes.Error, detail.Status)
	}

	if summary, err := containerInspectSummaryJSON(ctx, containerID); err == nil {
		span.SetAttributes(attribute.String("container.inspect_summary", redactForSnapshot(summary, tc)))
	}

	// Capture ps output even on non-zero exit: a container whose processes
	// have all died is exactly the case where "no processes found" is the
	// useful signal rather than a missing attribute.
	if psOut, _ := containerExec(ctx, containerID, "ps", "-ef"); psOut != "" {
		span.SetAttributes(attribute.String("container.ps_output", redactForSnapshot(psOut, tc)))
	}

	// Only the env-file key names are exported. The values are the task's
	// environment, which routinely holds credentials, and the key list is
	// enough to diagnose a missing or malformed variable.
	if tc.taskConfig != nil && tc.taskConfig.EnvFileHostDir != "" {
		if keys, err := containerEnvFileKeys(tc.taskConfig.EnvFileHostDir); err == nil {
			span.SetAttributes(attribute.StringSlice("container.env_file_keys", keys))
		}
	}

	// Task stdout/stderr is not captured here: it flows through Jasper to the
	// remote log service, and `docker logs` only sees PID 1 (`sleep infinity`).
}

// containerInspectSummary is the allowlisted subset of docker inspect output
// that may be exported to telemetry. The full document is deliberately
// excluded because it embeds the container's environment and image config.
type containerInspectSummary struct { //nolint:unused
	Status       string `json:"status"`
	ExitCode     int    `json:"exit_code"`
	OOMKilled    bool   `json:"oom_killed"`
	Error        string `json:"error"`
	StartedAt    string `json:"started_at"`
	FinishedAt   string `json:"finished_at"`
	RestartCount int    `json:"restart_count"`
}

// containerInspectSummaryJSON returns the allowlisted inspect fields as JSON.
func containerInspectSummaryJSON(ctx context.Context, containerID string) (string, error) { //nolint:unused
	info, err := inspectContainer(ctx, containerID)
	if err != nil {
		return "", err
	}

	var summary containerInspectSummary
	if info.ContainerJSONBase != nil {
		summary.RestartCount = info.RestartCount
		if info.State != nil {
			summary.Status = info.State.Status
			summary.ExitCode = info.State.ExitCode
			summary.OOMKilled = info.State.OOMKilled
			summary.Error = info.State.Error
			summary.StartedAt = info.State.StartedAt
			summary.FinishedAt = info.State.FinishedAt
		}
	}

	data, err := json.Marshal(summary)
	return string(data), errors.Wrap(err, "marshalling inspect summary")
}

// containerEnvFileKeys returns the sorted key names present in the newest
// container env file. Values are intentionally discarded.
func containerEnvFileKeys(dir string) ([]string, error) {
	data, err := readLatestContainerEnvFile(dir)
	if err != nil {
		return nil, err
	}

	var keys []string
	for line := range strings.SplitSeq(string(data), "\n") {
		key, _, found := strings.Cut(line, "=")
		if !found || key == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

func readLatestContainerEnvFile(dir string) ([]byte, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, errors.Wrap(err, "reading container env directory")
	}

	var latestName string
	var latestModTime time.Time
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), containerEnvFilePrefix) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, errors.Wrapf(err, "reading container env file info for '%s'", entry.Name())
		}
		if latestName == "" || info.ModTime().After(latestModTime) || (info.ModTime().Equal(latestModTime) && entry.Name() > latestName) {
			latestName = entry.Name()
			latestModTime = info.ModTime()
		}
	}
	if latestName == "" {
		return nil, errors.New("container env file not found")
	}

	data, err := os.ReadFile(filepath.Join(dir, latestName))
	return data, errors.Wrap(err, "reading latest container env file")
}

// containerExec runs a command inside a container and returns its combined
// output. This forensic path uses the docker CLI rather than the SDK to keep
// its footprint small; the main lifecycle (CreateAndStart, Destroy) uses the
// SDK.
func containerExec(ctx context.Context, containerID string, args ...string) (string, error) { //nolint:unused
	execCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmdArgs := append([]string{"exec", containerID}, args...)
	out, err := exec.CommandContext(execCtx, "docker", cmdArgs...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// redactionUnavailable replaces snapshot content when the task config needed
// to redact secrets is missing. Exporting the raw value instead would risk
// leaking credentials into telemetry, so redaction fails closed.
const redactionUnavailable = "<REDACTION_UNAVAILABLE>"

// redactForSnapshot applies the task's expansion redactions to s so that
// secrets never appear in Honeycomb attributes. Values are applied
// longest-first (matching the canonical redacting_sender order) to prevent a
// shorter secret that is a prefix of a longer one from producing a partial
// substitution that leaks the suffix.
func redactForSnapshot(s string, tc *taskContext) string {
	if tc == nil || tc.taskConfig == nil {
		return redactionUnavailable
	}
	conf := tc.taskConfig

	type kv struct{ key, val string }
	var all []kv

	for _, info := range conf.NewExpansions.GetRedacted() {
		if info.Value != "" {
			all = append(all, kv{info.Key, info.Value})
		}
	}
	// Use a fresh slice to avoid aliasing conf.Redacted's backing array.
	allToRedact := append([]string(nil), conf.Redacted...)
	allToRedact = append(allToRedact, globals.ExpansionsToRedact...)
	for _, name := range allToRedact {
		if val := conf.NewExpansions.Get(name); val != "" {
			all = append(all, kv{name, val})
		}
	}
	conf.InternalRedactions.Range(func(k, v string) bool {
		if v != "" {
			all = append(all, kv{k, v})
		}
		return true
	})

	// Sort longest value first to prevent prefix-substitution leaks.
	sort.Slice(all, func(i, j int) bool { return len(all[i].val) > len(all[j].val) })

	for _, entry := range all {
		s = strings.ReplaceAll(s, entry.val, fmt.Sprintf("<REDACTED:%s>", entry.key))
	}
	return s
}

// augmentOOMTrackerWithContainerSignal supplements the dmesg-based OOM report
// with the container-native OOMKilled signal from docker inspect, which is
// more reliable under containers because dmesg PIDs are host-side and the
// buffer can carry messages from prior containers.
//
// This is deliberately independent of tc.oomTrackerEnabled(cloudProvider):
// that gate controls the dmesg report for host processes, whereas OOMKilled is
// a container-specific fact worth surfacing whenever a container is active.
func (a *Agent) augmentOOMTrackerWithContainerSignal(ctx context.Context, tc *taskContext, detail *apimodels.TaskEndDetail) { //nolint:unused
	if a.currentContainer == nil {
		return
	}
	oomKilled, err := checkContainerOOM(ctx, a.currentContainer.GetID())
	if err != nil {
		tc.logger.Execution().Warningf(ctx, "Could not check container OOM status via docker inspect: %s", err)
		return
	}
	if !oomKilled {
		tc.logger.Execution().Debugf(ctx, "docker inspect: container '%s' OOMKilled=false.", a.currentContainer.GetName())
		return
	}
	tc.logger.Execution().Infof(ctx, "docker inspect: container '%s' OOMKilled=true; task was OOM-killed.", a.currentContainer.GetName())
	if detail.OOMTracker == nil {
		detail.OOMTracker = &apimodels.OOMTrackerInfo{}
	}
	detail.OOMTracker.Detected = true
}

// hostEnvVars are environment variables set by /etc/profile.d/*.sh on the host
// that configure toolchain access. docker exec does not source profile scripts,
// so these vars must be explicitly forwarded into the container.
var hostEnvVars = []string{
	"PATH",
	"GOROOT",
	"JAVA_HOME",
	"ANT_HOME",
	"LD_LIBRARY_PATH",
	"PKG_CONFIG_PATH",
	"PYTHONPATH",
	"NODE_PATH",
	"CPATH",
	"LIBRARY_PATH",
}

// hostEnvSentinel marks the start of the env dump in the login shell's stdout.
// A login shell also runs profile scripts, which may print banners; everything
// before the sentinel is discarded so it cannot corrupt the env file.
const hostEnvSentinel = "__EVG_HOST_ENV_BEGIN__"

// writeHostEnvFile captures the host's toolchain-related env vars and writes
// them to path in KEY=VALUE format. This file is passed as a --env-file to
// docker exec so containerized processes can find toolchains installed at
// /opt/golang, /opt/mongodbtoolchain, etc. without sourcing profile scripts.
//
// The agent runs as a systemd service whose PATH does not include toolchain
// directories set by /etc/profile.d/*.sh, so values are captured from a login
// shell. If that capture fails the agent's own environment is written instead
// and the capture error is returned, because the result is degraded.
func writeHostEnvFile(ctx context.Context, path string) error {
	captureCtx, cancel := context.WithTimeout(ctx, hostEnvCaptureTimeout)
	defer cancel()

	cmd := exec.CommandContext(captureCtx, "bash", "-l", "-c", hostEnvShellCommand())
	cmd.WaitDelay = hostEnvCommandWaitDelay
	out, err := cmd.Output()
	if err != nil {
		if writeErr := os.WriteFile(path, []byte(formatHostEnv(agentHostEnv())), 0600); writeErr != nil {
			return errors.Wrap(writeErr, "writing fallback host env file")
		}
		return errors.Wrap(err, "capturing login shell environment, wrote agent environment instead")
	}
	return errors.Wrap(os.WriteFile(path, []byte(formatHostEnv(parseHostEnv(out))), 0600), "writing host env file")
}

// hostEnvShellCommand builds a script printing the wanted variables as
// NUL-delimited KEY=VALUE records. Expansions are quoted so values containing
// spaces or glob characters are neither word-split nor pathname-expanded, and
// NUL delimiting stops a value containing a newline from being mistaken for a
// record boundary.
func hostEnvShellCommand() string {
	parts := []string{fmt.Sprintf("printf '%%s\\0' '%s'", hostEnvSentinel)}
	for _, key := range hostEnvVars {
		parts = append(parts, fmt.Sprintf("[ -n \"$%s\" ] && printf '%%s=%%s\\0' '%s' \"$%s\"", key, key, key))
	}
	// A trailing true keeps the exit status zero when the final variable is unset.
	parts = append(parts, "true")
	return strings.Join(parts, "\n")
}

// parseHostEnv extracts KEY=VALUE records from the NUL-delimited capture. It
// discards anything profile scripts printed before the sentinel and drops
// values containing newlines, which an env file cannot represent.
func parseHostEnv(out []byte) []string {
	_, after, found := strings.Cut(string(out), hostEnvSentinel+"\x00")
	if !found {
		return nil
	}

	var entries []string
	for record := range strings.SplitSeq(after, "\x00") {
		key, value, found := strings.Cut(record, "=")
		if !found || key == "" || strings.ContainsAny(value, "\n\r") {
			continue
		}
		entries = append(entries, key+"="+value)
	}
	return entries
}

// agentHostEnv reads the wanted variables from the agent's own environment.
// It lacks profile-set toolchain paths but preserves a usable base PATH.
func agentHostEnv() []string {
	var entries []string
	for _, key := range hostEnvVars {
		if val := os.Getenv(key); val != "" && !strings.ContainsAny(val, "\n\r") {
			entries = append(entries, key+"="+val)
		}
	}
	return entries
}

func formatHostEnv(entries []string) string {
	if len(entries) == 0 {
		return ""
	}
	return strings.Join(entries, "\n") + "\n"
}
