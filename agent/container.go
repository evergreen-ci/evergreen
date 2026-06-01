package agent

import (
	"context"

	agentcontainer "github.com/evergreen-ci/evergreen/agent/container"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// maybeStartContainer starts a Docker container for task isolation if the
// distro has container isolation enabled. It sets conf.ContainerID so
// commands know to route execution through `docker exec`.
func (a *Agent) maybeStartContainer(ctx context.Context, conf *internal.TaskConfig) error {
	if conf.Distro == nil || conf.Distro.ContainerIsolation == nil {
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
	grip.Infof(ctx, "Started isolation container '%s' (image=%s) for task '%s'.", tc.Name, ci.Image, conf.Task.Id)
	return nil
}

// destroyContainer tears down the isolation container started by maybeStartContainer.
func (a *Agent) destroyContainer(ctx context.Context, conf *internal.TaskConfig) {
	if a.currentContainer == nil {
		return
	}
	if err := a.currentContainer.Destroy(ctx); err != nil {
		grip.Warningf(ctx, "Failed to destroy isolation container for task '%s': %s", conf.Task.Id, err)
	}
	a.currentContainer = nil
	conf.ContainerID = ""
	conf.EnvFileHostDir = ""
}
