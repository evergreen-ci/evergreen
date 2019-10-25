package route

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

type commitQueueGetHandler struct {
	project string

	sc data.Connector
}

func makeGetCommitQueueItems(sc data.Connector) gimlet.RouteHandler {
	return &commitQueueGetHandler{
		sc: sc,
	}
}

func (cq *commitQueueGetHandler) Factory() gimlet.RouteHandler {
	return &commitQueueGetHandler{
		sc: cq.sc,
	}
}

func (cq *commitQueueGetHandler) Parse(ctx context.Context, r *http.Request) error {
	cq.project = gimlet.GetVars(r)["project_id"]
	return nil
}

func (cq *commitQueueGetHandler) Run(ctx context.Context) gimlet.Responder {
	commitQueue, err := cq.sc.FindCommitQueueByID(cq.project)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "can't get commit queue"))
	}
	if commitQueue == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("no matching project for ID %s", cq.project),
		})
	}

	return gimlet.NewJSONResponse(commitQueue)
}

type commitQueueDeleteItemHandler struct {
	project string
	item    string

	sc  data.Connector
	env evergreen.Environment
}

func makeDeleteCommitQueueItems(sc data.Connector, env evergreen.Environment) gimlet.RouteHandler {
	return &commitQueueDeleteItemHandler{
		sc:  sc,
		env: env,
	}
}

func (cq commitQueueDeleteItemHandler) Factory() gimlet.RouteHandler {
	return &commitQueueDeleteItemHandler{
		sc:  cq.sc,
		env: cq.env,
	}
}

func (cq *commitQueueDeleteItemHandler) Parse(ctx context.Context, r *http.Request) error {
	vars := gimlet.GetVars(r)
	cq.project = vars["project_id"]
	cq.item = vars["item"]

	return nil
}

func (cq *commitQueueDeleteItemHandler) Run(ctx context.Context) gimlet.Responder {
	found, err := cq.sc.CommitQueueRemoveItem(cq.project, cq.item)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "can't delete item"))
	}
	if !found {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("no matching item for project ID '%s', item '%s'", cq.project, cq.item),
		})
	}

	// Send GitHub status
	projectRef, err := cq.sc.FindProjectById(cq.project)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "can't find project '%s'", cq.project))
	}
	if projectRef.CommitQueue.PatchType == commitqueue.PRPatchType {
		itemInt, err := strconv.Atoi(cq.item)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "item '%s' is not an int", cq.item))
		}
		pr, err := cq.sc.GetGitHubPR(ctx, projectRef.Owner, projectRef.Repo, itemInt)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "can't get PR for %s:%s, %d", projectRef.Owner, projectRef.Repo, itemInt))
		}
		if pr == nil || pr.Head == nil || pr.Head.GetSHA() == "" {
			return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "GitHub returned a PR missing a HEAD SHA"))
		}
		pushJob := units.NewGithubStatusUpdateJobForDeleteFromCommitQueue(projectRef.Owner, projectRef.Repo, *pr.Head.SHA, itemInt)
		q := cq.env.LocalQueue()
		err = q.Put(ctx, pushJob)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "Can't enqueue a GitHub status update"))
		}
	}

	response := gimlet.NewJSONResponse(struct{}{})
	if err := response.SetStatus(http.StatusNoContent); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "Cannot set HTTP status code to %d", http.StatusNoContent))
	}
	return response
}

type commitQueueClearAllHandler struct {
	sc data.Connector
}

func makeClearCommitQueuesHandler(sc data.Connector) gimlet.RouteHandler {
	return &commitQueueClearAllHandler{
		sc: sc,
	}
}

func (cq *commitQueueClearAllHandler) Factory() gimlet.RouteHandler {
	return &commitQueueClearAllHandler{
		sc: cq.sc,
	}
}

func (cq *commitQueueClearAllHandler) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (cq *commitQueueClearAllHandler) Run(ctx context.Context) gimlet.Responder {
	clearedCount, err := cq.sc.CommitQueueClearAll()
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "can't clear commit queues"))
	}

	return gimlet.NewJSONResponse(struct {
		ClearedCount int `json:"cleared_count"`
	}{clearedCount})
}

