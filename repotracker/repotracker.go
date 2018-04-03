package repotracker

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/notify"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
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
	GetRemoteConfig(ctx context.Context, revision string) (*model.Project, error)

	// Fetches a list of all filepaths modified by a given revision.
	GetChangedFiles(ctx context.Context, revision string) ([]string, error)

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
func (repoTracker *RepoTracker) FetchRevisions(ctx context.Context, numNewRepoRevisionsToFetch int) error {
	settings := repoTracker.Settings
	projectRef := repoTracker.ProjectRef
	projectIdentifier := projectRef.String()

	if !projectRef.Enabled {
		// this is somewhat belt-and-suspenders, as the
		// repotracker runner process doesn't run for disabled
		// proejcts.
		grip.Info(message.Fields{
			"message": "skip disabled project",
			"project": projectRef,
			"runner":  RunnerName,
		})
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
		grip.Debug(message.Fields{
			"runner":  RunnerName,
			"project": projectRef,
			"message": "no last recorded revision, using most recent revisions",
			"number":  numNewRepoRevisionsToFetch,
		})
		revisions, err = repoTracker.GetRecentRevisions(numNewRepoRevisionsToFetch)
	} else {
		grip.Debug(message.Fields{
			"message":  "found last recorded revision",
			"project":  projectRef,
			"runner":   RunnerName,
			"revision": lastRevision,
		})
		// if the projectRef has a repotracker error then don't get the revisions
		if projectRef.RepotrackerError != nil {
			if projectRef.RepotrackerError.Exists {
				grip.Warning(message.Fields{
					"runner":  RunnerName,
					"message": "repotracker error for base revision",
					"project": projectRef,
					"path":    fmt.Sprintf("%s/%s:%s", projectRef.Owner, projectRef.Repo, projectRef.Branch),
				})
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
		grip.Error(message.WrapError(err, message.Fields{
			"message": "problem fetching revisions for repository",
			"runner":  RunnerName,
			"project": projectRef.Identifier,
		}))
		return nil
	}

	if len(revisions) > 0 {
		var lastVersion *version.Version
		lastVersion, err = repoTracker.StoreRevisions(ctx, revisions)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "problem sorting revisions for repository",
				"runner":  RunnerName,
				"project": projectRef,
			}))
			return errors.WithStack(err)
		}
		err = model.UpdateLastRevision(lastVersion.Identifier, lastVersion.Revision)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "problem updating last revision for repository",
				"project": projectRef,
				"runner":  RunnerName,
			}))
			return errors.WithStack(err)
		}
	}

	if err := repoTracker.activationForProject(projectIdentifier); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "problem activating recent commit for project",
			"project": projectIdentifier,
			"runner":  RunnerName,
			"mode":    "ingestion",
		}))

		return errors.WithStack(err)
	}

	return nil
}

