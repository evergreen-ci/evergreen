package route

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v52/github"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	githubActionClosed          = "closed"
	githubActionOpened          = "opened"
	githubActionSynchronize     = "synchronize"
	githubActionReopened        = "reopened"
	githubActionChecksRequested = "checks_requested"

	// pull request comments
	retryComment            = "evergreen retry"
	refreshStatusComment    = "evergreen refresh"
	patchComment            = "evergreen patch"
	commitQueueMergeComment = "evergreen merge"
	evergreenHelpComment    = "evergreen help"
	keepDefinitionsComment  = "evergreen keep-definitions"
	resetDefinitionsComment = "evergreen reset-definitions"

	refTags = "refs/tags/"

	// skipCIDescriptionCharLimit is the maximum number of characters that will
	// be scanned in the PR description for a skip CI label.
	skipCIDescriptionCharLimit = 100
)

// skipCILabels are a set of labels which will skip creating PR patch if part of
// the PR title or description.
var skipCILabels = []string{"[skip ci]", "[skip-ci]"}

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
	payload := getGitHubPayload(r.Context())
	if payload == nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "payload not in context",
		}
	}

	gh.eventType = r.Header.Get("X-Github-Event")
	gh.msgID = r.Header.Get("X-Github-Delivery")

	if len(gh.secret) == 0 || gh.queue == nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "webhooks are not configured and therefore disabled",
		}
	}

	var err error
	gh.event, err = github.ParseWebHook(gh.eventType, payload)
	if err != nil {
		return errors.Wrap(err, "parsing webhook")
	}

	return nil
}

// shouldSkipWebhook returns true if the event is from a GitHub app and the app is available for the owner/repo or,
// the event is from webhooks and the app is not available for the owner/repo.
func (gh *githubHookApi) shouldSkipWebhook(ctx context.Context, owner, repo string, fromApp bool) bool {
	hasApp, err := gh.settings.HasGitHubApp(ctx, owner, repo)
	if err != nil {
		hasApp = false
	}
	return (fromApp && !hasApp) || (!fromApp && hasApp)
}

