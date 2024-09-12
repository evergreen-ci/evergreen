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
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/githubapp"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/repotracker"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v52/github"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	githubActionClosed          = "closed"
	githubActionOpened          = "opened"
	githubActionSynchronize     = "synchronize"
	githubActionReopened        = "reopened"
	githubActionChecksRequested = "checks_requested"
	githubActionRerequested     = "rerequested"

	// pull request comments
	retryComment            = "evergreen retry"
	refreshStatusComment    = "evergreen refresh"
	patchComment            = "evergreen patch"
	aliasArgument           = "--alias"
	commitQueueMergeComment = "evergreen merge"
	evergreenHelpComment    = "evergreen help"
	keepDefinitionsComment  = "evergreen keep-definitions"
	resetDefinitionsComment = "evergreen reset-definitions"

	refTags = "refs/tags/"

	// skipCIDescriptionCharLimit is the maximum number of characters that will
	// be scanned in the PR description for a skip CI label.
	skipCIDescriptionCharLimit = 100

	// githubWebhookTimeout is the maximum timeout for processing a GitHub webhook.
	githubWebhookTimeout = 60 * time.Second
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
	comments  githubComments
}

func makeGithubHooksRoute(sc data.Connector, queue amboy.Queue, secret []byte, settings *evergreen.Settings) gimlet.RouteHandler {
	return &githubHookApi{
		sc:       sc,
		settings: settings,
		queue:    queue,
		secret:   secret,
		comments: newGithubComments(settings.Ui.UIv2Url),
	}
}

func (gh *githubHookApi) Factory() gimlet.RouteHandler {
	return &githubHookApi{
		queue:    gh.queue,
		secret:   gh.secret,
		sc:       gh.sc,
		settings: gh.settings,
		comments: newGithubComments(gh.settings.Ui.UIv2Url),
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
	hasApp, err := githubapp.CreateGitHubAppAuth(gh.settings).IsGithubAppInstalledOnRepo(ctx, owner, repo)
	if err != nil {
		hasApp = false
	}
	return (fromApp && !hasApp) || (!fromApp && hasApp)
}

func (gh *githubHookApi) Run(ctx context.Context) gimlet.Responder {
	// GitHub occasionally aborts requests early before we are able to complete the full operation
	// (for example enqueueing a PR to the commit queue). We therefore want to use a custom context
	// instead of using the request context.
	ctx, cancel := context.WithTimeout(context.Background(), githubWebhookTimeout)
	defer cancel()

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
			if err := gh.AddIntentForPR(ctx, event.PullRequest, event.Sender.GetLogin(), patch.AutomatedCaller, "", false); err != nil {
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

	case *github.CheckRunEvent:
		if event.GetAction() == githubActionRerequested {
			return gh.handleCheckRunRerequested(ctx, event)
		}

	case *github.CheckSuiteEvent:
		if event.GetAction() == githubActionRerequested {
			return gh.handleCheckSuiteRerequested(ctx, event)
		}
	}

	return gimlet.NewJSONResponse(struct{}{})
}

func (gh *githubHookApi) rerunCheckRun(ctx context.Context, owner, repo string, uid int, checkRun *github.CheckRun) error {
	taskIDFromCheckrun := checkRun.GetExternalID()
	if taskIDFromCheckrun == "" {
		grip.Error(message.Fields{
			"source":  "GitHub hook",
			"msg_id":  gh.msgID,
			"event":   gh.eventType,
			"owner":   owner,
			"repo":    repo,
			"message": "check run GitHub event doesn't carry task",
		})
		return errors.New("check run GitHub event doesn't carry task")
	}
	taskToRestart, taskErr := data.FindTask(taskIDFromCheckrun)
	if taskErr != nil {
		grip.Error(message.Fields{
			"source":  "GitHub hook",
			"msg_id":  gh.msgID,
			"event":   gh.eventType,
			"owner":   owner,
			"repo":    repo,
			"task":    taskIDFromCheckrun,
			"message": "finding task",
		})
		return errors.Wrapf(taskErr, "finding task '%s' for check run", taskIDFromCheckrun)
	}
	if !utility.StringSliceContains(evergreen.TaskCompletedStatuses, taskToRestart.Status) {
		return errors.Errorf("task '%s' is not in a completed state", taskIDFromCheckrun)
	}
	githubUser, err := user.FindByGithubUID(uid)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"source":  "GitHub hook",
			"msg_id":  gh.msgID,
			"event":   gh.eventType,
			"owner":   owner,
			"repo":    repo,
			"user":    uid,
			"message": "finding user by GitHub ID",
		}))
		return errors.Wrapf(err, "finding user by GitHub ID '%d'", uid)
	}
	if githubUser == nil {
		return errors.Errorf("user with GitHub ID '%d' not found", uid)
	}
	if err := model.ResetTaskOrDisplayTask(ctx, gh.settings, taskToRestart, githubUser.Id, evergreen.GithubCheckRun, false, nil); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"source":  "GitHub hook",
			"msg_id":  gh.msgID,
			"event":   gh.eventType,
			"owner":   owner,
			"repo":    repo,
			"task":    taskIDFromCheckrun,
			"message": "restarting task",
		}))
		return errors.Wrap(err, "resetting task")
	}

	output := &github.CheckRunOutput{
		Title:   utility.ToStringPtr("Task restarted"),
		Summary: utility.ToStringPtr("Please wait for task to complete"),
	}

	// Get the task again to ensure we have the latest execution for link to task.
	// Should still update check run even if task isn't refreshed.
	latestExecutionForTask, taskErr := data.FindTask(taskIDFromCheckrun)
	if taskErr != nil {
		grip.Error(message.Fields{
			"source":  "GitHub hook",
			"msg_id":  gh.msgID,
			"event":   gh.eventType,
			"owner":   owner,
			"repo":    repo,
			"task":    taskIDFromCheckrun,
			"message": "finding refreshed task",
		})
		latestExecutionForTask = taskToRestart
	}

	// Check run status should stay the same while task is being re-run.
	latestExecutionForTask.Status = taskToRestart.Status
	_, err = thirdparty.UpdateCheckRun(ctx, owner, repo, gh.settings.ApiUrl, checkRun.GetID(), latestExecutionForTask, output)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"source":    "GitHub hook",
			"msg_id":    gh.msgID,
			"event":     gh.eventType,
			"owner":     owner,
			"repo":      repo,
			"check_run": checkRun.GetName(),
			"message":   "updating check run",
		}))
		return errors.Wrap(err, "updating check run")
	}
	return nil
}

