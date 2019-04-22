package dependency

import (
	"os"

	"github.com/mongodb/grip"
)

// LocalFile describes a dependency between a task and local
// files. Has a notion of targets, and dependencies, and a la make
// dependency resolution returns state Passed (e.g. noop) if the
// target or targets are all newer than the dependency or
// dependencies. LocalFile will return state Ready in
// ambiguous cases where targets or dependencies are missing.
//
// Importantly, the edges, which amboy Jobs and Queue should always
// assume are *not* files, are distinct from the Targets and
// Dependencies in the context of this dependency.Manager. Add edges
// (task names) for other amboy.Job IDs in the queue if you need to
// express that kind of relationship.
type LocalFile struct {
	// A list of file names that the task would in theory create.
	Targets []string `bson:"targets" json:"targets" yaml:"targets"`

	// A list of file names that represent dependencies that the
	// Target depends on.
	Dependencies []string `bson:"dependencies" json:"dependencies" yaml:"dependencies"`

	T TypeInfo `bson:"type" json:"type" yaml:"type"`
	JobEdges
}

const localFileTypeName = "local-file"

// NewLocalFile creates a dependency object that checks if
// dependencies on the local file system are created. This constructor
// takes, as arguments, a target name and a variable number of
// successive arguments that are dependencies. All arguments should be
// file names, relative to the working directory of the
// program.
func NewLocalFile(target string, dependencies ...string) *LocalFile {
	d := MakeLocalFile()
	d.Targets = []string{target}
	d.Dependencies = dependencies

	return d
}

// MakeLocalFile constructs an empty local file instance.
func MakeLocalFile() *LocalFile {
	return &LocalFile{
		T: TypeInfo{
			Name:    localFileTypeName,
			Version: 0,
		},
		JobEdges: NewJobEdges(),
	}
}

// State reports if the dependency is satisfied. If the targets or are
// not specified *or* the file names of any target do not exist, then
// State returns Ready. If a dependency does not exist, This call will
// log a a warning.
//
// Otherwise, If any dependency has a modification time that is after
// the earliest modification time of the targets, then State returns
// Ready. When all targets were modified after all of the
// dependencies, then State returns Passed.
func (d *LocalFile) State() State {
	if len(d.Targets) == 0 {
		return Ready
	}
	// presumably it might make sense to do these checks in
	// parallel, but that seems premature, given that it could be
	// (potentially) a lot of IO, but that seems overkill at this
	// point.

	// first, find what the oldest target it, if any of the
	// dependencies are newer than it, we need to rebuild.
	var oldestTarget string
	var winningTargetStat os.FileInfo

	for _, target := range d.Targets {
		thisStat, err := os.Stat(target)
		if os.IsNotExist(err) {
			return Ready
		}

		// presumably this happens for the first target only,
		// but it's good to check.
		if oldestTarget == "" {
			oldestTarget = target
			winningTargetStat = thisStat
			continue
		}

		if thisStat.ModTime().Before(winningTargetStat.ModTime()) {
			oldestTarget = target
			winningTargetStat = thisStat
		}
	}

	// then, now find the newest dependency.
	var newestDependency string
	var winningDependencyStat os.FileInfo

	for _, dep := range d.Dependencies {
		thisStat, err := os.Stat(dep)
		if os.IsNotExist(err) {
			// this shouldn't trigger a rebuild.
			grip.Warningf("dependency %s does not exist", dep)
		}

		// presumably this happens for the first dependency
		// only, but it's good to check.
		if newestDependency == "" {
			newestDependency = dep
			winningDependencyStat = thisStat
			continue
		}

		if thisStat.ModTime().After(winningDependencyStat.ModTime()) {
			newestDependency = dep
			winningDependencyStat = thisStat
		}

		// this is a short circuit: if we find *one*
		// dependency that's newer than the oldest target, we
		// can return ready without stating a bunch of deps
		// unnecessarily.
		if thisStat.ModTime().After(winningTargetStat.ModTime()) {
			return Ready
		}
	}

	// if the last dependency check didn't return early, then the
	// task can be a noop.
	return Passed

}

// Type returns a TypeInfo object for the Dependency object. Used by
// the registry and interchange systems.
func (d *LocalFile) Type() TypeInfo {
	return d.T
}
