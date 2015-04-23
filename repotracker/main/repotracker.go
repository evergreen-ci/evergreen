package main

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"10gen.com/mci/repotracker"
	"10gen.com/mci/util"
	"10gen.com/mci/validator"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/shelman/angier"
	"gopkg.in/yaml.v1"
	"io/ioutil"
	"path/filepath"
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
	mci.Logger.Logf(slogger.INFO, "Running repository tracker with db “%v” "+
		"and config dir “%v”", mciSettings.Db, mciSettings.ConfigDir)

	allProjects, err := mci.AllProjectNames(mciSettings.ConfigDir)
	if err != nil {
		panic(fmt.Sprintf("Error finding all project names: %v", err))
	}

	configRoot, err := mci.FindMCIConfig(mciSettings.ConfigDir)
	if err != nil {
		panic(fmt.Sprintf("Error finding config root: %v", err))
	}

	for _, filename := range allProjects {
		data, err := ioutil.ReadFile(filepath.Join(configRoot, "project", filename+".yml"))
		if err != nil {
			panic(fmt.Sprintf("Error reading project file “%v”: %v",
				filename, err))
		}
		projectRef := &model.ProjectRef{}
		if err := yaml.Unmarshal(data, projectRef); err != nil {
			panic(fmt.Sprintf("Error unmarshalling project file “%v”: %v",
				filename, err))
		}
		project := &model.Project{}
		if err = angier.TransferByFieldNames(projectRef, project); err != nil {
			panic(fmt.Sprintf("Error transferring project file “%v”: %v",
				filename, err))
		}

		// validate the project has all necessary fields
		errs := validator.EnsureHasNecessaryProjectFields(project)
		if len(errs) != 0 {
			var message string
			for _, err := range errs {
				message += fmt.Sprintf("\n\t=> %v", err)
			}
			// TODO: MCI-1893 - send notification to project maintainer instead
			// of panicking
			panic(fmt.Sprintf("Project '%v' does not contain one or more "+
				"necessary fields: %v", filename, message))
		}

		// validate local configs and panic if there's an error
		if !project.Remote {
			project, err = model.FindProject("", filename, mciSettings.ConfigDir)
			if err != nil {
				panic(fmt.Sprintf("Error fetching project file “%v”: %v", filename,
					err))
			}
			errs = validator.CheckProjectSyntax(project, mciSettings)
			if len(errs) != 0 {
				var message string
				for _, err := range errs {
					message += fmt.Sprintf("\n\t=> %v", err)
				}
				// TODO: MCI-1893 - send notification to project maintainer
				// instead of panicking
				panic(fmt.Sprintf("Project '%v' contains invalid "+
					"configuration: %v", filename, message))
			}
		}

		// upsert the repository ref
		if err = projectRef.Upsert(); err != nil {
			panic(fmt.Sprintf("Error upserting '%v' repository ref: %v",
				projectRef.Identifier, err))
		}

		repotrackerInstance := repotracker.NewRepositoryTracker(
			mciSettings, projectRef, repotracker.NewGithubRepositoryPoller(
				projectRef, mciSettings.Credentials[project.RepoKind]),
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

	// untrack all project_refs that no longer have project files
	if err = model.UntrackStaleProjectRefs(allProjects); err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error untracking missing projects: "+
			"%v", err)
	}

	runtime := time.Now().Sub(startTime)
	err = model.SetProcessRuntimeCompleted(mci.RepotrackerPackage, runtime)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error updating process status: %v",
			err)
	}
	mci.Logger.Logf(slogger.INFO, "Repository tracker took %v to run", runtime)
}