func (gh *githubHookApi) Run(ctx context.Context) gimlet.Responder {
	switch event := gh.event.(type) {
	case *github.PingEvent:
		fromApp := event.GetInstallation() != nil
		if gh.shouldSkipWebhook(ctx, event.Repo.Owner.GetLogin(), event.Repo.GetName(), fromApp) {
			break
		}
		if event.HookID == nil {
			return gimlet.NewJSONErrorResponse(gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "malformed ping event",
			})
		}
		grip.Info(message.Fields{
			"source":  "GitHub hook",
			"msg_id":  gh.msgID,
			"event":   gh.eventType,
			"hook_id": event.HookID,
		})

	case *github.PullRequestEvent:
		fromApp := event.GetInstallation() != nil
		if gh.shouldSkipWebhook(ctx, event.Repo.Owner.GetLogin(), event.Repo.GetName(), fromApp) {
			break
		}
		if event.Action == nil {
			err := gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "pull request has no action",
			}
			grip.Error(message.WrapError(err, message.Fields{
				"source": "GitHub hook",
				"msg_id": gh.msgID,
				"event":  gh.eventType,
			}))

			return gimlet.NewJSONErrorResponse(err)
		}

		action := utility.FromStringPtr(event.Action)
		if action == githubActionOpened || action == githubActionSynchronize ||
			action == githubActionReopened {
			grip.Info(message.Fields{
				"source":    "GitHub hook",
				"msg_id":    gh.msgID,
				"event":     gh.eventType,
				"repo":      *event.PullRequest.Base.Repo.FullName,
				"ref":       *event.PullRequest.Base.Ref,
				"pr_number": *event.PullRequest.Number,
				"hash":      *event.PullRequest.Head.SHA,
				"user":      *event.Sender.Login,
				"message":   "PR accepted, attempting to queue",
			})
			if err := gh.AddIntentForPR(event.PullRequest, event.Sender.GetLogin(), patch.AutomatedCaller); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"source":    "GitHub hook",
					"msg_id":    gh.msgID,
					"event":     gh.eventType,
					"repo":      *event.PullRequest.Base.Repo.FullName,
					"ref":       *event.PullRequest.Base.Ref,
					"pr_number": *event.PullRequest.Number,
					"user":      *event.Sender.Login,
					"message":   "can't add intent",
				}))
				return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "adding patch intent"))
			}
		} else if action == githubActionClosed {
			grip.Info(message.Fields{
				"source":  "GitHub hook",
				"msg_id":  gh.msgID,
				"event":   gh.eventType,
				"message": "pull request closed; aborting patch",
			})

			if err := data.AbortPatchesFromPullRequest(ctx, event); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"source":  "GitHub hook",
					"msg_id":  gh.msgID,
					"event":   gh.eventType,
					"action":  *event.Action,
					"message": "failed to abort patches",
				}))
				return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "aborting patches"))
			}

			return gimlet.NewJSONResponse(struct{}{})
		}
	case *github.PushEvent:
		fromApp := event.GetInstallation() != nil
		if gh.shouldSkipWebhook(ctx, event.Repo.Owner.GetLogin(), event.Repo.GetName(), fromApp) {
			break
		}
		grip.Debug(message.Fields{
			"source":     "GitHub hook",
			"msg_id":     gh.msgID,
			"event":      gh.eventType,
			"event_data": event,
			"ref":        event.GetRef(),
			"is_tag":     isTag(event.GetRef()),
		})
		// Regardless of whether a tag or commit is being pushed, we want to trigger the repotracker
		// to ensure we're up-to-date on the commit the tag is being pushed to.
		if err := data.TriggerRepotracker(ctx, gh.queue, gh.msgID, event); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "triggering repotracker"))
		}
		if isTag(event.GetRef()) {
			if err := gh.handleGitTag(ctx, event); err != nil {
				return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "handling git tag"))
			}
			return gimlet.NewJSONResponse(struct{}{})
		}

	case *github.IssueCommentEvent:
		fromApp := event.GetInstallation() != nil
		if gh.shouldSkipWebhook(ctx, event.Repo.Owner.GetLogin(), event.Repo.GetName(), fromApp) {
			break
		}
		if err := gh.handleComment(ctx, event); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(err)
		}

	case *github.MergeGroupEvent:
		fromApp := event.GetInstallation() != nil
		if gh.shouldSkipWebhook(ctx, event.Repo.Owner.GetLogin(), event.Repo.GetName(), fromApp) {
			break
		}
		if event.GetAction() == githubActionChecksRequested {
			return gh.handleMergeGroupChecksRequested(event)
		}
	}

	return gimlet.NewJSONResponse(struct{}{})
}

func (gh *githubHookApi) handleMergeGroupChecksRequested(event *github.MergeGroupEvent) gimlet.Responder {
	org := event.GetOrg().GetLogin()
	repo := event.GetRepo().GetName()
	branch := strings.TrimPrefix(event.MergeGroup.GetBaseRef(), "refs/heads/")
	grip.Info(message.Fields{
		"source":   "GitHub hook",
		"msg_id":   gh.msgID,
		"event":    gh.eventType,
		"org":      org,
		"repo":     repo,
		"base_sha": event.GetMergeGroup().GetBaseSHA(),
		"head_sha": event.GetMergeGroup().GetHeadSHA(),
		"message":  "merge group received",
	})
	ref, err := model.FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch(org, repo, branch)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"source":   "GitHub hook",
			"msg_id":   gh.msgID,
			"event":    gh.eventType,
			"org":      org,
			"repo":     repo,
			"base_sha": event.GetMergeGroup().GetBaseSHA(),
			"head_sha": event.GetMergeGroup().GetHeadSHA(),
			"message":  "finding project ref",
		}))
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "finding project ref"))
	}
	if ref == nil {
		grip.Error(message.Fields{
			"source":   "GitHub hook",
			"msg_id":   gh.msgID,
			"event":    gh.eventType,
			"org":      org,
			"repo":     repo,
			"base_sha": event.GetMergeGroup().GetBaseSHA(),
			"head_sha": event.GetMergeGroup().GetHeadSHA(),
			"message":  "no matching project ref",
		})
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "no matching project ref"))
	}
	if ref.CommitQueue.MergeQueue == model.MergeQueueGitHub {
		err = gh.AddIntentForGithubMerge(event)
	} else {
		return gimlet.NewJSONResponse(struct{}{})
	}
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"source":   "GitHub hook",
			"msg_id":   gh.msgID,
			"event":    gh.eventType,
			"org":      org,
			"repo":     repo,
			"base_sha": event.GetMergeGroup().GetBaseSHA(),
			"head_sha": event.GetMergeGroup().GetHeadSHA(),
			"message":  "adding project intent",
		}))
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "adding patch intent"))
	}
	return nil
}

