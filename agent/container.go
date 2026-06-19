package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
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
)

// ContainerHandle is the interface through which the agent manages an active
// task isolation container. Using an interface lets unit tests inject a fake
// without requiring a live Docker daemon.
type ContainerHandle interface {
	GetID() string
	GetName() string
	GetEnvFileHostDir() string
	Close()
	Destroy(ctx context.Context) error
}

// containerFactory creates and starts a new isolation container. The default
// implementation calls agentcontainer.CreateAndStart; tests replace it with a
// stub that returns a fake ContainerHandle without Docker.
type containerFactoryFunc func(ctx context.Context, cfg agentcontainer.Config) (ContainerHandle, error)

func defaultContainerFactory(ctx context.Context, cfg agentcontainer.Config) (ContainerHandle, error) {
	return agentcontainer.CreateAndStart(ctx, cfg)
}

// tryReapOrphanContainers removes any evergreen-task-* containers left behind
// by a prior agent crash or host reboot. Called once at agent startup when
// the --cleanup flag is set, before any tasks are dispatched. Best-effort:
// if Docker is not running (non-container-enabled host) the function returns
// silently; individual removal failures are warned but do not block startup.
// reaperTimeout bounds the total time the orphan-container reaper spends
// connecting to Docker, listing containers, and removing them. Using a
// background-derived context (not the agent's Start ctx) ensures that a
// shutdown signal arriving during the --cleanup phase doesn't silently skip
// the reap entirely: the agent ctx would be cancelled before any Docker call
// completes, leaving orphans on the host.
const reaperTimeout = 30 * time.Second