// handleCheckRunRerequested restarts the task associated with the check run that was rerequested to be re-run and
// updates the check run to indicate that the task has been restarted.
func (gh *githubHookApi) handleCheckRunRerequested(ctx context.Context, event *github.CheckRunEvent) gimlet.Responder {
	owner := event.Repo.Owner.GetLogin()
	repo := event.Repo.GetName()

	checkRun := event.GetCheckRun()
	if checkRun == nil {
		grip.Error(message.Fields{
			"source":  "GitHub hook",
			"msg_id":  gh.msgID,
			"event":   gh.eventType,
			"owner":   owner,
			"repo":    repo,
			"message": "unable to find check run in event",
		})
		return gimlet.NewJSONInternalErrorResponse(errors.New("check run not sent from GitHub event"))
	}

	err := gh.rerunCheckRun(ctx, owner, repo, int(event.GetSender().GetID()), checkRun)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "rerunning check run '%s'", checkRun.GetName()))
	}
	return nil
}

// handleCheckSuiteRerequested restarts the task associated with the check suite that was rerequested to be re-run and
// updates the check suite to indicate that the task has been restarted.
func (gh *githubHookApi) handleCheckSuiteRerequested(ctx context.Context, event *github.CheckSuiteEvent) gimlet.Responder {
	owner := event.Repo.Owner.GetLogin()
	repo := event.Repo.GetName()
	checkRunIDs, err := thirdparty.ListCheckRunCheckSuite(ctx, owner, repo, event.CheckSuite.GetID())
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"source":      "GitHub hook",
			"msg_id":      gh.msgID,
			"event":       gh.eventType,
			"owner":       owner,
			"repo":        repo,
			"check_suite": event.CheckSuite.GetID(),
			"message":     "unable to list check runs for check suite",
		}))
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "listing check runs for check suite"))
	}
	catcher := grip.NewBasicCatcher()
	for _, checkRunID := range checkRunIDs {
		checkRun, err := thirdparty.GetCheckRun(ctx, owner, repo, checkRunID)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"source":    "GitHub hook",
				"msg_id":    gh.msgID,
				"event":     gh.eventType,
				"owner":     owner,
				"repo":      repo,
				"check_run": checkRunID,
				"message":   "unable to find check run in event",
			}))
			catcher.Add(errors.Wrapf(err, "getting check run '%d'", checkRunID))
		}
		if err := gh.rerunCheckRun(ctx, owner, repo, int(event.GetSender().GetID()), checkRun); err != nil {
			catcher.Add(errors.Wrapf(err, "rerunning check run '%d'", checkRunID))
		}
	}
	if catcher.HasErrors() {
		return gimlet.NewJSONInternalErrorResponse(catcher.Resolve())
	}
	return nil
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
		"branch":   branch,
		"base_sha": event.GetMergeGroup().GetBaseSHA(),
		"head_sha": event.GetMergeGroup().GetHeadSHA(),
		"message":  "merge group received",
	})
	if err := gh.AddIntentForGithubMerge(event); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"source":   "GitHub hook",
			"msg_id":   gh.msgID,
			"event":    gh.eventType,
			"org":      org,
			"repo":     repo,
			"branch":   branch,
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

	if triggerPatch := triggersPatch(commentBody); triggerPatch {
		callerType := parsePRCommentForCaller(commentBody)
		var alias string
		if isPatchComment(commentBody) {
			alias = parsePRCommentForAlias(commentBody)
		}
		grip.Info(gh.getCommentLogWithMessage(event, fmt.Sprintf("'%s' triggered", commentBody)))

		err := gh.createPRPatch(ctx, event.Repo.Owner.GetLogin(), event.Repo.GetName(), callerType, alias, event.Issue.GetNumber())
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
		res += fmt.Sprintf(formatStr, patchComment, "attempts to create a new PR patch; "+
			"this is required to create a PR patch when only manual PR testing is enabled (use `--alias` to specify a patch alias)")
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

func (gh *githubHookApi) createPRPatch(ctx context.Context, owner, repo, calledBy, alias string, prNumber int) error {
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

	return gh.AddIntentForPR(ctx, pr, pr.User.GetLogin(), calledBy, alias, true)
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
	if p == nil {
		return errors.Errorf("couldn't find patch for PR '%s/%s:%d'", owner, repo, prNumber)
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

// AddIntentForPR processes a PR and decides whether to create a patch for it or not. It does this by checking a
// few things:
// - If the PR has any skip CI labels in the title or description, it will skip creating a patch.
// - If overrideExisting is false, it will only create a patch if no other patch exists with the same head sha.
// - If overrideExisting is true, it will abort any other patches with the same head sha and create a new patch.
// GitHub checks only allow one check per head sha, so we need to abort any other patches with the same head sha
// before creating a new one. For example, if multiple patches with the same head sha exist, the PR(s) will get
// both updates from patches and be in a race condition for which one GitHub checks shows last- so we want
// to avoid this state when possible.
func (gh *githubHookApi) AddIntentForPR(ctx context.Context, pr *github.PullRequest, owner, calledBy, alias string, overrideExisting bool) error {
	// Verify that the owner/repo uses PR testing before inserting the intent.
	baseRepoName := pr.Base.Repo.GetFullName()
	baseRepo := strings.Split(baseRepoName, "/")
	projectRef, err := model.FindOneProjectRefByRepoAndBranchWithPRTesting(baseRepo[0],
		baseRepo[1], pr.Base.GetRef(), calledBy)
	if err != nil {
		return errors.Wrap(err, "finding project ref for patch")
	}
	if projectRef == nil {
		return nil
	}
	var mergeBase string
	if projectRef.OldestAllowedMergeBase != "" {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		baseOwnerAndRepo := strings.Split(pr.Base.Repo.GetFullName(), "/")
		if len(baseOwnerAndRepo) != 2 {
			return errors.New("PR base repo name is invalid (expected [owner]/[repo])")
		}
		mergeBase, err = thirdparty.GetGithubMergeBaseRevision(ctx, "", baseOwnerAndRepo[0], baseOwnerAndRepo[1], pr.Base.GetRef(), pr.Head.GetRef())
		if err != nil {
			return errors.Wrapf(err, "getting merge base between branches '%s' and '%s'", pr.Base.GetRef(), pr.Head.GetRef())
		}
	}
	ghi, err := patch.NewGithubIntent(gh.msgID, owner, calledBy, alias, mergeBase, pr)
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
				"message": "skipping CI on PR due to skip label in title/description",
				"owner":   pr.Base.User.GetLogin(),
				"repo":    pr.Base.Repo.GetName(),
				"ref":     pr.Head.GetRef(),
				"pr_num":  pr.GetNumber(),
				"label":   label,
			})
			return nil
		}
	}

	conflictingPatches, err := getOtherPatchesWithHash(pr.Head.GetSHA(), pr.GetNumber())
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":           "error getting same hash patches",
			"owner":             pr.Base.User.GetLogin(),
			"repo":              pr.Base.Repo.GetName(),
			"ref":               pr.Head.GetRef(),
			"pr_num":            pr.GetNumber(),
			"hash":              pr.Head.GetSHA(),
			"override_existing": overrideExisting,
		}))
		return errors.Wrapf(err, "getting same hash patches")
	}

	// If no conflicting patches exist, we can create the patch
	if len(conflictingPatches) == 0 {
		return errors.Wrap(data.AddPRPatchIntent(ghi, gh.queue), "saving GitHub patch intent")
	}

	// If we don't want to override any existing patches, comment to inform the user and no-op.
	if !overrideExisting {
		// We want to comment on explaining why a new patch will not be ran.
		grip.Info(message.Fields{
			"message":         "skipping CI on PR due to patch already existing",
			"owner":           pr.Base.User.GetLogin(),
			"repo":            pr.Base.Repo.GetName(),
			"ref":             pr.Head.GetRef(),
			"pr_num":          pr.GetNumber(),
			"head_sha":        pr.Head.GetSHA(),
			"conflicting_prs": gh.comments.getLinksForPRPatches(conflictingPatches),
		})
		return gh.sc.AddCommentToPR(ctx, pr.Base.User.GetLogin(), pr.Base.Repo.GetName(), pr.GetNumber(), gh.comments.existingPatches(conflictingPatches))
	}

	// If we do want to override any existing patches, we override them and then create a new patch.
	if err = gh.overrideOtherPRs(ctx, pr, conflictingPatches); err != nil {
		return errors.Wrap(err, "overriding other PRs")
	}

	return errors.Wrap(data.AddPRPatchIntent(ghi, gh.queue), "saving GitHub patch intent")
}

