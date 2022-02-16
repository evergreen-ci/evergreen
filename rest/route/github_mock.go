package route

import (
	"context"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v34/github"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type mockGithubHookApi struct {
	queue  amboy.Queue
	secret []byte

	event     interface{}
	eventType string
	msgID     string
	sc        data.Connector
	settings  *evergreen.Settings
}

func makeMockGithubHooksRoute(sc data.Connector, queue amboy.Queue, secret []byte, settings *evergreen.Settings) gimlet.RouteHandler {
	return &mockGithubHookApi{
		sc:       sc,
		settings: settings,
		queue:    queue,
		secret:   secret,
	}
}

func (gh *mockGithubHookApi) Factory() gimlet.RouteHandler {
	return &mockGithubHookApi{
		queue:    gh.queue,
		secret:   gh.secret,
		sc:       gh.sc,
		settings: gh.settings,
	}
}

func (gh *mockGithubHookApi) Parse(ctx context.Context, r *http.Request) error {
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

func (gh *mockGithubHookApi) Run(ctx context.Context) gimlet.Responder {
	switch event := gh.event.(type) {

	case *github.IssueCommentEvent:
		if event.Issue.IsPullRequest() {
			if commitqueue.TriggersCommitQueue(*event.Action, *event.Comment.Body) {
				if err := gh.commitQueueEnqueue(ctx, event); err != nil {
					return gimlet.MakeJSONErrorResponder(err)
				}
			}
		}
	}

	return gimlet.NewJSONResponse(struct{}{})
}

func (gh *mockGithubHookApi) createVersionForTag(ctx context.Context, pRef model.ProjectRef, existingVersion *model.Version,
	revision model.Revision, tag model.GitTag, token string) (*model.Version, error) {
	if !pRef.IsGitTagVersionsEnabled() {
		return nil, nil
	}

	if !pRef.AuthorizedForGitTag(ctx, tag.Pusher, token) {
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
	// run everything in the yaml that's provided
	if gh.settings == nil {
		gh.settings, err = evergreen.GetConfig()
		if err != nil {
			return nil, errors.New("error getting settings config")
		}
	}
	projectInfo := model.ProjectInfo{}
	projectInfo.Ref = &pRef
	projectInfo.IntermediateProject = &model.ParserProject{}
	return &model.Version{
		Requester:         evergreen.GitTagRequester,
		TriggeredByGitTag: metadata.GitTag,
	}, nil
}

func (gh *mockGithubHookApi) commitQueueEnqueue(ctx context.Context, event *github.IssueCommentEvent) error {
	userRepo := data.UserRepoInfo{
		Username: *event.Comment.User.Login,
		Owner:    *event.Repo.Owner.Login,
		Repo:     *event.Repo.Name,
	}
	authorized, err := gh.sc.MockIsAuthorizedToPatchAndMerge(ctx, gh.settings, userRepo)
	if err != nil {
		return errors.Wrap(err, "can't get user info from GitHub API")
	}
	if !authorized {
		return errors.Errorf("user '%s' is not authorized to merge", userRepo.Username)
	}

	prNum := *event.Issue.Number
	pr, err := gh.sc.MockGetGitHubPR(ctx, userRepo.Owner, userRepo.Repo, prNum)
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

	patchId, err := gh.sc.MockAddPatchForPr(ctx, *projectRef, prNum, cqInfo.Modules, cqInfo.MessageOverride)
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
	_, err = gh.sc.MockEnqueueItem(projectRef.Id, item, false)
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
