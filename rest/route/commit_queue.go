package route

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/evergreen-ci/evergreen"
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

type commitQueueGetHandler struct {
	project string
}

func makeGetCommitQueueItems() gimlet.RouteHandler {
	return &commitQueueGetHandler{}
}

func (cq *commitQueueGetHandler) Factory() gimlet.RouteHandler {
	return &commitQueueGetHandler{}
}

func (cq *commitQueueGetHandler) Parse(ctx context.Context, r *http.Request) error {
	cq.project = gimlet.GetVars(r)["project_id"]
	return nil
}

func (cq *commitQueueGetHandler) Run(ctx context.Context) gimlet.Responder {
	commitQueue, err := data.FindCommitQueueForProject(cq.project)
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
	env     evergreen.Environment
}

func makeDeleteCommitQueueItems(env evergreen.Environment) gimlet.RouteHandler {
	return &commitQueueDeleteItemHandler{
		env: env,
	}
}

func (cq commitQueueDeleteItemHandler) Factory() gimlet.RouteHandler {
	return &commitQueueDeleteItemHandler{
		env: cq.env,
	}
}

func (cq *commitQueueDeleteItemHandler) Parse(ctx context.Context, r *http.Request) error {
	vars := gimlet.GetVars(r)
	cq.item = vars["patch_id"]
	requestedPatch, err := patch.FindOneId(cq.item)
	if err != nil {
		return errors.Wrapf(err, "finding commit queue item '%s'", cq.item)
	}
	if requestedPatch == nil {
		return errors.New("commit queue item not found")
	}
	cq.project = requestedPatch.Project
	return nil
}

func (cq *commitQueueDeleteItemHandler) Run(ctx context.Context) gimlet.Responder {
	dc := data.DBCommitQueueConnector{}

	removed, err := data.CommitQueueRemoveItem(cq.project, cq.item, gimlet.GetUser(ctx).DisplayName())
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "can't delete item"))
	}
	if removed == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("no matching item for project ID '%s', item '%s'", cq.project, cq.item),
		})
	}

	// Send GitHub status
	if utility.FromStringPtr(removed.Source) == commitqueue.SourcePullRequest {
		projectRef, err := data.FindProjectById(cq.project, true, false)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "can't find project '%s'", cq.project))
		}
		if projectRef == nil {
			return gimlet.MakeJSONErrorResponder(errors.Errorf("project '%s' doesn't exist", cq.project))
		}
		itemInt, err := strconv.Atoi(cq.item)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "item '%s' is not an int", cq.item))
		}
		pr, err := dc.GetGitHubPR(ctx, projectRef.Owner, projectRef.Repo, itemInt)
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

type commitQueueClearAllHandler struct{}

func makeClearCommitQueuesHandler() gimlet.RouteHandler {
	return &commitQueueClearAllHandler{}
}

func (cq *commitQueueClearAllHandler) Factory() gimlet.RouteHandler {
	return &commitQueueClearAllHandler{}
}

func (cq *commitQueueClearAllHandler) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (cq *commitQueueClearAllHandler) Run(ctx context.Context) gimlet.Responder {
	clearedCount, err := commitqueue.ClearAllCommitQueues()
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
}

func makeCommitQueueEnqueueItem() gimlet.RouteHandler {
	return &commitQueueEnqueueItemHandler{}
}

func (cq commitQueueEnqueueItemHandler) Factory() gimlet.RouteHandler {
	return &commitQueueEnqueueItemHandler{}
}

func (cq *commitQueueEnqueueItemHandler) Parse(ctx context.Context, r *http.Request) error {
	vars := gimlet.GetVars(r)
	cq.item = vars["patch_id"]
	patch, err := data.FindPatchById(cq.item)
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
	patchEmpty, err := patch.IsPatchEmpty(cq.item)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "can't find patch to enqueue"))
	}
	if patchEmpty {
		return gimlet.MakeJSONErrorResponder(errors.New("can't enqueue item, patch is empty"))
	}
	patchId := utility.ToStringPtr(cq.item)
	position, err := data.EnqueueItem(cq.project, model.APICommitQueueItem{Issue: patchId, PatchId: patchId, Source: utility.ToStringPtr(commitqueue.SourceDiff)}, cq.force)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "can't enqueue item"))
	}

	return gimlet.NewJSONResponse(model.APICommitQueuePosition{Position: position})
}

type cqMessageForPatch struct {
	patchID string
}

func makecqMessageForPatch() gimlet.RouteHandler {
	return &cqMessageForPatch{}
}

func (p *cqMessageForPatch) Factory() gimlet.RouteHandler {
	return &cqMessageForPatch{}
}

func (p *cqMessageForPatch) Parse(ctx context.Context, r *http.Request) error {
	p.patchID = gimlet.GetVars(r)["patch_id"]
	if p.patchID == "" {
		return errors.New("patch_id must be specified")
	}

	return nil
}

func (p *cqMessageForPatch) Run(ctx context.Context) gimlet.Responder {
	message, err := dbModel.GetMessageForPatch(p.patchID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	return gimlet.NewTextResponse(message)
}

type commitQueueConcludeMerge struct {
	Status string `json:"status"`

	patchId string
}

func makeCommitQueueConcludeMerge() gimlet.RouteHandler {
	return &commitQueueConcludeMerge{}
}

func (p *commitQueueConcludeMerge) Factory() gimlet.RouteHandler {
	return &commitQueueConcludeMerge{}
}

func (p *commitQueueConcludeMerge) Parse(ctx context.Context, r *http.Request) error {
	body := utility.NewRequestReader(r)
	defer body.Close()
	p.patchId = gimlet.GetVars(r)["patch_id"]
	if p.patchId == "" {
		return errors.New("patch ID must be specified")
	}

	if err := utility.ReadJSON(body, p); err != nil {
		return errors.Wrap(err, "unable to parse request body")
	}
	if p.Status == "" {
		return errors.New("status must be specified")
	}

	return nil
}

func (p *commitQueueConcludeMerge) Run(ctx context.Context) gimlet.Responder {
	err := data.ConcludeMerge(p.patchId, p.Status)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	return gimlet.NewJSONResponse(struct{}{})
}

type commitQueueAdditionalPatches struct {
	patchId string
}

func makeCommitQueueAdditionalPatches() gimlet.RouteHandler {
	return &commitQueueAdditionalPatches{}
}

func (p *commitQueueAdditionalPatches) Factory() gimlet.RouteHandler {
	return &commitQueueAdditionalPatches{}
}

func (p *commitQueueAdditionalPatches) Parse(ctx context.Context, r *http.Request) error {
	p.patchId = gimlet.GetVars(r)["patch_id"]
	if p.patchId == "" {
		return errors.New("patch_id must be specified")
	}
	return nil
}

func (p *commitQueueAdditionalPatches) Run(ctx context.Context) gimlet.Responder {
	additional, err := data.GetAdditionalPatches(p.patchId)
	if err != nil {
		gimlet.NewJSONErrorResponse(err)
	}
	return gimlet.NewJSONResponse(additional)
}
