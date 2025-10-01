package model

import (
	"regexp"
	"time"

	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/pkg/errors"
)

// TaskCreationInfo contains the needed parameters to construct new builds and tasks for a given version.
type TaskCreationInfo struct {
	Version             *Version
	Project             *Project
	ProjectRef          *ProjectRef
	BuildVariant        *BuildVariant           // If creating tasks in a specific BV, the BV definition
	Build               *build.Build            // If creating tasks in an existing build, the build itself
	Pairs               TaskVariantPairs        // New variant-tasks to be created
	BuildVariantName    string                  // If creating tasks in a specific BV, the name of the BV
	TaskIDs             TaskIdConfig            // Pre-generated IDs for the tasks to be created
	ActivateBuild       bool                    // True if the build should be scheduled
	ActivationInfo      specificActivationInfo  // Indicates if the task has a specific activation or is a stepback task
	TasksInBuild        []task.Task             // The set of task names that already exist for the given build, including display tasks
	TaskNames           []string                // Names of tasks to create (used in patches). Will create all if empty
	DisplayNames        []string                // Names of display tasks to create (used in patches). Will create all if empty
	GeneratedBy         string                  // ID of the task that generated this build
	SourceRev           string                  // Githash of the revision that triggered this build
	DefinitionID        string                  // Definition ID of the trigger used to create this build
	Aliases             ProjectAliases          // Project aliases to use to filter tasks created
	DistroAliases       distro.AliasLookupTable // Map of distro aliases to names of distros
	TaskCreateTime      time.Time               // Create time of tasks in the build
	GithubChecksAliases ProjectAliases          // Project aliases to use to filter tasks to count towards the github checks, if any
	// ActivatedTasksAreEssentialToSucceed indicates whether or not all tasks
	// that are being created and activated immediately are required to finish
	// in order for the build/version to be finished. Tasks with specific
	// activation conditions (e.g. cron, activate) are not considered essential.
	ActivatedTasksAreEssentialToSucceed bool
	TestSelection                       TestSelectionParams // Task creation parameters for test selection
}

// TestSelectionParams contains parameters for enabling test selection on tasks
// in a build variant.
type TestSelectionParams struct {
	CanBuildVariantEnableTestSelection bool             // Whether or not any of the tasks in the build variant can use test selection.
	IncludeBuildVariants               []*regexp.Regexp // Regexes for build variants to include when enabling test selection.
	ExcludeBuildVariants               []*regexp.Regexp // Regexes for build variants to exclude when enabling test selection.
	IncludeTasks                       []*regexp.Regexp // Regexes for tasks to include when enabling test selection.
	ExcludeTasks                       []*regexp.Regexp // Regexes for tasks to exclude when enabling test selection.
}

func newTestSelectionParams(p *patch.Patch) (*TestSelectionParams, error) {
	testSelectionIncludeBVs, err := toRegexps(p.RegexTestSelectionBuildVariants)
	if err != nil {
		return nil, errors.Wrap(err, "compiling test selection build variant regexes")
	}
	testSelectionExcludeBVs, err := toRegexps(p.RegexTestSelectionExcludedBuildVariants)
	if err != nil {
		return nil, errors.Wrap(err, "compiling test selection excluded build variant regexes")
	}
	testSelectionIncludeTasks, err := toRegexps(p.RegexTestSelectionTasks)
	if err != nil {
		return nil, errors.Wrap(err, "compiling test selection task regexes")
	}
	testSelectionExcludeTasks, err := toRegexps(p.RegexTestSelectionExcludedTasks)
	if err != nil {
		return nil, errors.Wrap(err, "compiling test selection excluded task regexes")
	}
	return &TestSelectionParams{
		IncludeBuildVariants: testSelectionIncludeBVs,
		ExcludeBuildVariants: testSelectionExcludeBVs,
		IncludeTasks:         testSelectionIncludeTasks,
		ExcludeTasks:         testSelectionExcludeTasks,
	}, nil
}

func toRegexps(strs []string) ([]*regexp.Regexp, error) {
	regexps := make([]*regexp.Regexp, 0, len(strs))
	for _, s := range strs {
		re, err := regexp.Compile(s)
		if err != nil {
			return nil, errors.Wrapf(err, "compiling regexp '%s'", s)
		}
		regexps = append(regexps, re)
	}
	return regexps, nil
}
