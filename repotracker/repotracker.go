package repotracker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/evergreen-ci/utility"
	"github.com/jpillora/backoff"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	// determines the default maximum number of revisions to fetch for a newly tracked repo
	// if not specified in configuration file
	DefaultNumNewRepoRevisionsToFetch = 200
	DefaultMaxRepoRevisionsToSearch   = 50
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

// The RepoPoller interface specifies behavior required of all repository poller
// implementations
type RepoPoller interface {
	// Fetches the contents of a remote repository's configuration data as at
	// the given revision.
	GetRemoteConfig(ctx context.Context, revision string) (model.ProjectInfo, error)

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

	if !projectRef.Enabled || projectRef.IsRepotrackerDisabled() {
		// this is somewhat belt-and-suspenders, as the
		// repotracker runner process doesn't run for disabled
		// projects.
		grip.Info(message.Fields{
			"message":            "skip disabled project",
			"project":            projectRef.Id,
			"project_identifier": projectRef.Identifier,
			"runner":             RunnerName,
		})
		return nil
	}

	repository, err := model.FindRepository(projectRef.Id)
	if err != nil {
		return errors.Wrapf(err, "finding repository '%s'", projectRef.Identifier)
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
			"runner":             RunnerName,
			"project":            projectRef.Id,
			"project_identifier": projectRef.Identifier,
			"message":            "no last recorded revision or ignoring last recorded revision, using most recent revisions",
			"number":             numRevisions,
		})
		revisions, err = repoTracker.GetRecentRevisions(numRevisions)
	} else {
		grip.Debug(message.Fields{
			"message":            "found last recorded revision",
			"project":            projectRef.Id,
			"project_identifier": projectRef.Identifier,
			"runner":             RunnerName,
			"revision":           lastRevision,
		})
		// if the projectRef has a repotracker error then don't get the revisions
		if projectRef.RepotrackerError != nil {
			if projectRef.RepotrackerError.Exists {
				grip.Warning(message.Fields{
					"runner":             RunnerName,
					"message":            "repotracker error for base revision",
					"project":            projectRef.Id,
					"project_identifier": projectRef.Identifier,
					"path":               fmt.Sprintf("%s/%s:%s", projectRef.Owner, projectRef.Repo, projectRef.Branch),
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
			"message":            "problem fetching revisions for repository",
			"runner":             RunnerName,
			"project":            projectRef.Id,
			"project_identifier": projectRef.Identifier,
		}))
		return nil
	}

	if len(revisions) > 0 {
		grip.Debug(message.Fields{
			"message":            "storing revisions",
			"project":            repoTracker.ProjectRef.Id,
			"project_identifier": repoTracker.ProjectRef.Identifier,
			"new_revisions":      revisions,
			"last_revision":      lastRevision,
		})
		err = repoTracker.StoreRevisions(ctx, revisions)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":            "problem storing revisions for repository",
				"runner":             RunnerName,
				"project":            projectRef.Id,
				"project_identifier": projectRef.Identifier,
			}))
			return errors.WithStack(err)
		}
	}
	ok, err := model.DoProjectActivation(ctx, projectRef.Id, time.Now())
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":            "problem activating recent commit for project",
			"project":            projectRef.Id,
			"project_identifier": projectRef.Identifier,
			"runner":             RunnerName,
			"mode":               "ingestion",
		}))
		return errors.WithStack(err)
	}
	if ok {
		grip.Debug(message.Fields{
			"message":            "activated recent commit for project",
			"project":            projectRef.Id,
			"project_identifier": projectRef.Identifier,
			"runner":             RunnerName,
		})
	}

	return nil
}

