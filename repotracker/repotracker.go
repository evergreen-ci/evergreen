package repotracker

import (
	"10gen.com/mci"
	"10gen.com/mci/model"
	"10gen.com/mci/notify"
	"10gen.com/mci/thirdparty"
	"10gen.com/mci/util"
	"10gen.com/mci/validator"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
)

// RepoTracker is used to manage polling repository changes and storing such
// changes. It contains a number of interfaces that specify behavior required by
// client implementations
type RepoTracker struct {
	*mci.MCISettings
	*model.ProjectRef
	RepoPoller
	util.Clock
}

// The RepoPoller interface specifies behavior required of all repository poller
// implementations
type RepoPoller interface {
	// Fetches the contents of a remote repository's configuration data as at
	// the given revision
	GetRemoteConfig(revision string) (*model.Project, error)
	// Fetches all changes since the 'revision' specified - with the most recent
	// revision appearing as the first element in the slice.
	//
	// 'maxRevisionsToSearch' determines the maximum number of revisions we
	// allow to search through - in order to find 'revision' - before we give
	// up. A value <= 0 implies we allow to search through till we hit the first
	// revision for the project.
	GetRevisionsSince(revision string, maxRevisionsToSearch int) (
		[]model.Revision, error)
	// Fetches the most recent 'numNewRepoRevisionsToFetch' revisions for a
	// project - with the most recent revision appearing as the first element in
	// the slice.
	GetRecentRevisions(numNewRepoRevisionsToFetch int) ([]model.Revision,
		error)
}

type projectConfigError struct {
	errors []string
}

func (p projectConfigError) Error() string {
	return "Invalid project configuration"
}

// NewProjectRef constructs a ProjectRef struct that is used by all
// RepoPoller implementations
func NewProjectRef(owner, repo, branch, repoKind, remotePath string,
	repoEnabled, repoPrivate, isRemote bool, batchTime int,
	identifier string, displayName string, tracked bool) *model.ProjectRef {

	// default to id if display_name not set
	if displayName == "" {
		displayName = identifier
	}
	return &model.ProjectRef{
		Owner:       owner,
		Repo:        repo,
		Branch:      branch,
		RepoKind:    repoKind,
		Enabled:     repoEnabled,
		Private:     repoPrivate,
		Remote:      isRemote,
		RemotePath:  remotePath,
		BatchTime:   batchTime,
		Identifier:  identifier,
		DisplayName: displayName,
		Tracked:     tracked,
	}
}

// NewRepositoryTracker constructs a new instance of a RepoTracker struct
func NewRepositoryTracker(mciSettings *mci.MCISettings,
	projectRef *model.ProjectRef, repositoryPoller RepoPoller, clock util.Clock) *RepoTracker {
	return &RepoTracker{
		mciSettings,
		projectRef,
		repositoryPoller,
		clock,
	}
}

// The FetchRevisions method is used by a RepoTracker to run the pipeline for
// tracking repositories. It performs everything from polling the repository to
// persisting any changes retrieved from the repository reference.
func (repoTracker *RepoTracker) FetchRevisions(numNewRepoRevisionsToFetch int) (
	err error) {
	settings := repoTracker.MCISettings
	projectRef := repoTracker.ProjectRef
	projectIdentifier := projectRef.String()

	if !projectRef.Enabled {
		mci.Logger.Logf(slogger.INFO, "Skipping disabled project “%v”",
			projectRef)
		return nil
	}

	repository, err := model.FindRepository(projectIdentifier)
	if err != nil {
		return fmt.Errorf("error finding repository '%v': %v",
			projectIdentifier, err)
	}

	var revisions []model.Revision
	var lastRevision string

	if repository != nil {
		lastRevision = repository.LastRevision
	}

	if lastRevision == "" {
		// if this is the first time we're running the tracker for this project,
		// fetch the most recent `numNewRepoRevisionsToFetch` revisions
		mci.Logger.Logf(slogger.INFO, "No last recorded repository revision "+
			"for “%v”. Proceeding to fetch most recent %v revisions",
			projectRef, numNewRepoRevisionsToFetch)
		revisions, err = repoTracker.GetRecentRevisions(
			numNewRepoRevisionsToFetch)
	} else {
		mci.Logger.Logf(slogger.INFO, "Last recorded repository revision for "+
			"“%v” is “%v”", projectRef, lastRevision)
		revisions, err = repoTracker.GetRevisionsSince(lastRevision,
			settings.RepoTracker.MaxRepoRevisionsToSearch)
	}

	if err != nil {
		mci.Logger.Logf(slogger.ERROR, "error fetching revisions for "+
			"repository “%v”: %v", projectRef, err)
		repoTracker.sendFailureNotification(lastRevision, err)
		return nil
	}

	var lastVersion *model.Version

	if len(revisions) > 0 {
		// TODO: this is inefficient as we re-parse the project config each time.
		// In the future it would be nice to hash the project config and see if it
		// has changed before re-parsing it from scratch
		lastVersion, err = repoTracker.StoreRepositoryRevisions(revisions)
		if err != nil {
			mci.Logger.Logf(slogger.ERROR, "error storing revisions for "+
				"repository %v: %v", projectRef, err)
			return err
		}
	} else {
		lastVersion, err = model.FindMostRecentVersion(projectIdentifier)
		if err != nil {
			mci.Logger.Logf(slogger.ERROR, "error getting most recent version for "+
				"repository %v: %v", projectRef, err)
			return err
		}
	}

	if lastVersion == nil {
		mci.Logger.Logf(slogger.WARN, "no version to activate for repository %v", projectIdentifier)
		return nil
	}

	err = repoTracker.activateVersionVariantsIfNeeded(lastVersion)

	if err != nil {
		mci.Logger.Logf(slogger.ERROR, "error activating variants: %v", err)
		return err
	}

	return nil
}

