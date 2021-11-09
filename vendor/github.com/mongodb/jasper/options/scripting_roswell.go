package options

import (
	"crypto/sha1"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/uuid"
)

// ScriptingRoswell describes the options needed to configure Roswell,
// a Common Lisp-based scripting and environment management tool, as
// a ScriptingHarness. Roswell uses Quicklip and must be
// installed on your systems to use with jasper.
type ScriptingRoswell struct {
	Path           string            `bson:"path" json:"path" yaml:"path"`
	Systems        []string          `bson:"systems" json:"systems" yaml:"systems"`
	Lisp           string            `bson:"lisp" json:"lisp" yaml:"lisp"`
	CachedDuration time.Duration     `bson:"cached_duration" json:"cached_duration" yaml:"cached_duration"`
	Environment    map[string]string `bson:"environment" json:"environment" yaml:"environment"`
	Output         Output            `bson:"output" json:"output" yaml:"output"`

	cachedAt   time.Time
	cachedHash string
}

// NewRoswellScriptingHarness generates a ScriptingHarness based on the
// arguments provided. Use this function for simple cases when you do not need
// or want to set as many aspects of the environment configuration.
func NewRoswellScriptingHarness(path string, systems ...string) ScriptingHarness {
	return &ScriptingRoswell{
		Path:           path,
		Systems:        systems,
		CachedDuration: time.Hour,
		Lisp:           "sbcl-bin",
	}
}

// Type returns the type of the interface.
func (opts *ScriptingRoswell) Type() string { return RoswellScriptingType }

// Interpreter returns the path to the interpreter or binary that runs scripts.
func (opts *ScriptingRoswell) Interpreter() string { return "ros" }

// Validate both ensures that the values are permissible and sets, where
// possible, good defaults.
func (opts *ScriptingRoswell) Validate() error {
	if opts.CachedDuration == 0 {
		opts.CachedDuration = 10 * time.Minute
	}

	if opts.Path == "" {
		opts.Path = filepath.Join("roswell", uuid.New().String())
	}

	if opts.Lisp == "" {
		opts.Lisp = "sbcl-bin"
	}

	return nil
}

// ID returns a hash value that uniquely summarizes the environment.
func (opts *ScriptingRoswell) ID() string {
	if opts.cachedHash != "" && time.Since(opts.cachedAt) < opts.CachedDuration {
		return opts.cachedHash
	}

	hash := sha1.New()

	_, _ = io.WriteString(hash, opts.Lisp)
	_, _ = io.WriteString(hash, opts.Path)

	sort.Strings(opts.Systems)
	for _, sys := range opts.Systems {
		_, _ = io.WriteString(hash, sys)
	}

	opts.cachedHash = fmt.Sprintf("%x", hash.Sum(nil))
	opts.cachedAt = time.Now()

	return opts.cachedHash
}
