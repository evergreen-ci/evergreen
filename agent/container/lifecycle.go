package container

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel"
	otelattribute "go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
)

const (
	// EnvFileMountTarget is the in-container path where the env tmpfs is
	// bind-mounted read-only for env-file forwarding.
	EnvFileMountTarget = "/var/run/evergreen-env"

	// envFileBaseDir is the host-side root for per-task env tmpfs directories.
	// Override with SetEnvFileBaseDir for local dev environments where
	// /var/run is not shared into the Docker VM (e.g. macOS with colima).
	envFileBaseDir = "/var/run/evergreen-env"

	// containerCreateMaxAttempts retries ContainerCreate on EOF, which can
	// happen when the service manager restarts Docker during provisioning —
	// the daemon responds to Ping before it can fully service ContainerCreate.
	containerCreateMaxAttempts = 6

	containerCreateRetryDelay = 5 * time.Second

	// OwnerLabel marks a container as agent-created for task isolation. The
	// orphan reaper matches this label rather than the container name so it
	// cannot remove containers owned by another agent or by a human.
	OwnerLabel = "evergreen.owner"

	// OwnerLabelValue is the value OwnerLabel carries.
	OwnerLabelValue = "evergreen-agent-task-isolation"

	// TaskIDLabel records the task a container was created for, so a leaked
	// container can be attributed to its task.
	TaskIDLabel = "evergreen.task_id"
)

// activeEnvFileBaseDir is the runtime base dir, defaulting to envFileBaseDir.
// Override via SetEnvFileBaseDir before any container operations.
var activeEnvFileBaseDir = envFileBaseDir

// SetEnvFileBaseDir overrides the host-side base directory for env tmpfs
// dirs. Intended for local dev environments where /var/run is not
// accessible inside the Docker daemon's VM (e.g. macOS with colima).
func SetEnvFileBaseDir(dir string) error {
	if !filepath.IsAbs(dir) {
		return errors.Errorf("env base dir must be absolute, got %q", dir)
	}
	activeEnvFileBaseDir = dir
	return nil
}

// Mount is a host→container bind mount layered on top of the workdir mount.
type Mount struct {
	Source   string // Absolute host path.
	Target   string // Absolute container path.
	ReadOnly bool
}

// Config holds the parameters for creating a task isolation container.
type Config struct {
	Image    string
	WorkDir  string // Host path to task working directory.
	TaskID   string
	MemoryMB int64 // 0 means no limit.
	CPUs     int64 // 0 means no limit. In units of whole CPUs.

	// ExtraMounts are additional host→container bind mounts layered on top
	// of the workdir mount. Sources and targets must be absolute paths.
	ExtraMounts []Mount

	// Logger receives operational messages (image pull progress, container
	// lifecycle events). If nil, messages fall back to the global grip
	// sender, which is not visible in the Evergreen task UI.
	Logger grip.Journaler
}

func (c Config) Validate() error {
	if c.Image == "" {
		return errors.New("container image is required")
	}
	if c.WorkDir == "" {
		return errors.New("work directory is required")
	}
	if !filepath.IsAbs(c.WorkDir) {
		return errors.Errorf("work directory must be absolute, got %q", c.WorkDir)
	}
	if c.TaskID == "" {
		return errors.New("task ID is required")
	}
	if c.MemoryMB < 0 {
		return errors.New("memory limit cannot be negative")
	}
	if c.CPUs < 0 {
		return errors.New("CPU limit cannot be negative")
	}
	for i, m := range c.ExtraMounts {
		if !filepath.IsAbs(m.Source) {
			return errors.Errorf("extra mount %d source must be absolute, got %q", i, m.Source)
		}
		if !filepath.IsAbs(m.Target) {
			return errors.Errorf("extra mount %d target must be absolute, got %q", i, m.Target)
		}
	}
	return nil
}

func (c Config) containerName() string {
	return fmt.Sprintf("evergreen-task-%s", c.TaskID)
}

// TaskContainer represents a running isolation container for a single task.
type TaskContainer struct {
	ID             string // Docker container ID (short hash).
	Name           string // Human-readable container name.
	EnvFileHostDir string // host-side tmpfs dir for env-file forwarding; empty if not provisioned.
	cli            *client.Client
}

// GetID returns the Docker container ID.
func (tc *TaskContainer) GetID() string { return tc.ID }

// GetName returns the human-readable container name.
func (tc *TaskContainer) GetName() string { return tc.Name }

// GetEnvFileHostDir returns the host-side tmpfs directory path.
func (tc *TaskContainer) GetEnvFileHostDir() string { return tc.EnvFileHostDir }