// Activates any builds if their BatchTimes have elapsed.
func (repoTracker *RepoTracker) activateVersionVariantsIfNeeded(version *model.Version) (err error) {
	projectIdentifier := repoTracker.ProjectRef.String()
	hasActivated := false
	now := repoTracker.Clock.Now()
	for i, status := range version.BuildVariants {
		// last comparison is to check that ActivateAt is actually set
		if !status.Activated && now.After(status.ActivateAt) && !status.ActivateAt.IsZero() {
			mci.Logger.Logf(slogger.INFO, "activating variant %v for project %v, revision %v",
				status.BuildVariant, projectIdentifier, version.Revision)

			// Go copies the slice value, we want to modify the actual value
			status.Activated = true
			status.ActivateAt = now
			version.BuildVariants[i] = status

			build, err := model.FindBuild(status.BuildId)
			if err != nil {
				mci.Logger.Logf(slogger.ERROR, "error retrieving build for project %v, variant %v, build %v: %v",
					projectIdentifier, status.BuildVariant, status.BuildId, err)
				continue
			}
			mci.Logger.Logf(slogger.INFO, "activating build %v for project %v, variant %v",
				status.BuildId, projectIdentifier, status.BuildVariant)
			// Don't need to set the version in here since we do it ourselves in a single update
			err = build.SetActivated(true, false)
			if err != nil {
				mci.Logger.Logf(slogger.ERROR, "error activating build %v for project %v, variant %v: %v",
					projectIdentifier, build, status.BuildVariant, build, err)
				continue
			}
			hasActivated = true
		}
	}

	// If any variants were activated, update the stored version so that we don't attempt to activate them
	// again
	if hasActivated {
		return version.UpdateBuildVariants()
	}
	return nil
}

// createStubVersion is used to create a place holder version when there's a
// problem fetching/validating a project's configuration data
func (repoTracker *RepoTracker) createStubVersion(projectRef *model.ProjectRef,
	revision *model.Revision, errMessages []string) (*model.Version, error) {
	// create a stub version
	mci.Logger.Logf(slogger.INFO, "Creating stub version for project "+
		"“%v” at revision “%v”", projectRef.Identifier,
		revision.Revision)
	return model.CreateStubVersionFromRevision(projectRef,
		revision, repoTracker.MCISettings, errMessages)
}

// sendFailureNotification sends a notification to the MCI Team when the
// repotracker is unable to fetch revisions from a given project ref
func (repoTracker *RepoTracker) sendFailureNotification(lastRevision string,
	err error) {
	// Send a notification to the MCI team
	projectRef := repoTracker.ProjectRef
	settings := repoTracker.MCISettings
	subject := fmt.Sprintf(notify.RepotrackerFailurePreface,
		projectRef.Identifier, lastRevision)
	url := fmt.Sprintf("%v/%v/%v/commits/%v", thirdparty.GithubBase,
		projectRef.Owner, projectRef.Repo, projectRef.Branch)
	message := fmt.Sprintf("Could not find last known revision '%v' "+
		"within the most recent %v revisions at %v: %v", lastRevision,
		settings.RepoTracker.MaxRepoRevisionsToSearch, url, err)
	nErr := notify.NotifyAdmins(subject, message, settings)
	if nErr != nil {
		mci.Logger.Logf(slogger.ERROR, "error sending email: %v", nErr)
	}
}

