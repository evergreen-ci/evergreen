package repotracker

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/google/go-github/github"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/x/network/command"
	mgobson "gopkg.in/mgo.v2/bson"
	yaml "gopkg.in/yaml.v2"
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

type VersionErrors struct {
	Errors   []string
	Warnings []string
}

// VersionMetadata is used to pass information about upstream versions to downstream version creation
type VersionMetadata struct {
	Revision            model.Revision
	TriggerID           string
	TriggerType         string
	EventID             string
	TriggerDefinitionID string
	SourceVersion       *model.Version
	IsAdHoc             bool
	User                *user.DBUser
	Message             string
	Alias               string
	PeriodicBuildID     string
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
func (repoTracker *RepoTracker) FetchRevisions(ctx context.Context) error {
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
		numRevisions := settings.RepoTracker.NumNewRepoRevisionsToFetch
		if numRevisions <= 0 {
			numRevisions = DefaultNumNewRepoRevisionsToFetch
		}
		// if this is the first time we're running the tracker for this project,
		// fetch the most recent `numNewRepoRevisionsToFetch` revisions
		grip.Debug(message.Fields{
			"runner":  RunnerName,
			"project": projectRef,
			"message": "no last recorded revision, using most recent revisions",
			"number":  numRevisions,
		})
		revisions, err = repoTracker.GetRecentRevisions(numRevisions)
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
		grip.Debug(message.Fields{
			"message":       "storing revisions",
			"project":       repoTracker.ProjectRef.Identifier,
			"new_revisions": revisions,
			"last_revision": lastRevision,
		})
		err = repoTracker.StoreRevisions(ctx, revisions)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "problem sorting revisions for repository",
				"runner":  RunnerName,
				"project": projectRef,
			}))
			return errors.WithStack(err)
		}
	}

	if err := model.DoProjectActivation(projectIdentifier); err != nil {
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

// Constructs all versions stored from recent repository revisions
// The additional complexity is due to support for project modifications on patch builds.
// We need to parse the remote config as it existed when each revision was created.
// The return value is the most recent version created as a result of storing the revisions.
// This function is idempotent with regard to storing the same version multiple times.
func (repoTracker *RepoTracker) StoreRevisions(ctx context.Context, revisions []model.Revision) error {
	var newestVersion *model.Version
	ref := repoTracker.ProjectRef
	for i := len(revisions) - 1; i >= 0; i-- {
		revision := revisions[i].Revision
		grip.Infof("Processing revision %s in project %s", revision, ref.Identifier)

		// We check if the version exists here so we can avoid fetching the github config unnecessarily
		existingVersion, err := model.VersionFindOne(model.VersionByProjectIdAndRevision(ref.Identifier, revisions[i].Revision))
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

		var versionErrs *VersionErrors
		project, err := repoTracker.GetProjectConfig(ctx, revision)
		if err != nil {
			// this is an error that implies the file is invalid - create a version and store the error
			projErr, isProjErr := err.(projectConfigError)
			if isProjErr {
				versionErrs = &VersionErrors{
					Warnings: projErr.Warnings,
					Errors:   projErr.Errors,
				}
				if len(versionErrs.Errors) > 0 {
					stubVersion, dbErr := shellVersionFromRevision(ref, VersionMetadata{Revision: revisions[i]})
					if dbErr != nil {
						grip.Error(message.WrapError(dbErr, message.Fields{
							"message":  "error creating shell version",
							"runner":   RunnerName,
							"project":  ref.Identifier,
							"revision": revision,
						}))
					}
					stubVersion.Errors = versionErrs.Errors
					stubVersion.Warnings = versionErrs.Warnings
					err = stubVersion.Insert()
					grip.Error(message.WrapError(err, message.Fields{
						"message":  "error inserting shell version",
						"runner":   RunnerName,
						"project":  ref.Identifier,
						"revision": revision,
					}))
					newestVersion = stubVersion
					continue
				}
			} else {
				grip.Error(message.WrapError(err, message.Fields{
					"message":  "error getting project config",
					"runner":   RunnerName,
					"project":  ref.Identifier,
					"revision": revision,
				}))
				return err
			}
		}

		// "Ignore" a version if all changes are to ignored files
		var ignore bool
		if len(project.Ignore) > 0 {
			var filenames []string
			filenames, err = repoTracker.GetChangedFiles(ctx, revision)
			if err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message":  "error checking GitHub for ignored files",
					"runner":   RunnerName,
					"project":  ref.Identifier,
					"revision": revision,
				}))
				continue
			}
			if project.IgnoresAllFiles(filenames) {
				ignore = true
			}
		}

		metadata := VersionMetadata{
			Revision: revisions[i],
		}
		v, err := CreateVersionFromConfig(ctx, ref, project, metadata, ignore, versionErrs)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":  "error creating version",
				"runner":   RunnerName,
				"project":  ref.Identifier,
				"revision": revision,
			}))
			continue
		}
		if err = AddBuildBreakSubscriptions(v, ref); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":  "error creating build break subscriptions",
				"runner":   RunnerName,
				"project":  ref.Identifier,
				"revision": revision,
			}))
			continue
		}
		_, err = CreateManifest(*v, project, ref.Branch, repoTracker.Settings)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":  "error creating manifest",
				"runner":   RunnerName,
				"project":  ref.Identifier,
				"revision": revision,
			}))
			continue
		}

		newestVersion = v
	}
	if newestVersion != nil {
		err := model.UpdateLastRevision(newestVersion.Identifier, newestVersion.Revision)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "problem updating last revision for repository",
				"project": ref.Identifier,
				"runner":  RunnerName,
			}))
			return errors.WithStack(err)
		}
	}
	return nil
}

