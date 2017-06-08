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

	jobs := make(chan *repoTrackerJob)

	go func(projects []model.ProjectRef) {
		for _, projectRef := range projects {
			jobs <- &repoTrackerJob{
				conf:  config,
				ref:   &projectRef,
				creds: config.Credentials["github"],
				num:   numNewRepoRevisionsToFetch,
			}
		}
		close(jobs)
	}(allProjects)

	var wg sync.WaitGroup
	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(jobs <-chan *repoTrackerJob) {
			defer wg.Done()
			for job := range jobs {
				job.Run()
			}
		}(jobs)
	}
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

type repoTrackerJob struct {
	conf  *evergreen.Settings
	ref   *model.ProjectRef
	creds string
	num   int
}

func (j *repoTrackerJob) Run() {
	tracker := &RepoTracker{
		j.conf,
		j.ref,
		NewGithubRepositoryPoller(j.ref, j.creds),
	}

	if err := tracker.FetchRevisions(j.num); err != nil {
		grip.Errorln("Error fetching revisions:", err)
	}
}
