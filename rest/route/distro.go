package route

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

///////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/distros/{distro_id}/setup

type distroIDGetSetupHandler struct {
	distroID string
}

func makeGetDistroSetup() gimlet.RouteHandler {
	return &distroIDGetSetupHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Get the setup script for a distro
//	@Description	Fetch a distro's setup script by its distro ID.
//	@Tags			distros
//	@Router			/distros/{distro_id}/setup [get]
//	@Security		Api-User || Api-Key
//	@Param			distro_id	path		string	true	"distro ID"
//	@Success		200			{string}	string	"The distro setup script"
func (h *distroIDGetSetupHandler) Factory() gimlet.RouteHandler {
	return &distroIDGetSetupHandler{}
}

// Parse fetches the distroId from the http request.
func (h *distroIDGetSetupHandler) Parse(ctx context.Context, r *http.Request) error {
	h.distroID = gimlet.GetVars(r)["distro_id"]

	return nil
}

// Run returns the given distro's setup script.
func (h *distroIDGetSetupHandler) Run(ctx context.Context) gimlet.Responder {
	d, err := distro.FindOneId(ctx, h.distroID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding distro '%s'", h.distroID))
	}
	if d == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("distro '%s' not found", h.distroID),
		})
	}

	apiDistro := &model.APIDistro{}
	apiDistro.BuildFromService(*d)

	return gimlet.NewJSONResponse(apiDistro.Setup)
}

///////////////////////////////////////////////////////////////////////
//
// PATCH /rest/v2/distros/{distro_id}/setup

type distroIDChangeSetupHandler struct {
	// The updated setup script.
	Setup    string `json:"setup"`
	distroID string
}

func makeChangeDistroSetup() gimlet.RouteHandler {
	return &distroIDChangeSetupHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Update the setup script for a distro
//	@Description	Update a distro's setup script. Accepts a JSON body with the new setup script.
//	@Tags			distros
//	@Router			/distros/{distro_id}/setup [patch]
//	@Security		Api-User || Api-Key
//	@Param			distro_id	path		string						true	"distro ID"
//	@Param			{object}	body		distroIDChangeSetupHandler	true	"the updated setup script"
//	@Success		200			{object}	model.APIDistro				"The updated distro with the new setup script"
func (h *distroIDChangeSetupHandler) Factory() gimlet.RouteHandler {
	return &distroIDChangeSetupHandler{}
}

// Parse fetches the distroId and JSON payload from the http request.
func (h *distroIDChangeSetupHandler) Parse(ctx context.Context, r *http.Request) error {
	h.distroID = gimlet.GetVars(r)["distro_id"]
	body := utility.NewRequestReader(r)
	defer body.Close()

	if err := utility.ReadJSON(body, h); err != nil {
		return errors.Wrap(err, "reading distro setup script parameters from request body")
	}

	return nil
}

// Run updates the setup script for the given distroId.
func (h *distroIDChangeSetupHandler) Run(ctx context.Context) gimlet.Responder {
	usr := MustHaveUser(ctx)
	d, err := distro.FindOneId(ctx, h.distroID)
	if err != nil || d == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("distro '%s' not found", h.distroID),
		})
	}

	oldDistro := *d
	d.Setup = h.Setup
	if err = data.UpdateDistro(ctx, d, d); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "updating distro '%s'", h.distroID))
	}
	event.LogDistroModified(ctx, h.distroID, usr.Username(), oldDistro.DistroData(), d.DistroData())

	apiDistro := &model.APIDistro{}
	apiDistro.BuildFromService(*d)

	return gimlet.NewJSONResponse(apiDistro)
}

///////////////////////////////////////////////////////////////////////
//
// PUT /rest/v2/distros/{distro_id}

type distroIDPutHandler struct {
	distroID string
	body     []byte
}

