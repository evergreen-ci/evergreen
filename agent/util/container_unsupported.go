//go:build !linux

package util

import (
	"github.com/mongodb/jasper/options"
)

// WrapWithContainer is a no-op on non-Linux platforms. Container isolation
// requires Docker on Linux.
func WrapWithContainer(_ *options.Create, _ string) {}
