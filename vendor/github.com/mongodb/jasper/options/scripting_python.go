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
	VirtualEnvPath        string            `bson:"virtual_env_path" json:"virtual_env_path" yaml:"virtual_env_path"`
	RequirementsFilePath  string            `bson:"requirements_path" json:"requirements_path" yaml:"requirements_path"`
	HostPythonInterpreter string            `bson:"host_python" json:"host_python" yaml:"host_python"`
	Packages              []string          `bson:"packages" json:"packages" yaml:"packages"`
	AddTestRequirements   bool              `bson:"add_test_deps" json:"add_test_deps" yaml:"add_test_deps"`
	LegacyPython          bool              `bson:"legacy_python" json:"legacy_python" yaml:"legacy_python"`
	CachedDuration        time.Duration     `bson:"cache_duration" json:"cache_duration" yaml:"cache_duration"`
	Environment           map[string]string `bson:"env" json:"env" yaml:"env"`
	Output                Output            `bson:"output" json:"output" yaml:"output"`
	requirementsMTime     time.Time
	cachedAt              time.Time
	requrementsHash       string
	cachedHash            string
}

// NewPythonScriptingEnvironmnet generates a ScriptingEnvironment
// based on the arguments provided. Use this function for
// simple cases when you do not need or want to set as many aspects of
// the environment configuration.
func NewPythonScriptingEnvironmnet(path, reqtxt string, packages ...string) ScriptingHarness {
	return &ScriptingPython{
		CachedDuration:        time.Hour,
		HostPythonInterpreter: "python3",
		Packages:              packages,
		VirtualEnvPath:        path,
		RequirementsFilePath:  reqtxt,
	}
}

// Type is part of the options.ScriptingEnvironment interface and
// returns the type of the interface.
func (opts *ScriptingPython) Type() string { return "python" }

// Interpreter is part of the options.ScriptingEnvironment interface
// and returns the path to the interpreter or binary that runs scripts.
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

	if opts.HostPythonInterpreter == "" {
		opts.HostPythonInterpreter = "python3"
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

	_, _ = io.WriteString(hash, opts.HostPythonInterpreter)
	_, _ = io.WriteString(hash, opts.VirtualEnvPath)

	if opts.requrementsHash == "" {
		stat, err := os.Stat(opts.RequirementsFilePath)
		if !os.IsNotExist(err) && (stat.ModTime() != opts.requirementsMTime) {
			reqData, err := ioutil.ReadFile(opts.RequirementsFilePath)
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