func makePutDistro() gimlet.RouteHandler {
	return &distroIDPutHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Replace or add a distro
//	@Description	Replace an existing distro by ID or add a new distro. If replacing an existing distro, all fields will be replaced with the new configuration.
//	@Tags			distros
//	@Router			/distros/{distro_id} [put]
//	@Security		Api-User || Api-Key
//	@Param			distro_id	path	string			true	"distro ID"
//	@Param			{object}	body	model.APIDistro	true	"the new distro"
//	@Success		201
//	@Success		200
func (h *distroIDPutHandler) Factory() gimlet.RouteHandler {
	return &distroIDPutHandler{}
}

// Parse fetches the distroId and JSON payload from the http request.
func (h *distroIDPutHandler) Parse(ctx context.Context, r *http.Request) error {
	h.distroID = gimlet.GetVars(r)["distro_id"]

	body := utility.NewRequestReader(r)
	defer body.Close()
	b, err := io.ReadAll(body)
	if err != nil {
		return errors.Wrap(err, "parsing request body")
	}
	h.body = b

	return nil
}

// Run either:
// (a) replaces an existing resource with the entity defined in the JSON payload, or
// (b) creates a new resource based on the Request-URI and JSON payload
func (h *distroIDPutHandler) Run(ctx context.Context) gimlet.Responder {
	user := MustHaveUser(ctx)
	original, err := distro.FindOneId(ctx, h.distroID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding distro '%s'", h.distroID))
	}

	apiDistro := &model.APIDistro{
		Name: utility.ToStringPtr(h.distroID),
		FinderSettings: model.APIFinderSettings{
			Version: utility.ToStringPtr(evergreen.FinderVersionLegacy),
		},
		PlannerSettings: model.APIPlannerSettings{
			Version: utility.ToStringPtr(evergreen.PlannerVersionTunable),
		},
		DispatcherSettings: model.APIDispatcherSettings{
			Version: utility.ToStringPtr(evergreen.DispatcherVersionRevisedWithDependencies),
		},
		HostAllocatorSettings: model.APIHostAllocatorSettings{
			Version:              utility.ToStringPtr(evergreen.HostAllocatorUtilization),
			AutoTuneMaximumHosts: true,
		},
		BootstrapSettings: model.APIBootstrapSettings{
			Method:        utility.ToStringPtr(distro.BootstrapMethodLegacySSH),
			Communication: utility.ToStringPtr(distro.CommunicationMethodLegacySSH),
		},
	}
	if err = json.Unmarshal(h.body, apiDistro); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "unmarshalling JSON request body into API distro model"))
	}

	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "getting admin settings"))
	}
	// Existing resource
	if original != nil {
		newDistro, respErr := validateDistro(ctx, apiDistro, h.distroID, settings, false)
		if respErr != nil {
			return respErr
		}

		if err = data.UpdateDistro(ctx, original, newDistro); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "updating existing distro '%s'", h.distroID))
		}
		event.LogDistroModified(ctx, h.distroID, user.Username(), original.DistroData(), newDistro.DistroData())
		if newDistro.GetDefaultAMI() != original.GetDefaultAMI() {
			event.LogDistroAMIModified(ctx, h.distroID, user.Username())
		}
		return gimlet.NewJSONResponse(struct{}{})
	}
	// New resource
	newDistro, respErr := validateDistro(ctx, apiDistro, h.distroID, settings, true)
	if respErr != nil {
		return respErr
	}

	responder := gimlet.NewJSONResponse(struct{}{})
	if err = responder.SetStatus(http.StatusCreated); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "setting HTTP status code to %d", http.StatusCreated))
	}
	if err = newDistro.Add(ctx, user); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "inserting new distro"))
	}

	return responder
}

// PUT /rest/v2/distros/{distro_id}/copy/{new_distro_id}

type distroCopyHandler struct {
	distroToCopy     string
	newDistroID      string
	singleTaskDistro bool
}