// StoreRevisions constructs all versions stored from recent repository revisions. The revisions should be given in
// order of most recent to least recent commit.
// The additional complexity is due to support for project modifications on patch builds.
// We need to parse the remote config as it existed when each revision was created.
// The return value is the most recent version created as a result of storing the revisions.
// This function is idempotent with regard to storing the same version multiple times.
func (repoTracker *RepoTracker) StoreRevisions(ctx context.Context, revisions []model.Revision) error {
	var newestVersion *model.Version
	ref := repoTracker.ProjectRef

	// Since the revisions are ordered most to least recent, iterate backwards so that they're processed in order of
	// least to most recent.
	for i := len(revisions) - 1; i >= 0; i-- {
		revision := revisions[i].Revision
		grip.Infof("Processing revision %s in project %s", revision, ref.Id)

		// We check if the version exists here so we can avoid fetching the github config unnecessarily
		existingVersion, err := model.VersionFindOne(model.BaseVersionByProjectIdAndRevision(ref.Id, revisions[i].Revision))
		grip.Error(message.WrapError(err, message.Fields{
			"message":            "problem looking up version for project",
			"runner":             RunnerName,
			"project":            ref.Id,
			"project_identifier": ref.Identifier,
			"revision":           revision,
		}))

		if existingVersion != nil {
			grip.Info(message.Fields{
				"message":            "skipping creating version because it already exists",
				"runner":             RunnerName,
				"project":            ref.Id,
				"project_identifier": ref.Identifier,
				"revision":           revision,
			})
			// We bind newestVersion here since we still need to return the most recent
			// version, even if it already exists
			newestVersion = existingVersion
			continue
		}

		var versionErrs *VersionErrors
		pInfo, err := repoTracker.GetProjectConfig(ctx, revision)
		if err != nil {
			// this is an error that implies the file is invalid - create a version and store the error
			projErr, isProjErr := err.(projectConfigError)
			if isProjErr {
				versionErrs = &VersionErrors{
					Warnings: projErr.Warnings,
					Errors:   projErr.Errors,
				}
				if len(versionErrs.Errors) > 0 {
					stubVersion, dbErr := ShellVersionFromRevision(ref, model.VersionMetadata{Revision: revisions[i]})
					if dbErr != nil {
						grip.Error(message.WrapError(dbErr, message.Fields{
							"message":            "error creating shell version",
							"runner":             RunnerName,
							"project":            ref.Id,
							"project_identifier": ref.Identifier,
							"revision":           revision,
						}))
					}
					stubVersion.Errors = versionErrs.Errors
					stubVersion.Warnings = versionErrs.Warnings
					err = stubVersion.Insert()
					grip.Error(message.WrapError(err, message.Fields{
						"message":            "error inserting shell version",
						"runner":             RunnerName,
						"project":            ref.Id,
						"project_identifier": ref.Identifier,
						"revision":           revision,
					}))
					newestVersion = stubVersion
					continue
				}
			} else {
				grip.Error(message.WrapError(err, message.Fields{
					"message":            "error getting project config",
					"runner":             RunnerName,
					"project":            ref.Id,
					"project_identifier": ref.Identifier,
					"revision":           revision,
				}))
				return err
			}
		} else if pInfo.Project == nil {
			grip.Error(message.Fields{
				"message":            fmt.Sprintf("unable to find project config for revision %s", revision),
				"runner":             RunnerName,
				"project":            ref.Id,
				"project_identifier": ref.Identifier,
			})
			return err
		}

		// "Ignore" a version if all changes are to ignored files
		var ignore bool
		if len(pInfo.Project.Ignore) > 0 {
			var filenames []string
			filenames, err = repoTracker.GetChangedFiles(ctx, revision)
			if err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message":            "error checking GitHub for ignored files",
					"runner":             RunnerName,
					"project":            ref.Id,
					"project_identifier": ref.Identifier,
					"revision":           revision,
				}))
				continue
			}
			if pInfo.Project.IgnoresAllFiles(filenames) {
				ignore = true
			}
		}

		metadata := model.VersionMetadata{
			Revision: revisions[i],
		}
		projectInfo := &model.ProjectInfo{
			Ref:                 ref,
			Project:             pInfo.Project,
			IntermediateProject: pInfo.IntermediateProject,
			Config:              pInfo.Config,
		}
		v, err := CreateVersionFromConfig(ctx, projectInfo, metadata, ignore, versionErrs)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":            "error creating version",
				"runner":             RunnerName,
				"project":            ref.Id,
				"project_identifier": ref.Identifier,
				"revision":           revision,
			}))
			continue
		}
		if err = AddBuildBreakSubscriptions(v, ref); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":            "error creating build break subscriptions",
				"runner":             RunnerName,
				"project":            ref.Id,
				"project_identifier": ref.Identifier,
				"revision":           revision,
			}))
			continue
		}
		if ref.IsGithubChecksEnabled() {
			if err = addGithubCheckSubscriptions(ctx, v); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message":            "error adding github check subscriptions",
					"runner":             RunnerName,
					"project":            ref.Id,
					"project_identifier": ref.Identifier,
					"revision":           revision,
				}))
			}
		}

		_, err = model.CreateManifest(v, pInfo.Project.Modules, ref, repoTracker.Settings)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":            "error creating manifest",
				"runner":             RunnerName,
				"project":            ref.Id,
				"project_identifier": ref.Identifier,
				"revision":           revision,
			}))
			continue
		}

		newestVersion = v
	}
	if newestVersion != nil {
		err := model.UpdateLastRevision(newestVersion.Identifier, newestVersion.Revision)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":            "problem updating last revision for repository",
				"project":            ref.Id,
				"project_identifier": ref.Identifier,
				"runner":             RunnerName,
			}))
			return errors.WithStack(err)
		}
	}
	return nil
}