func (repoTracker *RepoTracker) activationForProject(projectId string) error {
	// fetch the most recent, non-ignored version version to activate
	activateVersion, err := version.FindOne(version.ByMostRecentNonignored(projectId))
	if err != nil {
		return errors.WithStack(err)
	}
	if activateVersion == nil {
		grip.Info(message.Fields{
			"message": "no version to activate for repository",
			"project": projectId,
			"runner":  RunnerName,
		})
		return nil
	}

	if err = repoTracker.activateElapsedBuilds(activateVersion); err != nil {
		return errors.WithStack(err)
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
			grip.Info(message.Fields{
				"message":  "activating revision",
				"variant":  status.BuildVariant,
				"project":  projectId,
				"revision": v.Revision,
				"runner":   RunnerName,
			})

			// Go copies the slice value, we want to modify the actual value
			status.Activated = true
			status.ActivateAt = now
			v.BuildVariants[i] = status

			b, err := build.FindOne(build.ById(status.BuildId))
			if err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message": "problem retrieving build",
					"runner":  RunnerName,
					"variant": status.BuildVariant,
					"build":   status.BuildId,
					"project": projectId,
				}))
				continue
			}

			grip.Info(message.Fields{
				"message": "activating build",
				"runner":  RunnerName,
				"variant": status.BuildVariant,
				"build":   status.BuildId,
				"project": projectId,
			})

			// Don't need to set the version in here since we do it ourselves in a single update
			if err = model.SetBuildActivation(b.Id, true, evergreen.DefaultTaskActivator); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"runner":  RunnerName,
					"message": "problem activating build",
					"variant": status.BuildVariant,
					"build":   status.BuildId,
					"project": projectId,
				}))
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
func (repoTracker *RepoTracker) sendFailureNotification(lastRevision string, err error) {
	// Send a notification to the MCI team
	settings := repoTracker.Settings
	max := settings.RepoTracker.MaxRepoRevisionsToSearch
	if max <= 0 {
		max = DefaultMaxRepoRevisionsToSearch
	}
	projectRef := repoTracker.ProjectRef
	subject := fmt.Sprintf(notify.RepotrackerFailurePreface,
		projectRef.Identifier, lastRevision)
	url := fmt.Sprintf("https://api.github.com/%v/%v/commits/%v",
		projectRef.Owner, projectRef.Repo, projectRef.Branch)
	msg := fmt.Sprintf("Could not find last known revision '%v' "+
		"within the most recent %v revisions at %v: %v", lastRevision, max, url, err)
	nErr := notify.NotifyAdmins(subject, msg, settings)
	if nErr != nil {
		grip.Error(message.WrapError(nErr, message.Fields{
			"message":  "error sending email",
			"runner":   RunnerName,
			"revision": lastRevision,
			"content":  msg,
			"subject":  subject,
		}))
	}
}

