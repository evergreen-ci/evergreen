package agent

import (
	"context"
	"strings"
	"time"

	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	agentcontainer "github.com/evergreen-ci/evergreen/agent/container"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

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
func (a *Agent) maybeStartContainer(ctx context.Context, conf *internal.TaskConfig) error {
	if conf.Distro == nil || conf.Distro.ContainerIsolation == nil {
		return nil
	}

	if a.currentContainer != nil {
		// Reuse the container that was started for the first task in this group.
		conf.ContainerID = a.currentContainer.ID
		conf.EnvFileHostDir = a.currentContainer.EnvFileHostDir
		return nil
	}

	ci := conf.Distro.ContainerIsolation
	tc, err := agentcontainer.CreateAndStart(ctx, agentcontainer.Config{
		Image:    ci.Image,
		WorkDir:  conf.WorkDir,
		TaskID:   conf.Task.Id,
		MemoryMB: ci.MemoryMB,
		CPUs:     ci.CPUs,
	})
	if err != nil {
		return errors.Wrap(err, "starting isolation container")
	}
	conf.ContainerID = tc.ID
	conf.EnvFileHostDir = tc.EnvFileHostDir
	a.currentContainer = tc
	grip.Infof(ctx, "Started isolation container '%s' (image=%s) for task group starting with task '%s'.", tc.Name, ci.Image, conf.Task.Id)
	return nil
}

// destroyContainer tears down the isolation container and clears the agent's
// container reference. conf may be nil (e.g. when called from the loop-exit
// defer before any task has run), in which case conf fields are not cleared.
// The container is identified by its name in logs rather than the task ID,
// since the container was created for the first task in the group and its name
// does not change across task-group members.
func (a *Agent) destroyContainer(ctx context.Context, conf *internal.TaskConfig) {
	if a.currentContainer == nil {
		return
	}
	containerName := a.currentContainer.Name
	if err := a.currentContainer.Destroy(ctx); err != nil {
		grip.Warningf(ctx, "Failed to destroy isolation container '%s': %s", containerName, err)
	}
	a.currentContainer = nil
	if conf != nil {
		conf.ContainerID = ""
		conf.EnvFileHostDir = ""
	}
}