// GetProjectConfig fetches the project configuration for a given repository
// returning a remote config if the project references a remote repository
// configuration file - via the Identifier. Otherwise it defaults to the local
// project file. An erroneous project file may be returned along with an error.
func (repoTracker *RepoTracker) GetProjectConfig(ctx context.Context, revision string) (*model.Project, error) {
	projectRef := repoTracker.ProjectRef
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
			return nil, projectConfigError{Errors: []string{msg.String()}, Warnings: nil}
		}
		// If we get here then we have an infrastructural error - e.g.
		// a thirdparty.APIUnmarshalError (indicating perhaps an API has
		// changed), a thirdparty.ResponseReadError(problem reading an
		// API response) or a thirdparty.APIResponseError (nil API
		// response) - or encountered a problem in fetching a local
		// configuration file. At any rate, this is bad enough that we
		// want to send a notification instead of just creating a stub
		// model.Version
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

		// this used to send email, but it happens so
		// infrequently, and mail is a bad format for this.
		grip.Critical(message.WrapError(err, message.Fields{
			"message":      "repotracker configuration problem",
			"project":      projectRef.Identifier,
			"runner":       RunnerName,
			"lastRevision": lastRevision,
		}))

		return nil, err
	}
	return project, nil
}

// AddBuildBreakSubscriptions will subscribe admins of a project to a version if no one
// else would receive a build break notification
func AddBuildBreakSubscriptions(v *model.Version, projectRef *model.ProjectRef) error {
	subscriptionBase := event.Subscription{
		ResourceType: event.ResourceTypeTask,
		Trigger:      "build-break",
		Selectors: []event.Selector{
			{
				Type: "object",
				Data: "task",
			},
			{
				Type: "project",
				Data: projectRef.Identifier,
			},
			{
				Type: "requester",
				Data: evergreen.RepotrackerVersionRequester,
			},
			{
				Type: "in-version",
				Data: v.Id,
			},
		},
	}
	subscribers := []event.Subscriber{}

	// if the commit author has subscribed to build break notifications,
	// don't send it to admins
	catcher := grip.NewSimpleCatcher()
	if v.AuthorID != "" && v.TriggerID == "" {
		author, err := user.FindOne(user.ById(v.AuthorID))
		if err != nil {
			catcher.Add(errors.Wrap(err, "unable to retrieve user"))
		} else if author.Settings.Notifications.BuildBreakID != "" {
			return nil
		}
	}

	// Only send to admins if the admins have enabled build break notifications
	if !projectRef.NotifyOnBuildFailure {
		return nil
	}
	// if the project has build break notifications, subscribe admins if no one subscribed
	for _, admin := range projectRef.Admins {
		subscriber, err := makeBuildBreakSubscriber(admin)
		if err != nil {
			catcher.Add(err)
			continue
		}
		if subscriber != nil {
			subscribers = append(subscribers, *subscriber)
		}
	}

	for _, subscriber := range subscribers {
		newSubscription := subscriptionBase
		newSubscription.Subscriber = subscriber
		catcher.Add(newSubscription.Upsert())
	}
	return catcher.Resolve()
}

