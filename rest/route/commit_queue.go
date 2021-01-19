package route

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
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
	commitQueue, err := cq.sc.FindCommitQueueForProject(cq.project)
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
	projectRef, err := cq.sc.FindProjectById(cq.project)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "can't find project '%s'", cq.project))
	}
	if projectRef == nil {
		return gimlet.MakeJSONErrorResponder(errors.Errorf("project '%s' doesn't exist", cq.project))
	}

	found, err := cq.sc.CommitQueueRemoveItem(projectRef.Id, cq.item, gimlet.GetUser(ctx).DisplayName())
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
	force   bool

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
	cq.project = *patch.ProjectId

	force := r.URL.Query().Get("force")
	if strings.ToLower(force) == "true" {
		cq.force = true
	}
	return nil
}

func (cq *commitQueueEnqueueItemHandler) Run(ctx context.Context) gimlet.Responder {
	patchEmpty, err := cq.sc.IsPatchEmpty(cq.item)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "can't find patch to enqueue"))
	}
	if patchEmpty {
		return gimlet.MakeJSONErrorResponder(errors.New("can't enqueue item, patch is empty"))
	}
	position, err := cq.sc.EnqueueItem(cq.project, model.APICommitQueueItem{Issue: model.ToStringPtr(cq.item)}, cq.force)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "can't enqueue item"))
	}

	return gimlet.NewJSONResponse(model.APICommitQueuePosition{Position: position})
}

type cqMessageForPatch struct {
	patchID string
	sc      data.Connector
}

func makecqMessageForPatch(sc data.Connector) gimlet.RouteHandler {
	return &cqMessageForPatch{
		sc: sc,
	}
}

func (p *cqMessageForPatch) Factory() gimlet.RouteHandler {
	return &cqMessageForPatch{
		sc: p.sc,
	}
}

func (p *cqMessageForPatch) Parse(ctx context.Context, r *http.Request) error {
	p.patchID = gimlet.GetVars(r)["patch_id"]
	if p.patchID == "" {
		return errors.New("patch_id must be specified")
	}

	return nil
}

func (p *cqMessageForPatch) Run(ctx context.Context) gimlet.Responder {
	message, err := p.sc.GetMessageForPatch(p.patchID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	return gimlet.NewTextResponse(message)
}
