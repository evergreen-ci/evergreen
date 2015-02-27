package main

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"10gen.com/mci/repotracker"
	"10gen.com/mci/util"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
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

	lockAcquired, err := db.WaitTillAcquireGlobalLock(mci.RepotrackerPackage,
		db.LockTimeout)
	if err != nil {
		panic(fmt.Sprintf("Error acquiring global lock: %v", err))
	}
	if !lockAcquired {
		panic(fmt.Sprintf("Cannot proceed with repository tracker because " +
			"the global lock could not be taken"))
	}

	defer func() {
		if err := db.ReleaseGlobalLock(mci.RepotrackerPackage); err != nil {
			panic(fmt.Sprintf("Error releasing global lock from tracker - "+
				"this is really bad: %v", err))
		}
	}()

	startTime := time.Now()
	mci.Logger.Logf(slogger.INFO, "Running repository tracker with db “%v”", mciSettings.Db)

	if err != nil {
		panic(fmt.Sprintf("Error finding config root: %v", err))
	}

	allProjects, err := model.FindAllTrackedProjectRefs()
	if err != nil {
		panic(fmt.Sprintf("Error finding tracked projects %v", err))
	}

	for _, projectRef := range allProjects {
		repotrackerInstance := repotracker.NewRepositoryTracker(
			mciSettings, &projectRef, repotracker.NewGithubRepositoryPoller(
				&projectRef, mciSettings.Credentials[projectRef.RepoKind]),
			util.SystemClock{},
		)

		numNewRepoRevisionsToFetch :=
			mciSettings.RepoTracker.NumNewRepoRevisionsToFetch
		if numNewRepoRevisionsToFetch <= 0 {
			numNewRepoRevisionsToFetch = DefaultNumNewRepoRevisionsToFetch
		}

		err = repotrackerInstance.FetchRevisions(numNewRepoRevisionsToFetch)
		if err != nil {
			panic(fmt.Sprintf("Error running repotracker: %v", err))
		}
	}

	runtime := time.Now().Sub(startTime)
	err = model.SetProcessRuntimeCompleted(mci.RepotrackerPackage, runtime)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error updating process status: %v",
			err)
	}
	mci.Logger.Logf(slogger.INFO, "Repository tracker took %v to run", runtime)
}
