package repotracker

import (
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/notify"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/evergreen/validator"
	"gopkg.in/yaml.v2"
	"time"
)

const (
	// determines the default maximum number of revisions to fetch for a newly tracked repo
	// if not specified in configuration file
	DefaultNumNewRepoRevisionsToFetch = 200
)

// RepoTracker is used to manage polling repository changes and storing such
// changes. It contains a number of interfaces that specify behavior required by
// client implementations
type RepoTracker struct {
	*evergreen.Settings
	*model.ProjectRef
	RepoPoller
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
	GetRevisionsSince(sinceRevision string, maxRevisions int) ([]model.Revision, error)
	// Fetches the most recent 'numNewRepoRevisionsToFetch' revisions for a
	// project - with the most recent revision appearing as the first element in
	// the slice.
	GetRecentRevisions(numNewRepoRevisionsToFetch int) ([]model.Revision, error)
}

type projectConfigError struct {
	Errors   []string
	Warnings []string
}

func (p projectConfigError) Error() string {
	return "Invalid project configuration"
}

// The FetchRevisions method is used by a RepoTracker to run the pipeline for
// tracking repositories. It performs everything from polling the repository to
// persisting any changes retrieved from the repository reference.
func (repoTracker *RepoTracker) FetchRevisions(numNewRepoRevisionsToFetch int) (
	err error) {
	settings := repoTracker.Settings
	projectRef := repoTracker.ProjectRef
	projectIdentifier := projectRef.String()

	if !projectRef.Enabled {
		evergreen.Logger.Logf(slogger.INFO, "Skipping disabled project “%v”", projectRef)
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
		evergreen.Logger.Logf(slogger.INFO, "No last recorded repository revision "+
			"for “%v”. Proceeding to fetch most recent %v revisions",
			projectRef, numNewRepoRevisionsToFetch)
		revisions, err = repoTracker.GetRecentRevisions(numNewRepoRevisionsToFetch)
	} else {
		evergreen.Logger.Logf(slogger.INFO, "Last recorded repository revision for "+
			"“%v” is “%v”", projectRef, lastRevision)
		// if the projectRef has a repotracker error then don't get the revisions
		if projectRef.RepotrackerError != nil {
			if projectRef.RepotrackerError.Exists {
				evergreen.Logger.Logf(slogger.ERROR, "repotracker error for base revision, %v",
					projectRef.RepotrackerError.InvalidRevision)
				return nil
			}
		}
		revisions, err = repoTracker.GetRevisionsSince(lastRevision,
			settings.RepoTracker.MaxRepoRevisionsToSearch)
	}

	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, "error fetching revisions for "+
			"repository “%v”: %v", projectRef, err)
		repoTracker.sendFailureNotification(lastRevision, err)
		return nil
	}

	var lastVersion *version.Version

	if len(revisions) > 0 {
		lastVersion, err = repoTracker.StoreRevisions(revisions)
		if err != nil {
			evergreen.Logger.Logf(slogger.ERROR, "error storing revisions for "+
				"repository %v: %v", projectRef, err)
			return err
		}
		err = model.UpdateLastRevision(lastVersion.Identifier, lastVersion.Revision)
		if err != nil {
			evergreen.Logger.Logf(slogger.ERROR, "error updating last revision for "+
				"repository %v: %v", projectRef, err)
			return err
		}
	} else {
		lastVersion, err = version.FindOne(version.ByMostRecentForRequester(projectIdentifier, evergreen.RepotrackerVersionRequester))
		if err != nil {
			evergreen.Logger.Logf(slogger.ERROR, "error getting most recent version for "+
				"repository %v: %v", projectRef, err)
			return err
		}
	}

	if lastVersion == nil {
		evergreen.Logger.Logf(slogger.WARN, "no version to activate for repository %v", projectIdentifier)
		return nil
	}

	err = repoTracker.activateElapsedBuilds(lastVersion)

	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, "error activating variants: %v", err)
		return err
	}

	return nil
}

