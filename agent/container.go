package agent

import (
	"context"

	agentcontainer "github.com/evergreen-ci/evergreen/agent/container"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

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
