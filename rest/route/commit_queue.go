package route

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/google/go-github/github"
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

	sc       data.Connector
	settings *evergreen.Settings
	env      evergreen.Environment
}

func makeDeleteCommitQueueItems(sc data.Connector, settings *evergreen.Settings, env evergreen.Environment) gimlet.RouteHandler {
	return &commitQueueDeleteItemHandler{
		sc:       sc,
		settings: settings,
		env:      env,
	}
}

func (cq commitQueueDeleteItemHandler) Factory() gimlet.RouteHandler {
	return &commitQueueDeleteItemHandler{
		sc:       cq.sc,
		settings: cq.settings,
		env:      cq.env,
	}
}

func (cq *commitQueueDeleteItemHandler) Parse(ctx context.Context, r *http.Request) error {
	vars := gimlet.GetVars(r)
	cq.project = vars["project_id"]
	cq.item = vars["item"]

	return nil
}

func (cq *commitQueueDeleteItemHandler) Run(ctx context.Context) gimlet.Responder {
	itemRemoved, err := cq.sc.CommitQueueRemoveItem(cq.project, cq.item)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "can't delete item"))
	}
	if itemRemoved == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("no matching item for project ID '%s', item '%s'", cq.project, cq.item),
		})
	}

	// Send GitHub status
	projectRef, err := cq.sc.FindProjectById(cq.project)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "can't find project %s", cq.project))
	}
	if projectRef.CommitQueue.PatchType == commitqueue.PRPatchType {
		pr, err := cq.sendDeletedStatusToPR(ctx, projectRef.Owner, projectRef.Repo, itemRemoved.Issue)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "can't send status for PR on '%s/%s' #%s", projectRef.Owner, projectRef.Repo, itemRemoved.Issue))
		}
		projectConfig, err := cq.sc.GetProjectConfigforPR(ctx, cq.settings, projectRef, pr)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "can't get config for PR '%s/%s' #%s", projectRef.Owner, projectRef.Repo, itemRemoved.Issue))
		}
		for _, module := range itemRemoved.Modules {
			projectModule, err := projectConfig.GetModuleByName(module.Module)
			if err != nil {
				return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "can't get module for module name '%s'", module.Module))
			}
			owner, repo, err := thirdparty.ParseGitUrl(projectModule.Repo)
			if err != nil {
				return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "can't parse module repo '%s' for owner/repo", projectModule.Repo))
			}
			_, err = cq.sendDeletedStatusToPR(ctx, owner, repo, module.Issue)
			if err != nil {
				return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "can't send status for PR on module '%s/%s' #%s", owner, repo, module.Issue))
			}
		}
	}

	response := gimlet.NewJSONResponse(struct{}{})
	if err := response.SetStatus(http.StatusNoContent); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "Cannot set HTTP status code to %d", http.StatusNoContent))
	}
	return response
}

func (cq *commitQueueDeleteItemHandler) sendDeletedStatusToPR(ctx context.Context, owner, repo, issue string) (*github.PullRequest, error) {
	itemInt, err := strconv.Atoi(issue)
	if err != nil {
		return nil, errors.Wrapf(err, "item '%s' is not an int", cq.item)
	}
	pr, err := cq.sc.GetGitHubPR(ctx, owner, repo, itemInt)
	if err != nil {
		return nil, errors.Wrapf(err, "can't get PR for %s:%s, %d", owner, repo, itemInt)
	}
	if pr == nil || pr.Head == nil || pr.Head.GetSHA() == "" {
		return nil, errors.Wrapf(err, "GitHub returned a PR missing a HEAD SHA")
	}
	pushJob := units.NewGithubStatusUpdateJobForDeleteFromCommitQueue(owner, repo, *pr.Head.SHA, itemInt)
	queue := cq.env.LocalQueue()
	err = queue.Put(ctx, pushJob)
	if err != nil {
		return pr, errors.Wrapf(err, "Can't enqueue a GitHub status update")
	}

	return pr, nil
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
