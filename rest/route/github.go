package route

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/github"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	githubActionClosed      = "closed"
	githubActionOpened      = "opened"
	githubActionSynchronize = "synchronize"
	githubActionReopened    = "reopened"
	commitUnsigned          = "unsigned"

	retryComment = "evergreen retry"
	refTags      = "refs/tags/"
)

type githubHookApi struct {
	queue  amboy.Queue
	secret []byte

	event     interface{}
	eventType string
	msgID     string
	sc        data.Connector
	settings  *evergreen.Settings
}

func makeGithubHooksRoute(sc data.Connector, queue amboy.Queue, secret []byte, settings *evergreen.Settings) gimlet.RouteHandler {
	return &githubHookApi{
		sc:       sc,
		settings: settings,
		queue:    queue,
		secret:   secret,
	}
}

func (gh *githubHookApi) Factory() gimlet.RouteHandler {
	return &githubHookApi{
		queue:    gh.queue,
		secret:   gh.secret,
		sc:       gh.sc,
		settings: gh.settings,
	}
}

func (gh *githubHookApi) Parse(ctx context.Context, r *http.Request) error {
	gh.eventType = r.Header.Get("X-Github-Event")
	gh.msgID = r.Header.Get("X-Github-Delivery")

	if len(gh.secret) == 0 || gh.queue == nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "webhooks are not configured and therefore disabled",
		}
	}

	body, err := github.ValidatePayload(r, gh.secret)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"source":  "github hook",
			"message": "rejecting github webhook",
			"msg_id":  gh.msgID,
			"event":   gh.eventType,
		}))
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "failed to read request body",
		}
	}

	gh.event, err = github.ParseWebHook(gh.eventType, body)
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		}
	}

	return nil
}

