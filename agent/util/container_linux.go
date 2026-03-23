//go:build linux

package util

import (
	"github.com/mongodb/jasper/options"
)

// WrapWithContainer prepends `docker exec <containerID>` to opts.Args,
// routing command execution into the specified Docker container. It is a
// no-op if containerID is empty.
func WrapWithContainer(opts *options.Create, containerID string) {
	if containerID == "" {
		return
	}
	args := []string{"docker", "exec", containerID}
	opts.Args = append(args, opts.Args...)
}
