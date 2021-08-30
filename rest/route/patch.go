package route

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/graphql"
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

////////////////////////////////////////////////////////////////////////
//
// PATCH /rest/v2/patches/{patch_id}

type patchChangeStatusHandler struct {
	Activated *bool  `json:"activated"`
	Priority  *int64 `json:"priority"`

	patchId string
	env     evergreen.Environment
	sc      data.Connector
}

func makeChangePatchStatus(sc data.Connector, env evergreen.Environment) gimlet.RouteHandler {
	return &patchChangeStatusHandler{
		sc:  sc,
		env: env,
	}
}

func (p *patchChangeStatusHandler) Factory() gimlet.RouteHandler {
	return &patchChangeStatusHandler{
		sc:  p.sc,
		env: p.env,
	}
}

func (p *patchChangeStatusHandler) Parse(ctx context.Context, r *http.Request) error {
	p.patchId = gimlet.GetVars(r)["patch_id"]
	body := util.NewRequestReader(r)
	defer body.Close()

	if err := utility.ReadJSON(body, p); err != nil {
		return errors.Wrap(err, "Argument read error")
	}

	if p.Activated == nil && p.Priority == nil {
		return gimlet.ErrorResponse{
			Message:    "Must set 'activated' or 'priority'",
			StatusCode: http.StatusBadRequest,
		}
	}
	return nil
}

func (p *patchChangeStatusHandler) Run(ctx context.Context) gimlet.Responder {
	user := MustHaveUser(ctx)

	foundPatch, err := p.sc.FindPatchById(p.patchId)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	if p.Priority != nil {
		priority := *p.Priority
		if ok := validPriority(priority, *foundPatch.ProjectId, user, p.sc); !ok {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				Message: fmt.Sprintf("Insufficient privilege to set priority to %d, "+
					"non-superusers can only set priority at or below %d", priority, evergreen.MaxTaskPriority),
				StatusCode: http.StatusForbidden,
			})
		}
		if err := p.sc.SetPatchPriority(p.patchId, priority, ""); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
		}
	}
	if p.Activated != nil {
		ctx, cancel := p.env.Context()
		defer cancel()
		if err := p.sc.SetPatchActivated(ctx, p.patchId, user.Username(), *p.Activated, p.env.Settings()); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
		}
	}

	return gimlet.NewJSONResponse(foundPatch)
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/patches/{patch_id}

type patchByIdHandler struct {
	patchId string
	sc      data.Connector
}

func makeFetchPatchByID(sc data.Connector) gimlet.RouteHandler {
	return &patchByIdHandler{
		sc: sc,
	}
}

func (p *patchByIdHandler) Factory() gimlet.RouteHandler {
	return &patchByIdHandler{sc: p.sc}
}

func (p *patchByIdHandler) Parse(ctx context.Context, r *http.Request) error {
	p.patchId = gimlet.GetVars(r)["patch_id"]
	return nil
}