// AddIntentForGithubMerge creates and inserts an intent document in response to a GitHub merge group event.
func (gh *githubHookApi) AddIntentForGithubMerge(mg *github.MergeGroupEvent) error {
	intent, err := patch.NewGithubMergeIntent(gh.msgID, patch.AutomatedCaller, mg)
	if err != nil {
		return errors.Wrap(err, "creating GitHub merge intent")
	}
	if err := data.AddGithubMergeIntent(intent, gh.queue); err != nil {
		return errors.Wrap(err, "saving GitHub merge intent")
	}
	return nil
}

// handleComment parses a given comment and takes the relevant action, if it's an Evergreen-tracked comment.
func (gh *githubHookApi) handleComment(ctx context.Context, event *github.IssueCommentEvent) error {
	if !event.Issue.IsPullRequest() {
		return nil
	}

	commentBody := event.Comment.GetBody()
	commentAction := event.GetAction()
	if commentAction == "deleted" {
		return nil
	}
	if triggersCommitQueue(event.Comment.GetBody()) {
		grip.Info(gh.getCommentLogWithMessage(event, "commit queue triggered"))

		_, err := data.EnqueuePRToCommitQueue(ctx, evergreen.GetEnvironment(), gh.sc, createEnqueuePRInfo(event))
		if err != nil {
			grip.Error(message.WrapError(err, gh.getCommentLogWithMessage(event, "can't enqueue on commit queue")))
			return errors.Wrap(err, "enqueueing in commit queue")
		}

		return nil
	}

	if triggerPatch, callerType := triggersPatch(commentBody); triggerPatch {
		grip.Info(gh.getCommentLogWithMessage(event, fmt.Sprintf("'%s' triggered", commentBody)))

		err := gh.createPRPatch(ctx, event.Repo.Owner.GetLogin(), event.Repo.GetName(), callerType, event.Issue.GetNumber())
		grip.Error(message.WrapError(err, gh.getCommentLogWithMessage(event,
			fmt.Sprintf("can't create PR for '%s'", commentBody))))
		return errors.Wrap(err, "creating patch")
	}

	if triggersStatusRefresh(commentBody) {
		grip.Info(gh.getCommentLogWithMessage(event, fmt.Sprintf("'%s' triggered", commentBody)))

		err := gh.refreshPatchStatus(ctx, event.Repo.Owner.GetLogin(), event.Repo.GetName(), event.Issue.GetNumber())
		grip.Error(message.WrapError(err, gh.getCommentLogWithMessage(event,
			"problem triggering status refresh")))
		return errors.Wrap(err, "triggering status refresh")
	}

	if triggersHelpText(commentBody) {
		grip.Info(gh.getCommentLogWithMessage(event, fmt.Sprintf("'%s' triggered", commentBody)))
		err := gh.displayHelpText(ctx, event.Repo.Owner.GetLogin(), event.Repo.GetName(), event.Issue.GetNumber())
		grip.Error(message.WrapError(err, gh.getCommentLogWithMessage(event,
			"problem sending help comment")))
		return errors.Wrap(err, "sending help comment")
	}

	if isKeepDefinitionsComment(commentBody) {
		grip.Info(gh.getCommentLogWithMessage(event, fmt.Sprintf("'%s' triggered", commentBody)))

		err := keepPRPatchDefinition(event.Repo.Owner.GetLogin(), event.Repo.GetName(), event.Issue.GetNumber())

		grip.Error(message.WrapError(err, gh.getCommentLogWithMessage(event,
			"problem keeping pr patch definitions")))

		return errors.Wrap(err, "keeping pr patch definition")

	}

	if isResetDefinitionsComment(commentBody) {
		grip.Info(gh.getCommentLogWithMessage(event, fmt.Sprintf("'%s' triggered", commentBody)))

		err := resetPRPatchDefinition(event.Repo.Owner.GetLogin(), event.Repo.GetName(), event.Issue.GetNumber())

		grip.Error(message.WrapError(err, gh.getCommentLogWithMessage(event,
			"problem resetting pr patch definitions")))

		return errors.Wrap(err, "resetting pr patch definition")

	}

	return nil
}

