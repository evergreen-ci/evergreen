//go:build tools

// This is a helper file to track gqlgen as a Go tooling dependency in Go
// modules.
// https://github.com/99designs/gqlgen
// https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module

package tools

import _ "github.com/99designs/gqlgen"