func (gh *githubHookApi) Run(ctx context.Context) gimlet.Responder {
	switch event := gh.event.(type) {
	case *github.PingEvent:
		if event.HookID == nil {
			return gimlet.NewJSONErrorResponse(gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "malformed ping event",
			})
		}
		grip.Info(message.Fields{
			"source":  "github hook",
			"msg_id":  gh.msgID,
			"event":   gh.eventType,
			"hook_id": event.HookID,
		})

	case *github.PullRequestEvent:
		if event.Action == nil {
			err := gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "pull request has no action",
			}
			grip.Error(message.WrapError(err, message.Fields{
				"source": "github hook",
				"msg_id": gh.msgID,
				"event":  gh.eventType,
			}))

			return gimlet.NewJSONErrorResponse(err)
		}

		if *event.Action == githubActionOpened || *event.Action == githubActionSynchronize ||
			*event.Action == githubActionReopened {
			grip.Info(message.Fields{
				"source":    "github hook",
				"msg_id":    gh.msgID,
				"event":     gh.eventType,
				"repo":      *event.PullRequest.Base.Repo.FullName,
				"ref":       *event.PullRequest.Base.Ref,
				"pr_number": *event.PullRequest.Number,
				"hash":      *event.PullRequest.Head.SHA,
				"user":      *event.Sender.Login,
				"message":   "pr accepted, attempting to queue",
			})
			if err := gh.AddIntentForPR(event.PullRequest, event.Sender.GetLogin()); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"source":    "github hook",
					"msg_id":    gh.msgID,
					"event":     gh.eventType,
					"action":    event.Action,
					"repo":      *event.PullRequest.Base.Repo.FullName,
					"ref":       *event.PullRequest.Base.Ref,
					"pr_number": *event.PullRequest.Number,
					"user":      *event.Sender.Login,
					"message":   "can't add intent",
				}))
				return gimlet.NewJSONErrorResponse(err)
			}
		} else if *event.Action == githubActionClosed {
			grip.Info(message.Fields{
				"source":  "github hook",
				"msg_id":  gh.msgID,
				"event":   gh.eventType,
				"action":  *event.Action,
				"message": "pull request closed; aborting patch",
			})

			err := gh.sc.AbortPatchesFromPullRequest(event)
			if err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"source":  "github hook",
					"msg_id":  gh.msgID,
					"event":   gh.eventType,
					"action":  *event.Action,
					"message": "failed to abort patches",
				}))
				return gimlet.MakeJSONErrorResponder(err)
			}

			// if the item is on a commit queue, remove it
			if err = gh.tryDequeueCommitQueueItemForPR(event.PullRequest); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"source":  "github hook",
					"msg_id":  gh.msgID,
					"event":   gh.eventType,
					"action":  *event.Action,
					"message": "commit queue item not dequeued",
				}))
				return gimlet.MakeJSONErrorResponder(err)
			}

			return gimlet.NewJSONResponse(struct{}{})
		}

	case *github.PushEvent:
		grip.Debug(message.Fields{
			"source":     "github hook",
			"msg_id":     gh.msgID,
			"event":      gh.eventType,
			"event_data": event,
			"ref":        event.GetRef(),
			"is_tag":     isTag(event.GetRef()),
		})
		if isTag(event.GetRef()) {
			if err := gh.handleGitTag(ctx, event); err != nil {
				return gimlet.MakeJSONErrorResponder(err)
			}
			return gimlet.NewJSONResponse(struct{}{})
		}
		if err := gh.sc.TriggerRepotracker(gh.queue, gh.msgID, event); err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}

	case *github.IssueCommentEvent:
		if event.Issue.IsPullRequest() {
			if commitqueue.TriggersCommitQueue(*event.Action, *event.Comment.Body) {
				grip.Info(message.Fields{
					"source":    "github hook",
					"msg_id":    gh.msgID,
					"event":     gh.eventType,
					"repo":      *event.Repo.FullName,
					"pr_number": *event.Issue.Number,
					"user":      *event.Sender.Login,
					"message":   "commit queue triggered",
				})
				if err := gh.commitQueueEnqueue(ctx, event); err != nil {
					grip.Error(message.WrapError(err, message.Fields{
						"source":    "github hook",
						"msg_id":    gh.msgID,
						"event":     gh.eventType,
						"action":    event.Action,
						"repo":      *event.Repo.FullName,
						"pr_number": *event.Issue.Number,
						"user":      *event.Sender.Login,
						"message":   "can't enqueue on commit queue",
					}))
					return gimlet.MakeJSONErrorResponder(err)
				}
			}
			if triggersRetry(*event.Action, *event.Comment.Body) {
				grip.Info(message.Fields{
					"source":    "github hook",
					"msg_id":    gh.msgID,
					"event":     gh.eventType,
					"repo":      *event.Repo.FullName,
					"pr_number": *event.Issue.Number,
					"user":      *event.Sender.Login,
					"message":   "retry triggered",
				})
				if err := gh.retryPRPatch(ctx, event.Repo.Owner.GetLogin(), event.Repo.GetName(), event.Issue.GetNumber()); err != nil {
					grip.Error(message.WrapError(err, message.Fields{
						"source":    "github hook",
						"msg_id":    gh.msgID,
						"event":     gh.eventType,
						"action":    event.Action,
						"repo":      *event.Repo.FullName,
						"pr_number": *event.Issue.Number,
						"user":      *event.Sender.Login,
						"message":   "can't retry PR",
					}))
					return gimlet.MakeJSONErrorResponder(err)
				}
			}
		}

	case *github.MetaEvent:
		if event.GetAction() == "deleted" {
			hookID := event.GetHookID()
			if hookID == 0 {
				msg := "invalid hook ID for deleted hook"
				grip.Error(message.Fields{
					"source":  "github hook",
					"msg_id":  gh.msgID,
					"event":   gh.eventType,
					"action":  event.Action,
					"hook":    event.Hook,
					"message": msg,
				})
				return gimlet.MakeJSONErrorResponder(errors.New(msg))
			}
			if err := model.RemoveGithubHook(int(hookID)); err != nil {
				return gimlet.MakeJSONErrorResponder(err)
			}
		}
	}

	return gimlet.NewJSONResponse(struct{}{})
}