// Activates any builds if their BatchTimes have elapsed.
func (repoTracker *RepoTracker) activateElapsedBuilds(v *version.Version) (err error) {
	projectId := repoTracker.ProjectRef.Identifier
	hasActivated := false
	now := time.Now()
	for i, status := range v.BuildVariants {
		// last comparison is to check that ActivateAt is actually set
		if !status.Activated && now.After(status.ActivateAt) && !status.ActivateAt.IsZero() {
			evergreen.Logger.Logf(slogger.INFO, "activating variant %v for project %v, revision %v",
				status.BuildVariant, projectId, v.Revision)

			// Go copies the slice value, we want to modify the actual value
			status.Activated = true
			status.ActivateAt = now
			v.BuildVariants[i] = status

			b, err := build.FindOne(build.ById(status.BuildId))
			if err != nil {
				evergreen.Logger.Logf(slogger.ERROR,
					"error retrieving build for project %v, variant %v, build %v: %v",
					projectId, status.BuildVariant, status.BuildId, err)
				continue
			}
			evergreen.Logger.Logf(slogger.INFO, "activating build %v for project %v, variant %v",
				status.BuildId, projectId, status.BuildVariant)
			// Don't need to set the version in here since we do it ourselves in a single update
			if err = model.SetBuildActivation(b.Id, true, evergreen.DefaultTaskActivator); err != nil {
				evergreen.Logger.Logf(slogger.ERROR, "error activating build %v for project %v, variant %v: %v",
					b.Id, projectId, status.BuildVariant, err)
				continue
			}
			hasActivated = true
		}
	}

	// If any variants were activated, update the stored version so that we don't
	// attempt to activate them again
	if hasActivated {
		return v.UpdateBuildVariants()
	}
	return nil
}

// sendFailureNotification sends a notification to the MCI Team when the
// repotracker is unable to fetch revisions from a given project ref
func (repoTracker *RepoTracker) sendFailureNotification(lastRevision string,
	err error) {
	// Send a notification to the MCI team
	projectRef := repoTracker.ProjectRef
	settings := repoTracker.Settings
	subject := fmt.Sprintf(notify.RepotrackerFailurePreface,
		projectRef.Identifier, lastRevision)
	url := fmt.Sprintf("%v/%v/%v/commits/%v", thirdparty.GithubBase,
		projectRef.Owner, projectRef.Repo, projectRef.Branch)
	message := fmt.Sprintf("Could not find last known revision '%v' "+
		"within the most recent %v revisions at %v: %v", lastRevision,
		settings.RepoTracker.MaxRepoRevisionsToSearch, url, err)
	nErr := notify.NotifyAdmins(subject, message, settings)
	if nErr != nil {
		evergreen.Logger.Logf(slogger.ERROR, "error sending email: %v", nErr)
	}
}

// Verifies that the given revision order number is higher than the latest number stored for the project.
func sanityCheckOrderNum(revOrderNum int, projectId string) error {
	latest, err := version.FindOne(version.ByMostRecentForRequester(projectId, evergreen.RepotrackerVersionRequester))
	if err != nil {
		return fmt.Errorf("Error getting latest version: %v", err.Error())
	}

	// When there are no versions in the db yet, sanity check is moot
	if latest != nil {
		if revOrderNum <= latest.RevisionOrderNumber {
			return fmt.Errorf("Commit order number isn't greater than last stored version's: %v <= %v",
				revOrderNum, latest.RevisionOrderNumber)
		}
	}
	return nil
}