// Verifies that the given revision order number is higher than the latest number stored for the project.
func sanityCheckOrderNum(revOrderNum int, projectId, revision string) error {
	latest, err := version.FindOne(version.ByMostRecentForRequester(projectId, evergreen.RepotrackerVersionRequester))
	if err != nil || latest == nil {
		return errors.Wrap(err, "Error getting latest version")
	}

	if latest.Revision == revision {
		grip.Critical(message.Fields{
			"project":   projectId,
			"runner":    RunnerName,
			"rev_num":   revOrderNum,
			"revision":  revision,
			"latest_id": latest.Id,
		})
		return errors.New("refusing to add a new version with a duplicate revision id")
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
func (repoTracker *RepoTracker) StoreRevisions(ctx context.Context, revisions []model.Revision) (newestVersion *version.Version, err error) {
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
		grip.Error(message.WrapError(err, message.Fields{
			"message":  "problem looking up version for project",
			"runner":   RunnerName,
			"project":  ref.Identifier,
			"revision": revision,
		}))

		if existingVersion != nil {
			grip.Info(message.Fields{
				"message":  "skipping creating version because it already exists",
				"runner":   RunnerName,
				"project":  ref.Identifier,
				"revision": revision,
			})
			// We bind newestVersion here since we still need to return the most recent
			// version, even if it already exists
			newestVersion = existingVersion
			continue
		}

		// Create the stub of the version (not stored in DB yet)
		v, err := NewVersionFromRevision(ref, revisions[i])
		grip.Info(message.WrapError(err, message.Fields{
			"message": "problem creating version for project",
			"runner":  RunnerName,
			"project": ref.Identifier,
		}))

		if err = sanityCheckOrderNum(v.RevisionOrderNumber, ref.Identifier, revisions[i].Revision); err != nil {
			// something seriously wrong (bad data in db?) so fail now
			//
			// this will get caught at the top level where it's logged as an alert.
			//
			// since all repotracker code now runs in an
			// amboy job, it won't actually retry, and it
			// won't take down the worker.
			panic(err)
		}
		project, err := repoTracker.GetProjectConfig(ctx, revision)
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
						grip.Error(message.WrapError(err, message.Fields{
							"message":  "failed storing stub version in project",
							"project":  ref.Identifier,
							"revision": revision,
							"runner":   RunnerName,
							"version":  v.Id,
						}))
						return nil, err
					}
					newestVersion = v
					continue
				}
			} else {
				// Fatal error - don't store the stub
				grip.Error(message.WrapError(err, message.Fields{
					"message":  "could not store project stub",
					"project":  ref.Identifier,
					"revision": revision,
					"runner":   RunnerName,
					"version":  v.Id,
				}))
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
			filenames, err := repoTracker.GetChangedFiles(ctx, revision)
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
			grip.Error(message.WrapError(err, message.Fields{
				"runner":  RunnerName,
				"project": ref.Identifier,
			}))
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
func (repoTracker *RepoTracker) GetProjectConfig(ctx context.Context, revision string) (*model.Project, error) {
	projectRef := repoTracker.ProjectRef
	if projectRef.LocalConfig != "" {
		// return the Local config from the project Ref.
		p, err := model.FindProject("", projectRef)
		return p, err
	}
	project, err := repoTracker.GetRemoteConfig(ctx, revision)
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
			msg := message.ConvertToComposer(level.Error, message.Fields{
				"message":  "problem finding project configuration",
				"runner":   RunnerName,
				"project":  projectRef.Identifier,
				"revision": revision,
				"path":     projectRef.RemotePath,
			})

			grip.Error(message.WrapError(err, msg))
			return nil, projectConfigError{[]string{msg.String()}, nil}
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
			grip.Error(message.WrapError(fErr, message.Fields{
				"message": "problem finding repository",
				"project": projectRef.Identifier,
				"runner":  RunnerName,
			}))
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
			"runner":      RunnerName,
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
	u, err := user.FindByGithubUID(rev.AuthorGithubUID)
	grip.Error(message.WrapError(err, message.Fields{
		"message": fmt.Sprintf("failed to fetch everg user with Github UID %d", rev.AuthorGithubUID),
	}))

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
	if u != nil {
		v.AuthorID = u.Id
	}
	return v, nil
}

// createVersionItems populates and stores all the tasks and builds for a version according to
// the given project config.
func createVersionItems(v *version.Version, ref *model.ProjectRef, project *model.Project) error {
	// generate all task Ids so that we can easily reference them for dependencies
	taskIds := model.NewTaskIdTable(project, v)

	// create all builds for the version
	for _, buildvariant := range project.BuildVariants {
		if buildvariant.Disabled {
			continue
		}

		buildId, err := model.CreateBuildFromVersion(project, v, taskIds, buildvariant.Name, false, nil, nil)
		if err != nil {
			return errors.WithStack(err)
		}

		lastActivated, err := version.FindOne(version.ByLastVariantActivation(ref.Identifier, buildvariant.Name))
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "problem getting activatation time for variant",
				"variant": buildvariant.Name,
				"project": ref.Identifier,
			}))
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

		grip.Info(message.Fields{
			"message": "activating build",
			"name":    buildvariant.Name,
			"project": ref.Identifier,
			"version": v.Id,
			"time":    activateAt,
			"runner":  RunnerName,
		})

		v.BuildIds = append(v.BuildIds, buildId)
		v.BuildVariants = append(v.BuildVariants, version.BuildStatus{
			BuildVariant: buildvariant.Name,
			Activated:    false,
			ActivateAt:   activateAt,
			BuildId:      buildId,
		})
	}

	err := v.Insert()
	if err != nil && !db.IsDuplicateKey(err) {
		grip.Error(message.WrapError(err, message.Fields{
			"runner":  RunnerName,
			"message": "problem inserting version",
			"id":      v.Id,
		}))
		for _, buildStatus := range v.BuildVariants {
			if buildErr := model.DeleteBuild(buildStatus.BuildId); buildErr != nil {
				grip.Error(message.WrapError(buildErr, message.Fields{
					"runner":     RunnerName,
					"message":    "issue deleting build",
					"version_id": v.Id,
					"build_id":   buildStatus.BuildId,
				}))
			}
		}
		return errors.WithStack(err)
	}
	return nil
}
