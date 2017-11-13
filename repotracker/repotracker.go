package repotracker

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/notify"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

const (
	// determines the default maximum number of revisions to fetch for a newly tracked repo
	// if not specified in configuration file
	DefaultNumNewRepoRevisionsToFetch = 200
	DefaultMaxRepoRevisionsToSearch   = 50
	DefaultNumConcurrentRequests      = 10
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
	// the given revision.
	GetRemoteConfig(revision string) (*model.Project, error)

	// Fetches a list of all filepaths modified by a given revision.
	GetChangedFiles(revision string) ([]string, error)

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
func (repoTracker *RepoTracker) FetchRevisions(numNewRepoRevisionsToFetch int) error {
	settings := repoTracker.Settings
	projectRef := repoTracker.ProjectRef
	projectIdentifier := projectRef.String()

	if !projectRef.Enabled {
		// this is somewhat belt-and-suspenders, as the
		// repotracker runner process doesn't run for disabled
		// proejcts.
		grip.Infoln("Skipping disabled project:", projectRef)
		return nil
	}

	repository, err := model.FindRepository(projectIdentifier)
	if err != nil {
		return errors.Wrapf(err, "error finding repository '%v'", projectIdentifier)
	}

	var revisions []model.Revision
	var lastRevision string

	if repository != nil {
		lastRevision = repository.LastRevision
	}

	if lastRevision == "" {
		// if this is the first time we're running the tracker for this project,
		// fetch the most recent `numNewRepoRevisionsToFetch` revisions
		grip.Debugln("No last recorded repository revision for", projectRef,
			"Proceeding to fetch most recent", numNewRepoRevisionsToFetch, "revisions")
		revisions, err = repoTracker.GetRecentRevisions(numNewRepoRevisionsToFetch)
	} else {
		grip.Debugf("Last recorded revision for %s is %s", projectRef, lastRevision)
		// if the projectRef has a repotracker error then don't get the revisions
		if projectRef.RepotrackerError != nil {
			if projectRef.RepotrackerError.Exists {
				grip.Warningf("repotracker error for base revision '%s' on project '%s/%s:%s'",
					projectRef.RepotrackerError.InvalidRevision,
					projectRef.Owner, projectRef.Repo, projectRef.Branch)
				return nil
			}
		}
		max := settings.RepoTracker.MaxRepoRevisionsToSearch
		if max <= 0 {
			max = DefaultMaxRepoRevisionsToSearch
		}
		revisions, err = repoTracker.GetRevisionsSince(lastRevision, max)
	}

	if err != nil {
		grip.Errorf("error fetching revisions for repository %s: %+v", projectRef.Identifier, err)
		repoTracker.sendFailureNotification(lastRevision, err)
		return nil
	}

	if len(revisions) > 0 {
		var lastVersion *version.Version
		lastVersion, err = repoTracker.StoreRevisions(revisions)
		if err != nil {
			grip.Errorf("error storing revisions for repository %s: %+v", projectRef, err)
			return err
		}
		err = model.UpdateLastRevision(lastVersion.Identifier, lastVersion.Revision)
		if err != nil {
			grip.Errorf("error updating last revision for repository %s: %+v", projectRef, err)
			return err
		}
	}

	// fetch the most recent, non-ignored version version to activate
	activateVersion, err := version.FindOne(version.ByMostRecentNonignored(projectIdentifier))
	if err != nil {
		grip.Errorf("error getting most recent version for repository %s: %+v", projectRef, err)
		return err
	}
	if activateVersion == nil {
		grip.Warningf("no version to activate for repository %s", projectIdentifier)
		return nil
	}

	err = repoTracker.activateElapsedBuilds(activateVersion)
	if err != nil {
		grip.Errorln("error activating variants:", err)
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
			grip.Infof("activating variant %s for project %s, revision %s",
				status.BuildVariant, projectId, v.Revision)

			// Go copies the slice value, we want to modify the actual value
			status.Activated = true
			status.ActivateAt = now
			v.BuildVariants[i] = status

			b, err := build.FindOne(build.ById(status.BuildId))
			if err != nil {
				grip.Errorf("error retrieving build for project %s, variant %s, build %s: %+v",
					projectId, status.BuildVariant, status.BuildId, err)
				continue
			}
			grip.Infof("activating build %s for project %s, variant %s",
				status.BuildId, projectId, status.BuildVariant)
			// Don't need to set the version in here since we do it ourselves in a single update
			if err = model.SetBuildActivation(b.Id, true, evergreen.DefaultTaskActivator); err != nil {
				grip.Errorf("error activating build %s for project %s, variant %s: %+v",
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
	settings := repoTracker.Settings
	max := settings.RepoTracker.MaxRepoRevisionsToSearch
	if max <= 0 {
		max = DefaultMaxRepoRevisionsToSearch
	}
	projectRef := repoTracker.ProjectRef
	subject := fmt.Sprintf(notify.RepotrackerFailurePreface,
		projectRef.Identifier, lastRevision)
	url := fmt.Sprintf("%v/%v/%v/commits/%v", thirdparty.GithubBase,
		projectRef.Owner, projectRef.Repo, projectRef.Branch)
	message := fmt.Sprintf("Could not find last known revision '%v' "+
		"within the most recent %v revisions at %v: %v", lastRevision, max, url, err)
	nErr := notify.NotifyAdmins(subject, message, settings)
	if nErr != nil {
		grip.Errorf("error sending email: %+v", nErr)
	}
}

// Verifies that the given revision order number is higher than the latest number stored for the project.
func sanityCheckOrderNum(revOrderNum int, projectId string) error {
	latest, err := version.FindOne(version.ByMostRecentForRequester(projectId, evergreen.RepotrackerVersionRequester))
	if err != nil {
		return errors.Wrap(err, "Error getting latest version")
	}

	// When there are no versions in the db yet, sanity check is moot
	if latest != nil {
		if revOrderNum <= latest.RevisionOrderNumber {
			return errors.Errorf("Commit order number isn't greater than last stored version's: %v <= %v",
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
		grip.Infof("Processing revision %s in project %s", revision, ref.Identifier)

		// We check if the version exists here so we can avoid fetching the github config unnecessarily
		existingVersion, err := version.FindOne(version.ByProjectIdAndRevision(ref.Identifier, revisions[i].Revision))
		grip.ErrorWhenf(err != nil, "Error looking up version at %s for project %s: %+v", ref.Identifier, revision, err)
		if existingVersion != nil {
			grip.Infof("Skipping creation of version for project %s, revision %s "+
				"since we already have a record for it", ref.Identifier, revision)
			// We bind newestVersion here since we still need to return the most recent
			// version, even if it already exists
			newestVersion = existingVersion
			continue
		}

		// Create the stub of the version (not stored in DB yet)
		v, err := NewVersionFromRevision(ref, revisions[i])
		if err != nil {
			grip.Infof("Error creating version for project %s: %+v", ref.Identifier, err)
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
					if err = v.Insert(); err != nil {
						grip.Errorf("Failed storing stub version for project %s: %+v",
							ref.Identifier, err)
						return nil, err
					}
					newestVersion = v
					continue
				}
			} else {
				// Fatal error - don't store the stub
				grip.Infof("Failed to get config for project %s at %s: %+v",
					ref.Identifier, revision, err)
				return nil, err
			}
		}

		// We have a config, so turn it into a usable yaml string to store with the version doc
		projectYamlBytes, err := yaml.Marshal(project)
		if err != nil {
			return nil, errors.Wrap(err, "Error marshaling config")
		}
		v.Config = string(projectYamlBytes)

		// "Ignore" a version if all changes are to ignored files
		if len(project.Ignore) > 0 {
			filenames, err := repoTracker.GetChangedFiles(revision)
			if err != nil {
				return nil, errors.Wrap(err, "error checking GitHub for ignored files")
			}
			if project.IgnoresAllFiles(filenames) {
				v.Ignored = true
			}
		}

		// We rebind newestVersion each iteration, so the last binding will be the newest version
		err = errors.Wrapf(createVersionItems(v, ref, project),
			"Error creating version items for %s in project %s",
			v.Id, ref.Identifier)
		if err != nil {
			grip.Error(err)
			return nil, err
		}
		newestVersion = v
	}
	return newestVersion, nil
}

// GetProjectConfig fetches the project configuration for a given repository
// returning a remote config if the project references a remote repository
// configuration file - via the Identifier. Otherwise it defaults to the local
// project file. An erroneous project file may be returned along with an error.
func (repoTracker *RepoTracker) GetProjectConfig(revision string) (*model.Project, error) {
	projectRef := repoTracker.ProjectRef
	if projectRef.LocalConfig != "" {
		// return the Local config from the project Ref.
		p, err := model.FindProject("", projectRef)
		return p, err
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
			message := fmt.Sprintf("error fetching project '%v' configuration "+
				"data at revision '%v' (remote path='%v'): %v",
				projectRef.Identifier, revision, projectRef.RemotePath, err)
			grip.Error(message)
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
			grip.Errorf("error finding repository '%s': %+v",
				projectRef.Identifier, fErr)
		} else {
			lastRevision = repository.LastRevision
		}
		repoTracker.sendFailureNotification(lastRevision, err)
		return nil, err
	}

	validationStart := time.Now()
	// check if project config is valid
	verrs, err := validator.CheckProjectSyntax(project)
	if err != nil {
		return nil, err
	}
	validationDuration := time.Since(validationStart)

	if len(verrs) != 0 {

		// We have syntax errors in the project.
		// Format them, as we need to store + display them to the user
		var projectErrors, projectWarnings []string

		for _, e := range verrs {
			if e.Level == validator.Warning {
				projectWarnings = append(projectWarnings, e.Error())
			} else {
				projectErrors = append(projectErrors, e.Error())
			}
		}

		grip.Notice(message.Fields{
			"runner":      "repotracker",
			"operation":   "project config validation",
			"warnings":    projectWarnings,
			"errors":      projectErrors,
			"hasErrors":   len(projectErrors) > 0,
			"hasWarnings": len(projectWarnings) > 0,
			"duration":    validationDuration,
			"span":        validationDuration.String(),
		})

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
	execTable, displayTable := model.NewTaskIdTable(project, v)

	// create all builds for the version
	for _, buildvariant := range project.BuildVariants {
		if buildvariant.Disabled {
			continue
		}
		buildId, err := model.CreateBuildFromVersion(project, v, execTable, displayTable, buildvariant.Name, false, nil)
		if err != nil {
			return errors.WithStack(err)
		}

		lastActivated, err := version.FindOne(version.ByLastVariantActivation(ref.Identifier, buildvariant.Name))
		if err != nil {
			grip.Errorln("Error getting activation time for variant", buildvariant.Name)
			return errors.WithStack(err)
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
		if lastActivation == nil {
			// if we don't have a last activation time then prepare to activate it immediately.
			activateAt = time.Now()
		} else {
			activateAt = lastActivation.Add(time.Minute * time.Duration(ref.GetBatchTime(&buildvariant)))
		}
		grip.Infof("Going to activate bv %s for project %s, version %s at %s",
			buildvariant.Name, ref.Identifier, v.Id, activateAt)

		v.BuildIds = append(v.BuildIds, buildId)
		v.BuildVariants = append(v.BuildVariants, version.BuildStatus{
			BuildVariant: buildvariant.Name,
			Activated:    false,
			ActivateAt:   activateAt,
			BuildId:      buildId,
		})
	}

	if err := v.Insert(); err != nil {
		grip.Errorf("inserting version %s: %+v", v.Id, err)
		for _, buildStatus := range v.BuildVariants {
			if buildErr := model.DeleteBuild(buildStatus.BuildId); buildErr != nil {
				grip.Errorf("deleting build %s: %+v", buildStatus.BuildId, buildErr)
			}
		}
		return errors.WithStack(err)
	}
	return nil
}