// envHostDir returns the host-side tmpfs directory path for the given task ID.
func envHostDir(taskID string) string {
	return filepath.Join(activeEnvFileBaseDir, taskID)
}

// CreateAndStart creates a Docker container for task isolation and starts it.
// The container runs `sleep infinity` while the agent docker execs commands
// into it. The host task working directory is bind-mounted at the same path
// inside the container. A per-task tmpfs is bind-mounted read-only into the
// container at EnvFileMountTarget for env-file forwarding.
// The caller must call Destroy when the task is complete.
//
// The container image must include the exec_user account (e.g. uid=1000
// ubuntu) in its /etc/passwd, since task commands run via
// `docker exec --user=<exec_user>`. Minimal base images like ubuntu:22.04
// only contain root and will produce "unable to find user" errors at exec time.
func CreateAndStart(ctx context.Context, cfg Config) (*TaskContainer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid config")
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, errors.Wrap(err, "creating Docker client")
	}

	// Pull the image if not already present.
	if err := ensureImage(ctx, cli, cfg.Image, cfg.Logger); err != nil {
		cli.Close()
		return nil, errors.Wrap(err, "ensuring container image")
	}

	// Provision the per-task tmpfs before container create so the bind
	// mount can be included at create time.
	envDir := envHostDir(cfg.TaskID)
	if err := provisionEnvTmpfs(envDir); err != nil {
		cli.Close()
		return nil, errors.Wrap(err, "provisioning env tmpfs")
	}

	containerCfg := &container.Config{
		Image:      cfg.Image,
		Cmd:        []string{"sleep", "infinity"},
		WorkingDir: cfg.WorkDir,
		Tty:        false,
		Labels: map[string]string{
			OwnerLabel:  OwnerLabelValue,
			TaskIDLabel: cfg.TaskID,
		},
	}

	mounts := []mount.Mount{
		{
			Type:   mount.TypeBind,
			Source: cfg.WorkDir,
			Target: cfg.WorkDir,
		},
		{
			Type:     mount.TypeBind,
			Source:   envDir,
			Target:   EnvFileMountTarget,
			ReadOnly: true,
		},
	}
	for _, m := range cfg.ExtraMounts {
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   m.Source,
			Target:   m.Target,
			ReadOnly: m.ReadOnly,
		})
	}

	hostCfg := &container.HostConfig{
		Init:   utility.TruePtr(),
		Mounts: mounts,
	}

	if cfg.MemoryMB > 0 {
		hostCfg.Resources.Memory = cfg.MemoryMB * 1024 * 1024
	}
	if cfg.CPUs > 0 {
		hostCfg.Resources.NanoCPUs = cfg.CPUs * 1e9
	}

	// Retry ContainerCreate on EOF. On some hosts the service manager
	// restarts Docker during provisioning; the daemon may respond to Ping
	// before it can fully service ContainerCreate.
	name := cfg.containerName()
	var resp container.CreateResponse
	for attempt := range containerCreateMaxAttempts {
		resp, err = cli.ContainerCreate(ctx, containerCfg, hostCfg, nil, nil, name)
		if err == nil {
			break
		}
		if !isDockerEOF(err) || attempt == containerCreateMaxAttempts-1 {
			_ = removeEnvTmpfs(envDir)
			cli.Close()
			return nil, errors.Wrap(err, "creating container")
		}
		select {
		case <-ctx.Done():
			_ = removeEnvTmpfs(envDir)
			cli.Close()
			return nil, ctx.Err()
		case <-time.After(containerCreateRetryDelay):
		}
	}

	if err := startContainer(ctx, cli, resp.ID); err != nil {
		_ = removeEnvTmpfs(envDir)
		cli.Close()
		return nil, err
	}

	return &TaskContainer{
		ID:             resp.ID,
		Name:           name,
		EnvFileHostDir: envDir,
		cli:            cli,
	}, nil
}

// containerStopTimeoutSecs is the grace period before force-removing the
// container. Docker sends SIGTERM to PID 1 on ContainerStop; if PID 1
// exits within this window the container stops cleanly, otherwise
// ContainerStop sends SIGKILL and Destroy force-removes the container.
//
// Because CreateAndStart sets Init=true, Docker's --init wrapper (tini)
// is PID 1. tini forwards SIGTERM to its direct child (`sleep infinity`),
// but processes started via `docker exec` are independent and do not
// receive the signal — they are force-killed by the SIGKILL after the
// timeout, not gracefully terminated.
const containerStopTimeoutSecs = 10

