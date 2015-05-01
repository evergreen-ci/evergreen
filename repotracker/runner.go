package repotracker

import (
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"time"
)

type Runner struct{}

const (
	RunnerName = "repotracker"
)

func (r *Runner) Name() string {
	return RunnerName
}

func (r *Runner) Run(config *evergreen.Settings) error {
	lockAcquired, err := db.WaitTillAcquireGlobalLock(RunnerName, db.LockTimeout)
	if err != nil {
		return evergreen.Logger.Errorf(slogger.ERROR, "Error acquiring global lock: %v", err)
	}

	if !lockAcquired {
		return evergreen.Logger.Errorf(slogger.ERROR, "Timed out acquiring global lock")
	}

	defer func() {
		if err := db.ReleaseGlobalLock(RunnerName); err != nil {
			evergreen.Logger.Errorf(slogger.ERROR, "Error releasing global lock: %v", err)
		}
	}()

	startTime := time.Now()
	evergreen.Logger.Logf(slogger.INFO, "Running repository tracker with db “%v”", config.Db)

	allProjects, err := model.FindAllTrackedProjectRefs()
	if err != nil {
		return evergreen.Logger.Errorf(slogger.ERROR, "Error finding tracked projects %v", err)
	}

	for _, projectRef := range allProjects {
		tracker := &RepoTracker{
			config,
			&projectRef,
			NewGithubRepositoryPoller(&projectRef, config.Credentials["github"]),
		}

		numNewRepoRevisionsToFetch := config.RepoTracker.NumNewRepoRevisionsToFetch
		if numNewRepoRevisionsToFetch <= 0 {
			numNewRepoRevisionsToFetch = DefaultNumNewRepoRevisionsToFetch
		}

		err = tracker.FetchRevisions(numNewRepoRevisionsToFetch)
		if err != nil {
			evergreen.Logger.Errorf(slogger.ERROR, "Error fetching revisions: %v", err)
			continue
		}
	}

	runtime := time.Now().Sub(startTime)
	if err = model.SetProcessRuntimeCompleted(RunnerName, runtime); err != nil {
		return evergreen.Logger.Errorf(slogger.ERROR, "Error updating process status: %v", err)
	}
	evergreen.Logger.Logf(slogger.INFO, "Repository tracker took %v to run", runtime)
	return nil
}