func (a *Agent) tryReapOrphanContainers(ctx context.Context) {
	defer recovery.LogStackTraceAndContinue("reap orphan containers")

	// Use a bounded background context so the reaper completes even if the
	// parent ctx is cancelled during a shutdown race. The parent ctx is still
	// used for mid-loop cancellation checks so a clean agent shutdown during
	// removal doesn't log spurious warnings.
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

	// The name filter is a substring match. GOAL-279 containers are named
	// evergreen-task-<task-id>, a prefix distinctive enough that accidental
	// collisions with human-created containers are unlikely. A label-based
	// filter would be more precise but requires labelling containers at
	// creation time (a larger change deferred to Phase 1).
	containers, err := cli.ContainerList(reaperCtx, dockercontainer.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.KeyValuePair{Key: "name", Value: "evergreen-task-"}),
	})
	if err != nil {
		grip.Warningf(ctx, "Orphan container reaper: could not list containers: %s", err)
		return
	}

	for _, c := range containers {
		shortID := c.ID
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}
		names := containerNames(c.Names)
		if err := cli.ContainerRemove(reaperCtx, c.ID, dockercontainer.RemoveOptions{Force: true}); err != nil {
			if ctx.Err() != nil {
				// Context cancelled during shutdown — not a real removal failure.
				return
			}
			grip.Warningf(ctx, "Orphan container reaper: could not remove container '%s' (%s): %s", shortID, names, err)
		} else {
			grip.Infof(ctx, "Orphan container reaper: removed stale container '%s' (%s).", shortID, names)
		}
	}
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

	factory := a.containerFactory
	if factory == nil {
		factory = defaultContainerFactory
	}
	tc, err := factory(ctx, agentcontainer.Config{
		Image:    ci.Image,
		WorkDir:  conf.WorkDir,
		TaskID:   conf.Task.Id,
		MemoryMB: ci.MemoryMB,
		CPUs:     ci.CPUs,
		Logger:   log,
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
	grip.Infof(ctx, "Started isolation container '%s' (image=%s) for task group starting with task '%s'.", tc.GetName(), ci.Image, conf.Task.Id)
	return nil
}

// destroyContainer tears down the isolation container and clears the agent's
// container reference. conf may be nil (e.g. when called from the loop-exit
// defer before any task has run), in which case conf fields are not cleared.
// The container is identified by its name in logs rather than the task ID,
// since the container was created for the first task in the group and its name
// does not change across task-group members.
//
// If a.retainContainerUntil is in the future, the container is intentionally
// left running for on-call post-mortem inspection (retain_on_failure_secs).
// The agent reference is cleared so subsequent task dispatch is unaffected;
// the reaper removes the container at next startup.
//
// Static-host note: the retention window is checked only when destroyContainer
// fires (at group teardown). If teardown fires within the window the container
// is left running until the orphan reaper at next agent restart — there is no
// background timer that removes it after the window elapses. On dynamic EC2
// hosts (the Phase 0 target) the host recycles before accumulation becomes a
// concern. Static hosts running many task groups should set
// container_retain_on_failure_secs=0 until a deadline-based sweep is added.
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
		grip.Infof(ctx, "Retaining isolation container '%s' until %s for on-call inspection (retain_on_failure_secs). The orphan reaper will remove it at next agent startup.",
			containerName, a.retainContainerUntil.Format(time.RFC3339))
		// Close the Docker client connection (it is no longer needed for
		// lifecycle management) but do NOT remove the container or its tmpfs.
		// The tmpfs remains mounted inside the container so on-call can read
		// the env-file; the container itself is left running for docker exec.
		// The orphan reaper at next agent startup handles the final cleanup.
		a.currentContainer.Close()
		a.currentContainer = nil
		a.retainContainerUntil = time.Time{}
		if conf != nil {
			conf.ContainerID = ""
			conf.EnvFileHostDir = ""
		}
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

// checkContainerOOM uses docker inspect to read the OOMKilled flag from the
// container state. Returns (true, nil) when the container was OOM-killed,
// (false, nil) when it was not, and (false, err) when inspect fails.
func checkContainerOOM(ctx context.Context, containerID string) (bool, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return false, fmt.Errorf("creating Docker client for OOM check: %w", err)
	}
	defer cli.Close()

	inspectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	info, err := cli.ContainerInspect(inspectCtx, containerID)
	if err != nil {
		return false, fmt.Errorf("inspecting container '%s': %w", containerID, err)
	}
	if info.State == nil {
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
func (a *Agent) scheduleContainerRetention(ctx context.Context, containerName string) {
	if a.opts.ContainerRetainOnFailureSecs <= 0 || a.currentContainer == nil {
		return
	}
	a.retainContainerUntil = time.Now().Add(time.Duration(a.opts.ContainerRetainOnFailureSecs) * time.Second)
	grip.Infof(ctx, "Task failed with isolation container '%s' active; scheduling retention until %s (retain_on_failure_secs=%d). Use `docker exec -it %s bash` for inspection.",
		containerName, a.retainContainerUntil.Format(time.RFC3339),
		a.opts.ContainerRetainOnFailureSecs, containerName)
}

// augmentOOMTrackerWithContainerSignal supplements the existing dmesg-based
// OOM report with the container-native OOMKilled signal from docker inspect.
// This is more reliable than dmesg under containers because dmesg PIDs are
// host-side (not matching container-namespaced PIDs) and the dmesg buffer can
// accumulate messages from prior containers.
//
// This check is intentionally independent of tc.oomTrackerEnabled(cloudProvider).
// That gate controls the dmesg-based report (a project-level setting for host
// processes). The container OOMKilled signal is a container-specific fact from
// docker inspect, always worth surfacing when a container is active regardless
// of project OOM-tracker preferences.
// emitContainerFailureSnapshot collects post-mortem forensics from the active
// isolation container and emits them as a container.failure_snapshot OTel span.
// It is called on non-zero task exit while the container is still running (before
// destroyContainer fires at group teardown). All string fields are run through
// the task config's redactor so secrets never appear in Honeycomb.
func (a *Agent) emitContainerFailureSnapshot(ctx context.Context, tc *taskContext, detail *apimodels.TaskEndDetail) {
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

	// docker inspect JSON.
	if inspectJSON, err := containerInspectJSON(ctx, containerID); err == nil {
		span.SetAttributes(attribute.String("container.inspect_json", redactForSnapshot(inspectJSON, tc)))
	}

	// ps -ef inside the container. Capture output even on non-zero exit — a
	// container whose processes have all died is exactly the case where we
	// want "no processes found" in Honeycomb rather than a missing attribute.
	if psOut, _ := containerExec(ctx, containerID, "ps", "-ef"); psOut != "" {
		span.SetAttributes(attribute.String("container.ps_output", redactForSnapshot(psOut, tc)))
	}

	// Env-file contents (the per-task tmpfs written by WrapWithContainer).
	if tc.taskConfig != nil && tc.taskConfig.EnvFileHostDir != "" {
		envFile := filepath.Join(tc.taskConfig.EnvFileHostDir, ".evg-env")
		if data, err := os.ReadFile(envFile); err == nil {
			span.SetAttributes(attribute.String("container.env_file", redactForSnapshot(string(data), tc)))
		}
	}

	// Task log tail is intentionally omitted: task output goes to the remote
	// Evergreen log service (not a local file) and is not accessible here.
}

// containerInspectJSON returns the full JSON from docker inspect for a container.
func containerInspectJSON(ctx context.Context, containerID string) (string, error) {
	inspectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return "", errors.Wrap(err, "creating Docker client for inspect")
	}
	defer cli.Close()

	info, err := cli.ContainerInspect(inspectCtx, containerID)
	if err != nil {
		return "", errors.Wrapf(err, "inspecting container '%s'", containerID)
	}
	data, err := json.Marshal(info)
	if err != nil {
		return "", errors.Wrap(err, "marshalling inspect result")
	}
	return string(data), nil
}

// containerExec runs a command inside a container and returns its combined output.
// This failure-snapshot path uses the docker CLI (not the SDK) because it is
// called only on task failure and the credential-helper flow for ECR is handled
// by host_provision.go (which also uses the CLI for the same reason). The SDK
// path for the main lifecycle (CreateAndStart, Destroy) is intentional and
// unrelated; the CLI is used here for the forensic path to keep its footprint
// simple and aligned with other CLI callers.
func containerExec(ctx context.Context, containerID string, args ...string) (string, error) {
	execCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmdArgs := append([]string{"exec", containerID}, args...)
	out, err := exec.CommandContext(execCtx, "docker", cmdArgs...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// redactForSnapshot applies the task's expansion redactions to s so that
// secrets never appear in Honeycomb attributes. Values are applied
// longest-first (matching the canonical redacting_sender order) to prevent a
// shorter secret that is a prefix of a longer one from producing a partial
// substitution that leaks the suffix.
func redactForSnapshot(s string, tc *taskContext) string {
	if tc == nil || tc.taskConfig == nil {
		return s
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

func (a *Agent) augmentOOMTrackerWithContainerSignal(ctx context.Context, tc *taskContext, detail *apimodels.TaskEndDetail) {
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