// Constructs all versions stored from recent repository revisions
// The additional complexity is due to support for project modifications on patch builds.
// We need to parse the remote config as it existed when each revision was created.
// The return value is the most recent version created as a result of storing the revisions.
// This function is idempotent with regard to storing the same version multiple times.
func (repoTracker *RepoTracker) StoreRevisions(revisions []model.Revision) (newestVersion *version.Version, err error) {
	defer func() {
		if newestVersion != nil {
			// Fetch the updated version doc, so that we include buildvariants in the result
			newestVersion, err = version.FindOne(version.ById(newestVersion.Id))
		}
	}()
	ref := repoTracker.ProjectRef

	for i := len(revisions) - 1; i >= 0; i-- {
		revision := revisions[i].Revision
		evergreen.Logger.Logf(slogger.INFO, "Processing revision %v in project %v", revision, ref.Identifier)

		// We check if the version exists here so we can avoid fetching the github config unnecessarily
		existingVersion, err := version.FindOne(version.ByProjectIdAndRevision(ref.Identifier, revisions[i].Revision))
		if err != nil {
			evergreen.Logger.Logf(slogger.ERROR,
				"Error looking up version at %v for project %v: %v", ref.Identifier, revision, err)
		}
		if existingVersion != nil {
			evergreen.Logger.Logf(slogger.INFO,
				"Skipping creation of version for project %v, revision %v since"+
					" we already have a record for it", ref.Identifier, revision)
			// We bind newestVersion here since we still need to return the most recent
			// version, even if it already exists
			newestVersion = existingVersion
			continue
		}

		// Create the stub of the version (not stored in DB yet)
		v, err := NewVersionFromRevision(ref, revisions[i])
		if err != nil {
			evergreen.Logger.Logf(slogger.ERROR, "Error creating version for project %v: %v", ref.Identifier, err)
		}
		err = sanityCheckOrderNum(v.RevisionOrderNumber, ref.Identifier)
		if err != nil { // something seriously wrong (bad data in db?) so fail now
			panic(err)
		}
		project, err := repoTracker.GetProjectConfig(revision)
		if err != nil {
			projectError, isProjectError := err.(projectConfigError)
			if isProjectError {
				if len(projectError.Warnings) > 0 {
					// Store the warnings and keep going. If we don't have
					// any true errors, the version will still be created.
					v.Warnings = projectError.Warnings
				}
				if len(projectError.Errors) > 0 {
					// Store just the stub version with the project errors
					v.Errors = projectError.Errors
					if err := v.Insert(); err != nil {
						evergreen.Logger.Logf(slogger.ERROR,
							"Failed storing stub version for project %v: %v", ref.Identifier, err)
						return nil, err
					}
					newestVersion = v
					continue
				}
			} else {
				// Fatal error - don't store the stub
				evergreen.Logger.Logf(slogger.INFO,
					"Failed to get config for project %v at revision %v: %v", ref.Identifier, revision, err)
				return nil, err
			}
		}

		// We have a config, so turn it into a usable yaml string to store with the version doc
		projectYamlBytes, err := yaml.Marshal(project)
		if err != nil {
			return nil, fmt.Errorf("Error marshalling config: %v", err)
		}
		v.Config = string(projectYamlBytes)

		// We rebind newestVersion each iteration, so the last binding will be the newest version
		err = createVersionItems(v, ref, project)
		if err != nil {
			evergreen.Logger.Logf(slogger.ERROR, "Error creating version items for %v in project %v: %v",
				v.Id, ref.Identifier, err)
			return nil, err
		}

		newestVersion = v
		if err != nil {
			evergreen.Logger.Logf(slogger.ERROR, "Unable to store revision %v for project %v: %v:",
				revision, ref.Identifier, err)
			return nil, err
		}
	}
	return
}

// GetProjectConfig fetches the project configuration for a given repository
// returning a remote config if the project references a remote repository
// configuration file - via the Identifier. Otherwise it defaults to the local
// project file. An erroneous project file may be returned along with an error.
func (repoTracker *RepoTracker) GetProjectConfig(revision string) (*model.Project, error) {
	projectRef := repoTracker.ProjectRef
	if projectRef.LocalConfig != "" {
		// return the Local config from the project Ref.
		return model.FindProject("", projectRef)
	}
	project, err := repoTracker.GetRemoteConfig(revision)
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
			evergreen.Logger.Logf(slogger.ERROR, message)
			return nil, projectConfigError{[]string{message}, nil}
		}
		// If we get here then we have an infrastructural error - e.g.
		// a thirdparty.APIUnmarshalError (indicating perhaps an API has
		// changed), a thirdparty.ResponseReadError(problem reading an
		// API response) or a thirdparty.APIResponseError (nil API
		// response) - or encountered a problem in fetching a local
		// configuration file. At any rate, this is bad enough that we
		// want to send a notification instead of just creating a stub
		// version.
		var lastRevision string
		repository, fErr := model.FindRepository(projectRef.Identifier)
		if fErr != nil || repository == nil {
			evergreen.Logger.Logf(slogger.ERROR, "error finding "+
				"repository '%v': %v", projectRef.Identifier, fErr)
		} else {
			lastRevision = repository.LastRevision
		}
		repoTracker.sendFailureNotification(lastRevision, err)
		return nil, err
	}

	// check if project config is valid
	verrs, err := validator.CheckProjectSyntax(project)
	if err != nil {
		return nil, err
	}
	if len(verrs) != 0 {
		// We have syntax errors in the project.
		// Format them, as we need to store + display them to the user
		var errMessage, warnMessage string
		var projectErrors, projectWarnings []string
		for _, e := range verrs {
			if e.Level == validator.Warning {
				warnMessage += fmt.Sprintf("\n\t=> %v", e)
				projectWarnings = append(projectWarnings, e.Error())
			} else {
				errMessage += fmt.Sprintf("\n\t=> %v", e)
				projectErrors = append(projectErrors, e.Error())
			}
		}
		if len(projectErrors) > 0 {
			evergreen.Logger.Logf(slogger.ERROR, "Error validating project '%v' "+
				"configuration at revision '%v': %v", projectRef.Identifier,
				revision, errMessage)
		}
		if len(projectWarnings) > 0 {
			evergreen.Logger.Logf(slogger.ERROR, "Warning while validating project '%v' "+
				"configuration at revision '%v': %v", projectRef.Identifier,
				revision, warnMessage)
		}

		return project, projectConfigError{projectErrors, projectWarnings}
	}
	return project, nil
}