func (gh *githubHookApi) retryPRPatch(ctx context.Context, owner, repo string, prNumber int) error {
	settings, err := gh.sc.GetEvergreenSettings()
	if err != nil {
		return errors.Wrap(err, "can't get Evergreen settings")
	}
	githubToken, err := settings.GetGithubOauthToken()
	if err != nil {
		return errors.Wrap(err, "can't get GitHub token from settings")
	}

	pr, err := thirdparty.GetGithubPullRequest(ctx, githubToken, owner, repo, prNumber)
	if err != nil {
		return errors.Wrapf(err, "can't get PR for repo %s:%s, PR #%d", owner, repo, prNumber)
	}

	return gh.AddIntentForPR(pr, pr.User.GetLogin())
}

func (gh *githubHookApi) AddIntentForPR(pr *github.PullRequest, owner string) error {
	ghi, err := patch.NewGithubIntent(gh.msgID, owner, pr)
	if err != nil {
		return errors.Wrap(err, "failed to create intent")
	}

	if err := gh.sc.AddPatchIntent(ghi, gh.queue); err != nil {
		return errors.Wrap(err, "error saving intent")
	}

	return nil
}

// handleGitTag adds the tag to the version it was pushed to, and triggers a new version if applicable
func (gh *githubHookApi) handleGitTag(ctx context.Context, event *github.PushEvent) error {
	if err := validatePushTagEvent(event); err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"source":  "github hook",
			"message": "error validating event",
			"ref":     event.GetRef(),
			"event":   gh.eventType,
		}))
		return err
	}
	token, err := gh.settings.GetGithubOauthToken()
	if err != nil {
		return errors.New("error getting github token")
	}
	pusher := event.GetPusher().GetName()
	tag := model.GitTag{
		Tag:    strings.TrimPrefix(event.GetRef(), refTags),
		Pusher: pusher,
	}
	ownerAndRepo := strings.Split(event.Repo.GetFullName(), "/")
	hash, err := thirdparty.GetTaggedCommitFromGithub(ctx, token, ownerAndRepo[0], ownerAndRepo[1], tag.Tag)
	if err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"source":  "github hook",
			"message": "error getting tagged commit from github",
			"ref":     event.GetRef(),
			"event":   gh.eventType,
			"owner":   ownerAndRepo[0],
			"repo":    ownerAndRepo[1],
			"tag":     tag,
		}))
		return errors.Wrapf(err, "error getting commit for tag '%s'", tag.Tag)
	}

	projectRefs, err := gh.sc.FindEnabledProjectRefsByOwnerAndRepo(ownerAndRepo[0], ownerAndRepo[1])
	if err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"source":  "github hook",
			"message": "error finding projects",
			"ref":     event.GetRef(),
			"event":   gh.eventType,
			"owner":   ownerAndRepo[0],
			"repo":    ownerAndRepo[1],
			"tag":     tag,
		}))
		return errors.Wrapf(err, "error finding projects of repo '%s'", ownerAndRepo)
	}
	if len(projectRefs) == 0 {
		grip.Debug(message.Fields{
			"source":  "github hook",
			"message": "no projects found",
			"ref":     event.GetRef(),
			"event":   gh.eventType,
			"owner":   ownerAndRepo[0],
			"repo":    ownerAndRepo[1],
			"tag":     tag,
		})
		return errors.Wrapf(err, "no projects found for repo '%s'", ownerAndRepo)
	}

	foundVersion := map[string]bool{}

	const (
		checkVersionAttempts      = 5
		checkVersionRetryMinDelay = 3500 * time.Millisecond
		checkVersionRetryMaxDelay = 15 * time.Second
	)

	catcher := grip.NewBasicCatcher()

	// iterate through all projects before retrying. Only retry on errors related to finding the version.
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			retryCatcher := grip.NewBasicCatcher()
			for _, pRef := range projectRefs {
				if foundVersion[pRef.Id] {
					continue
				}
				var existingVersion *model.Version
				// If a version for this revision exists for this project, add tag
				// Retry in case a commit and a tag are pushed at around the same time, and the version isn't ready yet
				existingVersion, err = gh.sc.FindVersionByProjectAndRevision(pRef.Id, hash)
				if err != nil {
					retryCatcher.Wrapf(err, "problem finding version for project '%s' with revision '%s'", pRef.Id, hash)
					continue
				}
				if existingVersion == nil {
					retryCatcher.Errorf("no version for project '%s' with revision '%s' to add tag to", pRef.Id, hash)
					continue
				}
				foundVersion[pRef.Id] = true
				grip.Debug(message.Fields{
					"source":  "github hook",
					"message": "adding tag to version",
					"version": existingVersion.Id,
					"ref":     event.GetRef(),
					"event":   gh.eventType,
					"branch":  pRef.Branch,
					"owner":   pRef.Owner,
					"repo":    pRef.Repo,
					"hash":    hash,
					"tag":     tag,
				})

				if err = gh.sc.AddGitTagToVersion(existingVersion.Id, tag); err != nil {
					catcher.Wrapf(err, "problem adding tag '%s' to version '%s''", tag.Tag, existingVersion.Id)
					continue
				}

				revision := model.Revision{
					Author:          existingVersion.Author,
					AuthorID:        existingVersion.AuthorID,
					AuthorEmail:     existingVersion.AuthorEmail,
					Revision:        existingVersion.Revision,
					RevisionMessage: existingVersion.Message,
				}
				var v *model.Version
				v, err = gh.createVersionForTag(ctx, pRef, existingVersion, revision, tag, token)
				if err != nil {
					catcher.Wrapf(err, "error adding new version for tag '%s'", tag.Tag)
					continue
				}
				if v != nil {
					grip.Info(message.Fields{
						"source":  "github hook",
						"msg_id":  gh.msgID,
						"event":   gh.eventType,
						"ref":     event.GetRef(),
						"owner":   ownerAndRepo[0],
						"repo":    ownerAndRepo[1],
						"tag":     tag,
						"version": v.Id,
						"message": "triggered version from git tag",
					})
				}
			}
			return retryCatcher.HasErrors(), retryCatcher.Resolve()
		}, utility.RetryOptions{
			MaxAttempts: checkVersionAttempts,
			MinDelay:    checkVersionRetryMinDelay,
			MaxDelay:    checkVersionRetryMaxDelay,
		})
	catcher.Add(err)
	grip.Error(message.WrapError(catcher.Resolve(), message.Fields{
		"source":  "github hook",
		"msg_id":  gh.msgID,
		"event":   gh.eventType,
		"ref":     event.GetRef(),
		"owner":   ownerAndRepo[0],
		"repo":    ownerAndRepo[1],
		"tag":     tag,
		"message": "errors updating/creating versions for git tag",
	}))
	return nil
}

