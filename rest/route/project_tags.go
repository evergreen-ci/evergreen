package route

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
)

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/projects/tag/{tag}

type getProjectsForTag struct {
	tag string
	sc  data.Connector
}

func makeGetProjectsForTag(sc data.Connector) gimlet.RouteHandler {
	return &getProjectsForTag{
		sc: sc,
	}
}

func (h *getProjectsForTag) Factory() gimlet.RouteHandler {
	return makeGetProjectsForTag(h.sc)
}

func (h *getProjectsForTag) Parse(ctx context.Context, r *http.Request) error {
	h.tag = gimlet.GetVars(r)["tag"]
	if h.tag == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "no tag specified",
		}
	}
	return nil
}

func (h *getProjectsForTag) Run(ctx context.Context) gimlet.Responder {
	refs, err := h.sc.FindProjectsByTag(h.tag)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	return gimlet.NewJSONResponse(refs)
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/projects/{project_id}/tag/{tag}

type checkTagForProject struct {
	tag       string
	projectID string
	sc        data.Connector
}

func makeCheckProjectTag(sc data.Connector) gimlet.RouteHandler {
	return &checkTagForProject{
		sc: sc,
	}
}

func (h *checkTagForProject) Factory() gimlet.RouteHandler {
	return makeCheckProjectTag(h.sc)
}

func (h *checkTagForProject) Parse(ctx context.Context, r *http.Request) error {
	vars := gimlet.GetVars(r)
	h.tag = vars["tag"]
	if h.tag == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "no tag specified",
		}
	}

	h.projectID = vars["project_id"]

	if h.tag == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "no project specified",
		}
	}

	return nil
}

func (h *checkTagForProject) Run(ctx context.Context) gimlet.Responder {
	pref, err := h.sc.FindProjectById(h.projectID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	if !util.StringSliceContains(pref.Tags, h.tag) {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("project '%s' does not have tag '%s'", h.projectID, h.tag),
		})
	}

	out := &restModel.APIProjectRef{}
	if err := out.BuildFromService(pref); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	return gimlet.NewJSONResponse(out)
}

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/projects/{project_id}/tag/{tag}

type addProjectTag struct {
	tag       string
	projectID string
	sc        data.Connector
}

func makeAddProjectTag(sc data.Connector) gimlet.RouteHandler {
	return &addProjectTag{
		sc: sc,
	}
}

func (h *addProjectTag) Factory() gimlet.RouteHandler {
	return makeAddProjectTag(h.sc)
}

func (h *addProjectTag) Parse(ctx context.Context, r *http.Request) error {
	vars := gimlet.GetVars(r)
	h.tag = vars["tag"]
	if h.tag == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "no tag specified",
		}
	}

	h.projectID = vars["project_id"]

	if h.tag == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "no project specified",
		}
	}

	return nil
}

func (h *addProjectTag) Run(ctx context.Context) gimlet.Responder {
	err := h.sc.AddTagsToProject(h.projectID, h.tag)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// DELETE /rest/v2/projects/{project_id}/tag/{tag}

type deleteProjectTag struct {
	tag       string
	projectID string
	sc        data.Connector
}

func makeDeleteProjectTag(sc data.Connector) gimlet.RouteHandler {
	return &deleteProjectTag{
		sc: sc,
	}
}

func (h *deleteProjectTag) Factory() gimlet.RouteHandler {
	return makeDeleteProjectTag(h.sc)
}

func (h *deleteProjectTag) Parse(ctx context.Context, r *http.Request) error {
	vars := gimlet.GetVars(r)
	h.tag = vars["tag"]
	if h.tag == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "no tag specified",
		}
	}

	h.projectID = vars["project_id"]

	if h.tag == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "no project specified",
		}
	}

	return nil
}

func (h *deleteProjectTag) Run(ctx context.Context) gimlet.Responder {
	err := h.sc.RemoveTagFromProject(h.projectID, h.tag)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	return gimlet.NewJSONResponse(struct{}{})
}