// overrideOtherPRs aborts the given patches and comments on each patch's PR to inform the user that their patch
// was aborted in favor of the new one. If any errors from commenting occur, we still abort the patches, log them,
// and ignore them. If any errors from aborting patches occur, we comment again to inform the user that there was
// an error and to retry- and we fail the entire operation.
func (gh *githubHookApi) overrideOtherPRs(ctx context.Context, pr *github.PullRequest, patches []patch.Patch) error {
	grip.Info(message.Fields{
		"message":         "aborting existing CI on same SHA patches",
		"owner":           pr.Base.User.GetLogin(),
		"repo":            pr.Base.Repo.GetName(),
		"ref":             pr.Head.GetRef(),
		"pr_num":          pr.GetNumber(),
		"conflicting_prs": gh.comments.getLinksForPRPatches(patches),
	})

	commentsCatcher := grip.NewBasicCatcher()
	cancelCatcher := grip.NewBasicCatcher()
	for _, p := range patches {
		commentsCatcher.Wrap(gh.sc.AddCommentToPR(ctx, p.GithubPatchData.BaseOwner, p.GithubPatchData.BaseRepo, p.GithubPatchData.PRNumber, gh.comments.overriddenPR(pr)), "adding comment to overridden PR")
		err := model.CancelPatch(ctx, &p, task.AbortInfo{User: evergreen.GithubPatchUser, NewVersion: "", PRClosed: false})
		if err == nil {
			continue
		}
		cancelCatcher.Wrap(err, "canceling patch")
		commentsCatcher.Wrap(gh.sc.AddCommentToPR(ctx, pr.Base.User.GetLogin(), pr.Base.Repo.GetName(), pr.GetNumber(), "There was an issue aborting the other patches, please try 'evergreen retry' again."), "adding comment to overriding PR when error")
	}

	grip.Warning(message.WrapError(commentsCatcher.Resolve(), message.Fields{
		"message": "commenting on patches with same hash to cancel",
		"owner":   pr.Base.User.GetLogin(),
		"repo":    pr.Base.Repo.GetName(),
		"ref":     pr.Head.GetRef(),
		"pr_num":  pr.GetNumber(),
	}))
	grip.Error(message.WrapError(cancelCatcher.Resolve(), message.Fields{
		"message": "cancelling patches with same hash",
		"owner":   pr.Base.User.GetLogin(),
		"repo":    pr.Base.Repo.GetName(),
		"ref":     pr.Head.GetRef(),
		"pr_num":  pr.GetNumber(),
	}))

	if !cancelCatcher.HasErrors() {
		return errors.Wrap(gh.sc.AddCommentToPR(ctx, pr.Base.User.GetLogin(), pr.Base.Repo.GetName(), pr.GetNumber(), gh.comments.overridingPR(patches)), "adding comment to overriding PR")
	}

	cancelCatcher.Add(errors.Wrap(commentsCatcher.Resolve(), "commenting on patches with same hash to cancel"))
	return errors.Wrap(cancelCatcher.Resolve(), "aborting overridden patches")

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
		metadata := model.VersionMetadata{
			Revision: revision,
			GitTag:   tag,
		}
		stubVersion, dbErr := repotracker.ShellVersionFromRevision(&pRef, metadata)
		if dbErr != nil {
			grip.Error(message.WrapError(dbErr, message.Fields{
				"message":            "error creating shell version",
				"project":            pRef.Id,
				"project_identifier": pRef.Identifier,
				"revision":           revision,
			}))
		}
		stubVersion.Errors = []string{errors.Errorf("user '%s' not authorized for git tag version", tag.Pusher).Error()}
		err := stubVersion.Insert()
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":            "error inserting stub version for failed git tag version",
				"project":            pRef.Id,
				"project_identifier": pRef.Identifier,
				"revision":           revision,
			}))
		}
		event.LogVersionStateChangeEvent(stubVersion.Id, evergreen.VersionFailed)
		userDoc, err := user.FindByGithubName(tag.Pusher)
		if err != nil {
			return nil, errors.Wrapf(err, "finding user '%s'", tag.Pusher)
		}
		if userDoc != nil {
			sender, err := evergreen.GetEnvironment().GetSender(evergreen.SenderEmail)
			if err != nil {
				return nil, errors.Wrap(err, "getting email sender")
			}

			subject, body := unauthorizedGitTagEmail(tag.Tag, tag.Pusher, fmt.Sprintf("https://spruce.mongodb.com/project/%s/settings/github-commitqueue", pRef.Identifier))
			email := message.Email{
				Recipients:        []string{userDoc.EmailAddress},
				PlainTextContents: false,
				Subject:           subject,
				Body:              body,
			}
			composer := message.NewEmailMessage(level.Notice, email)
			sender.Send(composer)
		}
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

