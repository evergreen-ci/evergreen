package repotracker

import (
	"fmt"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/tychoish/grip"
	"github.com/tychoish/grip/slogger"
)

type Runner struct{}

const (
	RunnerName  = "repotracker"
	Description = "poll version control for new commits"
)

func (r *Runner) Name() string {
	return RunnerName
}

func (r *Runner) Description() string {
	return Description
}

func (r *Runner) Run(config *evergreen.Settings) error {
	status, err := thirdparty.GetGithubAPIStatus()
	if err != nil {
		errM := fmt.Errorf("contacting github: %v", err)
		grip.Error(errM)
		return errM
	}
	if status != thirdparty.GithubAPIStatusGood {
		errM := fmt.Errorf("bad github api status: %v", status)
		grip.Error(errM)
		return errM
	}
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
	evergreen.Logger.Logf(slogger.INFO, "Running repository tracker with db “%v”", config.Database.DB)

	allProjects, err := model.FindAllTrackedProjectRefs()
	if err != nil {
		return evergreen.Logger.Errorf(slogger.ERROR, "Error finding tracked projects %v", err)
	}

	numNewRepoRevisionsToFetch := config.RepoTracker.NumNewRepoRevisionsToFetch
	if numNewRepoRevisionsToFetch <= 0 {
		numNewRepoRevisionsToFetch = DefaultNumNewRepoRevisionsToFetch
	}

	var wg sync.WaitGroup
	wg.Add(len(allProjects))
	for _, projectRef := range allProjects {
		go func(projectRef model.ProjectRef) {
			defer wg.Done()

			tracker := &RepoTracker{
				config,
				&projectRef,
				NewGithubRepositoryPoller(&projectRef, config.Credentials["github"]),
			}

			err = tracker.FetchRevisions(numNewRepoRevisionsToFetch)
			if err != nil {
				evergreen.Logger.Errorf(slogger.ERROR, "Error fetching revisions: %v", err)
			}
		}(projectRef)
	}
	wg.Wait()

	runtime := time.Now().Sub(startTime)
	if err = model.SetProcessRuntimeCompleted(RunnerName, runtime); err != nil {
		return evergreen.Logger.Errorf(slogger.ERROR, "Error updating process status: %v", err)
	}
	evergreen.Logger.Logf(slogger.INFO, "Repository tracker took %v to run", runtime)
	return nil
}