func (p *patchByIdHandler) Run(ctx context.Context) gimlet.Responder {
	foundPatch, err := p.sc.FindPatchById(p.patchId)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	return gimlet.NewJSONResponse(foundPatch)
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/patches/{patch_id}/raw

type patchRawHandler struct {
	patchID    string
	moduleName string
	sc         data.Connector
}

func makePatchRawHandler(sc data.Connector) gimlet.RouteHandler {
	return &patchByIdHandler{
		sc: sc,
	}
}

func (p *patchRawHandler) Factory() gimlet.RouteHandler {
	return &patchByIdHandler{sc: p.sc}
}

func (p *patchRawHandler) Parse(ctx context.Context, r *http.Request) error {
	p.patchID = gimlet.GetVars(r)["patch_id"]
	p.moduleName = r.URL.Query().Get("module")

	return nil
}

func (p *patchRawHandler) Run(ctx context.Context) gimlet.Responder {
	patchMap, err := p.sc.GetPatchRawPatches(p.patchID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	return gimlet.NewTextResponse(patchMap[p.moduleName])
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/users/<id>/patches

type patchesByUserHandler struct {
	limit int
	key   time.Time
	user  string
	sc    data.Connector
}

func makeUserPatchHandler(sc data.Connector) gimlet.RouteHandler {
	return &patchesByUserHandler{
		sc: sc,
	}
}

func (p *patchesByUserHandler) Factory() gimlet.RouteHandler {
	return &patchesByUserHandler{
		sc: p.sc,
	}
}

func (p *patchesByUserHandler) Parse(ctx context.Context, r *http.Request) error {
	p.user = gimlet.GetVars(r)["user_id"]
	vals := r.URL.Query()

	var err error
	if vals.Get("start_at") == "" {
		p.key = time.Now()
	} else {
		p.key, err = model.ParseTime(vals.Get("start_at"))
		if err != nil {
			return gimlet.ErrorResponse{
				Message:    fmt.Sprintf("problem parsing time from '%s' (%s)", p.key, err.Error()),
				StatusCode: http.StatusBadRequest,
			}
		}
	}

	p.limit, err = getLimit(vals)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (p *patchesByUserHandler) Run(ctx context.Context) gimlet.Responder {
	// sortAsc set to false in order to display patches in desc chronological order
	patches, err := p.sc.FindPatchesByUser(p.user, p.key, p.limit+1)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	resp := gimlet.NewResponseBuilder()
	if err = resp.SetFormat(gimlet.JSON); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	if len(patches) > p.limit {
		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start_at",
				BaseURL:         p.sc.GetURL(),
				Key:             patches[p.limit].CreateTime.Format(model.APITimeFormat),
				Limit:           p.limit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err,
				"problem paginating response"))
		}
		patches = patches[:p.limit]
	}
	for _, model := range patches {
		err = resp.AddData(model)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "problem forming response data"))
		}
	}

	return resp
}

////////////////////////////////////////////////////////////////////////
//
// Handler for the patches for a project
//
//    /projects/{project_id}/patches

type patchesByProjectHandler struct {
	projectId string
	key       time.Time
	limit     int
	sc        data.Connector
}

func makePatchesByProjectRoute(sc data.Connector) gimlet.RouteHandler {
	return &patchesByProjectHandler{
		sc: sc,
	}
}

func (p *patchesByProjectHandler) Factory() gimlet.RouteHandler {
	return &patchesByProjectHandler{
		sc: p.sc,
	}
}

func (p *patchesByProjectHandler) Parse(ctx context.Context, r *http.Request) error {
	p.projectId = gimlet.GetVars(r)["project_id"]

	vals := r.URL.Query()

	var err error
	if vals.Get("start_at") == "" {
		p.key = time.Now()
	} else {
		p.key, err = time.ParseInLocation(model.APITimeFormat, vals.Get("start_at"), time.FixedZone("", 0))
		if err != nil {
			return gimlet.ErrorResponse{
				Message:    fmt.Sprintf("problem parsing time from '%s' (%s)", p.key, err.Error()),
				StatusCode: http.StatusBadRequest,
			}
		}
	}

	p.limit, err = getLimit(vals)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (p *patchesByProjectHandler) Run(ctx context.Context) gimlet.Responder {
	patches, err := p.sc.FindPatchesByProject(p.projectId, p.key, p.limit+1)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	resp := gimlet.NewResponseBuilder()
	err = resp.SetFormat(gimlet.JSON)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "unable to set response format"))
	}
	if len(patches) > p.limit {
		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start_at",
				BaseURL:         p.sc.GetURL(),
				Key:             patches[p.limit].CreateTime.Format(model.APITimeFormat),
				Limit:           p.limit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err,
				"problem paginating response"))
		}
		patches = patches[:p.limit]
	}
	for _, model := range patches {
		err = resp.AddData(model)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "problem forming response data"))
		}
	}
	err = resp.SetStatus(http.StatusOK)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "unable to set response status"))
	}

	return resp
}

