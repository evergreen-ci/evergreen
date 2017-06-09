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
	// githubAPILimitCeiling is arbitrary but corresponds to when we start logging errors in
	// thirdparty/github.go/getGithubRateLimit
	githubAPILimitCeiling = 20
	githubCredentialsKey  = "github"
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

	token, ok := config.Credentials[githubCredentialsKey]
	if !ok {
		err = errors.New("Github credentials not specified in Evergreen credentials file")
		grip.Error(err)
		return err
	}
	remaining, err := thirdparty.CheckGithubAPILimit(token)
	if err != nil {
		err = errors.Wrap(err, "Error checking Github API limit")
		grip.Error(err)
		return err
	}
	if remaining < githubAPILimitCeiling {
		err = errors.Errorf("Too few Github API requests remaining: %d < %d", remaining, githubAPILimitCeiling)
		grip.Alert(err)
		return err
	}
	grip.Debugf("%d Github API requests remaining", remaining)

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
		if err = db.ReleaseGlobalLock(RunnerName); err != nil {
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

	numRequests := config.RepoTracker.MaxConcurrentRequests
	if numRequests <= 0 {
		numRequests = DefaultNumConcurrentRequests
	}

	jobs := make(chan model.ProjectRef, len(allProjects))
	for _, p := range allProjects {
		jobs <- p
	}
	close(jobs)
	grip.Debugf("sent %d jobs to the repotracker", len(allProjects))

	wg := &sync.WaitGroup{}

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go repoTrackerWorker(config, numNewRepoRevisionsToFetch, jobs, i, wg)
	}

	grip.Debugf("waiting for repotracker %d jobs to complete on %d workers", len(allProjects), numRequests)
	wg.Wait()

	runtime := time.Since(startTime)
	if err = model.SetProcessRuntimeCompleted(RunnerName, runtime); err != nil {
		err = errors.Wrap(err, "Error updating process status")
		grip.Error(err)
		return err
	}
	grip.Infof("Repository tracker took %s to run", runtime)
	return nil
}

func repoTrackerWorker(conf *evergreen.Settings, num int, projects <-chan model.ProjectRef, id int, wg *sync.WaitGroup) {
	grip.Debugln("starting repotracker worker number:", id)
	done := 0
	defer wg.Done()

	for project := range projects {
		tracker := &RepoTracker{
			conf,
			&project,
			NewGithubRepositoryPoller(&project, conf.Credentials["github"]),
		}

		grip.Debugln("running repotracker for:", project.Identifier)

		if err := tracker.FetchRevisions(num); err != nil {
			grip.Errorln("problem fetching revisions for %s: %+v", project.Identifier, err)
			done++
			continue
		}

		done++
		grip.Infof("fetched all revisions for '%s' without errors", project.Identifier)
	}
	grip.Debugf("shutting down repotracker worker %d after %d jobs", id, done)
}