type commitQueueEnqueueItemHandler struct {
	item    string
	project string

	sc data.Connector
}

func makeCommitQueueEnqueueItem(sc data.Connector) gimlet.RouteHandler {
	return &commitQueueEnqueueItemHandler{
		sc: sc,
	}
}

func (cq commitQueueEnqueueItemHandler) Factory() gimlet.RouteHandler {
	return &commitQueueEnqueueItemHandler{
		sc: cq.sc,
	}
}

func (cq *commitQueueEnqueueItemHandler) Parse(ctx context.Context, r *http.Request) error {
	vars := gimlet.GetVars(r)
	cq.item = vars["patch_id"]
	patch, err := cq.sc.FindPatchById(cq.item)
	if err != nil {
		return errors.Wrap(err, "can't find item")
	}
	cq.project = patch.Project

	return nil
}

func (cq *commitQueueEnqueueItemHandler) Run(ctx context.Context) gimlet.Responder {
	position, err := cq.sc.EnqueueItem(cq.project, model.APICommitQueueItem{Issue: model.ToAPIString(cq.item)})
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "can't enqueue item"))
	}

	return gimlet.NewJSONResponse(model.APICommitQueuePosition{Position: position})
}

type getCommitQueueItemAuthorHandler struct {
	item      string
	projectID string

	sc data.Connector
}

func makeGetCommitQueueItemAuthor(sc data.Connector) gimlet.RouteHandler {
	return &getCommitQueueItemAuthorHandler{
		sc: sc,
	}
}

func (cq *getCommitQueueItemAuthorHandler) Factory() gimlet.RouteHandler {
	return &getCommitQueueItemAuthorHandler{
		sc: cq.sc,
	}
}

func (cq *getCommitQueueItemAuthorHandler) Parse(ctx context.Context, r *http.Request) error {
	vars := gimlet.GetVars(r)
	cq.item = vars["item"]
	cq.projectID = gimlet.GetVars(r)["project_id"]
	return nil
}

func (cq *getCommitQueueItemAuthorHandler) Run(ctx context.Context) gimlet.Responder {
	pRef, err := cq.sc.FindProjectById(cq.projectID)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "problem finding project"))
	}

	resp := model.APICommitQueueItemAuthor{}
	if pRef.CommitQueue.PatchType == commitqueue.CLIPatchType {
		p, err := cq.sc.FindPatchById(cq.item)
		if err != nil {
			return gimlet.NewJSONErrorResponse(errors.Wrap(err, "problem finding patch"))
		}
		resp.Author = model.ToAPIString(p.Author)
		return gimlet.NewJSONResponse(resp)
	}
	if pRef.CommitQueue.PatchType == commitqueue.PRPatchType {
		prNum, err := strconv.Atoi(cq.item)
		if err != nil {
			return gimlet.NewJSONErrorResponse(gimlet.ErrorResponse{
				Message:    "must pass a valid PR number",
				StatusCode: http.StatusBadRequest,
			})
		}
		pr, err := cq.sc.GetGitHubPR(ctx, pRef.Owner, pRef.Repo, prNum)
		if err != nil {
			return gimlet.NewJSONErrorResponse(errors.Wrap(err, "problem finding pull request"))
		}

		var author string
		if pr != nil && pr.User != nil && pr.User.ID != nil {
			dbUser, _ := user.FindByGithubUID(int(*pr.User.ID))
			if dbUser != nil {
				author = dbUser.Username()
			} else { // return github login if evergreen username unavailable
				author = pr.User.GetLogin()
			}
		}
		if author != "" {
			resp.Author = model.ToAPIString(author)
			return gimlet.NewJSONResponse(resp)
		}
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "unable to find author"))
	}
	return gimlet.NewJSONInternalErrorResponse(errors.New("project has invalid commit queue patch type"))
}
