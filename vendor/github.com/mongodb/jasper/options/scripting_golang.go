package options

import (
	"crypto/sha1"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

// ScriptingGolang describes a Go environment for building and running
// arbitrary code.
type ScriptingGolang struct {
	Gopath string `bson:"gopath" json:"gopath" yaml:"gopath"`
	Goroot string `bson:"goroot" json:"goroot" yaml:"goroot"`
	// Packages describes the required packages for running scripts.
	// TODO: better support for go modules for getting specific versions?
	Packages []string `bson:"packages" json:"packages" yaml:"packages"`
	// Directory is the base working directory in which scripting is performed.
	Directory string `bson:"directory" json:"directory" yaml:"directory"`
	// UpdatePackages will update any required packages that already exist.
	UpdatePackages bool `bson:"update_packages" json:"update_packages" yaml:"update_packages"`

	CachedDuration time.Duration     `bson:"cached_duration" json:"cached_duration" yaml:"cached_duration"`
	Environment    map[string]string `bson:"environment" json:"environment" yaml:"environment"`
	Output         Output            `bson:"output" json:"output" yaml:"output"`

	cachedAt   time.Time
	cachedHash string
}

// NewGolangScriptingHarness generates a scripting.Harness
// based on the arguments provided. Use this function for
// simple cases when you do not need or want to set as many aspects of
// the environment configuration.
func NewGolangScriptingHarness(gopath, goroot string, packages ...string) ScriptingHarness {
	return &ScriptingGolang{
		Gopath:         gopath,
		Goroot:         goroot,
		CachedDuration: time.Hour,
		Packages:       packages,
	}
}

// Type returns the type of the interface.
func (opts *ScriptingGolang) Type() string { return GolangScriptingType }

// Interpreter returns the path to the binary that will run scripts.
func (opts *ScriptingGolang) Interpreter() string { return filepath.Join(opts.Goroot, "bin", "go") }

// Validate both ensures that the values are permissible and sets, where
// possible, good defaults.
func (opts *ScriptingGolang) Validate() error {
	if opts.Goroot == "" {
		return errors.New("must specify a GOROOT")
	}

	if opts.CachedDuration == 0 {
		opts.CachedDuration = 10 * time.Minute
	}

	if opts.Gopath == "" {
		opts.Gopath = filepath.Join("go", uuid.New().String())
	}

	return nil
}

// ID returns a hash value that uniquely summarizes the environment.
func (opts *ScriptingGolang) ID() string {
	if opts.cachedHash != "" && time.Since(opts.cachedAt) < opts.CachedDuration {
		return opts.cachedHash
	}
	hash := sha1.New()

	_, _ = io.WriteString(hash, opts.Goroot)
	_, _ = io.WriteString(hash, opts.Gopath)

	sort.Strings(opts.Packages)
	for _, str := range opts.Packages {
		_, _ = io.WriteString(hash, str)
	}

	_, _ = io.WriteString(hash, opts.Directory)

	opts.cachedHash = fmt.Sprintf("%x", hash.Sum(nil))
	opts.cachedAt = time.Now()
	return opts.cachedHash
}
