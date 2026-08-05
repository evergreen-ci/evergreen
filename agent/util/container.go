package util

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mongodb/grip"
	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
)

const (
	// containerEnvFileName is the prefix for the per-command env-file written to
	// the tmpfs dir.
	containerEnvFileName = ".evg-env"
	// containerHostEnvFileName is the env-file capturing the host's profile
	// environment (PATH, GOROOT, etc.), written during container setup.
	containerHostEnvFileName = ".evg-host-env"
)

// WrapWithContainer rewrites opts.Args to run the command inside the given
// Docker container via `docker exec`, applying workdir, environment, exec user,
// and nice settings. It is a no-op if containerID is empty.
//
// If envFileHostDir is non-empty, opts.Environment is written there as an
// env-file readable only by the agent's user. Docker's --env-file parser does
// not strip surrounding quotes from values, so expansions must be set without
// literal surrounding quotes or they reach the container with the quotes intact.
func WrapWithContainer(ctx context.Context, opts *options.Create, containerID, workdir, envFileHostDir string) error {
	if containerID == "" {
		return nil
	}

	// -i keeps stdin open so the container process receives it. Without this,
	// docker exec closes the container process's stdin immediately, causing
	// shell.exec commands that pass the script via stdin (the default mode) to
	// get no input and exit 0 silently rather than running the script.
	args := []string{"docker", "exec", "-i"}

	if workdir != "" {
		args = append(args, "--workdir="+workdir)
	}

	if envFileHostDir != "" {
		// Docker applies --env-file args in order, so the host-env file goes
		// first and the per-command file second in order to let command-specific
		// values override the host's.
		hostEnvPath := filepath.Join(envFileHostDir, containerHostEnvFileName)
		switch _, err := os.Stat(hostEnvPath); {
		case err == nil:
			args = append(args, "--env-file="+hostEnvPath)
		case !os.IsNotExist(err):
			// The file exists but is unreadable, so the container would silently
			// lose PATH and surface it later as a command-not-found failure.
			grip.Warningf(ctx, "checking container host env file '%s', continuing without it: %s", hostEnvPath, err)
		}

		if len(opts.Environment) > 0 {
			envFilePath, err := writeEnvFile(ctx, envFileHostDir, opts.Environment)
			if err != nil {
				return errors.Wrap(err, "writing container env file")
			}
			args = append(args, "--env-file="+envFilePath)
		}
	}

	// Translate the `sudo -u <user>` prefix that Jasper's SudoAs emits into
	// `docker exec --user`, so the container image does not need sudo installed.
	// Plain Sudo(true) emits ["sudo", ...rest] with no user and is forwarded into
	// the container as-is; Evergreen only uses SudoAs for container-eligible
	// commands.
	if len(opts.Args) > 3 && opts.Args[0] == "sudo" && opts.Args[1] == "-u" {
		args = append(args, "--user="+opts.Args[2])
		opts.Args = opts.Args[3:]
	}

	args = append(args, containerID)
	// The agent resets its nice to DefaultNice before forking host subprocesses,
	// but that does not cross the container boundary because the Docker daemon
	// starts the in-container process independently, so re-apply it in the
	// container. `nice -n N` is a relative increment, so this only lands on
	// DefaultNice when the daemon itself runs at nice 0, which holds for the
	// system-managed daemons on all target hosts. The prefix is skipped when it
	// would be a no-op, since it would otherwise require every task image to
	// provide nice.
	if DefaultNice != 0 {
		args = append(args, "nice", "-n", strconv.Itoa(DefaultNice))
	}
	opts.Args = append(args, opts.Args...)
	return nil
}

// writeEnvFile serializes env as KEY=VALUE lines to a unique file in dir with
// mode 0600. Each command gets its own file so that concurrent wrappers cannot
// replace each other's environment before docker exec reads it; the files live
// on the container's env tmpfs and are removed when it is torn down. Values
// containing newlines are dropped with a warning because the Docker env-file
// format cannot represent them.
func writeEnvFile(ctx context.Context, dir string, env map[string]string) (string, error) {
	var sb strings.Builder
	for k, v := range env {
		if strings.ContainsAny(v, "\n\r") {
			grip.Warningf(ctx, "skipping expansion '%s' in container env file: value contains a newline and cannot be forwarded via docker exec --env-file", k)
			continue
		}
		fmt.Fprintf(&sb, "%s=%s\n", k, v)
	}

	tmp, err := os.CreateTemp(dir, containerEnvFileName+"-")
	if err != nil {
		return "", errors.Wrapf(err, "creating env file in '%s'", dir)
	}
	path := tmp.Name()

	_, writeErr := tmp.WriteString(sb.String())
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(path)
		if writeErr != nil {
			return "", errors.Wrap(writeErr, "writing env file")
		}
		return "", errors.Wrap(closeErr, "closing env file")
	}

	return path, nil
}
