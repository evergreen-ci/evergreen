package route

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

////////////////////////////////////////////////////////////////////////
//
// PATCH /rest/v2/patches/{patch_id}

type patchChangeStatusHandler struct {
	// The priority to set the patch to
	Activated *bool `json:"activated"`
	// The activation status to set the patch to
	Priority *int64 `json:"priority"`

	patchId string
	env     evergreen.Environment
}

func makeChangePatchStatus(env evergreen.Environment) gimlet.RouteHandler {
	return &patchChangeStatusHandler{
		env: env,
	}
}

// Factory creates an instance of the handler.
//
//	@Summary		Change patch status
//	@Description	Sets the priority and activation status of a single patch to the input values
//	@Tags			patches
//	@Router			/patches/{patch_id} [patch]
//	@Security		Api-User || Api-Key
//	@Param			patch_id	path		string						true	"patch ID"
//	@Param			{object}	body		patchChangeStatusHandler	true	"parameters"
//	@Success		200			{object}	model.APIPatch
func (p *patchChangeStatusHandler) Factory() gimlet.RouteHandler {
	return &patchChangeStatusHandler{
		env: p.env,
	}
}

func (p *patchChangeStatusHandler) Parse(ctx context.Context, r *http.Request) error {
	p.patchId = gimlet.GetVars(r)["patch_id"]
	body := utility.NewRequestReader(r)
	defer body.Close()

	if err := utility.ReadJSON(body, p); err != nil {
		return errors.Wrap(err, "reading patch change options from JSON request body")
	}

	if p.Activated == nil && p.Priority == nil {
		return errors.New("must set 'activated' or 'priority'")
	}
	return nil
}

func (p *patchChangeStatusHandler) Run(ctx context.Context) gimlet.Responder {
	user := MustHaveUser(ctx)
	foundPatch, err := data.FindPatchById(p.patchId)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding patch '%s'", p.patchId))
	}

	if p.Priority != nil {
		priority := *p.Priority
		if ok := validPriority(priority, *foundPatch.ProjectId, user); !ok {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				Message: fmt.Sprintf("insufficient privilege to set priority to %d, "+
					"non-superusers can only set priority at or below %d", priority, evergreen.MaxTaskPriority),
				StatusCode: http.StatusForbidden,
			})
		}
		if err := dbModel.SetVersionsPriority(ctx, []string{p.patchId}, priority, ""); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "setting patch priority"))
		}
	}
	if p.Activated != nil {
		ctx, cancel := p.env.Context()
		defer cancel()
		if err := data.SetPatchActivated(ctx, p.patchId, user.Username(), *p.Activated, p.env.Settings()); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "setting patch activation"))
		}
	}

	return gimlet.NewJSONResponse(foundPatch)
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/patches/{patch_id}

type patchByIdHandler struct {
	patchId string
}