func getOtherPatchesWithHash(githash string, prNum int) ([]patch.Patch, error) {
	patches, err := patch.Find(patch.ByGithash(githash))
	if err != nil {
		return nil, errors.Wrapf(err, "getting same hash patches")
	}
	// Remove a patch associated with this PR if applicable.
	filtered := []patch.Patch{}
	for _, p := range patches {
		if p.GithubPatchData.PRNumber != prNum {
			filtered = append(filtered, p)
		}
	}
	return filtered, nil
}

func unauthorizedGitTagEmail(tag, user, gitTagSettingsUrl string) (string, string) {
	subject := fmt.Sprintf("Creating Evergreen version for tag '%s' has failed!", tag)
	body := fmt.Sprintf(`<html>
	<head>
	</head>
	<body>
	<p>Version creation for git tag '%s' has failed.</p>
	<p>User '%s' is not authorized to create an Evergreen git tag version</p>
	<p>Manage your git tag authorization <a href="%s">here</a> in the project settings.</p>

	</body>
	</html>
	`, tag, user, gitTagSettingsUrl)
	return subject, body
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
	// strings.Join(strings.Fields()) is to handle multiple spaces between words
	return strings.Join(strings.Fields(strings.ToLower(comment)), " ")
}

func isRetryComment(comment string) bool {
	return trimComment(comment) == retryComment
}

func isPatchComment(comment string) bool {
	return strings.HasPrefix(trimComment(comment), patchComment)
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

// Return whether the patch should be created or not.
func triggersPatch(comment string) bool {
	return isPatchComment(comment) || isRetryComment(comment)
}

// Return the caller for the command.
func parsePRCommentForCaller(comment string) string {
	if isPatchComment(comment) {
		return patch.ManualCaller
	}
	if isRetryComment(comment) {
		return patch.AllCallers
	}
	return ""
}

// Return the alias from a comment like `evergreen patch --alias <alias>`
func parsePRCommentForAlias(comment string) string {
	comment = strings.TrimSpace(comment)
	expectedPrefix := strings.Join([]string{patchComment, aliasArgument}, " ")
	if !strings.HasPrefix(comment, expectedPrefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(comment, expectedPrefix))
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
