package repotracker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/github"
	"github.com/jpillora/backoff"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
	mgobson "gopkg.in/mgo.v2/bson"
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

// The RepoPoller interface specifies behavior required of all repository poller
// implementations
type RepoPoller interface {
	// Fetches the contents of a remote repository's configuration data as at
	// the given revision.
	GetRemoteConfig(ctx context.Context, revision string) (*model.Project, *model.ParserProject, error)

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

	if !projectRef.IsEnabled() || projectRef.IsRepotrackerDisabled() {
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
		return errors.Wrapf(err, "error finding repository '%v'", projectRef.Identifier)
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
			"message":            "no last recorded revision, using most recent revisions",
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
				"message":            "problem sorting revisions for repository",
				"runner":             RunnerName,
				"project":            projectRef.Id,
				"project_identifier": projectRef.Identifier,
			}))
			return errors.WithStack(err)
		}
	}
	ok, err := model.DoProjectActivation(projectRef.Id, time.Now())
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
		project, intermediateProject, err := repoTracker.GetProjectConfig(ctx, revision)
		if err != nil {
			// this is an error that implies the file is invalid - create a version and store the error
			projErr, isProjErr := err.(projectConfigError)
			if isProjErr {
				versionErrs = &VersionErrors{
					Warnings: projErr.Warnings,
					Errors:   projErr.Errors,
				}
				if len(versionErrs.Errors) > 0 {
					stubVersion, dbErr := shellVersionFromRevision(ctx, ref, model.VersionMetadata{Revision: revisions[i]})
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
		}

		// "Ignore" a version if all changes are to ignored files
		var ignore bool
		if len(project.Ignore) > 0 {
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
			if project.IgnoresAllFiles(filenames) {
				ignore = true
			}
		}

		metadata := model.VersionMetadata{
			Revision: revisions[i],
		}
		projectInfo := &model.ProjectInfo{
			Ref:                 ref,
			Project:             project,
			IntermediateProject: intermediateProject,
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
			if err = addGithubCheckSubscriptions(v); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message":            "error adding github check subscriptions",
					"runner":             RunnerName,
					"project":            ref.Id,
					"project_identifier": ref.Identifier,
					"revision":           revision,
				}))
			}
		}

		_, err = CreateManifest(*v, project, ref, repoTracker.Settings)
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
func (repoTracker *RepoTracker) GetProjectConfig(ctx context.Context, revision string) (*model.Project, *model.ParserProject, error) {
	projectRef := repoTracker.ProjectRef
	project, intermediateProj, err := repoTracker.GetRemoteConfig(ctx, revision)
	if err != nil {
		// Only create a stub version on API request errors that pertain
		// to actually fetching a config. Those errors currently include:
		// thirdparty.APIRequestError, thirdparty.FileNotFoundError and
		// thirdparty.YAMLFormatError
		_, apiReqErr := errors.Cause(err).(thirdparty.APIRequestError)
		_, ymlFmtErr := errors.Cause(err).(thirdparty.YAMLFormatError)
		_, noFileErr := errors.Cause(err).(thirdparty.FileNotFoundError)
		parsingErr := strings.Contains(err.Error(), model.TranslateProjectError)
		mergingErr := strings.Contains(err.Error(), model.MergeProjectConfigError)
		if apiReqErr || noFileErr || ymlFmtErr || parsingErr || mergingErr {
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
			return nil, nil, projectConfigError{Errors: []string{msg.String()}, Warnings: nil}
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

		return nil, nil, err
	}
	return project, intermediateProj, nil
}

// addGithubCheckSubscriptions adds subscriptions to send the status of the version to Github.
func addGithubCheckSubscriptions(v *model.Version) error {
	catcher := grip.NewBasicCatcher()
	ghSub := event.NewGithubCheckAPISubscriber(event.GithubCheckSubscriber{
		Owner: v.Owner,
		Repo:  v.Repo,
		Ref:   v.Revision,
	})

	versionSub := event.NewVersionGithubCheckOutcomeSubscription(v.Id, ghSub)
	if err := versionSub.Upsert(); err != nil {
		catcher.Wrap(err, "failed to insert version github check subscription")
	}
	buildSub := event.NewGithubCheckBuildOutcomeSubscriptionByVersion(v.Id, ghSub)
	if err := buildSub.Upsert(); err != nil {
		catcher.Wrap(err, "failed to insert build github check subscription")
	}
	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		catcher.Add(errors.Wrap(err, "error retrieving admin settings"))
		return catcher.Resolve()
	}
	if flags.GithubStatusAPIDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"runner":  RunnerName,
			"message": "github status updates are disabled, not updating status",
		})
		return catcher.Resolve()
	}
	env := evergreen.GetEnvironment()
	uiConfig := evergreen.UIConfig{}
	if err = uiConfig.Get(env); err != nil {
		catcher.Add(err)
		return catcher.Resolve()
	}
	urlBase := uiConfig.Url
	if urlBase == "" {
		catcher.New("url base doesn't exist")
		return catcher.Resolve()
	}
	status := &message.GithubStatus{
		Owner:       v.Owner,
		Repo:        v.Repo,
		Ref:         v.Revision,
		URL:         fmt.Sprintf("%s/version/%s", urlBase, v.Id),
		Context:     "evergreen",
		State:       message.GithubStatePending,
		Description: "version created",
	}
	sender, err := env.GetSender(evergreen.SenderGithubStatus)
	if err != nil {
		catcher.Add(err)
		return catcher.Resolve()
	}

	c := message.MakeGithubStatusMessageWithRepo(*status)
	if !c.Loggable() {
		catcher.Add(errors.Errorf("status message is invalid: %+v", status))
		return catcher.Resolve()
	}

	if err = c.SetPriority(level.Notice); err != nil {
		catcher.Add(err)
		return catcher.Resolve()
	}

	sender.Send(c)
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
				Type: "object",
				Data: "task",
			},
			{
				Type: "project",
				Data: projectRef.Id,
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
		return nil, errors.Errorf("user %s does not exist", userID)
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
			subscriber.Target = u.Settings.SlackUsername
		} else {
			return nil, errors.Errorf("invalid subscription preference for build break: %s", preference)
		}
	}

	return subscriber, nil
}