func makeFetchPatchByID() gimlet.RouteHandler {
	return &patchByIdHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Fetch patch by ID
//	@Description	Fetch a single patch using its ID
//	@Tags			patches
//	@Router			/patches/{patch_id} [get]
//	@Security		Api-User || Api-Key
//	@Param			patch_id	path		string	true	"patch ID"
//	@Param			module		query		string	false	"A module to get the diff for. Returns the empty string when no patch exists for the module."
//	@Success		200			{object}	model.APIPatch
func (p *patchByIdHandler) Factory() gimlet.RouteHandler {
	return &patchByIdHandler{}
}

func (p *patchByIdHandler) Parse(ctx context.Context, r *http.Request) error {
	p.patchId = gimlet.GetVars(r)["patch_id"]
	return nil
}

func (p *patchByIdHandler) Run(ctx context.Context) gimlet.Responder {
	foundPatch, err := data.FindPatchById(p.patchId)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding patch '%s'", p.patchId))
	}

	return gimlet.NewJSONResponse(foundPatch)
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/patches/{patch_id}/raw

type patchRawHandler struct {
	patchID    string
	moduleName string
}

func makePatchRawHandler() gimlet.RouteHandler {
	return &patchRawHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Get patch diff
//	@Description	Fetch the raw diff for a patch
//	@Tags			patches
//	@Router			/patches/{patch_id}/raw [get]
//	@Security		Api-User || Api-Key
//	@Param			patch_id	path		string	true	"patch ID"
//	@Param			module		query		string	false	"A module to get the diff for. Returns the empty string when no patch exists for the module."
//	@Success		200			{string}	string
func (p *patchRawHandler) Factory() gimlet.RouteHandler {
	return &patchRawHandler{}
}

func (p *patchRawHandler) Parse(ctx context.Context, r *http.Request) error {
	p.patchID = gimlet.GetVars(r)["patch_id"]
	p.moduleName = r.URL.Query().Get("module")

	return nil
}

func (p *patchRawHandler) Run(ctx context.Context) gimlet.Responder {
	rawPatches, err := data.GetRawPatches(p.patchID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "getting raw patches for patch '%s'", p.patchID))
	}
	if rawPatches == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("raw patch not found for '%s'", p.patchID),
		})
	}

	if p.moduleName == "" {
		return gimlet.NewTextResponse(rawPatches.Patch.Diff)
	}
	modules := rawPatches.RawModules
	for _, m := range modules {
		if m.Name == p.moduleName {
			return gimlet.NewTextResponse(m.Diff)
		}
	}
	return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
		StatusCode: http.StatusNotFound,
		Message:    fmt.Sprintf("module '%s' not found", p.moduleName),
	})
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/patches/{patch_id}/raw_modules

type moduleRawHandler struct {
	patchID string
}

func makeModuleRawHandler() gimlet.RouteHandler {
	return &moduleRawHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Get patch diff with module diffs
//	@Description	Fetch the raw diff for a patch along with the module diffs.
//	@Tags			patches
//	@Router			/projects/{patch_id}/raw_modules [get]
//	@Security		Api-User || Api-Key
//	@Param			patch_id	path		string	true	"the project ID"
//	@Success		200			{object}	model.APIRawPatch
func (p *moduleRawHandler) Factory() gimlet.RouteHandler {
	return &moduleRawHandler{}
}

func (p *moduleRawHandler) Parse(ctx context.Context, r *http.Request) error {
	p.patchID = gimlet.GetVars(r)["patch_id"]
	return nil
}

func (p *moduleRawHandler) Run(ctx context.Context) gimlet.Responder {
	rawPatches, err := data.GetRawPatches(p.patchID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting raw patches for patch '%s'", p.patchID))
	}
	return gimlet.NewJSONResponse(rawPatches)
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/users/<id>/patches

type patchesByUserHandler struct {
	limit int
	key   time.Time
	user  string
	url   string
}

func makeUserPatchHandler(url string) gimlet.RouteHandler {
	return &patchesByUserHandler{url: url}
}

// Factory creates an instance of the handler.
//
//	@Summary		Fetch patches by user
//	@Description	Returns a paginated list of all patches associated with a specific user
//	@Tags			patches
//	@Router			/users/{user_id}/patches [get]
//	@Security		Api-User || Api-Key
//	@Param			user_id		path	string	true	"the user's ID"
//	@Param			project_id	path	string	true	"the project ID"
//	@Param			start_at	query	string	false	"The create_time of the patch to start at in the pagination. Defaults to now"
//	@Param			limit		query	int		false	"The number of patches to be returned per page of pagination. Defaults to 100"
//	@Success		200			{array}	model.APIPatch
func (p *patchesByUserHandler) Factory() gimlet.RouteHandler {
	return &patchesByUserHandler{url: p.url}
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
			return errors.Wrapf(err, "parsing 'start at' time %s", p.key)
		}
	}

	p.limit, err = getLimit(vals)
	if err != nil {
		return errors.Wrap(err, "parsing limit")
	}

	return nil
}

func (p *patchesByUserHandler) Run(ctx context.Context) gimlet.Responder {
	// sortAsc set to false in order to display patches in desc chronological order
	patches, err := data.FindPatchesByUser(p.user, p.key, p.limit+1)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding patches for user '%s'", p.user))
	}

	resp := gimlet.NewResponseBuilder()
	if len(patches) > p.limit {
		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start_at",
				BaseURL:         p.url,
				Key:             patches[p.limit].CreateTime.Format(model.APITimeFormat),
				Limit:           p.limit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "paginating response"))
		}
		patches = patches[:p.limit]
	}
	for _, model := range patches {
		err = resp.AddData(model)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "adding response data for patch '%s'", utility.FromStringPtr(model.Id)))
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
	url       string
}

