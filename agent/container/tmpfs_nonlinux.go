//go:build !linux

package container

import (
	"os"

	"github.com/pkg/errors"
)

// provisionEnvTmpfs creates a regular directory as a non-tmpfs fallback for
// non-Linux platforms (e.g. macOS dev environments). The "never on disk"
// security guarantee only applies on Linux production hosts.
func provisionEnvTmpfs(dir string) error {
	return errors.Wrapf(os.MkdirAll(dir, 0700), "creating env dir '%s'", dir)
}

// removeEnvTmpfs removes the env directory created by provisionEnvTmpfs.
func removeEnvTmpfs(dir string) error {
	if dir == "" {
		return nil
	}
	return errors.Wrapf(os.RemoveAll(dir), "removing env dir '%s'", dir)
}
