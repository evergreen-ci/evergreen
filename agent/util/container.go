package util

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mongodb/grip"
	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
)

const (
	// containerEnvFileName is the name of the env-file written to the tmpfs dir.
	containerEnvFileName = ".evg-env"
)

// WrapWithContainer prepends `docker exec <containerID>` to opts.Args,
// routing command execution into the specified Docker container. It also:
//   - adds `--workdir=<workdir>` if workdir is non-empty
//   - writes opts.Environment to <envFileHostDir>/.evg-env (mode 0600) and adds
//     `--env-file=<path>` if envFileHostDir is non-empty
//   - prepends `nice -n <DefaultNice>` to the in-container argv so workload
//     processes run at DefaultNice on standard EC2 hosts where the Docker daemon
//     runs at nice 0. POSIX fork inheritance of nice breaks at the container
//     boundary (docker exec starts the process via the daemon rather than
//     inheriting from the agent's pre-fork nice reset); the prefix is the
//     workaround. See inline comment for the known daemon-nice-offset caveat.
//
// Note: Docker's --env-file parser does not strip surrounding quotes from
// values, so a value like "-Xmx1g" reaches the container with quotes intact.
// Expansion values with literal surrounding quotes should be set without them.
//
// It is a no-op if containerID is empty.
func WrapWithContainer(opts *options.Create, containerID, workdir, envFileHostDir string) error {
	if containerID == "" {
		return nil
	}

	args := []string{"docker", "exec"}

	if workdir != "" {
		args = append(args, "--workdir="+workdir)
	}

	if envFileHostDir != "" {
		envFilePath := filepath.Join(envFileHostDir, containerEnvFileName)
		if err := writeEnvFile(envFilePath, opts.Environment); err != nil {
			return errors.Wrap(err, "writing container env file")
		}
		args = append(args, "--env-file="+envFilePath)
	}

	// If the command was wrapped with `sudo -u <user>` to run as exec_user,
	// strip that prefix and use `docker exec --user` instead. Docker handles
	// the user switch natively, removing the need for sudo to be installed
	// inside the container image.
	if len(opts.Args) >= 3 && opts.Args[0] == "sudo" && opts.Args[1] == "-u" {
		args = append(args, "--user="+opts.Args[2])
		opts.Args = opts.Args[3:]
	}

	args = append(args, containerID)
	// Reset the in-container process nice via `nice -n DefaultNice`. The agent
	// runs at AgentNice and resets to DefaultNice before forking host
	// subprocesses, but that reset does not propagate across the container
	// boundary (Docker daemon starts the in-container process independently).
	// `nice -n N` is a relative increment: it adjusts the current process's
	// niceness by N. On standard EC2 hosts the Docker daemon runs at nice 0,
	// so `nice -n 0` results in nice 0 (DefaultNice). If the daemon runs at a
	// non-zero nice, the increment preserves that offset; we accept this as a
	// known limitation for Phase 0 where all target hosts use system-managed
	// Docker daemons at default nice.
	args = append(args, "nice", "-n", fmt.Sprintf("%d", DefaultNice))
	opts.Args = append(args, opts.Args...)
	return nil
}

// writeEnvFile serializes env as KEY=VALUE lines to path with mode 0600.
// The write is atomic (temp file + rename) to prevent concurrent docker exec
// invocations from observing a partially-written file.
// Values that contain newlines are skipped because the Docker env-file format
// does not support multi-line values; a warning is logged for each dropped var.
func writeEnvFile(path string, env map[string]string) error {
	var sb strings.Builder
	for k, v := range env {
		if strings.ContainsAny(v, "\n\r") {
			grip.Warningf(context.Background(), "skipping expansion '%s' in container env file: value contains a newline and cannot be forwarded via docker exec --env-file", k)
			continue
		}
		fmt.Fprintf(&sb, "%s=%s\n", k, v)
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".evg-env-tmp-")
	if err != nil {
		return errors.Wrapf(err, "creating temp env file in '%s'", dir)
	}
	tmpPath := tmp.Name()

	_, writeErr := tmp.WriteString(sb.String())
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(tmpPath)
		if writeErr != nil {
			return errors.Wrap(writeErr, "writing temp env file")
		}
		return errors.Wrap(closeErr, "closing temp env file")
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return errors.Wrapf(err, "installing env file at '%s'", path)
	}
	return nil
}