// NewVersionFromRevision populates a new Version with metadata from a model.Revision.
// Does not populate its config or store anything in the database.
func NewVersionFromRevision(ref *model.ProjectRef, rev model.Revision) (*version.Version, error) {
	number, err := model.GetNewRevisionOrderNumber(ref.Identifier)
	if err != nil {
		return nil, err
	}
	v := &version.Version{
		Author:              rev.Author,
		AuthorEmail:         rev.AuthorEmail,
		Branch:              ref.Branch,
		CreateTime:          rev.CreateTime,
		Id:                  util.CleanName(fmt.Sprintf("%v_%v", ref.String(), rev.Revision)),
		Identifier:          ref.Identifier,
		Message:             rev.RevisionMessage,
		Owner:               ref.Owner,
		RemotePath:          ref.RemotePath,
		Repo:                ref.Repo,
		RepoKind:            ref.RepoKind,
		Requester:           evergreen.RepotrackerVersionRequester,
		Revision:            rev.Revision,
		Status:              evergreen.VersionCreated,
		RevisionOrderNumber: number,
	}
	return v, nil
}

// createVersionItems populates and stores all the tasks and builds for a version according to
// the given project config.
func createVersionItems(v *version.Version, ref *model.ProjectRef, project *model.Project) error {
	// generate all task Ids so that we can easily reference them for dependencies
	taskIdTable := model.BuildTaskIdTable(project, v)

	// create all builds for the version
	for _, buildvariant := range project.BuildVariants {
		if buildvariant.Disabled {
			continue
		}
		buildId, err := model.CreateBuildFromVersion(project, v, taskIdTable, buildvariant.Name, false, nil)
		if err != nil {
			return err
		}

		lastActivated, err := version.FindOne(version.ByLastVariantActivation(ref.Identifier, buildvariant.Name))
		if err != nil {
			evergreen.Logger.Logf(slogger.ERROR, "Error getting activation time for bv %v", buildvariant.Name)
			return err
		}

		var lastActivation *time.Time
		if lastActivated != nil {
			for _, buildStatus := range lastActivated.BuildVariants {
				if buildStatus.BuildVariant == buildvariant.Name && buildStatus.Activated {
					lastActivation = &buildStatus.ActivateAt
					break
				}
			}
		}

		var activateAt time.Time
		var activated bool
		if lastActivation == nil {
			// if we don't have a last activation time then activate now.
			activateAt = time.Now()
			activated = true
		} else {
			activateAt = lastActivation.Add(time.Minute * time.Duration(ref.GetBatchTime(&buildvariant)))
			evergreen.Logger.Logf(slogger.INFO, "Going to activate bv %v for project %v, version %v at %v",
				buildvariant.Name, ref.Identifier, v.Id, activateAt)
		}

		v.BuildIds = append(v.BuildIds, buildId)
		v.BuildVariants = append(v.BuildVariants, version.BuildStatus{
			BuildVariant: buildvariant.Name,
			Activated:    activated,
			ActivateAt:   activateAt,
			BuildId:      buildId,
		})
	}

	if err := v.Insert(); err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "Error inserting version %v: %v", v.Id, err)
		for _, buildStatus := range v.BuildVariants {
			if buildErr := model.DeleteBuild(buildStatus.BuildId); buildErr != nil {
				evergreen.Logger.Errorf(slogger.ERROR, "Error deleting build %v: %v",
					buildStatus.BuildId, buildErr)
			}
		}
		return err
	}
	return nil
}