// GetProjectConfig fetches the project configuration for a given repository
// returning a remote config if the project references a remote repository
// configuration file - via the Id. Otherwise it defaults to the local
// project file. An erroneous project file may be returned along with an error.
func (repoTracker *RepoTracker) GetProjectConfig(ctx context.Context, revision string) (model.ProjectInfo, error) {
	projectRef := repoTracker.ProjectRef
	projectInfo, err := repoTracker.GetRemoteConfig(ctx, revision)
	if err != nil {
		// Only create a stub version on API request errors that pertain
		// to actually fetching a config. Those errors currently include:
		// thirdparty.APIRequestError, thirdparty.FileNotFoundError and
		// thirdparty.YAMLFormatError
		_, apiReqErr := errors.Cause(err).(thirdparty.APIRequestError)
		_, ymlFmtErr := errors.Cause(err).(thirdparty.YAMLFormatError)
		_, noFileErr := errors.Cause(err).(thirdparty.FileNotFoundError)
		parsingErr := strings.Contains(err.Error(), model.TranslateProjectError)
		configErr := strings.Contains(err.Error(), model.TranslateProjectConfigError) || strings.Contains(err.Error(), model.MergeProjectConfigError)
		if apiReqErr || noFileErr || ymlFmtErr || parsingErr || configErr {
			// If there's an error getting the remote config, e.g. because it
			// does not exist, we treat this the same as when the remote config
			// is invalid - but add a different error message
			msg := message.ConvertToComposer(level.Error, message.Fields{
				"message":            fmt.Sprintf("problem with project configuration: %s", errors.Cause(err)),
				"runner":             RunnerName,
				"project":            projectRef.Id,
				"project_identifier": projectRef.Identifier,
				"revision":           revision,
				"path":               projectRef.RemotePath,
			})

			grip.Error(message.WrapError(err, msg))
			return model.ProjectInfo{}, projectConfigError{Errors: []string{msg.String()}, Warnings: nil}
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
		repository, fErr := model.FindRepository(projectRef.Id)
		if fErr != nil || repository == nil {
			grip.Error(message.WrapError(fErr, message.Fields{
				"message":            "problem finding repository",
				"project":            projectRef.Id,
				"project_identifier": projectRef.Identifier,
				"runner":             RunnerName,
			}))
		} else {
			lastRevision = repository.LastRevision
		}

		grip.Error(message.WrapError(err, message.Fields{
			"message":            "repotracker configuration problem",
			"project":            projectRef.Id,
			"project_identifier": projectRef.Identifier,
			"runner":             RunnerName,
			"lastRevision":       lastRevision,
		}))

		return model.ProjectInfo{}, err
	}
	return projectInfo, nil
}

// addGithubCheckSubscriptions adds subscriptions to send the status of the version to Github.
func addGithubCheckSubscriptions(ctx context.Context, v *model.Version) error {
	catcher := grip.NewBasicCatcher()
	ghSub := event.NewGithubCheckAPISubscriber(event.GithubCheckSubscriber{
		Owner: v.Owner,
		Repo:  v.Repo,
		Ref:   v.Revision,
	})

	versionSub := event.NewVersionGithubCheckOutcomeSubscription(v.Id, ghSub)
	if err := versionSub.Upsert(); err != nil {
		catcher.Wrap(err, "inserting version GitHub check subscription")
	}
	buildSub := event.NewGithubCheckBuildOutcomeSubscriptionByVersion(v.Id, ghSub)
	if err := buildSub.Upsert(); err != nil {
		catcher.Wrap(err, "inserting build GitHub check subscription")
	}
	input := thirdparty.SendGithubStatusInput{
		VersionId: v.Id,
		Owner:     v.Owner,
		Repo:      v.Repo,
		Ref:       v.Revision,
		Desc:      "version created",
		Caller:    RunnerName,
		Context:   thirdparty.GithubStatusDefaultContext,
	}
	err := thirdparty.SendPendingStatusToGithub(ctx, input, "")
	if err != nil {
		catcher.Wrap(err, "failed to send version status to GitHub")
	}
	return catcher.Resolve()
}

// AddBuildBreakSubscriptions will subscribe admins of a project to a version if no one
// else would receive a build break notification
func AddBuildBreakSubscriptions(v *model.Version, projectRef *model.ProjectRef) error {
	subscriptionBase := event.Subscription{
		ResourceType: event.ResourceTypeTask,
		Trigger:      "build-break",
		LastUpdated:  time.Now(),
		Selectors: []event.Selector{
			{
				Type: event.SelectorObject,
				Data: event.ObjectTask,
			},
			{
				Type: event.SelectorProject,
				Data: projectRef.Id,
			},
			{
				Type: event.SelectorRequester,
				Data: evergreen.RepotrackerVersionRequester,
			},
			{
				Type: event.SelectorInVersion,
				Data: v.Id,
			},
		},
		Filter: event.Filter{
			Object:    event.ObjectTask,
			Project:   projectRef.Id,
			Requester: evergreen.RepotrackerVersionRequester,
			InVersion: v.Id,
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
	if !projectRef.ShouldNotifyOnBuildFailure() {
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
		return nil, errors.Errorf("user '%s' does not exist", userID)
	}
	var subscriber *event.Subscriber
	preference := u.Settings.Notifications.BuildBreak
	if preference != "" && preference != user.PreferenceNone {
		subscriber = &event.Subscriber{
			Type: string(preference),
		}
		if preference == user.PreferenceEmail {
			subscriber.Target = u.Email()
		} else if preference == user.PreferenceSlack {
			slackTarget := fmt.Sprintf("@%s", strings.TrimPrefix(u.Settings.SlackUsername, "@"))
			if u.Settings.SlackMemberId != "" {
				slackTarget = u.Settings.SlackMemberId
			}
			subscriber.Target = slackTarget
		} else {
			return nil, errors.Errorf("invalid subscription preference for build break: %s", preference)
		}
	}

	return subscriber, nil
}

// CreateVersionFromConfig will create a version document from a project config
// and insert it into the database along with its associated tasks and builds.
func CreateVersionFromConfig(ctx context.Context, projectInfo *model.ProjectInfo,
	metadata model.VersionMetadata, ignore bool, versionErrs *VersionErrors) (*model.Version, error) {
	if projectInfo.NotPopulated() {
		return nil, errors.New("project ref and parser project cannot be nil")
	}

	// create a version document
	v, err := ShellVersionFromRevision(projectInfo.Ref, metadata)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create shell version")
	}
	if err = verifyOrderNum(v.RevisionOrderNumber, projectInfo.Ref.Id); err != nil {
		return nil, errors.Wrap(err, "inconsistent version order")
	}

	if projectInfo.Project == nil {
		projectInfo.Project, err = model.TranslateProject(projectInfo.IntermediateProject)
		if err != nil {
			return nil, errors.Wrap(err, "translating intermediate project")
		}
	}
	projectInfo.IntermediateProject.Init(v.Id, v.CreateTime)
	if projectInfo.Config != nil {
		projectInfo.Config.Id = v.Id
	}
	v.Ignored = ignore

	verrs := validator.CheckProject(ctx, projectInfo.Project, projectInfo.Config, projectInfo.Ref, true, projectInfo.Ref.Id, nil)
	if len(verrs) > 0 || versionErrs != nil {
		// We have errors in the project.
		// Format them, as we need to store + display them to the user
		var projectErrors, projectWarnings []string
		for _, e := range verrs {
			if e.Level == validator.Error {
				projectErrors = append(projectErrors, e.Error())
			} else if e.Level == validator.Warning {
				projectWarnings = append(projectWarnings, e.Error())
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
			ppStorageMethod, err := model.ParserProjectUpsertOneWithS3Fallback(ctx, evergreen.GetEnvironment().Settings(), evergreen.ProjectStorageMethodDB, projectInfo.IntermediateProject)
			if err != nil {
				return nil, errors.Wrapf(err, "upserting parser project '%s' for version '%s'", projectInfo.IntermediateProject.Id, v.Id)
			}
			v.ProjectStorageMethod = ppStorageMethod
			if err = v.Insert(); err != nil {
				return nil, errors.Wrap(err, "inserting version")
			}
			return v, nil

		}
	}

	var aliases model.ProjectAliases
	if metadata.Alias == evergreen.GitTagAlias {
		aliases, err = model.FindMatchingGitTagAliasesInProject(projectInfo.Ref.Id, metadata.GitTag.Tag)
		if err != nil {
			return v, errors.Wrapf(err, "error finding project alias for tag '%s'", metadata.GitTag.Tag)
		}
		grip.Debug(message.Fields{
			"message":            "aliases for creating version",
			"tag":                metadata.GitTag.Tag,
			"project":            projectInfo.Ref.Id,
			"project_identifier": projectInfo.Ref.Identifier,
			"aliases":            aliases,
		})
	} else if metadata.Alias != "" {
		aliases, err = model.FindAliasInProjectRepoOrConfig(projectInfo.Ref.Id, metadata.Alias)
		if err != nil {
			return v, errors.Wrap(err, "error finding project alias")
		}
	}

	return v, errors.Wrap(createVersionItems(ctx, v, metadata, projectInfo, aliases), "error creating version items")
}

// ShellVersionFromRevision populates a new Version with metadata from a model.Revision.
// Does not populate its config or store anything in the database.
func ShellVersionFromRevision(ref *model.ProjectRef, metadata model.VersionMetadata) (*model.Version, error) {
	var usr *user.DBUser
	var err error
	// Default to the pusher of the git tag, if relevant.
	if metadata.GitTag.Pusher != "" {
		usr, err = user.FindByGithubName(metadata.GitTag.Pusher)
		grip.Error(message.WrapError(err, message.Fields{
			"message":        "failed to fetch Evergreen user with GitHub info",
			"method":         "git_tag",
			"git_tag_pusher": metadata.GitTag.Pusher,
		}))
	}
	if usr == nil && metadata.Revision.AuthorGithubUID != 0 {
		usr, err = user.FindByGithubUID(metadata.Revision.AuthorGithubUID)
		grip.Error(message.WrapError(err, message.Fields{
			"message":             "failed to fetch Evergreen user with GitHub info",
			"method":              "user_uid",
			"revision_author_uid": metadata.Revision.AuthorGithubUID,
		}))
	}
	if usr != nil {
		metadata.User = usr
	}

	number, err := model.GetNewRevisionOrderNumber(ref.Id)
	if err != nil {
		return nil, err
	}
	v := &model.Version{
		Author:               metadata.Revision.Author,
		AuthorID:             metadata.Revision.AuthorID,
		AuthorEmail:          metadata.Revision.AuthorEmail,
		Branch:               ref.Branch,
		CreateTime:           metadata.Revision.CreateTime,
		Identifier:           ref.Id,
		Message:              metadata.Revision.RevisionMessage,
		Owner:                ref.Owner,
		RemotePath:           ref.RemotePath,
		Repo:                 ref.Repo,
		Requester:            evergreen.RepotrackerVersionRequester,
		Revision:             metadata.Revision.Revision,
		Status:               evergreen.VersionCreated,
		RevisionOrderNumber:  number,
		TriggerID:            metadata.TriggerID,
		TriggerType:          metadata.TriggerType,
		TriggerEvent:         metadata.EventID,
		PeriodicBuildID:      metadata.PeriodicBuildID,
		ProjectStorageMethod: evergreen.ProjectStorageMethodDB,
		Activated:            utility.ToBoolPtr(metadata.Activate),
	}
	if metadata.TriggerType != "" {
		var revision string
		if metadata.TriggerType == model.ProjectTriggerLevelPush {
			revision = metadata.SourceCommit
			v.TriggerSHA = revision
		}
		createTime := metadata.Revision.CreateTime
		if metadata.SourceVersion != nil {
			revision = metadata.SourceVersion.Revision
			createTime = metadata.SourceVersion.CreateTime
		}
		v.Id = util.CleanName(fmt.Sprintf("%s_%s_%s", ref.Identifier, revision, metadata.TriggerDefinitionID))
		v.Requester = evergreen.TriggerRequester
		v.CreateTime = createTime
	} else if metadata.IsAdHoc {
		v.Id = mgobson.NewObjectId().Hex()
		if metadata.PeriodicBuildID != "" {
			v.Requester = evergreen.AdHocRequester
		}
		v.CreateTime = time.Now()
		if metadata.Message != "" {
			v.Message = metadata.Message
		}
	} else if metadata.GitTag.Tag != "" {
		if !ref.IsGitTagVersionsEnabled() {
			return nil, errors.Errorf("git tag versions are not enabled for project '%s'", ref.Id)
		}
		v.Id = makeVersionIdWithTag(ref.Identifier, metadata.GitTag.Tag, mgobson.NewObjectId().Hex())
		v.Requester = evergreen.GitTagRequester
		v.CreateTime = time.Now()
		v.TriggeredByGitTag = metadata.GitTag
		v.Message = fmt.Sprintf("Triggered From Git Tag '%s': %s", metadata.GitTag.Tag, v.Message)
		if metadata.RemotePath != "" {
			v.RemotePath = metadata.RemotePath
		}
	} else {
		v.Id = makeVersionId(ref.Identifier, metadata.Revision.Revision)
	}
	if metadata.User != nil {
		v.AuthorID = metadata.User.Id
		v.Author = metadata.User.DisplayName()
		v.AuthorEmail = metadata.User.Email()
	}
	return v, nil
}

func makeVersionId(project, revision string) string {
	return util.CleanName(fmt.Sprintf("%s_%s", project, revision))
}

func makeVersionIdWithTag(project, tag, id string) string {
	return util.CleanName(fmt.Sprintf("%s_%s_%s", project, tag, id))
}

// Verifies that the given revision order number is higher than the latest number stored for the project.
func verifyOrderNum(revOrderNum int, projectId string) error {
	latest, err := model.VersionFindOne(model.VersionByMostRecentSystemRequester(projectId))
	if err != nil || latest == nil {
		return errors.Wrap(err, "getting latest version")
	}

	// When there are no versions in the db yet, verification is moot.
	if revOrderNum <= latest.RevisionOrderNumber {
		return errors.Errorf("commit order number isn't greater than last stored version's: %d <= %d",
			revOrderNum, latest.RevisionOrderNumber)
	}
	return nil
}

// createVersionItems populates and stores all the tasks and builds for a version according to
// the given project config.
func createVersionItems(ctx context.Context, v *model.Version, metadata model.VersionMetadata, projectInfo *model.ProjectInfo, aliases model.ProjectAliases) error {
	distroAliases, err := distro.NewDistroAliasesLookupTable(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	// generate all task Ids so that we can easily reference them for dependencies
	sourceRev := ""
	if metadata.SourceVersion != nil {
		sourceRev = metadata.SourceVersion.Revision
	}

	// create all builds for the version
	buildsToCreate := []interface{}{}
	tasksToCreate := task.Tasks{}
	pairsToCreate := model.TVPairSet{}
	// build all pairsToCreate before creating builds, to handle dependencies (if applicable)
	for _, buildvariant := range projectInfo.Project.BuildVariants {
		if ctx.Err() != nil {
			return errors.Wrapf(ctx.Err(), "aborting version creation for version '%s'", v.Id)
		}
		if len(aliases) > 0 {
			var aliasesMatchingVariant model.ProjectAliases
			aliasesMatchingVariant, err = aliases.AliasesMatchingVariant(buildvariant.Name, buildvariant.Tags)
			if err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message": "error checking project aliases",
					"project": projectInfo.Project.Identifier,
					"version": v.Id,
				}))
				continue
			}
			if len(aliasesMatchingVariant) == 0 {
				continue
			}
			for _, t := range buildvariant.Tasks {
				var match bool
				name, tags, ok := projectInfo.Project.GetTaskNameAndTags(t)
				if !ok {
					grip.Debug(message.Fields{
						"message": "task doesn't exist in project",
						"project": projectInfo.Project.Identifier,
						"task":    t,
						"version": v.Id,
					})
				}
				match, err = aliasesMatchingVariant.HasMatchingTask(name, tags)
				if err != nil {
					grip.Error(message.WrapError(err, message.Fields{
						"message":                  "error finding tasks with alias filter",
						"task":                     t.Name,
						"project":                  projectInfo.Project.Identifier,
						"aliases_matching_variant": aliasesMatchingVariant,
					}))
					continue
				}
				if match {
					pairsToCreate = append(pairsToCreate, model.TVPair{Variant: buildvariant.Name, TaskName: t.Name})
				}
			}
		}
	}

	pairsToCreate, err = model.IncludeDependencies(projectInfo.Project, pairsToCreate, v.Requester, nil)
	grip.Warning(message.WrapError(err, message.Fields{
		"message": "error including dependencies",
		"project": projectInfo.Project.Identifier,
		"version": v.Id,
	}))
	batchTimeCatcher := grip.NewBasicCatcher()
	debuggingData := map[string]string{}

	var githubCheckAliases model.ProjectAliases
	if v.Requester == evergreen.RepotrackerVersionRequester && projectInfo.Ref.IsGithubChecksEnabled() {
		githubCheckAliases, err = model.FindAliasInProjectRepoOrConfig(v.Identifier, evergreen.GithubChecksAlias)
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error getting github check aliases",
			"project": projectInfo.Project.Identifier,
			"version": v.Id,
		}))
	}

	taskIds := model.NewTaskIdConfigForRepotrackerVersion(projectInfo.Project, v, pairsToCreate, sourceRev, metadata.TriggerDefinitionID)

	for _, buildvariant := range projectInfo.Project.BuildVariants {
		taskNames := pairsToCreate.TaskNames(buildvariant.Name)
		var aliasesMatchingVariant model.ProjectAliases
		aliasesMatchingVariant, err = githubCheckAliases.AliasesMatchingVariant(buildvariant.Name, buildvariant.Tags)
		grip.Error(message.WrapError(err, message.Fields{
			"message":            "error getting aliases matching variant",
			"project":            projectInfo.Ref.Id,
			"project_identifier": projectInfo.Ref.Identifier,
			"version":            v.Id,
			"variant":            buildvariant.Name,
		}))
		creationInfo := model.TaskCreationInfo{
			Project:             projectInfo.Project,
			ProjectRef:          projectInfo.Ref,
			Version:             v,
			TaskIDs:             taskIds,
			TaskNames:           taskNames,
			BuildVariantName:    buildvariant.Name,
			ActivateBuild:       utility.FromBoolPtr(v.Activated),
			SourceRev:           sourceRev,
			DefinitionID:        metadata.TriggerDefinitionID,
			Aliases:             aliases,
			DistroAliases:       distroAliases,
			TaskCreateTime:      v.CreateTime,
			GithubChecksAliases: aliasesMatchingVariant,
		}

		b, tasks, err := model.CreateBuildFromVersionNoInsert(ctx, creationInfo)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":            "error creating build",
				"project":            projectInfo.Ref.Id,
				"project_identifier": projectInfo.Ref.Identifier,
				"version":            v.Id,
			}))
			debuggingData[buildvariant.Name] = "error creating build"
			continue
		}
		if len(tasks) == 0 {
			debuggingData[buildvariant.Name] = "no tasks for buildvariant"
			continue
		}
		buildsToCreate = append(buildsToCreate, *b)
		taskNameToId := map[string]string{}
		for _, t := range tasks {
			taskNameToId[t.DisplayName] = t.Id
			tasksToCreate = append(tasksToCreate, t)
		}

		activateVariantAt := time.Now()
		taskStatuses := []model.BatchTimeTaskStatus{}
		if v.Requester == evergreen.RepotrackerVersionRequester && evergreen.ShouldConsiderBatchtime(v.Requester) {
			activateVariantAt, err = projectInfo.Ref.GetActivationTimeForVariant(&buildvariant, v.CreateTime, time.Now())
			batchTimeCatcher.Add(errors.Wrapf(err, "unable to get activation time for variant '%s'", buildvariant.Name))
			// add only tasks that require activation times
			for _, bvt := range buildvariant.Tasks {
				tId, ok := taskNameToId[bvt.Name]
				if !ok || !bvt.HasSpecificActivation() {
					continue
				}
				activateTaskAt, err := projectInfo.Ref.GetActivationTimeForTask(&bvt, v.CreateTime, time.Now())
				batchTimeCatcher.Add(errors.Wrapf(err, "unable to get activation time for task '%s' (variant '%s')", bvt.Name, buildvariant.Name))

				taskStatuses = append(taskStatuses,
					model.BatchTimeTaskStatus{
						TaskName: bvt.Name,
						TaskId:   tId,
						ActivationStatus: model.ActivationStatus{
							ActivateAt: activateTaskAt,
							Activated:  false,
						},
					})
			}
		}

		grip.Debug(message.Fields{
			"message":            "created build",
			"name":               buildvariant.Name,
			"project":            projectInfo.Ref.Id,
			"project_identifier": projectInfo.Ref.Identifier,
			"version":            v.Id,
			"time":               activateVariantAt,
			"runner":             RunnerName,
		})
		v.BuildIds = append(v.BuildIds, b.Id)
		v.BuildVariants = append(v.BuildVariants, model.VersionBuildStatus{
			BuildVariant:   buildvariant.Name,
			BuildId:        b.Id,
			BatchTimeTasks: taskStatuses,
			ActivationStatus: model.ActivationStatus{
				ActivateAt: activateVariantAt,
				Activated:  false,
			},
		})
	}

	grip.ErrorWhen(len(buildsToCreate) == 0, message.Fields{
		"message":           "version has no builds",
		"version":           v.Id,
		"revision":          v.Revision,
		"author":            v.Author,
		"identifier":        v.Identifier,
		"requester":         v.Requester,
		"owner":             v.Owner,
		"repo":              v.Repo,
		"branch":            v.Branch,
		"buildvariant_data": debuggingData,
	})
	if len(buildsToCreate) == 0 {
		aliasString := ""
		for _, a := range aliases {
			aliasString += a.Alias + ","
		}
		return errors.Errorf("version '%s' in project '%s' using alias '%s' has no variants", v.Id, projectInfo.Ref.Identifier, aliasString)
	}
	grip.Error(message.WrapError(batchTimeCatcher.Resolve(), message.Fields{
		"message": "unable to get all activation times",
		"runner":  RunnerName,
		"version": v.Id,
	}))

	env := evergreen.GetEnvironment()

	ppStorageMethod, err := model.ParserProjectUpsertOneWithS3Fallback(ctx, env.Settings(), evergreen.ProjectStorageMethodDB, projectInfo.IntermediateProject)
	if err != nil {
		return errors.Wrapf(err, "upserting parser project '%s' for version '%s'", projectInfo.IntermediateProject.Id, v.Id)
	}
	v.ProjectStorageMethod = ppStorageMethod

	txFunc := func(sessCtx mongo.SessionContext) error {
		err := sessCtx.StartTransaction()
		if err != nil {
			return errors.Wrap(err, "starting transaction")
		}
		db := env.DB()
		_, err = db.Collection(model.VersionCollection).InsertOne(sessCtx, v)
		if err != nil {
			grip.Notice(message.WrapError(err, message.Fields{
				"message": "aborting transaction",
				"cause":   "can't insert version",
				"version": v.Id,
			}))
			if abortErr := sessCtx.AbortTransaction(sessCtx); abortErr != nil {
				return errors.Wrap(abortErr, "aborting transaction")
			}
			return errors.Wrapf(err, "inserting version '%s'", v.Id)
		}
		if projectInfo.Config != nil {
			_, err = db.Collection(model.ProjectConfigCollection).InsertOne(sessCtx, projectInfo.Config)
			if err != nil {
				grip.Notice(message.WrapError(err, message.Fields{
					"message": "aborting transaction",
					"cause":   "can't insert project config",
					"version": v.Id,
				}))
				if abortErr := sessCtx.AbortTransaction(sessCtx); abortErr != nil {
					return errors.Wrap(abortErr, "aborting transaction")
				}
				return errors.Wrapf(err, "inserting project config '%s'", v.Id)
			}
		}
		_, err = db.Collection(build.Collection).InsertMany(sessCtx, buildsToCreate)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "aborting transaction",
				"cause":   "can't insert builds",
				"version": v.Id,
			}))
			if abortErr := sessCtx.AbortTransaction(sessCtx); abortErr != nil {
				return errors.Wrap(abortErr, "aborting transaction")
			}

			return errors.Wrap(err, "inserting builds")
		}
		err = tasksToCreate.InsertUnordered(sessCtx)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "aborting transaction",
				"cause":   "can't insert tasks",
				"version": v.Id,
			}))
			if abortErr := sessCtx.AbortTransaction(sessCtx); abortErr != nil {
				return errors.Wrap(abortErr, "aborting transaction")
			}
			return errors.Wrap(err, "inserting tasks")
		}
		err = sessCtx.CommitTransaction(sessCtx)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "aborting transaction",
				"cause":   "unable to commit transaction",
				"version": v.Id,
			}))
			if abortErr := sessCtx.AbortTransaction(sessCtx); abortErr != nil {
				return errors.Wrap(abortErr, "aborting transaction")
			}

			return errors.Wrapf(err, "committing transaction for version '%s'", v.Id)
		}
		grip.Info(message.Fields{
			"message": "successfully created version",
			"version": v.Id,
			"hash":    v.Revision,
			"project": v.Identifier,
			"runner":  RunnerName,
		})
		return nil
	}

	return transactionWithRetries(ctx, v.Id, txFunc)
}

// If we error in aborting transaction, we create a new session and start again.
// If we abort successfully and the error is a transient transaction error, we retry using the same session.
func transactionWithRetries(ctx context.Context, versionId string, sessionFunc func(sessCtx mongo.SessionContext) error) error {
	const retryCount = 5
	const minBackoffInterval = 1 * time.Second
	const maxBackoffInterval = 60 * time.Second

	client := evergreen.GetEnvironment().Client()
	errs := grip.NewBasicCatcher()
	interval := backoff.Backoff{
		Min:    minBackoffInterval,
		Max:    maxBackoffInterval,
		Factor: 2,
	}
	for i := 0; i < retryCount; i++ {
		err := client.UseSession(ctx, sessionFunc)
		if err == nil {
			return nil
		}
		errs.Add(err)
		time.Sleep(interval.Duration())
	}
	return errors.Wrapf(errs.Resolve(), "hit max client retries for version '%s'", versionId)
}