func (gh *githubHookApi) getCommentLogWithMessage(event *github.IssueCommentEvent, msg string) message.Fields {
	return message.Fields{
		"source":    "GitHub hook",
		"msg_id":    gh.msgID,
		"event":     gh.eventType,
		"repo":      *event.Repo.FullName,
		"pr_number": *event.Issue.Number,
		"user":      *event.Sender.Login,
		"message":   msg,
	}
}

func (gh *githubHookApi) displayHelpText(ctx context.Context, owner, repo string, prNum int) error {
	pr, err := gh.sc.GetGitHubPR(ctx, owner, repo, prNum)
	if err != nil {
		return errors.Wrap(err, "getting PR from GitHub API")
	}

	if pr == nil || pr.Base == nil || pr.Base.Ref == nil {
		return errors.New("PR contains no base branch label")
	}
	branch := pr.Base.GetRef()
	repoRef, err := model.FindRepoRefByOwnerAndRepo(owner, repo)
	if err != nil {
		return errors.Wrapf(err, "fetching repo ref for '%s'%s'", owner, repo)
	}
	projectRefs, err := model.FindMergedEnabledProjectRefsByRepoAndBranch(owner, repo, branch)
	if err != nil {
		return errors.Wrapf(err, "fetching merged project refs for repo '%s/%s' with branch '%s'",
			owner, repo, branch)
	}

	helpMarkdown := getHelpTextFromProjects(repoRef, projectRefs)
	return gh.sc.AddCommentToPR(ctx, owner, repo, prNum, helpMarkdown)
}

func getHelpTextFromProjects(repoRef *model.RepoRef, projectRefs []model.ProjectRef) string {
	var manualPRProjectEnabled, autoPRProjectEnabled, cqProjectEnabled bool
	var cqProjectMessage string

	canInheritRepoPRSettings := true
	for _, p := range projectRefs {
		if p.Enabled && p.IsManualPRTestingEnabled() {
			manualPRProjectEnabled = true
		}
		if p.Enabled && p.IsAutoPRTestingEnabled() {
			autoPRProjectEnabled = true
		}
		// If the project is explicitly disabled, then we shouldn't consider if the repo allows for PR testing.
		if !p.Enabled {
			canInheritRepoPRSettings = false
		}

		if err := p.CommitQueueIsOn(); err == nil {
			cqProjectEnabled = true
		} else if cqMessage := p.CommitQueue.Message; cqMessage != "" {
			cqProjectMessage += cqMessage
		}
	}

	// If the branch isn't explicitly disabled, also consider the repo settings.
	if canInheritRepoPRSettings && repoRef != nil {
		if repoRef.IsManualPRTestingEnabled() {
			manualPRProjectEnabled = true
		}
		if repoRef.IsAutoPRTestingEnabled() {
			autoPRProjectEnabled = true
		}
	}

	formatStr := "- `%s`    - %s\n"
	formatStrStrikethrough := "- ~`%s`~ \n    - %s\n"
	res := fmt.Sprintf("### %s\n", "Available Evergreen Comment Commands")
	if autoPRProjectEnabled {
		res += fmt.Sprintf(formatStr, retryComment, "attempts to create a new PR patch; "+
			"this is useful when something went wrong with automatically creating PR patches")
	}
	if manualPRProjectEnabled {
		res += fmt.Sprintf(formatStr, patchComment, "attempts to create a new PR patch; "+""+
			"this is required to create a PR patch when only manual PR testing is enabled")
	}
	if autoPRProjectEnabled || manualPRProjectEnabled {
		res += fmt.Sprintf(formatStr, keepDefinitionsComment, "reuse the tasks from the previous patch in subsequent patches")
		res += fmt.Sprintf(formatStr, resetDefinitionsComment, "reset the patch tasks to the original definition")
		res += fmt.Sprintf(formatStr, refreshStatusComment, "resyncs PR GitHub checks")
	}
	if cqProjectEnabled {
		res += fmt.Sprintf(formatStr, commitQueueMergeComment, "adds PR to the commit queue")
	} else if cqProjectMessage != "" {
		// Should only display this if the commit queue isn't enabled for a different branch.
		text := fmt.Sprintf("the commit queue is NOT enabled for this branch: %s", cqProjectMessage)
		res += fmt.Sprintf(formatStrStrikethrough, commitQueueMergeComment, text)
	}
	if !autoPRProjectEnabled && !manualPRProjectEnabled && !cqProjectEnabled && cqProjectMessage == "" {
		res = "No commands available for this branch."
	}
	return res
}

