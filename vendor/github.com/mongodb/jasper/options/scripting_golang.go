package options

import (
	"crypto/sha1"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"time"

	uuid "github.com/satori/go.uuid"
)

// ScriptingGolang describes a Go environment for building and running
// arbitrary code.
type ScriptingGolang struct {
	Gopath         string            `bson:"gopath" json:"gopath" yaml:"gopath"`
	Goroot         string            `bson:"goroot" json:"goroot" yaml:"goroot"`
	Packages       []string          `bson:"packages" json:"packages" yaml:"packages"`
	Context        string            `bson:"context" json:"context" yaml:"context"`
	WithUpdate     bool              `bson:"with_update" json:"with_update" yaml:"with_update"`
	CachedDuration time.Duration     `bson:"cached_duration" json:"cached_duration" yaml:"cached_duration"`
	Environment    map[string]string `bson:"environment" json:"environment" yaml:"environment"`
	Output         Output            `bson:"output" json:"output" yaml:"output"`

	cachedAt   time.Time
	cachedHash string
}

// NewGolangScriptingEnvironment generates a ScriptingEnvironment
// based on the arguments provided. Use this function for
// simple cases when you do not need or want to set as many aspects of
// the environment configuration.
func NewGolangScriptingEnvironment(gopath, goroot string, packages ...string) ScriptingEnvironment {
	return &ScriptingGolang{
		Gopath:         gopath,
		Goroot:         goroot,
		CachedDuration: time.Hour,
		Packages:       packages,
	}
}

// Type is part of the options.ScriptingEnvironment interface and
// returns the type of the interface.
func (opts *ScriptingGolang) Type() string { return "go" }

// Interpreter is part of the options.ScriptingEnvironment interface
// and returns the path to the interpreter or binary that runs scripts.
func (opts *ScriptingGolang) Interpreter() string { return filepath.Join(opts.Goroot, "bin", "go") }

// Validate is part of the options.ScriptingEnvironment interface and
// both ensures that the values are permissible and sets, where
// possible, good defaults.
func (opts *ScriptingGolang) Validate() error {
	if opts.CachedDuration == 0 {
		opts.CachedDuration = 10 * time.Minute
	}

	if opts.Gopath == "" {
		opts.Gopath = filepath.Join("go", uuid.Must(uuid.NewV4()).String())
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

	_, _ = io.WriteString(hash, opts.Context)

	opts.cachedHash = fmt.Sprintf("%x", hash.Sum(nil))
	opts.cachedAt = time.Now()
	return opts.cachedHash
}
