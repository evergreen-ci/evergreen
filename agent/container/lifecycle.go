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
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel"
	otelattribute "go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
)

const (
	// EnvFileMountTarget is the in-container path where the env tmpfs is bind-mounted (read-only).
	// Phase 0 consumers read the env-file host-side (docker exec --env-file reads the host path
	// directly). The in-container mount is reserved for Phase 1, where in-process consumers
	// inside the container will read expansions from this path directly. Keeping the mount now
	// avoids a container-spec change at Phase 1 boundary.
	EnvFileMountTarget = "/var/run/evergreen-env"

	// envFileBaseDir is the host-side root for per-task env tmpfs directories.
	// Override with SetEnvFileBaseDir for local dev environments (e.g. macOS
	// with colima where /var/run is not shared into the Docker VM).
	envFileBaseDir = "/var/run/evergreen-env"

	// containerCreateMaxAttempts is the number of times ContainerCreate is
	// retried on EOF. Docker responds to Ping before it can fully service
	// ContainerCreate, so on hosts where the service manager restarts Docker
	// during provisioning we retry rather than failing open immediately.
	containerCreateMaxAttempts = 6

	// containerCreateRetryDelay is the pause between ContainerCreate retries.
	containerCreateRetryDelay = 5 * time.Second
)

// activeEnvFileBaseDir is the runtime base dir, defaulting to envFileBaseDir.
// Override via SetEnvFileBaseDir before any container operations.
var activeEnvFileBaseDir = envFileBaseDir

// SetEnvFileBaseDir overrides the host-side base directory for env tmpfs dirs.
// Must be called before CreateAndStart. Intended for local dev environments
// where /var/run is not accessible inside the Docker daemon's VM (e.g. macOS
// with colima). Has no effect on production Linux hosts.
func SetEnvFileBaseDir(dir string) error {
	if !filepath.IsAbs(dir) {
		return errors.Errorf("env base dir must be absolute, got %q", dir)
	}
	activeEnvFileBaseDir = dir
	return nil
}

// Mount is a host→container bind mount layered on top of the workdir mount.
// Used by the GOAL-279 design's host-bind toolchain delivery (e.g. host /opt
// mounted read-only into the container) so toolchains live on the host and
// are not baked into the image.
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
	// lifecycle events). If nil, messages fall back to the global grip sender,
	// which writes to the host's local log and is not visible in the
	// Evergreen task UI.
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
// The container runs `sleep infinity` to stay alive while the agent `docker exec`s
// commands into it. The host task working directory is bind-mounted at the same
// path inside the container (same-path semantics). A per-task tmpfs is provisioned
// on the host and bind-mounted read-only into the container at EnvFileMountTarget
// for env-file forwarding.
// The caller must call Destroy when the task is complete.
//
// Image requirements: the container image must include the exec_user account
// (e.g. uid=1000 ubuntu) in its /etc/passwd. Task commands are run via
// `docker exec --user=<exec_user>`, which requires the user to exist inside
// the container. Minimal base images (e.g. ubuntu:22.04) only contain root
// and will produce "unable to find user" errors at exec time.
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

	// Provision the per-task tmpfs for env-file forwarding before container
	// create, so the bind mount can be included at create time.
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
		Init:   boolPtr(true),
		Mounts: mounts,
	}

	if cfg.MemoryMB > 0 {
		hostCfg.Resources.Memory = cfg.MemoryMB * 1024 * 1024
	}
	if cfg.CPUs > 0 {
		hostCfg.Resources.NanoCPUs = cfg.CPUs * 1e9
	}

	// Create the container, retrying on EOF. On some hosts (e.g. AL2023 arm64)
	// the service manager restarts Docker as part of provisioning. Docker may
	// respond to Ping before it can fully service ContainerCreate, so Ping-gating
	// alone isn't sufficient. On EOF we wait briefly and retry; the daemon is
	// typically ready within a few seconds of starting.
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

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true}) //nolint
		_ = removeEnvTmpfs(envDir)
		cli.Close()
		return nil, errors.Wrap(err, "starting container")
	}

	return &TaskContainer{
		ID:             resp.ID,
		Name:           name,
		EnvFileHostDir: envDir,
		cli:            cli,
	}, nil
}

// Close releases the underlying Docker client connection without removing the
// container or its tmpfs. Use when the container is intentionally left running
// (e.g. retain_on_failure_secs) and lifecycle management is complete.
func (tc *TaskContainer) Close() {
	_ = tc.cli.Close()
}

// containerStopTimeoutSecs is the grace period (in seconds) given to
// in-container processes during a graceful shutdown before force-removing
// the container. Docker sends SIGTERM to PID 1 on ContainerStop; if PID 1
// exits within this window the container stops cleanly. If the grace period
// elapses, ContainerStop sends SIGKILL and Destroy proceeds to the
// force-remove fallback so it never hangs indefinitely.
//
// Known limitation: CreateAndStart sets hostCfg.Init=true, so Docker's
// --init wrapper (tini) is PID 1, not `sleep infinity`. tini forwards
// SIGTERM to its direct child (`sleep infinity`), but processes started
// via `docker exec` are independent — they are not children of tini and
// do not receive the signal. Workload processes launched through docker
// exec are therefore force-killed by the SIGKILL after the timeout, not
// gracefully terminated. This is an accepted Phase 0 limitation; the
// graceful stop still ensures the container's init tree exits cleanly,
// and the force-remove fallback ensures cleanup always completes.
const containerStopTimeoutSecs = 10