func makePatchesByProjectRoute(url string) gimlet.RouteHandler {
	return &patchesByProjectHandler{url: url}
}

// Factory creates an instance of the handler.
//
//	@Summary		Fetch patches by project
//	@Description	Returns a paginated list of all patches associated with a specific project
//	@Tags			patches
//	@Router			/projects/{project_id}/patches [get]
//	@Security		Api-User || Api-Key
//	@Param			project_id	path	string	true	"the project ID"
//	@Param			start_at	query	string	false	"The create_time of the patch to start at in the pagination. Defaults to now"
//	@Param			limit		query	int		false	"The number of patches to be returned per page of pagination. Defaults to 100"
//	@Success		200			{array}	model.APIPatch
func (p *patchesByProjectHandler) Factory() gimlet.RouteHandler {
	return &patchesByProjectHandler{url: p.url}
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
			return errors.Wrapf(err, "parsing 'start at' time %s", p.key)
		}
	}

	p.limit, err = getLimit(vals)
	if err != nil {
		return errors.Wrap(err, "parsing limit")
	}

	return nil
}

func (p *patchesByProjectHandler) Run(ctx context.Context) gimlet.Responder {
	patches, err := data.FindPatchesByProject(p.projectId, p.key, p.limit+1)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding patches for project '%s'", p.projectId))
	}

	resp := gimlet.NewResponseBuilder()
	if len(patches) > p.limit {
		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start_at",
				BaseURL:         p.url,
				Key:             patches[p.limit].CreateTime.Format(model.APITimeFormat),
				Limit:           p.limit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "paginating response"))
		}
		patches = patches[:p.limit]
	}
	for _, model := range patches {
		err = resp.AddData(model)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "adding response data for patch '%s'", utility.FromStringPtr(model.Id)))
		}
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
}

func makeAbortPatch() gimlet.RouteHandler {
	return &patchAbortHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Abort a patch
//	@Description	Aborts a single patch using its ID and returns the patch
//	@Tags			patches
//	@Router			/patches/{patch_id}/abort [post]
//	@Security		Api-User || Api-Key
//	@Param			patch_id	path		string	true	"the patch ID"
//	@Success		200			{object}	model.APIPatch
func (p *patchAbortHandler) Factory() gimlet.RouteHandler {
	return &patchAbortHandler{}
}

func (p *patchAbortHandler) Parse(ctx context.Context, r *http.Request) error {
	p.patchId = gimlet.GetVars(r)["patch_id"]
	return nil
}

func (p *patchAbortHandler) Run(ctx context.Context) gimlet.Responder {
	usr := MustHaveUser(ctx)
	if err := data.AbortPatch(ctx, p.patchId, usr.Id); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "aborting patch '%s'", p.patchId))
	}

	// Patch may be deleted by abort (eg not finalized) and not found here
	foundPatch, err := data.FindPatchById(p.patchId)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding patch '%s'", p.patchId))
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
}

func makeRestartPatch() gimlet.RouteHandler {
	return &patchRestartHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		restart a patch
//	@Description	restarts a single patch using its ID and returns the patch
//	@Tags			patches
//	@Router			/patches/{patch_id}/restart [post]
//	@Security		Api-User || Api-Key
//	@Param			patch_id	path		string	true	"the patch ID"
//	@Success		200			{object}	model.APIPatch
func (p *patchRestartHandler) Factory() gimlet.RouteHandler {
	return &patchRestartHandler{}
}

func (p *patchRestartHandler) Parse(ctx context.Context, r *http.Request) error {
	p.patchId = gimlet.GetVars(r)["patch_id"]
	return nil
}

func (p *patchRestartHandler) Run(ctx context.Context) gimlet.Responder {
	// If the version has not been finalized, returns NotFound
	usr := MustHaveUser(ctx)
	if err := dbModel.RestartVersion(ctx, p.patchId, nil, true, usr.Id); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "restarting tasks in patch '%s'", p.patchId))
	}

	foundPatch, err := data.FindPatchById(p.patchId)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding patch '%s'", p.patchId))
	}

	return gimlet.NewJSONResponse(foundPatch)
}