func makeBuildBreakSubscriber(userID string) (*event.Subscriber, error) {
	u, err := user.FindOne(user.ById(userID))
	if err != nil {
		return nil, errors.Wrap(err, "unable to find user")
	}
	if u == nil {
		return nil, errors.Errorf("user %s does not exist", userID)
	}
	var subscriber *event.Subscriber
	preference := u.Settings.Notifications.BuildBreak
	if preference != "" {
		subscriber = &event.Subscriber{
			Type: string(preference),
		}
		if preference == user.PreferenceEmail {
			subscriber.Target = u.Email()
		} else if preference == user.PreferenceSlack {
			subscriber.Target = u.Settings.SlackUsername
		} else {
			return nil, errors.Errorf("invalid subscription preference for build break: %s", preference)
		}
	}

	return subscriber, nil
}

func CreateManifest(v model.Version, proj *model.Project, branch string, settings *evergreen.Settings) (*manifest.Manifest, error) {
	if len(proj.Modules) == 0 {
		return nil, nil
	}
	newManifest := &manifest.Manifest{
		Id:          v.Id,
		Revision:    v.Revision,
		ProjectName: v.Identifier,
		Branch:      branch,
	}
	token, err := settings.GetGithubOauthToken()
	if err != nil {
		return nil, errors.Wrap(err, "error getting github oauth token")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var gitBranch *github.Branch
	var gitCommit *github.RepositoryCommit
	modules := map[string]*manifest.Module{}
	for _, module := range proj.Modules {
		var sha, url string
		owner, repo := module.GetRepoOwnerAndName()
		if module.Ref == "" {
			gitBranch, err = thirdparty.GetBranchEvent(ctx, token, owner, repo, module.Branch)
			if err != nil {
				return nil, errors.Wrapf(err, "problem retrieving getting git branch for module %s", module.Name)
			}
			if gitBranch != nil && gitBranch.Commit != nil {
				sha = *gitBranch.Commit.SHA
				url = *gitBranch.Commit.URL
			}
		} else {
			sha = module.Ref
			gitCommit, err = thirdparty.GetCommitEvent(ctx, token, owner, repo, module.Ref)
			if err != nil {
				return nil, errors.Wrapf(err, "problem retrieving getting git commit for module %s with hash %s", module.Name, module.Ref)
			}
			if gitCommit != nil {
				url = *gitCommit.URL
			}
		}

		modules[module.Name] = &manifest.Module{
			Branch:   module.Branch,
			Revision: sha,
			Repo:     repo,
			Owner:    owner,
			URL:      url,
		}

	}
	newManifest.Modules = modules
	grip.Debug(message.Fields{
		"message":     "creating manifest",
		"version_id":  v.Id,
		"manifest":    newManifest,
		"project":     v.Identifier,
		"modules":     modules,
		"branch_info": gitBranch,
	})
	_, err = newManifest.TryInsert()

	return newManifest, errors.Wrap(err, "error inserting manifest")
}

func CreateVersionFromConfig(ctx context.Context, ref *model.ProjectRef, config *model.Project,
	metadata VersionMetadata, ignore bool, versionErrs *VersionErrors) (*model.Version, error) {
	if ref == nil || config == nil {
		return nil, errors.New("project ref and project cannot be nil")
	}

	// create a version document
	v, err := shellVersionFromRevision(ref, metadata)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create shell version")
	}
	if err = sanityCheckOrderNum(v.RevisionOrderNumber, ref.Identifier, metadata.Revision.Revision); err != nil {
		return nil, errors.Wrap(err, "inconsistent version order")
	}
	configYaml, err := yaml.Marshal(config)
	if err != nil {
		return nil, errors.Wrap(err, "error marshaling config")
	}
	v.Config = string(configYaml)
	v.Ignored = ignore

	// validate the project
	verrs := validator.CheckProjectSyntax(config)
	if len(verrs) > 0 || versionErrs != nil {
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
		v.Warnings = projectWarnings
		v.Errors = projectErrors
		if versionErrs != nil && versionErrs.Warnings != nil {
			v.Warnings = append(v.Warnings, versionErrs.Warnings...)
		}
		if versionErrs != nil && versionErrs.Errors != nil {
			v.Errors = append(v.Errors, versionErrs.Errors...)
		}
		if len(v.Errors) > 0 {
			return v, errors.Wrap(v.Insert(), "error inserting version")
		}
	}
	var aliases model.ProjectAliases
	if metadata.Alias != "" {
		aliases, err = model.FindAliasInProject(ref.Identifier, metadata.Alias)
		if err != nil {
			return v, errors.Wrap(err, "error finding project alias")
		}
	}

	return v, errors.Wrap(createVersionItems(ctx, v, ref, metadata, config, aliases), "error creating version items")
}

