package options

import (
	"crypto/sha1"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/uuid"
)

// ScriptingPython defines the configuration of a python environment.
type ScriptingPython struct {
	VirtualEnvPath string `bson:"virtual_env_path" json:"virtual_env_path" yaml:"virtual_env_path"`
	// InterpreterBinary is the global python binary interpreter.
	InterpreterBinary string `bson:"interpreter_binary" json:"interpreter_binary" yaml:"interpreter_binary"`
	// RequirementsPath specifies a path to the requirements.txt containing the
	// required packages.
	RequirementsPath string `bson:"requirements_path" json:"requirements_path" yaml:"requirements_path"`
	// Packages specifies all required packages for running scripts.
	Packages []string `bson:"packages" json:"packages" yaml:"packages"`
	// AddTestRequirements determines whether or not to install the dependencies
	// necessary for executing scripts if they are not already installed.
	AddTestRequirements bool `bson:"add_test_reqs" json:"add_test_reqs" yaml:"add_test_reqs"`
	UpdatePackages      bool `bson:"update_packages" json:"update_packges" yaml:"update_packages"`
	// TODO: should this be an enum to handle when it's not just python 2 vs 3?
	LegacyPython bool `bson:"legacy_python" json:"legacy_python" yaml:"legacy_python"`

	CachedDuration time.Duration     `bson:"cache_duration" json:"cache_duration" yaml:"cache_duration"`
	Environment    map[string]string `bson:"env" json:"env" yaml:"env"`
	Output         Output            `bson:"output" json:"output" yaml:"output"`

	requirementsModTime time.Time
	cachedAt            time.Time
	requrementsHash     string
	cachedHash          string
}

// NewPythonScriptingEnvironment generates a ScriptingEnvironment for python 3
// based on the arguments provided. Use this function for
// simple cases when you do not need or want to set as many aspects of
// the environment configuration.
func NewPythonScriptingEnvironment(path, reqtxt string, packages ...string) ScriptingHarness {
	return &ScriptingPython{
		CachedDuration:    time.Hour,
		InterpreterBinary: "python3",
		Packages:          packages,
		VirtualEnvPath:    path,
		RequirementsPath:  reqtxt,
	}
}

// Type is part of the options.ScriptingEnvironment interface and
// returns the type of the interface.
func (opts *ScriptingPython) Type() string {
	if opts.LegacyPython {
		return Python2ScriptingType
	}
	return Python3ScriptingType
}

// Interpreter is part of the options.ScriptingEnvironment interface
// and returns the path to the binary that will run scripts.
func (opts *ScriptingPython) Interpreter() string {
	return filepath.Join(opts.VirtualEnvPath, "bin", "python")
}

// Validate is part of the options.ScriptingEnvironment interface and
// both ensures that the values are permissible and sets, where
// possible, good defaults.
func (opts *ScriptingPython) Validate() error {
	if opts.CachedDuration == 0 {
		opts.CachedDuration = 10 * time.Minute
	}

	if opts.VirtualEnvPath == "" {
		opts.VirtualEnvPath = filepath.Join("venv", uuid.New().String())
	}

	if opts.InterpreterBinary == "" {
		opts.InterpreterBinary = "python3"
	}

	if opts.AddTestRequirements {
		opts.Packages = appendWhenNotContains(opts.Packages, "pytest")
		opts.Packages = appendWhenNotContains(opts.Packages, "pytest-repeat")
		opts.Packages = appendWhenNotContains(opts.Packages, "pytest-timeout")
	}

	return nil
}

// ID returns a hash value that uniquely summarizes the environment.
func (opts *ScriptingPython) ID() string {
	if opts.cachedHash != "" && time.Since(opts.cachedAt) < opts.CachedDuration {
		return opts.cachedHash
	}
	hash := sha1.New()

	_, _ = io.WriteString(hash, opts.InterpreterBinary)
	_, _ = io.WriteString(hash, opts.VirtualEnvPath)

	if opts.requrementsHash == "" {
		stat, err := os.Stat(opts.RequirementsPath)
		if !os.IsNotExist(err) && (stat.ModTime() != opts.requirementsModTime) {
			reqData, err := ioutil.ReadFile(opts.RequirementsPath)
			if err == nil {
				reqHash := sha1.New()
				_, _ = reqHash.Write(reqData)
				opts.requrementsHash = fmt.Sprintf("%x", reqHash.Sum(nil))
			}
		}
	}

	_, _ = io.WriteString(hash, opts.requrementsHash)

	sort.Strings(opts.Packages)
	for _, str := range opts.Packages {
		_, _ = io.WriteString(hash, str)
	}

	opts.cachedHash = fmt.Sprintf("%x", hash.Sum(nil))
	opts.cachedAt = time.Now()
	return opts.cachedHash
}

func appendWhenNotContains(list []string, value string) []string {
	for _, str := range list {
		if str == value {
			return list
		}
	}

	return append(list, value)
}