func (gh *githubHookApi) createVersionForTag(ctx context.Context, pRef model.ProjectRef, existingVersion *model.Version,
	revision model.Revision, tag model.GitTag, token string) (*model.Version, error) {
	if !pRef.IsGitTagVersionsEnabled() {
		return nil, nil
	}

	if !pRef.AuthorizedForGitTag(ctx, tag.Pusher, token) {
		grip.Debug(message.Fields{
			"source":             "github hook",
			"msg_id":             gh.msgID,
			"event":              gh.eventType,
			"project":            pRef.Id,
			"project_identifier": pRef.Identifier,
			"tag":                tag,
			"message":            "user not authorized for git tag version",
		})
		return nil, nil
	}

	hasAliases, remotePath, err := gh.sc.HasMatchingGitTagAliasAndRemotePath(pRef.Id, tag.Tag)
	if err != nil {
		return nil, err
	}
	if !hasAliases {
		return nil, nil
	}
	metadata := model.VersionMetadata{
		Revision:   revision,
		GitTag:     tag,
		RemotePath: remotePath,
	}
	info := &model.ProjectInfo{Ref: &pRef}
	if remotePath != "" {
		// run everything in the yaml that's provided
		if gh.settings == nil {
			gh.settings, err = evergreen.GetConfig()
			if err != nil {
				return nil, errors.New("error getting settings config")
			}
		}
		info.Project, info.IntermediateProject, err = gh.sc.GetProjectFromFile(ctx, pRef, remotePath, token)
		if err != nil {
			return nil, errors.Wrap(err, "unable to unmarshal yaml config")
		}
	} else {
		// use the standard project config with the git tag alias
		info.Project, info.IntermediateProject, err = gh.sc.LoadProjectForVersion(existingVersion, pRef.Id)
		if err != nil {
			return nil, errors.Wrapf(err, "problem getting project for  '%s'", pRef.Identifier)
		}
		metadata.Alias = evergreen.GitTagAlias
	}

	return gh.sc.CreateVersionFromConfig(ctx, info, metadata, true)
}