func makeCopyDistro() gimlet.RouteHandler {
	return &distroCopyHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Copy an existing distro
//	@Description	Create a new distro by copying an existing distro. Specifying "single task distro" will mark the copied distro as a single task distro. Aliases will not be copied.
//	@Tags			distros
//	@Router			/distros/{distro_id}/copy/{new_distro_id} [put]
//	@Security		Api-User || Api-Key
//	@Param			distro_id			path	string	true	"distro ID to copy"
//	@Param			new_distro_id		path	string	true	"ID of new distro to be created"
//	@Param			single_task_distro	query	bool	false	"if should the copied distro should be single task"
//	@Success		201
//	@Success		200
func (h *distroCopyHandler) Factory() gimlet.RouteHandler {
	return &distroCopyHandler{}
}

// Parse fetches the distroId and JSON payload from the http request.
func (h *distroCopyHandler) Parse(ctx context.Context, r *http.Request) error {
	h.distroToCopy = gimlet.GetVars(r)["distro_id"]
	h.newDistroID = gimlet.GetVars(r)["new_distro_id"]
	h.singleTaskDistro = r.URL.Query().Get("single_task_distro") == "true"

	if h.distroToCopy == h.newDistroID {
		return errors.New("new and existing distro IDs cannot be identical")
	}

	return nil
}

// Run creates a new distro by copying an existing distro.
func (h *distroCopyHandler) Run(ctx context.Context) gimlet.Responder {
	user := MustHaveUser(ctx)
	toCopy, err := distro.FindOneId(ctx, h.distroToCopy)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding distro '%s'", h.distroToCopy))
	}
	if toCopy == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("distro '%s' not found", h.distroToCopy),
		})
	}

	toCopy.Id = h.newDistroID
	if h.singleTaskDistro {
		toCopy.SingleTaskDistro = true
	}

	// Do not copy aliases because it could lead to the wrong distros being used.
	toCopy.Aliases = nil

	err = data.NewDistro(ctx, toCopy, user)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "creating and inserting new distro '%s'", h.newDistroID))
	}

	responder := gimlet.NewJSONResponse(struct{}{})
	if err = responder.SetStatus(http.StatusCreated); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "setting HTTP status code to %d", http.StatusCreated))
	}
	return responder
}

///////////////////////////////////////////////////////////////////////
//
// DELETE /rest/v2/distros/{distro_id}

type distroIDDeleteHandler struct {
	distroID string
}

func makeDeleteDistroByID() gimlet.RouteHandler {
	return &distroIDDeleteHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Delete a distro
//	@Description	Delete a distro by ID.
//	@Tags			distros
//	@Router			/distros/{distro_id} [delete]
//	@Security		Api-User || Api-Key
//	@Param			distro_id	path	string	true	"distro ID"
//	@Success		200
func (h *distroIDDeleteHandler) Factory() gimlet.RouteHandler {
	return &distroIDDeleteHandler{}
}

// Parse fetches the distroId from the http request.
func (h *distroIDDeleteHandler) Parse(ctx context.Context, r *http.Request) error {
	h.distroID = gimlet.GetVars(r)["distro_id"]

	return nil
}

// Run deletes a distro by id.
func (h *distroIDDeleteHandler) Run(ctx context.Context) gimlet.Responder {
	user := MustHaveUser(ctx)
	d, err := distro.FindOneId(ctx, h.distroID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding distro '%s'", h.distroID))
	}
	if d == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("distro '%s' not found", h.distroID),
		})
	}
	if err = data.DeleteDistroById(ctx, user, h.distroID); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "deleting distro '%s'", h.distroID))
	}

	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// PATCH /rest/v2/distros/{distro_id}

type distroIDPatchHandler struct {
	distroID string
	body     []byte
}