// shellVersionFromRevision populates a new Version with metadata from a model.Revision.
// Does not populate its config or store anything in the database.
func shellVersionFromRevision(ref *model.ProjectRef, metadata VersionMetadata) (*model.Version, error) {
	u, err := user.FindByGithubUID(metadata.Revision.AuthorGithubUID)
	grip.Error(message.WrapError(err, message.Fields{
		"message": fmt.Sprintf("failed to fetch everg user with Github UID %d", metadata.Revision.AuthorGithubUID),
	}))

	number, err := model.GetNewRevisionOrderNumber(ref.Identifier)
	if err != nil {
		return nil, err
	}
	v := &model.Version{
		Author:              metadata.Revision.Author,
		AuthorEmail:         metadata.Revision.AuthorEmail,
		Branch:              ref.Branch,
		CreateTime:          metadata.Revision.CreateTime,
		Identifier:          ref.Identifier,
		Message:             metadata.Revision.RevisionMessage,
		Owner:               ref.Owner,
		RemotePath:          ref.RemotePath,
		Repo:                ref.Repo,
		RepoKind:            ref.RepoKind,
		Requester:           evergreen.RepotrackerVersionRequester,
		Revision:            metadata.Revision.Revision,
		Status:              evergreen.VersionCreated,
		RevisionOrderNumber: number,
		TriggerID:           metadata.TriggerID,
		TriggerType:         metadata.TriggerType,
		TriggerEvent:        metadata.EventID,
		PeriodicBuildID:     metadata.PeriodicBuildID,
	}
	if metadata.TriggerType != "" {
		v.Id = util.CleanName(fmt.Sprintf("%s_%s_%s", ref.String(), metadata.SourceVersion.Revision, metadata.TriggerDefinitionID))
		v.Requester = evergreen.TriggerRequester
		v.CreateTime = metadata.SourceVersion.CreateTime
	} else if metadata.IsAdHoc {
		v.Id = mgobson.NewObjectId().Hex()
		v.Requester = evergreen.AdHocRequester
		v.CreateTime = time.Now()
		v.Message = metadata.Message
		if metadata.User != nil {
			num, err := metadata.User.IncPatchNumber()
			if err != nil {
				return nil, errors.Wrap(err, "error incrementing patch number")
			}
			v.RevisionOrderNumber = num
		}
	} else {
		v.Id = makeVersionId(ref.String(), metadata.Revision.Revision)
	}
	if u != nil {
		v.AuthorID = u.Id
	}
	return v, nil
}

func makeVersionId(project, revision string) string {
	return util.CleanName(fmt.Sprintf("%s_%s", project, revision))
}

