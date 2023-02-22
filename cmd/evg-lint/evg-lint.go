//go:build tools

// This is a helper file to track evg-lint as a Go tooling dependency in Go
// modules.
// https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module

package tools

import _ "github.com/evergreen-ci/evg-lint/evg-lint"