// //////////////////////////////////////////////////////////////////////
//
// Handler for creating a new merge patch from an existing patch
//
//	/patches/{patch_id}/merge_patch
type mergePatchHandler struct {
	CommitMessage string `json:"commit_message"`

	patchId string
	env     evergreen.Environment
}

func makeMergePatch(env evergreen.Environment) gimlet.RouteHandler {
	return &mergePatchHandler{env: env}
}

func (p *mergePatchHandler) Factory() gimlet.RouteHandler {
	return &mergePatchHandler{env: p.env}
}

func (p *mergePatchHandler) Parse(ctx context.Context, r *http.Request) error {
	p.patchId = gimlet.GetVars(r)["patch_id"]

	body := utility.NewRequestReader(r)
	defer body.Close()
	if err := utility.ReadJSON(body, p); err != nil {
		return errors.Wrap(err, "reading commit message from JSON request body")
	}

	return nil
}

func (p *mergePatchHandler) Run(ctx context.Context) gimlet.Responder {
	apiPatch, err := data.CreatePatchForMerge(ctx, p.env.Settings(), p.patchId, p.CommitMessage)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "creating merge patch '%s'", p.patchId))
	}
	return gimlet.NewJSONResponse(apiPatch)
}

// /patches/{patch_id}/estimated_generated_tasks
type countEstimatedGeneratedTasksHandler struct {
	patchId string
	files   []dbModel.TVPair
}

func makeCountEstimatedGeneratedTasks() gimlet.RouteHandler {
	return &countEstimatedGeneratedTasksHandler{}
}

func (p *countEstimatedGeneratedTasksHandler) Factory() gimlet.RouteHandler {
	return &countEstimatedGeneratedTasksHandler{}
}

func (p *countEstimatedGeneratedTasksHandler) Parse(ctx context.Context, r *http.Request) error {
	p.patchId = gimlet.GetVars(r)["patch_id"]

	body := utility.NewRequestReader(r)
	defer body.Close()
	if err := utility.ReadJSON(body, &p.files); err != nil {
		return errors.Wrap(err, "reading JSON request body")
	}

	return nil
}

func (p *countEstimatedGeneratedTasksHandler) Run(ctx context.Context) gimlet.Responder {
	existingPatch, err := patch.FindOneId(p.patchId)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting patch '%s'", p.patchId))
	}
	if existingPatch == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    "patch not found",
		})
	}
	numTasksToFinalize := 0
	for _, vt := range p.files {
		dbTask, err := task.FindOne(db.Query(bson.M{
			task.ProjectKey:      existingPatch.Project,
			task.BuildVariantKey: vt.Variant,
			task.DisplayNameKey:  vt.TaskName,
			task.GenerateTaskKey: true,
		}).Sort([]string{"-" + task.FinishTimeKey}))
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting task with variant '%s' and name '%s'", vt.Variant, vt.TaskName))
		}
		if dbTask != nil {
			numTasksToFinalize += utility.FromIntPtr(dbTask.EstimatedNumActivatedGeneratedTasks)
		}
	}
	return gimlet.NewJSONResponse(&model.APINumTasksToFinalize{
		NumTasksToFinalize: utility.ToIntPtr(numTasksToFinalize),
	})
}

type patchTasks struct {
	// Optional, if sent will update the patch's description
	Description string `json:"description"`
	// Required, these are the variants and tasks that the patch should run.
	// For an already-scheduled patch, any new tasks in this array will be
	// created and any existing tasks not in this array will be unscheduled.
	Variants []variant `json:"variants"`
}

type variant struct {
	Id    string   `json:"id"`
	Tasks []string `json:"tasks"`
}

// POST /patches/{patch_id}/configure

type schedulePatchHandler struct {
	variantTasks patchTasks

	patchId string
	patch   patch.Patch
	env     evergreen.Environment
}

