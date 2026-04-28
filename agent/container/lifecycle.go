package container

import (
	"context"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"
)

const (
	// WorkDirInContainer is where the host task workdir is mounted inside the container.
	WorkDirInContainer = "/work"
)

// Config holds the parameters for creating a task isolation container.
type Config struct {
	Image    string
	WorkDir  string // Host path to task working directory.
	TaskID   string
	MemoryMB int64 // 0 means no limit.
	CPUs     int64 // 0 means no limit. In units of whole CPUs.
}

func (c Config) Validate() error {
	if c.Image == "" {
		return errors.New("container image is required")
	}
	if c.WorkDir == "" {
		return errors.New("work directory is required")
	}
	if c.TaskID == "" {
		return errors.New("task ID is required")
	}
	return nil
}

func (c Config) containerName() string {
	return fmt.Sprintf("evergreen-task-%s", c.TaskID)
}

// TaskContainer represents a running isolation container for a single task.
type TaskContainer struct {
	ID   string // Docker container ID (short hash).
	Name string // Human-readable container name.
	cli  *client.Client
}

// CreateAndStart creates a Docker container for task isolation and starts it.
// The container runs `sleep infinity` to stay alive while the agent `docker exec`s
// commands into it. The host task working directory is bind-mounted at /work.
// The caller must call Destroy when the task is complete.
func CreateAndStart(ctx context.Context, cfg Config) (*TaskContainer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid config")
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, errors.Wrap(err, "creating Docker client")
	}

	// Pull the image if not already present.
	if err := ensureImage(ctx, cli, cfg.Image); err != nil {
		cli.Close()
		return nil, errors.Wrap(err, "ensuring container image")
	}

	containerCfg := &container.Config{
		Image:      cfg.Image,
		Cmd:        []string{"sleep", "infinity"},
		WorkingDir: WorkDirInContainer,
		Tty:        false,
	}

	hostCfg := &container.HostConfig{
		Init: boolPtr(true),
		Mounts: []mount.Mount{{
			Type:   mount.TypeBind,
			Source: cfg.WorkDir,
			Target: WorkDirInContainer,
		}},
	}

	if cfg.MemoryMB > 0 {
		hostCfg.Resources.Memory = cfg.MemoryMB * 1024 * 1024
	}
	if cfg.CPUs > 0 {
		hostCfg.Resources.NanoCPUs = cfg.CPUs * 1e9
	}

	name := cfg.containerName()
	resp, err := cli.ContainerCreate(ctx, containerCfg, hostCfg, nil, nil, name)
	if err != nil {
		cli.Close()
		return nil, errors.Wrap(err, "creating container")
	}

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true}) //nolint
		cli.Close()
		return nil, errors.Wrap(err, "starting container")
	}

	return &TaskContainer{
		ID:   resp.ID,
		Name: name,
		cli:  cli,
	}, nil
}

// Destroy force-removes the container and closes the Docker client.
func (tc *TaskContainer) Destroy(ctx context.Context) error {
	defer tc.cli.Close()
	return errors.Wrap(
		tc.cli.ContainerRemove(ctx, tc.ID, container.RemoveOptions{Force: true}),
		"removing container",
	)
}

// ensureImage pulls the image if it is not already present locally.
func ensureImage(ctx context.Context, cli *client.Client, img string) error {
	_, _, err := cli.ImageInspectWithRaw(ctx, img)
	if err == nil {
		return nil // Already present.
	}
	reader, err := cli.ImagePull(ctx, img, image.PullOptions{})
	if err != nil {
		return errors.Wrapf(err, "pulling image '%s'", img)
	}
	defer reader.Close()
	_, _ = io.Copy(io.Discard, reader)
	return nil
}

func boolPtr(b bool) *bool { return &b }