func CreateManifest(v model.Version, proj *model.Project, projectRef *model.ProjectRef, settings *evergreen.Settings) (*manifest.Manifest, error) {
	if len(proj.Modules) == 0 {
		return nil, nil
	}
	newManifest := &manifest.Manifest{
		Id:          v.Id,
		Revision:    v.Revision,
		ProjectName: v.Identifier,
		Branch:      projectRef.Branch,
		IsBase:      v.Requester == evergreen.RepotrackerVersionRequester,
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
			var commit *github.RepositoryCommit
			commit, err = thirdparty.GetCommitEvent(ctx, token, projectRef.Owner, projectRef.Repo, v.Revision)
			if err != nil {
				return nil, errors.Wrapf(err, "can't get commit '%s' on '%s/%s'", v.Revision, projectRef.Owner, projectRef.Repo)
			}
			if commit == nil || commit.Commit == nil || commit.Commit.Committer == nil {
				return nil, errors.New("malformed GitHub commit response")
			}
			revisionTime := commit.Commit.Committer.GetDate()
			var branchCommits []*github.RepositoryCommit
			branchCommits, _, err = thirdparty.GetGithubCommits(ctx, token, owner, repo, module.Branch, revisionTime, 0)
			if err != nil {
				return nil, errors.Wrapf(err, "problem retrieving getting git branch for module %s", module.Name)
			}
			if len(branchCommits) > 0 {
				sha = branchCommits[0].GetSHA()
				url = branchCommits[0].GetURL()
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
		"message":            "creating manifest",
		"version_id":         v.Id,
		"manifest":           newManifest,
		"project":            v.Identifier,
		"project_identifier": projectRef.Identifier,
		"modules":            modules,
		"branch_info":        gitBranch,
	})
	_, err = newManifest.TryInsert()

	return newManifest, errors.Wrap(err, "error inserting manifest")
}

func CreateVersionFromConfig(ctx context.Context, projectInfo *model.ProjectInfo,
	metadata model.VersionMetadata, ignore bool, versionErrs *VersionErrors) (*model.Version, error) {
	if projectInfo.NotPopulated() {
		return nil, errors.New("project ref and parser project cannot be nil")
	}

	// create a version document
	v, err := shellVersionFromRevision(ctx, projectInfo.Ref, metadata)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create shell version")
	}
	if err = sanityCheckOrderNum(v.RevisionOrderNumber, projectInfo.Ref.Id, metadata.Revision.Revision); err != nil {
		return nil, errors.Wrap(err, "inconsistent version order")
	}

	if projectInfo.Project == nil {
		projectInfo.Project, err = model.TranslateProject(projectInfo.IntermediateProject)
		if err != nil {
			return nil, errors.Wrap(err, "error translating intermediate project")
		}
	}
	projectInfo.IntermediateProject.Id = v.Id
	projectInfo.IntermediateProject.CreateTime = v.CreateTime
	v.Ignored = ignore
	v.Activated = utility.FalsePtr()

	// validate the project
	verrs := validator.CheckProjectSyntax(projectInfo.Project, true)
	verrs = append(verrs, validator.CheckProjectSettings(projectInfo.Project, projectInfo.Ref)...)
	if len(verrs) > 0 || versionErrs != nil {
		// We have errors in the project.
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
			if err = v.Insert(); err != nil {
				return nil, errors.Wrap(err, "error inserting version")
			}
			if err = projectInfo.IntermediateProject.Insert(); err != nil {
				return v, errors.Wrap(err, "error inserting project")
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
		aliases, err = model.FindAliasInProjectOrRepo(projectInfo.Ref.Id, metadata.Alias)
		if err != nil {
			return v, errors.Wrap(err, "error finding project alias")
		}
	}

	return v, errors.Wrap(createVersionItems(ctx, v, metadata, projectInfo, aliases), "error creating version items")
}

