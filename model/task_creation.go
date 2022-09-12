package model

import (
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"time"
)

// add comment!
type TaskCreationInfo struct {
	Version             *Version
	Project             *Project
	ProjectRef          *ProjectRef
	buildVariant        *BuildVariant
	build               *build.Build
	pairs               TaskVariantPairs
	TaskIDs             TaskIdConfig            // pre-generated IDs for the tasks to be created
	BuildName           string                  // name of the buildvariant
	ActivateBuild       bool                    // true if the build should be scheduled
	ActivationInfo      specificActivationInfo  // indicates if the task has a specific activation or is a stepback task
	TasksInBuild        []task.Task             // the set of task names that already exist for the given build, including display tasks
	TaskNames           []string                // names of tasks to create (used in patches). Will create all if nil
	DisplayNames        []string                // names of display tasks to create (used in patches). Will create all if nil
	GeneratedBy         string                  // ID of the task that generated this build
	SourceRev           string                  // githash of the revision that triggered this build
	DefinitionID        string                  // definition ID of the trigger used to create this build
	Aliases             ProjectAliases          // project aliases to use to filter tasks created
	DistroAliases       distro.AliasLookupTable // map of distro aliases to names of distros
	TaskCreateTime      time.Time               // create time of tasks in the build
	GithubChecksAliases ProjectAliases          // project aliases to use to filter tasks to count towards the github checks, if any
	SyncAtEndOpts       patch.SyncAtEndOptions
}