func (gh *githubHookApi) createPRPatch(ctx context.Context, owner, repo, calledBy string, prNumber int) error {
	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return errors.Wrap(err, "getting admin settings")
	}
	githubToken, err := settings.GetGithubOauthToken()
	if err != nil {
		return errors.Wrap(err, "getting GitHub OAuth token from admin settings")
	}

	pr, err := thirdparty.GetGithubPullRequest(ctx, githubToken, owner, repo, prNumber)
	if err != nil {
		return errors.Wrapf(err, "getting PR for repo '%s:%s', PR #%d", owner, repo, prNumber)
	}

	return gh.AddIntentForPR(pr, pr.User.GetLogin(), calledBy)
}

// keepPRPatchDefinition looks for the most recent patch created for the pr number and updates the
// RepeatPatchIdNextPatch field in the githubPatchData to the patch id of the latest patch.
// When the next github patch intent is created for that PR, it will look at this field on the last pr patch
// to determine if the task definitions should be reused from the specified ID or the default definition
func keepPRPatchDefinition(owner, repo string, prNumber int) error {
	p, err := patch.FindLatestGithubPRPatch(owner, repo, prNumber)
	if err != nil || p == nil {
		return errors.Wrap(err, "getting most recent patch for pr")
	}
	return p.UpdateRepeatPatchId(p.Id.Hex())
}

func resetPRPatchDefinition(owner, repo string, prNumber int) error {
	p, err := patch.FindLatestGithubPRPatch(owner, repo, prNumber)
	if err != nil {
		return errors.Wrap(err, "getting most recent patch for pr")
	}
	return p.UpdateRepeatPatchId("")
}

func (gh *githubHookApi) refreshPatchStatus(ctx context.Context, owner, repo string, prNumber int) error {
	p, err := patch.FindLatestGithubPRPatch(owner, repo, prNumber)
	if err != nil {
		return errors.Wrap(err, "finding patch")
	}
	if p == nil {
		return errors.Errorf("couldn't find patch for PR '%s/%s:%d'", owner, repo, prNumber)
	}
	if p.Version == "" {
		return errors.Errorf("patch '%s' not finalized", p.Version)
	}

	job := units.NewGithubStatusRefreshJob(p)
	job.Run(ctx)
	return nil
}

