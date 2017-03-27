package repotracker

import (
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
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
		errM := errors.Wrap(err, "contacting github")
		grip.Error(errM)
		return errM
	}
	if status != thirdparty.GithubAPIStatusGood {
		errM := errors.Errorf("bad github api status: %v", status)
		grip.Error(errM)
		return errM
	}
	lockAcquired, err := db.WaitTillAcquireGlobalLock(RunnerName, db.LockTimeout)
	if err != nil {
		err = errors.Wrap(err, "Error acquiring global lock")
		grip.Error(err)
		return err
	}

	if !lockAcquired {
		err = errors.New("Timed out acquiring global lock")
		grip.Error(err)
		return err
	}

	defer func() {
		if err := db.ReleaseGlobalLock(RunnerName); err != nil {
			grip.Errorln("Error releasing global lock:", err)
		}
	}()

	startTime := time.Now()
	grip.Infoln("Running repository tracker with db:", config.Database.DB)

	allProjects, err := model.FindAllTrackedProjectRefs()
	if err != nil {
		err = errors.Wrap(err, "Error finding tracked projects")
		grip.Error(err)
		return err
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
				grip.Errorln("Error fetching revisions:", err)
			}
		}(projectRef)
	}
	wg.Wait()

	runtime := time.Now().Sub(startTime)
	if err = model.SetProcessRuntimeCompleted(RunnerName, runtime); err != nil {
		err = errors.Wrap(err, "Error updating process status")
		grip.Error(err)
		return err
	}
	grip.Infof("Repository tracker took %s to run", runtime)
	return nil
}
