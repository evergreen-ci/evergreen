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
	EnvFileMountTarget = "/var/run/evergreen-env"

	// envFileBaseDir is the host-side root for per-task env tmpfs directories.
	// Override with SetEnvFileBaseDir for local dev environments (e.g. macOS
	// with colima where /var/run is not shared into the Docker VM).
	envFileBaseDir = "/var/run/evergreen-env"
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

	name := cfg.containerName()
	resp, err := cli.ContainerCreate(ctx, containerCfg, hostCfg, nil, nil, name)
	if err != nil {
		_ = removeEnvTmpfs(envDir)
		cli.Close()
		return nil, errors.Wrap(err, "creating container")
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

// Destroy force-removes the container and cleans up the env tmpfs, then closes
// the Docker client.
func (tc *TaskContainer) Destroy(ctx context.Context) error {
	defer tc.cli.Close()

	removeErr := errors.Wrap(
		tc.cli.ContainerRemove(ctx, tc.ID, container.RemoveOptions{Force: true}),
		"removing container",
	)

	var envErr error
	if tc.EnvFileHostDir != "" {
		envErr = errors.Wrap(removeEnvTmpfs(tc.EnvFileHostDir), "removing env tmpfs")
	}

	if removeErr != nil {
		if envErr != nil {
			// Container removal failed, so log the tmpfs cleanup failure rather
			// than dropping it; the mount may persist on the host.
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