func (gh *githubHookApi) AddIntentForPR(pr *github.PullRequest, owner, calledBy string) error {
	ghi, err := patch.NewGithubIntent(gh.msgID, owner, calledBy, pr)
	if err != nil {
		return errors.Wrap(err, "creating GitHub patch intent")
	}
	// If there are no errors with the PR, verify that we aren't skipping CI before adding the intent.
	for _, label := range skipCILabels {
		title := strings.ToLower(pr.GetTitle())
		limitedDesc := strings.ToLower(pr.GetBody())
		if len(limitedDesc) >= skipCIDescriptionCharLimit {
			limitedDesc = limitedDesc[:skipCIDescriptionCharLimit]
		}
		if strings.Contains(title, label) || strings.Contains(limitedDesc, label) {
			grip.Info(message.Fields{
				"message": "skipping CI on PR",
				"owner":   pr.Base.User.GetLogin(),
				"repo":    pr.Base.Repo.GetName(),
				"ref":     pr.Head.GetRef(),
				"pr_num":  pr.GetNumber(),
				"label":   label,
			})
			return nil
		}
	}

	if err := data.AddPatchIntent(ghi, gh.queue); err != nil {
		return errors.Wrap(err, "saving GitHub patch intent")
	}

	return nil
}

// handleGitTag adds the tag to the version it was pushed to, and triggers a new version if applicable
func (gh *githubHookApi) handleGitTag(ctx context.Context, event *github.PushEvent) error {
	if err := validatePushTagEvent(event); err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"source":  "GitHub hook",
			"message": "error validating event",
			"ref":     event.GetRef(),
			"event":   gh.eventType,
		}))
		return errors.Wrap(err, "validating event")
	}
	token, err := gh.settings.GetGithubOauthToken()
	if err != nil {
		return errors.New("getting GitHub token")
	}
	pusher := event.GetPusher().GetName()
	tag := model.GitTag{
		Tag:    strings.TrimPrefix(event.GetRef(), refTags),
		Pusher: pusher,
	}
	ownerAndRepo := strings.Split(event.Repo.GetFullName(), "/")
	hash, err := thirdparty.GetTaggedCommitFromGithub(ctx, token, ownerAndRepo[0], ownerAndRepo[1], event.GetRef())
	if err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"source":  "GitHub hook",
			"message": "getting tagged commit from GitHub",
			"ref":     event.GetRef(),
			"event":   gh.eventType,
			"owner":   ownerAndRepo[0],
			"repo":    ownerAndRepo[1],
			"tag":     tag,
		}))
		return errors.Wrapf(err, "getting commit for tag '%s'", tag.Tag)
	}
	projectRefs, err := model.FindMergedEnabledProjectRefsByOwnerAndRepo(ownerAndRepo[0], ownerAndRepo[1])
	if err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"source":  "GitHub hook",
			"message": "error finding projects",
			"ref":     event.GetRef(),
			"event":   gh.eventType,
			"owner":   ownerAndRepo[0],
			"repo":    ownerAndRepo[1],
			"tag":     tag,
		}))
		return errors.Wrapf(err, "finding projects for repo '%s'", ownerAndRepo)
	}
	if len(projectRefs) == 0 {
		grip.Debug(message.Fields{
			"source":  "GitHub hook",
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
				existingVersion, err = model.VersionFindOne(model.BaseVersionByProjectIdAndRevision(pRef.Id, hash))
				if err != nil {
					retryCatcher.Wrapf(err, "finding version for project '%s' with revision '%s'", pRef.Id, hash)
					continue
				}
				if existingVersion == nil {
					retryCatcher.Errorf("no version for project '%s' with revision '%s' to add tag to", pRef.Id, hash)
					continue
				}
				foundVersion[pRef.Id] = true
				grip.Debug(message.Fields{
					"source":  "GitHub hook",
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

				if err = model.AddGitTag(existingVersion.Id, tag); err != nil {
					catcher.Wrapf(err, "adding tag '%s' to version '%s''", tag.Tag, existingVersion.Id)
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
					catcher.Wrapf(err, "adding new version for tag '%s'", tag.Tag)
					continue
				}
				if v != nil {
					grip.Info(message.Fields{
						"source":  "GitHub hook",
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
		"source":  "GitHub hook",
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

	if !pRef.AuthorizedForGitTag(ctx, tag.Pusher, token, pRef.Owner, pRef.Repo) {
		grip.Debug(message.Fields{
			"source":             "GitHub hook",
			"msg_id":             gh.msgID,
			"event":              gh.eventType,
			"project":            pRef.Id,
			"project_identifier": pRef.Identifier,
			"tag":                tag,
			"message":            "user not authorized for git tag version",
		})
		return nil, nil
	}
	hasAliases, remotePath, err := model.HasMatchingGitTagAliasAndRemotePath(pRef.Id, tag.Tag)
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
		Activate:   true,
	}
	var projectInfo model.ProjectInfo
	if remotePath != "" {
		// run everything in the yaml that's provided
		if gh.settings == nil {
			gh.settings, err = evergreen.GetConfig(ctx)
			if err != nil {
				return nil, errors.Wrap(err, "getting admin settings")
			}
		}
		projectInfo, err = gh.sc.GetProjectFromFile(ctx, pRef, remotePath, token)
		if err != nil {
			return nil, errors.Wrap(err, "loading project info from file")
		}
	} else {
		// use the standard project config with the git tag alias
		projectInfo, err = model.LoadProjectInfoForVersion(ctx, gh.settings, existingVersion, pRef.Id)
		if err != nil {
			return nil, errors.Wrapf(err, "getting project '%s'", pRef.Identifier)
		}
		metadata.Alias = evergreen.GitTagAlias
	}
	projectInfo.Ref = &pRef
	return gh.sc.CreateVersionFromConfig(ctx, &projectInfo, metadata)
}

func validatePushTagEvent(event *github.PushEvent) error {
	if len(strings.Split(event.Repo.GetFullName(), "/")) != 2 {
		return errors.New("repo name is invalid (expected [owner]/[repo])")
	}

	// check if tag is valid for project
	if event.GetRef() == "" {
		return errors.New("base ref is empty")
	}

	if event.GetSender().GetLogin() == "" || event.GetSender().GetID() == 0 {
		return errors.New("GitHub sender missing login name or UID")
	}

	if event.GetPusher().GetName() == "" {
		return errors.New("GitHub pusher missing login name")
	}
	if !event.GetCreated() {
		return errors.New("not a tag creation event")
	}
	return nil
}

func createEnqueuePRInfo(event *github.IssueCommentEvent) commitqueue.EnqueuePRInfo {
	return commitqueue.EnqueuePRInfo{
		Username:      *event.Comment.User.Login,
		Owner:         *event.Repo.Owner.Login,
		Repo:          *event.Repo.Name,
		PR:            *event.Issue.Number,
		CommitMessage: *event.Comment.Body,
	}
}

func isItemOnCommitQueue(id, item string) (bool, error) {
	cq, err := commitqueue.FindOneId(id)
	if err != nil {
		return false, errors.Wrapf(err, "finding commit queue '%s'", id)
	}
	if cq == nil {
		return false, errors.Errorf("commit queue '%s' not found", id)
	}

	pos := cq.FindItem(item)
	if pos >= 0 {
		return true, nil
	}
	return false, nil
}

func trimComment(comment string) string {
	return strings.Join(strings.Fields(strings.ToLower(comment)), " ")
}

func isRetryComment(comment string) bool {
	return trimComment(comment) == retryComment
}

func isPatchComment(comment string) bool {
	return trimComment(comment) == patchComment
}

// triggersCommitQueue checks if "evergreen merge" is present in the comment, as
// it may be followed by a newline and a message.
func triggersCommitQueue(comment string) bool {
	return strings.HasPrefix(trimComment(comment), commitQueueMergeComment)
}

func isKeepDefinitionsComment(comment string) bool {
	return trimComment(comment) == keepDefinitionsComment
}

func isResetDefinitionsComment(comment string) bool {
	return trimComment(comment) == resetDefinitionsComment
}

// The bool value returns whether the patch should be created or not.
// The string value returns the correct caller for the command.
func triggersPatch(comment string) (bool, string) {
	if isPatchComment(comment) {
		return true, patch.ManualCaller
	}
	if isRetryComment(comment) {
		return true, patch.AllCallers
	}

	return false, ""
}
func triggersStatusRefresh(comment string) bool {
	return trimComment(comment) == refreshStatusComment
}

func triggersHelpText(comment string) bool {
	return trimComment(comment) == evergreenHelpComment
}

func isTag(ref string) bool {
	return strings.Contains(ref, refTags)
}
