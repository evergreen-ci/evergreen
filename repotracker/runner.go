package repotracker

import (
	"context"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/admin"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

type Runner struct{}

const (
	// the repotracker polls version control (github) for new commits
	RunnerName = "repotracker"

	// githubAPILimitCeiling is arbitrary but corresponds to when we start logging errors in
	// thirdparty/github.go/getGithubRateLimit
	githubAPILimitCeiling = 20
	githubCredentialsKey  = "github"
)

func (r *Runner) Name() string { return RunnerName }

func (r *Runner) Run(ctx context.Context, config *evergreen.Settings) error {
	startTime := time.Now()
	adminSettings, err := admin.GetSettings()
	if err != nil {
		return errors.Wrap(err, "error retrieving admin settings")
	}
	if adminSettings.ServiceFlags.RepotrackerDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"runner":  RunnerName,
			"message": "repotracker is disabled, exiting",
		})
		return nil
	}
	grip.Info(message.Fields{
		"runner":  RunnerName,
		"status":  "starting",
		"time":    startTime,
		"message": "starting runner process",
	})

	if err := runRepoTracker(config); err != nil {
		grip.Error(message.Fields{
			"runner":  RunnerName,
			"error":   err.Error(),
			"status":  "failed",
			"runtime": time.Since(startTime),
			"span":    time.Since(startTime).String(),
		})

		return errors.Wrap(err, "problem running repotracker")
	}

	runnerRuntime := time.Since(startTime)
	if err := model.SetProcessRuntimeCompleted(RunnerName, runnerRuntime); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":  "problem updating process status",
			"duration": runnerRuntime,
			"span":     runnerRuntime.String(),
			"runner":   RunnerName,
		}))
	}

	grip.Info(message.Fields{
		"runner":  RunnerName,
		"runtime": runnerRuntime,
		"status":  "success",
		"span":    runnerRuntime.String(),
	})

	return nil
}

func runRepoTracker(config *evergreen.Settings) error {
	if !CheckGithubAPIResources(config) {
		return errors.New("github API is is not ready to run the repotracker")
	}

	lockAcquired, err := db.WaitTillAcquireLock(RunnerName)
	if err != nil {
		return errors.Wrap(err, "Error acquiring lock")
	}

	if !lockAcquired {
		return errors.New("Timed out acquiring lock")
	}

	defer func() {
		if err = db.ReleaseLock(RunnerName); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"runner":  RunnerName,
				"message": "Error releasing lock",
			}))
		}
	}()

	startTime := time.Now()
	grip.Info(message.Fields{
		"runner":   RunnerName,
		"message":  "running repository tracker",
		"database": config.Database.DB,
	})

	allProjects, err := model.FindAllTrackedProjectRefs()
	if err != nil {
		return errors.Wrap(err, "Error finding tracked projects")
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
	grip.Debug(message.Fields{
		"message": "dispatched jobs",
		"number":  len(allProjects),
		"runner":  RunnerName,
	})

	wg := &sync.WaitGroup{}

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go repoTrackerWorker(config, numNewRepoRevisionsToFetch, jobs, i, wg)
	}

	grip.Debug(message.Fields{
		"message": "waiting for jobs to complete jobs",
		"number":  len(allProjects),
		"workers": numRequests,
		"runner":  RunnerName,
	})
	wg.Wait()

	runtime := time.Since(startTime)
	if err = model.SetProcessRuntimeCompleted(RunnerName, runtime); err != nil {
		return errors.Wrap(err, "Error updating process status")
	}
	grip.Info(message.Fields{
		"runner":  RunnerName,
		"runtime": runtime,
		"span":    runtime.String(),
		"message": "repostory tracker completed without errors",
	})
	return nil
}

func repoTrackerWorker(conf *evergreen.Settings, num int, projects <-chan model.ProjectRef, id int, wg *sync.WaitGroup) {
	grip.Debug(message.Fields{
		"runner":  RunnerName,
		"message": "starting repotracker worker",
		"worker":  id,
	})
	defer wg.Done()

	var (
		disabled  []string
		completed []string
		errored   []string
	)

	for project := range projects {
		switch errors.Cause(CollectRevisionsForProject(conf, project, num)) {
		case errProjectDisabled:
			disabled = append(disabled, project.String())
		case errEncounteredError:
			errored = append(errored, project.String())
		default:
			completed = append(completed, project.String())
		}
	}

	grip.Info(message.Fields{
		"runner":    RunnerName,
		"operation": "repotracker runner complete",
		"worker":    id,
		"disabled":  disabled,
		"errored":   errored,
		"completed": completed,
	})
}