////////////////////////////////////////////////////////////////////////
//
// Handler for aborting patches by id
//
//    /patches/{patch_id}/abort

type patchAbortHandler struct {
	patchId string
	sc      data.Connector
}

func makeAbortPatch(sc data.Connector) gimlet.RouteHandler {
	return &patchAbortHandler{
		sc: sc,
	}
}

func (p *patchAbortHandler) Factory() gimlet.RouteHandler {
	return &patchAbortHandler{sc: p.sc}
}

func (p *patchAbortHandler) Parse(ctx context.Context, r *http.Request) error {
	p.patchId = gimlet.GetVars(r)["patch_id"]
	return nil
}

func (p *patchAbortHandler) Run(ctx context.Context) gimlet.Responder {
	usr := MustHaveUser(ctx)

	if err := p.sc.AbortPatch(p.patchId, usr.Id); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Abort error"))
	}

	// Patch may be deleted by abort (eg not finalized) and not found here
	foundPatch, err := p.sc.FindPatchById(p.patchId)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	return gimlet.NewJSONResponse(foundPatch)
}

////////////////////////////////////////////////////////////////////////
//
// Handler for restarting patches by id
//
//    /patches/{patch_id}/restart

type patchRestartHandler struct {
	patchId string
	sc      data.Connector
}

func makeRestartPatch(sc data.Connector) gimlet.RouteHandler {
	return &patchRestartHandler{
		sc: sc,
	}
}

func (p *patchRestartHandler) Factory() gimlet.RouteHandler {
	return &patchRestartHandler{
		sc: p.sc,
	}
}

func (p *patchRestartHandler) Parse(ctx context.Context, r *http.Request) error {
	p.patchId = gimlet.GetVars(r)["patch_id"]
	return nil
}

func (p *patchRestartHandler) Run(ctx context.Context) gimlet.Responder {
	// If the version has not been finalized, returns NotFound
	usr := MustHaveUser(ctx)

	if err := p.sc.RestartVersion(p.patchId, usr.Id); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Restart error"))
	}

	foundPatch, err := p.sc.FindPatchById(p.patchId)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	return gimlet.NewJSONResponse(foundPatch)
}

////////////////////////////////////////////////////////////////////////
//
// Handler for creating a new merge patch from an existing patch
//
//    /patches/{patch_id}/merge_patch
type mergePatchHandler struct {
	CommitMessage string `json:"commit_message"`

	patchId string
	sc      data.Connector
}

func makeMergePatch(sc data.Connector) gimlet.RouteHandler {
	return &mergePatchHandler{
		sc: sc,
	}
}

func (p *mergePatchHandler) Factory() gimlet.RouteHandler {
	return &mergePatchHandler{
		sc: p.sc,
	}
}

func (p *mergePatchHandler) Parse(ctx context.Context, r *http.Request) error {
	p.patchId = gimlet.GetVars(r)["patch_id"]

	body := util.NewRequestReader(r)
	defer body.Close()
	if err := utility.ReadJSON(body, p); err != nil {
		return errors.Wrap(err, "Argument read error")
	}

	return nil
}

func (p *mergePatchHandler) Run(ctx context.Context) gimlet.Responder {
	apiPatch, err := p.sc.CreatePatchForMerge(ctx, p.patchId, p.CommitMessage)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "can't create merge patch"))
	}
	return gimlet.NewJSONResponse(apiPatch)
}

type patchTasks struct {
	Description string    `json:"description"`
	Variants    []variant `json:"variants"`
}

type variant struct {
	Id    string   `json:"id"`
	Tasks []string `json:"tasks"`
}

type schedulePatchHandler struct {
	variantTasks patchTasks

	patchId string
	patch   patch.Patch
	sc      data.Connector
}

func makeSchedulePatchHandler(sc data.Connector) gimlet.RouteHandler {
	return &schedulePatchHandler{
		sc: sc,
	}
}