// Constructs all versions stored from recent repository revisions
// The additional complexity is due to support for project modifications on patch builds.
// We need to parse the remote config as it existed when each revision was created.
// The return value is the most recent version created as a result of storing the revisions.
// This function is idempotent with regard to storing the same version multiple times.
func (repoTracker *RepoTracker) StoreRepositoryRevisions(revisions []model.Revision) (
	newestVersion *model.Version, err error) {
	projectRef := repoTracker.ProjectRef
	projectIdentifier := projectRef.String()

	for i := len(revisions) - 1; i >= 0; i-- {

		revision := revisions[i].Revision
		mci.Logger.Logf(slogger.INFO, "Now processing “%v” revision “%v”",
			projectIdentifier, revision)

		// We check if the version exists here so we can avoid fetching the github config
		// unnecessarily
		existingVersion, err := model.FindVersionByProjectAndRevision(projectRef, &revisions[i])
		if err != nil {
			mci.Logger.Logf(slogger.ERROR,
				"Unable to check existence of version for project %v, revision %v",
				projectIdentifier, revision)
		}
		if existingVersion != nil {
			mci.Logger.Logf(slogger.INFO,
				"Skipping creation of version for project %v, revision %v since"+
					" we already have a record for it", projectIdentifier, revision)
			// We bind newestVersion here since we still need to return the most recent
			// version, even if it already exists
			newestVersion = existingVersion
			continue
		}

		project, err := repoTracker.GetProjectConfig(revision)
		if err != nil {
			projectError, isProjectError := err.(projectConfigError)
			if isProjectError {
				newestVersion, err = repoTracker.createStubVersion(projectRef,
					&revisions[i], projectError.errors)
				continue
			} else {
				// Infrastructure error, don't create stub
				mci.Logger.Logf(slogger.INFO,
					"Unable to get project config for project %v for revision %v: %v",
					projectIdentifier, revision, err)
				return nil, err
			}
		}
		// We rebind newestVersion each iteration, so the last binding will be the newest version
		newestVersion, err = repoTracker.storeRevision(&revisions[i], project)
		if err != nil {
			mci.Logger.Logf(slogger.ERROR, "Unable to store revision %v for project %v: %v:",
				revision, projectIdentifier, err)
			return nil, err
		}
	}
	return newestVersion, err
}

// Creates a Version for a single revision and project configuration
func (repoTracker *RepoTracker) storeRevision(revision *model.Revision, project *model.Project) (
	*model.Version, error) {
	projectIdentifier := repoTracker.ProjectRef.String()
	mci.Logger.Logf(slogger.INFO, "Creating version for project "+
		"“%v” at revision “%v”", projectIdentifier, revision)
	version, err := model.CreateVersionFromRevision(repoTracker.ProjectRef, project,
		revision, repoTracker.MCISettings)
	if err != nil {
		return nil, fmt.Errorf("error creating version for "+
			"project “%v” at revision “%v”: %v", projectIdentifier, revision, err)
	}
	return version, nil
}

// GetProjectConfig fetches the project configuration for a given repository
// returning a remote config if the project references a remote repository
// configuration file - via the Identifier. Otherwise it defaults to the local
// project file
func (repoTracker *RepoTracker) GetProjectConfig(revision string) (
	project *model.Project, err error) {
	projectRef := repoTracker.ProjectRef
	mciSettings := repoTracker.MCISettings
	if !projectRef.Remote {
		// return the local config
		return model.FindProject("", projectRef.String(), mciSettings.ConfigDir)
	}
	project, err = repoTracker.GetRemoteConfig(revision)
	if err != nil {
		// Only create a stub version on API request errors that pertain
		// to actually fetching a config. Those errors currently include:
		// thirdparty.APIRequestError, thirdparty.FileNotFoundError and
		// thirdparty.YAMLFormatError
		_, apiReqErr := err.(thirdparty.APIRequestError)
		_, ymlFmtErr := err.(thirdparty.YAMLFormatError)
		_, noFileErr := err.(thirdparty.FileNotFoundError)
		if apiReqErr || noFileErr || ymlFmtErr {
			// If there's an error getting the remote config, e.g. because it
			// does not exist, we treat this the same as when the remote config
			// is invalid - but add a different error message
			message := fmt.Sprintf("error fetching project “%v” configuration "+
				"data at revision “%v” (remote path=“%v”): %v",
				projectRef.Identifier, revision, projectRef.RemotePath, err)
			mci.Logger.Logf(slogger.ERROR, message)
			return nil, projectConfigError{[]string{message}}
		}
		// If we get here then we have an infrastructural error - e.g.
		// a thirdparty.APIUnmarshalError (indicating perhaps an API has
		// changed), a thirdparty.ResponseReadError(problem reading an
		// API response) or a thirdparty.APIResponseError (nil API
		// response) - or encountered a problem in fetching a local
		// configuration file. At any rate, this is bad enough that we
		// want to send a notification instead of just creating a stub
		// version.
		//
		// Tentative TODO: MCI-1893 - send notification to project
		// maintainer in addition to creating a stub version
		var lastRevision string
		repository, fErr := model.FindRepository(projectRef.Identifier)
		if fErr != nil || repository == nil {
			mci.Logger.Logf(slogger.ERROR, "error finding "+
				"repository '%v': %v", projectRef.Identifier, fErr)
		} else {
			lastRevision = repository.LastRevision
		}
		repoTracker.sendFailureNotification(lastRevision, err)
		return nil, err
	}

	// check if project config is valid
	errs := validator.CheckProjectSyntax(project, repoTracker.MCISettings)
	if len(errs) != 0 {
		// We have syntax errors in the project.
		// Format them, as we need to store + display them to the user
		var message string
		var projectParseErrors []string
		for _, configError := range errs {
			message += fmt.Sprintf("\n\t=> %v", configError)
			projectParseErrors = append(projectParseErrors, configError.Error())
		}
		mci.Logger.Logf(slogger.ERROR, "error validating project '%v' "+
			"configuration at revision '%v': %v", projectRef.Identifier,
			revision, message)
		return nil, projectConfigError{projectParseErrors}
	}
	return project, nil

}