// containerStopClientBufferSecs is extra time added to the client-side
// context beyond the daemon-side StopOptions.Timeout. The daemon needs
// the full StopOptions.Timeout to send SIGTERM, wait, and SIGKILL before
// it can respond. If the client context expires at the same time, the
// client gets a context-deadline error even though the daemon is still
// gracefully stopping the container — which would log a spurious "graceful
// container stop failed" and fall through to force-remove, defeating the
// purpose of the graceful stop. The buffer covers network round-trip and
// daemon processing overhead.
const containerStopClientBufferSecs = 5

// containerRemoveTimeoutSecs bounds the force-remove operation. An
// unresponsive Docker daemon can block ContainerRemove indefinitely;
// this timeout ensures Destroy completes even if the daemon is stuck.
const containerRemoveTimeoutSecs = 30

// Destroy gracefully stops the container (SIGTERM + grace period), then
// force-removes it and cleans up the env tmpfs. If the graceful stop fails or
// times out, the force-remove ensures the container is still removed. The
// Docker client is always closed.
//
// Cleanup operations use bounded background contexts detached from the caller
// so that caller-side cancellation (e.g. task timeout, agent shutdown) does
// not bypass container removal or tmpfs cleanup — both of which must complete
// to avoid leaking containers and mounts on the host.
func (tc *TaskContainer) Destroy(ctx context.Context) error {
	defer tc.cli.Close()

	// Use detached, bounded contexts for cleanup so caller cancellation
	// do not bypass container removal or tmpfs cleanup, while still
	// preventing an unresponsive Docker daemon from hanging Destroy.
	// The client context gets buffer beyond the daemon-side timeout so
	// the daemon has time to complete its SIGTERM→wait→SIGKILL cycle
	// before the client cancels.
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

// imagePullTimeout caps the time allowed to pull a container image. Long
// enough to accommodate large images over a slow link, short enough that a
// hung or unauthenticated pull doesn't stall the agent silently for the
// entire task duration.
const imagePullTimeout = 5 * time.Minute

// isDockerEOF reports whether err is the specific "connection closed before
// response" error returned by the Docker SDK when the daemon drops the socket
// mid-request. This happens when Docker is restarting — the daemon accepts the
// TCP connection but closes it before writing a response. Retrying after a
// brief pause allows the new daemon instance to finish starting up.
func isDockerEOF(err error) bool {
	return err != nil && strings.Contains(err.Error(), "EOF")
}

// ensureImage pulls the image if it is not already present locally. For ECR
// registries it fetches a short-lived auth token via the instance's IAM role
// and passes it directly to the Docker API, avoiding any dependency on
// external credential helpers or CLI tooling.
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
	// ImagePull only returns a top-level error for connection failures; auth
	// errors and "image not found" come through in the stream as JSONMessage
	// objects with a non-nil Error field. Loop until io.EOF rather than
	// dec.More() because More() returns false on context expiry without
	// propagating the error, making a timed-out pull indistinguishable from
	// a successful one.
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

// isECRImage reports whether img is hosted on Amazon ECR (private or public).
func isECRImage(img string) bool {
	host := imageRegistryHost(img)
	return strings.Contains(host, ".dkr.ecr.") && strings.HasSuffix(host, ".amazonaws.com")
}

// imageRegistryHost extracts the registry hostname from a Docker image reference.
func imageRegistryHost(img string) string {
	// Format: [host[:port]/]name[:tag|@digest]
	// A registry prefix is only present when there is a '/' AND the component
	// before it contains a '.' or ':', or equals "localhost". Without a '/',
	// the entire string is a Docker Hub name (possibly with a tag), not a host.
	first, _, hasSlash := strings.Cut(img, "/")
	if hasSlash && (strings.ContainsAny(first, ".:") || first == "localhost") {
		return first
	}
	return "registry-1.docker.io"
}

// ecrRegistryAuth fetches a short-lived ECR authorization token via the
// instance's IAM role and returns it base64-encoded in the format the Docker
// API expects for RegistryAuth.
func ecrRegistryAuth(ctx context.Context, img string) (string, error) {
	host := imageRegistryHost(img)

	// ECR private registry format: <account>.dkr.ecr.<region>.amazonaws.com
	// Extract the region from the hostname.
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

	// Use a short dedicated timeout for the token fetch so it doesn't consume
	// the pull context's budget. 30 seconds is generous for a single API call.
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

// logInfo sends an Info-level message to log if non-nil, otherwise falls back
// to the global grip sender. This lets callers that have a task-specific
// logger (whose output is visible in the Evergreen UI) surface container
// lifecycle events in the task's agent log tab.
func logInfo(ctx context.Context, log grip.Journaler, msg interface{}) {
	if log != nil {
		log.Info(ctx, msg)
		return
	}
	grip.Info(ctx, msg)
}

func boolPtr(b bool) *bool { return &b }