func validatePushTagEvent(event *github.PushEvent) error {
	if len(strings.Split(event.Repo.GetFullName(), "/")) != 2 {
		return errors.New("Repo name is invalid (expected [owner]/[repo])")
	}

	// check if tag is valid for project
	if event.GetRef() == "" {
		return errors.New("Base ref is empty")
	}

	if event.GetSender().GetLogin() == "" || event.GetSender().GetID() == 0 {
		return errors.New("Github sender missing login name or uid")
	}

	if event.GetPusher().GetName() == "" {
		return errors.New("Github pusher missing login name")
	}
	if !event.GetCreated() {
		return errors.New("not a tag creation event")
	}
	return nil
}

func (gh *githubHookApi) commitQueueEnqueue(ctx context.Context, event *github.IssueCommentEvent) error {
	userRepo := data.UserRepoInfo{
		Username: *event.Comment.User.Login,
		Owner:    *event.Repo.Owner.Login,
		Repo:     *event.Repo.Name,
	}
	authorized, err := gh.sc.IsAuthorizedToPatchAndMerge(ctx, gh.settings, userRepo)
	if err != nil {
		return errors.Wrap(err, "can't get user info from GitHub API")
	}
	if !authorized {
		return errors.Errorf("user '%s' is not authorized to merge", userRepo.Username)
	}

	prNum := *event.Issue.Number
	pr, err := gh.sc.GetGitHubPR(ctx, userRepo.Owner, userRepo.Repo, prNum)
	if err != nil {
		return errors.Wrap(err, "can't get PR from GitHub API")
	}

	if pr == nil || pr.Base == nil || pr.Base.Ref == nil {
		return errors.New("PR contains no base branch label")
	}

	cqInfo := restModel.ParseGitHubComment(*event.Comment.Body)
	baseBranch := *pr.Base.Ref
	projectRef, err := gh.sc.GetProjectWithCommitQueueByOwnerRepoAndBranch(userRepo.Owner, userRepo.Repo, baseBranch)
	if err != nil {
		return errors.Wrapf(err, "can't get project for '%s:%s' tracking branch '%s'", userRepo.Owner, userRepo.Repo, baseBranch)
	}
	if projectRef == nil {
		return errors.Errorf("no project with commit queue enabled for '%s:%s' tracking branch '%s'", userRepo.Owner, userRepo.Repo, baseBranch)
	}

	if utility.FromBoolPtr(projectRef.CommitQueue.RequireSigned) {
		err = gh.requireSigned(ctx, userRepo, *pr.Head.Ref, pr, prNum)
		if err != nil {
			sendErr := thirdparty.SendCommitQueueGithubStatus(pr, message.GithubStateFailure, "can't enqueue with unsigned commits", "")
			grip.Error(message.WrapError(sendErr, message.Fields{
				"message": "error sending patch creation failure to github",
				"owner":   userRepo.Owner,
				"repo":    userRepo.Repo,
				"pr":      prNum,
			}))
			return errors.Wrapf(err, "can't enqueue patch")
		}
	}

	patchId, err := gh.sc.AddPatchForPr(ctx, *projectRef, prNum, cqInfo.Modules, cqInfo.MessageOverride)
	if err != nil {
		sendErr := thirdparty.SendCommitQueueGithubStatus(pr, message.GithubStateFailure, "failed to create patch", "")
		grip.Error(message.WrapError(sendErr, message.Fields{
			"message": "error sending patch creation failure to github",
			"owner":   userRepo.Owner,
			"repo":    userRepo.Repo,
			"pr":      prNum,
		}))
		return errors.Wrap(err, "error adding patch")
	}

	item := restModel.APICommitQueueItem{
		Issue:           utility.ToStringPtr(strconv.Itoa(prNum)),
		MessageOverride: &cqInfo.MessageOverride,
		Modules:         cqInfo.Modules,
		Source:          utility.ToStringPtr(commitqueue.SourcePullRequest),
		PatchId:         &patchId,
	}
	_, err = gh.sc.EnqueueItem(projectRef.Id, item, false)
	if err != nil {
		return errors.Wrap(err, "can't enqueue item on commit queue")
	}

	if pr == nil || pr.Head == nil || pr.Head.SHA == nil {
		return errors.New("PR contains no head branch SHA")
	}
	pushJob := units.NewGithubStatusUpdateJobForPushToCommitQueue(userRepo.Owner, userRepo.Repo, *pr.Head.SHA, prNum, patchId)
	q := evergreen.GetEnvironment().LocalQueue()
	grip.Error(message.WrapError(q.Put(ctx, pushJob), message.Fields{
		"source":  "github hook",
		"msg_id":  gh.msgID,
		"event":   gh.eventType,
		"action":  event.Action,
		"owner":   userRepo.Owner,
		"repo":    userRepo.Repo,
		"item":    prNum,
		"message": "failed to queue notification for commit queue push",
	}))

	return nil
}

