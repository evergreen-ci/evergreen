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

	args = append(args, containerID)
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