func makePatchDistroByID() gimlet.RouteHandler {
	return &distroIDPatchHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Update an existing distro
//	@Description	Update an existing distro by ID. Only updates the fields that are present in the request body.
//	@Tags			distros
//	@Router			/distros/{distro_id} [patch]
//	@Security		Api-User || Api-Key
//	@Param			distro_id	path		string			true	"distro ID"
//	@Param			{object}	body		model.APIDistro	true	"the updated distro"
//	@Success		200			{object}	model.APIDistro	"The updated distro"
func (h *distroIDPatchHandler) Factory() gimlet.RouteHandler {
	return &distroIDPatchHandler{}
}

// Parse fetches the distroId from the http request.
func (h *distroIDPatchHandler) Parse(ctx context.Context, r *http.Request) error {
	h.distroID = gimlet.GetVars(r)["distro_id"]
	body := utility.NewRequestReader(r)
	defer body.Close()
	b, err := io.ReadAll(body)
	if err != nil {
		return errors.Wrap(err, "reading request body")
	}
	h.body = b

	return nil
}

// Run updates a distro by id.
func (h *distroIDPatchHandler) Run(ctx context.Context) gimlet.Responder {
	user := MustHaveUser(ctx)
	old, err := distro.FindOneId(ctx, h.distroID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding distro '%s'", h.distroID))
	}
	if old == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("distro '%s' not found", h.distroID),
		})
	}

	apiDistro := &model.APIDistro{}
	apiDistro.BuildFromService(*old)
	oldSettingsList := apiDistro.ProviderSettingsList
	apiDistro.ProviderSettingsList = nil
	if err = json.Unmarshal(h.body, apiDistro); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "unmarshalling request body into distro API model"))
	}

	if len(apiDistro.ProviderSettingsList) == 0 {
		apiDistro.ProviderSettingsList = oldSettingsList
	}

	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "getting admin settings"))
	}
	d, respErr := validateDistro(ctx, apiDistro, h.distroID, settings, false)
	if respErr != nil {
		return respErr
	}

	if err = data.UpdateDistro(ctx, old, d); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "updating distro '%s'", h.distroID))
	}
	event.LogDistroModified(ctx, h.distroID, user.Username(), old.DistroData(), d.DistroData())
	if d.GetDefaultAMI() != old.GetDefaultAMI() {
		event.LogDistroAMIModified(ctx, h.distroID, user.Username())
	}
	return gimlet.NewJSONResponse(apiDistro)
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/distros/{distro_id}

type distroIDGetHandler struct {
	distroID string
}

func makeGetDistroByID() gimlet.RouteHandler {
	return &distroIDGetHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Get a single distro
//	@Description	Fetch a single distro by its ID.
//	@Tags			distros
//	@Router			/distros/{distro_id} [get]
//	@Security		Api-User || Api-Key
//	@Param			distro_id	path		string	true	"distro ID"
//	@Success		200			{object}	model.APIDistro
func (h *distroIDGetHandler) Factory() gimlet.RouteHandler {
	return &distroIDGetHandler{}
}

// Parse fetches the distroId from the http request.
func (h *distroIDGetHandler) Parse(ctx context.Context, r *http.Request) error {
	h.distroID = gimlet.GetVars(r)["distro_id"]

	return nil
}

// Run calls the data FindDistroById function and returns the distro from the provider.
func (h *distroIDGetHandler) Run(ctx context.Context) gimlet.Responder {
	d, err := distro.FindOneId(ctx, h.distroID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding distro '%s'", h.distroID))
	}
	if d == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("distro '%s' not found", h.distroID),
		})
	}

	apiDistro := &model.APIDistro{}
	apiDistro.BuildFromService(*d)

	return gimlet.NewJSONResponse(apiDistro)
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/distros/{distro_id}/ami
// This is an agent route and should not be documented.

type distroAMIHandler struct {
	distroID string
	region   string
}

func makeGetDistroAMI() gimlet.RouteHandler {
	return &distroAMIHandler{}
}

func (h *distroAMIHandler) Factory() gimlet.RouteHandler {
	return &distroAMIHandler{}
}

func (h *distroAMIHandler) Parse(ctx context.Context, r *http.Request) error {
	h.distroID = gimlet.GetVars(r)["distro_id"]
	vals := r.URL.Query()
	h.region = vals.Get("region")
	if h.region == "" {
		h.region = evergreen.DefaultEC2Region
	}
	return nil
}