func (gh *githubHookApi) requireSigned(ctx context.Context, userRepo data.UserRepoInfo, baseBranch string, pr *github.PullRequest, prNum int) error {
	settings, err := gh.sc.GetEvergreenSettings()
	if err != nil {
		return errors.Wrap(err, "can't get Evergreen settings")
	}

	githubToken, err := settings.GetGithubOauthToken()
	if err != nil {
		return errors.Wrap(err, "can't get GitHub token from settings")
	}

	commits, err := thirdparty.GetGithubPullRequestCommits(ctx, githubToken, userRepo.Owner, userRepo.Repo, prNum)
	if err != nil {
		return errors.Wrap(err, "can't get GitHub commits")
	}

	for _, c := range commits {
		commit := c.GetCommit()
		if commit.Verification != nil && !utility.FromBoolPtr(commit.Verification.Verified) &&
			(utility.FromStringPtr(commit.Verification.Reason) == commitUnsigned) {
			return errors.Errorf("the commit '%s' is not signed", utility.FromStringPtr(commit.SHA))
		}

	}
	return nil
}

// Because the PR isn't necessarily on a commit queue, we only error if item is on the queue and can't be removed correctly
func (gh *githubHookApi) tryDequeueCommitQueueItemForPR(pr *github.PullRequest) error {
	err := thirdparty.ValidatePR(pr)
	if err != nil {
		return errors.Wrap(err, "GitHub sent an incomplete PR")
	}

	projRef, err := gh.sc.GetProjectWithCommitQueueByOwnerRepoAndBranch(*pr.Base.Repo.Owner.Login, *pr.Base.Repo.Name, *pr.Base.Ref)
	if err != nil {
		return errors.Wrapf(err, "can't find valid project for %s/%s, branch %s", *pr.Base.Repo.Owner.Login, *pr.Base.Repo.Name, *pr.Base.Ref)
	}
	if projRef == nil {
		return nil
	}

	exists, err := gh.sc.IsItemOnCommitQueue(projRef.Id, strconv.Itoa(*pr.Number))
	if err != nil {
		return errors.Wrapf(err, "can't determine if item is on commit queue %s", projRef.Id)
	}
	if !exists {
		return nil
	}

	_, err = gh.sc.CommitQueueRemoveItem(projRef.Id, strconv.Itoa(*pr.Number), evergreen.GithubPatchUser)
	if err != nil {
		return errors.Wrapf(err, "can't remove item %d from commit queue %s", *pr.Number, projRef.Id)
	}
	return nil
}

func triggersRetry(action, comment string) bool {
	if action == "deleted" {
		return false
	}
	comment = strings.TrimSpace(comment)
	return comment == retryComment
}

func isTag(ref string) bool {
	return strings.Contains(ref, refTags)
}