func (p *schedulePatchHandler) Factory() gimlet.RouteHandler {
	return &schedulePatchHandler{
		sc: p.sc,
	}
}

func (p *schedulePatchHandler) Parse(ctx context.Context, r *http.Request) error {
	p.patchId = gimlet.GetVars(r)["patch_id"]
	if p.patchId == "" {
		return errors.New("must specify a patch ID")
	}
	var err error
	apiPatch, err := p.sc.FindPatchById(p.patchId)
	if err != nil {
		return err
	}
	if apiPatch == nil {
		return errors.New("patch not found")
	}
	dbPatch, err := apiPatch.ToService()
	if err != nil {
		return errors.Wrap(err, "unable to parse patch")
	}
	p.patch = dbPatch.(patch.Patch)
	body := util.NewRequestReader(r)
	defer body.Close()
	tasks := patchTasks{}
	if err = utility.ReadJSON(body, &tasks); err != nil {
		return errors.Wrap(err, "Argument read error")
	}
	if len(tasks.Variants) == 0 {
		return errors.New("no variants specified")
	}
	p.variantTasks = tasks
	return nil
}

func (p *schedulePatchHandler) Run(ctx context.Context) gimlet.Responder {
	settings, err := evergreen.GetConfig()
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "unable to get config"))
	}
	token, err := settings.GetGithubOauthToken()
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "unable to get token"))
	}
	dbVersion, _ := p.sc.FindVersionById(p.patchId)
	var project *dbModel.Project
	if dbVersion == nil {
		project, _, err = dbModel.GetPatchedProject(ctx, &p.patch, token)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "unable to find project from patch"))
		}
	} else {
		project, err = dbModel.FindProjectFromVersionID(dbVersion.Id)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "unable to find project from version"))
		}
	}
	patchUpdateReq := graphql.PatchUpdate{
		Description: p.variantTasks.Description,
	}
	if patchUpdateReq.Description == "" && dbVersion != nil {
		patchUpdateReq.Description = dbVersion.Message
	}
	for _, v := range p.variantTasks.Variants {
		variantToSchedule := patch.VariantTasks{Variant: v.Id}
		if len(v.Tasks) > 0 && v.Tasks[0] == "*" {
			projectVariant := project.FindBuildVariant(v.Id)
			if projectVariant == nil {
				return gimlet.MakeJSONErrorResponder(errors.Errorf("variant not found: %s", v.Id))
			}
			variantToSchedule.DisplayTasks = projectVariant.DisplayTasks
			for _, projectTask := range projectVariant.Tasks {
				variantToSchedule.Tasks = append(variantToSchedule.Tasks, projectTask.Name)
			}
		} else {
			for _, t := range v.Tasks {
				dt := project.GetDisplayTask(v.Id, t)
				if dt != nil {
					variantToSchedule.DisplayTasks = append(variantToSchedule.DisplayTasks, *dt)
				} else {
					variantToSchedule.Tasks = append(variantToSchedule.Tasks, t)
				}
			}
		}
		patchUpdateReq.VariantsTasks = append(patchUpdateReq.VariantsTasks, variantToSchedule)
	}
	err, code, msg, versionId := graphql.SchedulePatch(ctx, p.patchId, dbVersion, patchUpdateReq)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "unable to schedule patch"))
	}
	if code != http.StatusOK {
		resp := gimlet.NewResponseBuilder()
		_ = resp.SetStatus(code)
		_ = resp.AddData(msg)
		return resp
	}
	dbVersion, err = p.sc.FindVersionById(versionId)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "unable to find patch version"))
	}
	if dbVersion == nil {
		return gimlet.NewJSONErrorResponse("no patch found")
	}
	restVersion := model.APIVersion{}
	if err = restVersion.BuildFromService(dbVersion); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "error converting version model"))
	}
	return gimlet.NewJSONResponse(restVersion)
}