func (h *distroAMIHandler) Run(ctx context.Context) gimlet.Responder {
	d, err := distro.FindOneId(ctx, h.distroID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding distro '%s'", h.distroID))
	}
	if d == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("distro '%s' not found", h.distroID),
		})
	}

	if !evergreen.IsEc2Provider(d.Provider) {
		return gimlet.NewJSONResponse("")
	}

	for _, ec2Settings := range d.ProviderSettingsList {
		curRegion, ok := ec2Settings.Lookup("region").StringValueOK()
		if !ok || curRegion != h.region {
			continue
		}

		ami, _ := ec2Settings.Lookup("ami").StringValueOK()
		return gimlet.NewTextResponse(ami)
	}

	return gimlet.MakeJSONErrorResponder(errors.Errorf("no settings available for region '%s' for distro '%s'", h.region, h.distroID))
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/distros

type distroGetHandler struct{}

func makeDistroRoute() gimlet.RouteHandler {
	return &distroGetHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Get all distros
//	@Description	Fetches all available distros.
//	@Tags			distros
//	@Router			/distros [get]
//	@Security		Api-User || Api-Key
//	@Success		200	{array}	model.APIDistro
func (h *distroGetHandler) Factory() gimlet.RouteHandler {
	return &distroGetHandler{}
}

func (h *distroGetHandler) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *distroGetHandler) Run(ctx context.Context) gimlet.Responder {
	distros, err := distro.AllDistros(ctx)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "finding all distros"))
	}
	if len(distros) == 0 {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    "no distros found",
		})
	}

	resp := gimlet.NewResponseBuilder()
	for _, d := range distros {
		distroModel := &model.APIDistro{}
		distroModel.BuildFromService(d)

		err = resp.AddData(distroModel)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "adding response data for distro '%s'", d.Id))
		}
	}

	return resp
}

////////////////////////////////////////////////////////////////////////

func validateDistro(ctx context.Context, apiDistro *model.APIDistro, resourceID string, settings *evergreen.Settings, isNewDistro bool) (*distro.Distro, gimlet.Responder) {
	d := apiDistro.ToService()

	id := utility.FromStringPtr(apiDistro.Name)
	if resourceID != id {
		return nil, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusForbidden,
			Message:    fmt.Sprintf("distro name '%s' is immutable so it cannot be renamed to '%s'", id, resourceID),
		})
	}
	vErrors, err := validator.CheckDistro(ctx, d, settings, isNewDistro)
	if err != nil {
		return nil, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		})
	}
	if len(vErrors) != 0 {
		return nil, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    vErrors.String(),
		})
	}

	return d, nil
}

// GET /rest/v2/distros/{distro_id}/client_urls
// This is an agent route and should not be documented.

type distroClientURLsGetHandler struct {
	env      evergreen.Environment
	distroID string
}

func makeGetDistroClientURLs(env evergreen.Environment) gimlet.RouteHandler {
	return &distroClientURLsGetHandler{
		env: env,
	}
}

func (rh *distroClientURLsGetHandler) Factory() gimlet.RouteHandler {
	return &distroClientURLsGetHandler{
		env: rh.env,
	}
}

func (rh *distroClientURLsGetHandler) Parse(ctx context.Context, r *http.Request) error {
	rh.distroID = gimlet.GetVars(r)["distro_id"]
	return nil
}

func (rh *distroClientURLsGetHandler) Run(ctx context.Context) gimlet.Responder {
	d, err := distro.FindOneId(ctx, rh.distroID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding distro '%s'", rh.distroID))
	}
	if d == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("distro '%s' not found", rh.distroID),
		})
	}

	var urls []string
	if rh.env.ClientConfig().S3URLPrefix != "" {
		urls = append(urls, d.S3ClientURL(rh.env))
	}

	return gimlet.NewJSONResponse(urls)
}