// containerStopClientBufferSecs is extra client-side context beyond the
// daemon-side stop timeout. Without this buffer the client gets a
// context-deadline error even though the daemon is still gracefully
// stopping the container, which would log a spurious failure and fall
// through to force-remove.
const containerStopClientBufferSecs = 5

// containerRemoveTimeoutSecs bounds the force-remove operation so an
// unresponsive Docker daemon cannot block Destroy indefinitely.
const containerRemoveTimeoutSecs = 30

type containerStartRemover interface {
	ContainerStart(context.Context, string, container.StartOptions) error
	ContainerRemove(context.Context, string, container.RemoveOptions) error
}

func startContainer(ctx context.Context, cli containerStartRemover, id string) error {
	if err := cli.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
		removeCtx, removeCancel := context.WithTimeout(context.WithoutCancel(ctx), time.Duration(containerRemoveTimeoutSecs)*time.Second)
		defer removeCancel()
		if removeErr := cli.ContainerRemove(removeCtx, id, container.RemoveOptions{Force: true}); removeErr != nil {
			grip.Warningf(removeCtx, "Failed to remove container '%s' after start error: %s", id, removeErr)
		}
		return errors.Wrap(err, "starting container")
	}
	return nil
}

// Destroy gracefully stops the container, force-removes it, and cleans up
// the env tmpfs. Cleanup uses bounded contexts detached from the caller so
// that caller-side cancellation does not bypass container removal or tmpfs
// cleanup, both of which must complete to avoid leaking resources on the host.
func (tc *TaskContainer) Destroy(ctx context.Context) error {
	defer tc.cli.Close()

	// Detached, bounded contexts so caller cancellation does not bypass
	// cleanup while still preventing an unresponsive daemon from hanging.
	// The client context gets buffer beyond the daemon-side timeout so
	// the daemon can complete its SIGTERM→wait→SIGKILL cycle.
	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Duration(containerStopTimeoutSecs+containerStopClientBufferSecs)*time.Second)
	stopTimeout := containerStopTimeoutSecs
	if err := tc.cli.ContainerStop(stopCtx, tc.ID, container.StopOptions{Timeout: &stopTimeout}); err != nil {
		grip.Debugf(stopCtx, "graceful container stop failed for '%s', proceeding to force-remove: %s", tc.ID, err)
	}
	stopCancel()

	removeCtx, removeCancel := context.WithTimeout(context.Background(), time.Duration(containerRemoveTimeoutSecs)*time.Second)
	removeErr := errors.Wrap(
		tc.cli.ContainerRemove(removeCtx, tc.ID, container.RemoveOptions{Force: true}),
		"removing container",
	)
	removeCancel()

	var envErr error
	if tc.EnvFileHostDir != "" {
		envErr = errors.Wrap(removeEnvTmpfs(tc.EnvFileHostDir), "removing env tmpfs")
	}

	if removeErr != nil {
		if envErr != nil {
			grip.Error(ctx, message.WrapError(envErr, message.Fields{
				"message": "env tmpfs cleanup failed after container removal error; mount may persist",
				"dir":     tc.EnvFileHostDir,
			}))
		}
		return removeErr
	}
	return envErr
}

// imagePullTimeout caps the time allowed to pull a container image.
const imagePullTimeout = 5 * time.Minute