// Verifies that the given revision order number is higher than the latest number stored for the project.
func sanityCheckOrderNum(revOrderNum int, projectId, revision string) error {
	latest, err := model.VersionFindOne(model.VersionByMostRecentSystemRequester(projectId))
	if err != nil || latest == nil {
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

// createVersionItems populates and stores all the tasks and builds for a version according to
// the given project config.
func createVersionItems(ctx context.Context, v *model.Version, ref *model.ProjectRef, metadata VersionMetadata, project *model.Project, aliases model.ProjectAliases) error {
	client := evergreen.GetEnvironment().Client()
	const retryCount = 5

	distroAliases, err := distro.NewDistroAliasesLookupTable()
	if err != nil {
		return errors.WithStack(err)
	}

	txFunc := func(sessCtx mongo.SessionContext) (bool, error) {
		// generate all task Ids so that we can easily reference them for dependencies
		sourceRev := ""
		if metadata.SourceVersion != nil {
			sourceRev = metadata.SourceVersion.Revision
		}
		taskIds := model.NewTaskIdTable(project, v, sourceRev, metadata.TriggerDefinitionID)

		err := sessCtx.StartTransaction()
		if err != nil {
			return false, errors.Wrap(err, "error starting transaction")
		}
		// create all builds for the version
		for _, buildvariant := range project.BuildVariants {
			if buildvariant.Disabled {
				continue
			}
			var match bool
			if len(aliases) > 0 {
				match, err = aliases.HasMatchingVariant(buildvariant.Name, buildvariant.Tags)
				if err != nil {
					grip.Error(err)
					continue
				}
				if !match {
					continue
				}
			}
			args := model.BuildCreateArgs{
				Project:       *project,
				Version:       *v,
				TaskIDs:       taskIds,
				BuildName:     buildvariant.Name,
				Activated:     false,
				SourceRev:     sourceRev,
				DefinitionID:  metadata.TriggerDefinitionID,
				Aliases:       aliases,
				Session:       sessCtx,
				DistroAliases: distroAliases,
			}
			var buildId string
			buildId, err = model.CreateBuildFromVersion(args)
			if err != nil {
				abortErr := sessCtx.AbortTransaction(sessCtx)
				grip.Notice(message.Fields{
					"message":    "aborting transaction",
					"cause":      "can't insert build items",
					"variant":    buildvariant.Name,
					"version":    v.Id,
					"insert_err": err.Error(),
					"abort_err":  abortErr.Error(),
				})
				if isTransientTxErr(err, v) {
					return true, nil
				}
				return false, errors.Wrapf(err, "error inserting build %s", buildId)
			}

			var lastActivated *model.Version
			activateAt := time.Now()
			if metadata.TriggerID == "" && v.Requester != evergreen.AdHocRequester {
				lastActivated, err = model.VersionFindOne(model.VersionByLastVariantActivation(ref.Identifier, buildvariant.Name))
				if err != nil {
					grip.Error(message.WrapError(err, message.Fields{
						"message": "error finding last activated",
						"version": v.Id,
					}))
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

				if lastActivation != nil {
					activateAt = lastActivation.Add(time.Minute * time.Duration(ref.GetBatchTime(&buildvariant)))
				}
			}

			grip.Debug(message.Fields{
				"message": "activating build",
				"name":    buildvariant.Name,
				"project": ref.Identifier,
				"version": v.Id,
				"time":    activateAt,
				"runner":  RunnerName,
			})
			v.BuildIds = append(v.BuildIds, buildId)
			v.BuildVariants = append(v.BuildVariants, model.VersionBuildStatus{
				BuildVariant: buildvariant.Name,
				Activated:    false,
				ActivateAt:   activateAt,
				BuildId:      buildId,
			})
		}

		_, err = evergreen.GetEnvironment().DB().Collection(model.VersionCollection).InsertOne(sessCtx, v)
		if err != nil {
			abortErr := sessCtx.AbortTransaction(sessCtx)
			grip.Notice(message.Fields{
				"message":    "aborting transaction",
				"cause":      "can't insert version",
				"version":    v.Id,
				"insert_err": err.Error(),
				"abort_err":  abortErr.Error(),
			})
			if isTransientTxErr(err, v) {
				return true, nil
			}
			return false, errors.Wrapf(err, "error inserting version %s", v.Id)
		}
		err = sessCtx.CommitTransaction(sessCtx)
		if err != nil {
			if isTransientTxErr(err, v) {
				return true, nil
			}
			grip.Error(message.WrapError(err, message.Fields{
				"message": "unable to commit transaction",
				"version": v.Id,
			}))
			return false, errors.Wrapf(err, "error committing transaction for version %s", v.Id)
		}
		grip.Info(message.Fields{
			"message": "successfully created version",
			"version": v.Id,
			"hash":    v.Revision,
			"project": v.Branch,
			"runner":  RunnerName,
		})
		return false, nil
	}

	return client.UseSession(ctx, func(sessCtx mongo.SessionContext) error {
		for i := 0; i < retryCount; i++ {
			shouldRetry, err := txFunc(sessCtx)
			if err != nil {
				return err
			}
			if !shouldRetry {
				break
			}
			if i >= retryCount-1 {
				return errors.Errorf("hit max retries for version %s", v.Id)
			}
		}
		return nil
	})
}

func isTransientTxErr(err error, version *model.Version) bool {
	rootErr := errors.Cause(err)
	cmdErr, isCmdErr := rootErr.(mongo.CommandError)
	if isCmdErr && cmdErr.HasErrorLabel(command.TransientTransactionError) {
		grip.Notice(message.WrapError(err, message.Fields{
			"message": "hit transient transaction error, will retry",
			"version": version.Id,
		}))
		return true
	}
	return false
}
