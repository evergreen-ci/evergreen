package main

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"10gen.com/mci/repotracker"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"os"
	"time"
)

const (
	// determines the default maximum number of revisions
	// we want to fetch for a newly tracked repo if no max
	// number is specified in the repotracker configuration
	DefaultNumNewRepoRevisionsToFetch = 50
)

func main() {
	mciSettings := mci.MustConfig()
	if mciSettings.RepoTracker.LogFile != "" {
		mci.SetLogger(mciSettings.RepoTracker.LogFile)
	}
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(mciSettings))

	if err := db.InitializeGlobalLock(); err != nil {
		panic(fmt.Sprintf("Error initializing global lock: %v", err))
	}

	lockAcquired, err := db.WaitTillAcquireGlobalLock(mci.RepotrackerPackage, db.LockTimeout)
	if err != nil {
		panic(fmt.Sprintf("Error acquiring global lock: %v", err))
	}
	if !lockAcquired {
		panic("Repotracker couldn't get global lock")
	}

	defer func() {
		if err := db.ReleaseGlobalLock(mci.RepotrackerPackage); err != nil {
			panic(fmt.Sprintf("Failed to release global lock from repotracker: %v", err))
		}
	}()

	startTime := time.Now()
	mci.Logger.Logf(slogger.INFO, "Running repository tracker with db “%v”", mciSettings.Db)

	allProjects, err := model.FindAllTrackedProjectRefs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding tracked projects %v\n", err)
		os.Exit(1)
	}

	for _, projectRef := range allProjects {
		tracker := &repotracker.RepoTracker{
			mciSettings,
			&projectRef,
			repotracker.NewGithubRepositoryPoller(&projectRef, mciSettings.Credentials["github"]),
		}

		numNewRepoRevisionsToFetch := mciSettings.RepoTracker.NumNewRepoRevisionsToFetch
		if numNewRepoRevisionsToFetch <= 0 {
			numNewRepoRevisionsToFetch = DefaultNumNewRepoRevisionsToFetch
		}

		err = tracker.FetchRevisions(numNewRepoRevisionsToFetch)
		if err != nil {
			mci.Logger.Errorf(slogger.ERROR, "Error fetching revisions: %v", err)
			continue
		}
	}

	runtime := time.Now().Sub(startTime)
	err = model.SetProcessRuntimeCompleted(mci.RepotrackerPackage, runtime)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error updating process status: %v", err)
	}
	mci.Logger.Logf(slogger.INFO, "Repository tracker took %v to run", runtime)
}