// isDockerEOF reports whether err is the "connection closed before response"
// error returned when the Docker daemon drops the socket mid-request during
// a restart. Retrying after a brief pause allows the new daemon to start up.
func isDockerEOF(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

// ensureImage pulls the image if not already present locally. For ECR
// registries it fetches a short-lived auth token via the instance's IAM
// role, avoiding any dependency on external credential helpers.
func ensureImage(ctx context.Context, cli *client.Client, img string, log grip.Journaler) error {
	_, _, err := cli.ImageInspectWithRaw(ctx, img)
	if err == nil {
		return nil // Already present.
	}

	ctx, pullSpan := otel.GetTracerProvider().Tracer("evergreen.agent.container").Start(ctx, "container.image_pull")
	defer pullSpan.End()
	pullSpan.SetAttributes(otelattribute.String("container.image", img))

	pullCtx, cancel := context.WithTimeout(ctx, imagePullTimeout)
	defer cancel()

	logInfo(ctx, log, message.Fields{
		"message": "pulling container image",
		"image":   img,
	})

	var registryAuth string
	if isECRImage(img) {
		registryAuth, err = ecrRegistryAuth(ctx, img)
		if err != nil {
			return errors.Wrap(err, "getting ECR registry credentials")
		}
	}

	reader, err := cli.ImagePull(pullCtx, img, image.PullOptions{RegistryAuth: registryAuth})
	if err != nil {
		pullSpan.SetStatus(otelcodes.Error, err.Error())
		return errors.Wrapf(err, "pulling image '%s'", img)
	}
	defer reader.Close()

	// Docker streams pull progress and errors as newline-delimited JSON.
	// Auth errors and "image not found" come through in the stream as
	// JSONMessage objects with a non-nil Error field, not as top-level
	// errors from ImagePull. Loop until io.EOF rather than dec.More()
	// because More() returns false on context expiry without propagating
	// the error, making a timed-out pull indistinguishable from success.
	dec := json.NewDecoder(reader)
	for {
		var msg jsonmessage.JSONMessage
		if decErr := dec.Decode(&msg); decErr != nil {
			if decErr == io.EOF {
				break // Stream ended cleanly.
			}
			if pullCtx.Err() != nil {
				return errors.Wrapf(pullCtx.Err(), "pulling image '%s'", img)
			}
			return errors.Wrap(decErr, "decoding pull response")
		}
		if msg.Error != nil {
			return errors.Wrapf(msg.Error, "pulling image '%s'", img)
		}
		if msg.ErrorMessage != "" {
			return errors.Errorf("pulling image '%s': %s", img, msg.ErrorMessage)
		}
	}

	logInfo(ctx, log, message.Fields{
		"message": "container image ready",
		"image":   img,
	})

	return nil
}

// isECRImage reports whether img is hosted on a private Amazon ECR registry.
func isECRImage(img string) bool {
	host := imageRegistryHost(img)
	return strings.Contains(host, ".dkr.ecr.") && strings.HasSuffix(host, ".amazonaws.com")
}

// imageRegistryHost extracts the registry hostname from a Docker image
// reference. A registry prefix is only present when there is a '/' and the
// component before it contains a '.' or ':', or equals "localhost".
func imageRegistryHost(img string) string {
	first, _, hasSlash := strings.Cut(img, "/")
	if hasSlash && (strings.ContainsAny(first, ".:") || first == "localhost") {
		return first
	}
	return "registry-1.docker.io"
}

// ecrRegistryAuth fetches a short-lived ECR authorization token via the
// instance's IAM role and returns it base64-encoded in the format the
// Docker API expects for RegistryAuth.
func ecrRegistryAuth(ctx context.Context, img string) (string, error) {
	host := imageRegistryHost(img)

	// ECR private registry format: <account>.dkr.ecr.<region>.amazonaws.com
	parts := strings.Split(host, ".")
	var region string
	for i, p := range parts {
		if p == "ecr" && i+1 < len(parts) {
			region = parts[i+1]
			break
		}
	}
	if region == "" {
		return "", errors.Errorf("could not determine AWS region from ECR host '%s'", host)
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return "", errors.Wrap(err, "loading AWS config")
	}

	// Use a dedicated timeout for the token fetch so it doesn't consume
	// the pull context's budget.
	ecrCtx, ecrCancel := context.WithTimeout(ctx, 30*time.Second)
	defer ecrCancel()

	resp, err := ecr.NewFromConfig(cfg).GetAuthorizationToken(ecrCtx, &ecr.GetAuthorizationTokenInput{})
	if err != nil {
		return "", errors.Wrap(err, "getting ECR authorization token")
	}
	if len(resp.AuthorizationData) == 0 {
		return "", errors.New("ECR returned no authorization data")
	}

	token := resp.AuthorizationData[0].AuthorizationToken
	if token == nil {
		return "", errors.New("ECR returned a nil authorization token")
	}

	// AuthorizationToken is base64("AWS:<password>").
	decoded, err := base64.StdEncoding.DecodeString(aws.ToString(token))
	if err != nil {
		return "", errors.Wrap(err, "decoding ECR authorization token")
	}
	username, password, ok := strings.Cut(string(decoded), ":")
	if !ok {
		return "", errors.New("unexpected ECR authorization token format")
	}

	authJSON, err := json.Marshal(registry.AuthConfig{
		Username:      username,
		Password:      password,
		ServerAddress: host,
	})
	if err != nil {
		return "", errors.Wrap(err, "encoding registry auth config")
	}
	return base64.URLEncoding.EncodeToString(authJSON), nil
}

// logInfo sends an Info-level message to log if non-nil, otherwise falls
// back to the global grip sender.
func logInfo(ctx context.Context, log grip.Journaler, msg any) {
	if log != nil {
		log.Info(ctx, msg)
		return
	}
	grip.Info(ctx, msg)
}