func makeSchedulePatchHandler(env evergreen.Environment) gimlet.RouteHandler {
	return &schedulePatchHandler{env: env}
}

// Factory creates an instance of the handler.
//
//	@Summary		Configure/schedule a patch
//	@Description	Update the list of tasks that the specified patch will run. This will only add on tasks, existing tasks will not be removed. This works both for initially specifying a patch's tasks, as well as for adding additional tasks to an already-scheduled patch.
//	@Tags			patches
//	@Router			/patches/{patch_id}/configure [post]
//	@Security		Api-User || Api-Key
//	@Param			patch_id	path		string		true	"the patch ID"
//	@Param			{object}	body		patchTasks	true	"parameters"
//	@Success		200			{object}	model.APIVersion
func (p *schedulePatchHandler) Factory() gimlet.RouteHandler {
	return &schedulePatchHandler{env: p.env}
}

func (p *schedulePatchHandler) Parse(ctx context.Context, r *http.Request) error {
	p.patchId = gimlet.GetVars(r)["patch_id"]
	if p.patchId == "" {
		return errors.New("must specify a patch ID")
	}
	var err error
	apiPatch, err := data.FindPatchById(p.patchId)
	if err != nil {
		return err
	}
	if apiPatch == nil {
		return errors.New("patch not found")
	}
	p.patch, err = apiPatch.ToService()
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "converting patch to service model").Error(),
		}
	}
	body := utility.NewRequestReader(r)
	defer body.Close()
	tasks := patchTasks{}
	if err = utility.ReadJSON(body, &tasks); err != nil {
		return errors.Wrap(err, "reading tasks from JSON request body")
	}
	if len(tasks.Variants) == 0 {
		return errors.New("no variants specified")
	}
	p.variantTasks = tasks
	return nil
}

func (p *schedulePatchHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)
	dbVersion, _ := dbModel.VersionFindOneId(p.patchId)
	var project *dbModel.Project
	var err error
	if dbVersion == nil {
		project, _, err = dbModel.GetPatchedProject(ctx, p.env.Settings(), &p.patch)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding project for patch '%s'", p.patchId))
		}
	} else {
		project, err = dbModel.FindProjectFromVersionID(dbVersion.Id)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding project for version '%s'", dbVersion.Id))
		}
	}
	patchUpdateReq := dbModel.PatchUpdate{
		Description: p.variantTasks.Description,
		Caller:      u.Id,
		// PatchUpdate must include all of the variant tasks for the patch (existing and new ones)
		VariantsTasks: p.patch.VariantsTasks,
	}
	if patchUpdateReq.Description == "" && dbVersion != nil {
		patchUpdateReq.Description = dbVersion.Message
	}
	for _, v := range p.variantTasks.Variants {
		// If patchUpdateReq.VariantsTasks already contains the variant, append to that variant.
		// Otherwise, create a new variant.
		variantToSchedule := patch.VariantTasks{}
		variantExists := false
		for _, variant := range patchUpdateReq.VariantsTasks {
			if variant.Variant == v.Id {
				variantToSchedule = variant
				variantExists = true
				break
			}
		}
		if !variantExists {
			variantToSchedule = patch.VariantTasks{Variant: v.Id}
		}

		if len(v.Tasks) > 0 && v.Tasks[0] == "*" {
			projectVariant := project.FindBuildVariant(v.Id)
			if projectVariant == nil {
				return gimlet.MakeJSONErrorResponder(errors.Errorf("variant '%s' not found", v.Id))
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
	code, err := units.SchedulePatch(ctx, p.env, p.patchId, dbVersion, patchUpdateReq)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "scheduling patch"))
	}
	if code != http.StatusOK {
		resp := gimlet.NewResponseBuilder()
		_ = resp.SetStatus(code)
		return resp
	}
	dbVersion, err = dbModel.VersionFindOneId(p.patchId)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding version for patch '%s'", p.patchId))
	}
	if dbVersion == nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Errorf("version for patch '%s' not found", p.patchId))
	}
	restVersion := model.APIVersion{}
	restVersion.BuildFromService(*dbVersion)
	return gimlet.NewJSONResponse(restVersion)
}