// shellVersionFromRevision populates a new Version with metadata from a model.Revision.
// Does not populate its config or store anything in the database.
func shellVersionFromRevision(ctx context.Context, ref *model.ProjectRef, metadata model.VersionMetadata) (*model.Version, error) {
	var u *user.DBUser
	var err error
	if metadata.Revision.AuthorGithubUID != 0 {
		u, err = user.FindByGithubUID(metadata.Revision.AuthorGithubUID)
		grip.Error(message.WrapError(err, message.Fields{
			"message": fmt.Sprintf("failed to fetch everg user with Github UID %d", metadata.Revision.AuthorGithubUID),
		}))
	}

	number, err := model.GetNewRevisionOrderNumber(ref.Id)
	if err != nil {
		return nil, err
	}
	v := &model.Version{
		Author:              metadata.Revision.Author,
		AuthorID:            metadata.Revision.AuthorID,
		AuthorEmail:         metadata.Revision.AuthorEmail,
		Branch:              ref.Branch,
		CreateTime:          metadata.Revision.CreateTime,
		Identifier:          ref.Id,
		Message:             metadata.Revision.RevisionMessage,
		Owner:               ref.Owner,
		RemotePath:          ref.RemotePath,
		Repo:                ref.Repo,
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
		v.Id = util.CleanName(fmt.Sprintf("%s_%s_%s", ref.Identifier, metadata.SourceVersion.Revision, metadata.TriggerDefinitionID))
		v.Requester = evergreen.TriggerRequester
		v.CreateTime = metadata.SourceVersion.CreateTime
	} else if metadata.IsAdHoc {
		v.Id = mgobson.NewObjectId().Hex()
		v.Requester = evergreen.AdHocRequester
		v.CreateTime = time.Now()
		if metadata.Message != "" {
			v.Message = metadata.Message
		}
		if metadata.User != nil {
			num, err := metadata.User.IncPatchNumber()
			if err != nil {
				return nil, errors.Wrap(err, "error incrementing patch number")
			}
			v.RevisionOrderNumber = num
		}
	} else if metadata.GitTag.Tag != "" {
		if !ref.IsGitTagVersionsEnabled() {
			return nil, errors.Errorf("git tag versions are not enabled for project '%s'", ref.Id)
		}
		settings, err := evergreen.GetConfig()
		if err != nil {
			return nil, errors.Wrap(err, "error getting settings")
		}
		token, err := settings.GetGithubOauthToken()
		if err != nil {
			return nil, errors.Wrap(err, "error getting github token")
		}

		if !ref.AuthorizedForGitTag(ctx, metadata.GitTag.Pusher, token) {
			return nil, errors.Errorf("user '%s' not authorized to create git tag versions for project '%s'",
				metadata.GitTag.Pusher, ref.Id)
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
	if u != nil {
		v.AuthorID = u.Id
		v.Author = u.DisplayName()
		v.AuthorEmail = u.Email()
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
func createVersionItems(ctx context.Context, v *model.Version, metadata model.VersionMetadata, projectInfo *model.ProjectInfo, aliases model.ProjectAliases) error {
	distroAliases, err := distro.NewDistroAliasesLookupTable()
	if err != nil {
		return errors.WithStack(err)
	}
	// generate all task Ids so that we can easily reference them for dependencies
	sourceRev := ""
	if metadata.SourceVersion != nil {
		sourceRev = metadata.SourceVersion.Revision
	}
	taskIds := model.NewTaskIdTable(projectInfo.Project, v, sourceRev, metadata.TriggerDefinitionID)

	// create all builds for the version
	buildsToCreate := []interface{}{}
	tasksToCreate := task.Tasks{}
	pairsToCreate := model.TVPairSet{}
	// build all pairsToCreate before creating builds, to handle dependencies (if applicable)
	for _, buildvariant := range projectInfo.Project.BuildVariants {
		if ctx.Err() != nil {
			return errors.Wrapf(err, "aborting version creation for version %s", v.Id)
		}
		if buildvariant.Disabled {
			continue
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

	pairsToCreate, err = model.IncludeDependencies(projectInfo.Project, pairsToCreate, v.Requester)
	grip.Warning(message.WrapError(err, message.Fields{
		"message": "error including dependencies",
		"project": projectInfo.Project.Identifier,
		"version": v.Id,
	}))
	batchTimeCatcher := grip.NewBasicCatcher()
	debuggingData := map[string]string{}
	var githubCheckAliases model.ProjectAliases
	if v.Requester == evergreen.RepotrackerVersionRequester && projectInfo.Ref.IsGithubChecksEnabled() {
		githubCheckAliases, err = model.FindAliasInProjectOrRepo(v.Identifier, evergreen.GithubChecksAlias)
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error getting github check aliases",
			"project": projectInfo.Project.Identifier,
			"version": v.Id,
		}))
	}
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
		args := model.BuildCreateArgs{
			Project:             *projectInfo.Project,
			Version:             *v,
			TaskIDs:             taskIds,
			TaskNames:           taskNames,
			BuildName:           buildvariant.Name,
			ActivateBuild:       false,
			SourceRev:           sourceRev,
			DefinitionID:        metadata.TriggerDefinitionID,
			Aliases:             aliases,
			DistroAliases:       distroAliases,
			TaskCreateTime:      v.CreateTime,
			GithubChecksAliases: aliasesMatchingVariant,
			ProjectIdentifier:   projectInfo.Ref.Identifier,
		}

		b, tasks, err := model.CreateBuildFromVersionNoInsert(args)
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
		if metadata.TriggerID == "" && evergreen.ShouldConsiderBatchtime(v.Requester) {
			activateVariantAt, err = projectInfo.Ref.GetActivationTimeForVariant(&buildvariant)
			batchTimeCatcher.Add(errors.Wrapf(err, "unable to get activation time for variant '%s'", buildvariant.Name))
			// add only tasks that require activation times
			for _, bvt := range buildvariant.Tasks {
				tId, ok := taskNameToId[bvt.Name]
				if !ok || !bvt.HasBatchTime() {
					continue
				}
				bvt.Variant = buildvariant.Name
				activateTaskAt, err := projectInfo.Ref.GetActivationTimeForTask(&bvt)
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

	txFunc := func(sessCtx mongo.SessionContext) error {
		err := sessCtx.StartTransaction()
		if err != nil {
			return errors.Wrap(err, "error starting transaction")
		}
		db := evergreen.GetEnvironment().DB()
		_, err = db.Collection(model.VersionCollection).InsertOne(sessCtx, v)
		if err != nil {
			grip.Notice(message.WrapError(err, message.Fields{
				"message": "aborting transaction",
				"cause":   "can't insert version",
				"version": v.Id,
			}))
			if abortErr := sessCtx.AbortTransaction(sessCtx); abortErr != nil {
				return errors.Wrap(abortErr, "error aborting transaction")
			}
			return errors.Wrapf(err, "error inserting version '%s'", v.Id)
		}
		_, err = db.Collection(model.ParserProjectCollection).InsertOne(sessCtx, projectInfo.IntermediateProject)
		if err != nil {
			grip.Notice(message.WrapError(err, message.Fields{
				"message": "aborting transaction",
				"cause":   "can't insert parser project",
				"version": v.Id,
			}))
			if abortErr := sessCtx.AbortTransaction(sessCtx); abortErr != nil {
				return errors.Wrap(abortErr, "error aborting transaction")
			}
			return errors.Wrapf(err, "error inserting parser project '%s'", v.Id)
		}
		_, err = db.Collection(build.Collection).InsertMany(sessCtx, buildsToCreate)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "aborting transaction",
				"cause":   "can't insert builds",
				"version": v.Id,
			}))
			if abortErr := sessCtx.AbortTransaction(sessCtx); abortErr != nil {
				return errors.Wrap(abortErr, "error aborting transaction")
			}

			return errors.Wrap(err, "error inserting builds")
		}
		err = tasksToCreate.InsertUnordered(sessCtx)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "aborting transaction",
				"cause":   "can't insert tasks",
				"version": v.Id,
			}))
			if abortErr := sessCtx.AbortTransaction(sessCtx); abortErr != nil {
				return errors.Wrap(abortErr, "error aborting transaction")
			}
			return errors.Wrap(err, "error inserting tasks")
		}
		err = sessCtx.CommitTransaction(sessCtx)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "aborting transaction",
				"cause":   "unable to commit transaction",
				"version": v.Id,
			}))
			if abortErr := sessCtx.AbortTransaction(sessCtx); abortErr != nil {
				return errors.Wrap(abortErr, "error aborting transaction")
			}

			return errors.Wrapf(err, "error committing transaction for version '%s'", v.Id)
		}
		grip.Info(message.Fields{
			"message":         "successfully created version",
			"version":         v.Id,
			"hash":            v.Revision,
			"project":         v.Identifier,
			"tasks_to_create": tasksToCreate,
			"runner":          RunnerName,
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
