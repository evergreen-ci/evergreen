package route

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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
	d, err := distro.FindOneId(ctx, h.distroID)
	if err != nil || d == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("distro '%s' not found", h.distroID),
		})
	}

	d.Setup = h.Setup
	if err = data.UpdateDistro(ctx, d, d); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "updating distro '%s'", h.distroID))
	}

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
			Version: utility.ToStringPtr(evergreen.PlannerVersionLegacy),
		},
		DispatcherSettings: model.APIDispatcherSettings{
			Version: utility.ToStringPtr(evergreen.DispatcherVersionRevisedWithDependencies),
		},
		HostAllocatorSettings: model.APIHostAllocatorSettings{
			Version: utility.ToStringPtr(evergreen.HostAllocatorUtilization),
		},
		BootstrapSettings: model.APIBootstrapSettings{
			Method:        utility.ToStringPtr(distro.BootstrapMethodLegacySSH),
			Communication: utility.ToStringPtr(distro.CommunicationMethodLegacySSH),
		},
		CloneMethod: utility.ToStringPtr(evergreen.CloneMethodLegacySSH),
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
		event.LogDistroModified(h.distroID, user.Username(), original.DistroData(), newDistro.DistroData())
		if newDistro.GetDefaultAMI() != original.GetDefaultAMI() {
			event.LogDistroAMIModified(h.distroID, user.Username())
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
	if err = newDistro.Insert(ctx); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "inserting new distro"))
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
	event.LogDistroModified(h.distroID, user.Username(), old.DistroData(), d.DistroData())
	if d.GetDefaultAMI() != old.GetDefaultAMI() {
		event.LogDistroAMIModified(h.distroID, user.Username())
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

type distroAMIHandler struct {
	distroID string
	region   string
}

func makeGetDistroAMI() gimlet.RouteHandler {
	return &distroAMIHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Get a single distro's AMI
//	@Description	Fetch a single distro's AMI by its distro ID.
//	@Tags			distros
//	@Router			/distros/{distro_id}/ami [get]
//	@Security		Api-User || Api-Key
//	@Param			distro_id	path		string	true	"distro ID"
//	@Success		200			{string}	string	"The distro's AMI"
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
	if err != nil || d == nil {
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
// PATCH /rest/v2/distros/settings modifies provider settings across all distros for the given region

type modifyDistrosSettingsHandler struct {
	settings *birch.Document
	region   string
	dryRun   bool
}

func makeModifyDistrosSettings() gimlet.RouteHandler {
	return &modifyDistrosSettingsHandler{
		settings: &birch.Document{},
	}
}

func (h *modifyDistrosSettingsHandler) Factory() gimlet.RouteHandler {
	return &modifyDistrosSettingsHandler{
		settings: &birch.Document{},
	}
}

func (h *modifyDistrosSettingsHandler) Parse(ctx context.Context, r *http.Request) error {
	body := utility.NewRequestReader(r)
	defer body.Close()
	b, err := io.ReadAll(body)
	if err != nil {
		return errors.Wrap(err, "reading request body")
	}
	if err = json.Unmarshal(b, h.settings); err != nil {
		return errors.Wrap(err, "unmarshalling request body as document")
	}

	var ok bool
	h.region, ok = h.settings.Lookup("region").StringValueOK()
	if !ok || h.region == "" {
		return errors.New("region must be explicitly defined")
	}

	vals := r.URL.Query()
	h.dryRun = vals.Get("dry_run") == "true"

	return nil
}

func (h *modifyDistrosSettingsHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)
	allDistros, err := distro.AllDistros(ctx)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "finding all distros"))
	}
	if len(allDistros) == 0 {
		return gimlet.NewJSONInternalErrorResponse(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    "no distros found",
		})
	}
	modifiedDistros := []distro.Distro{}
	modifiedAMIDistroIds := []string{}
	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "getting admin settings"))
	}
	catcher := grip.NewBasicCatcher()
	for _, d := range allDistros {
		if !evergreen.IsEc2Provider(d.Provider) || len(d.ProviderSettingsList) <= 1 {
			continue
		}
		originalAMI := d.GetDefaultAMI()
		for i, doc := range d.ProviderSettingsList {
			// validate distro with old settings
			originalErrors, err := validator.CheckDistro(ctx, &d, settings, false)
			if err != nil {
				catcher.Wrapf(err, "validating original distro '%s'", d.Id)
				continue
			}

			if region, ok := doc.Lookup("region").StringValueOK(); !ok || region != h.region {
				continue
			}

			// update doc with new changes
			for _, elem := range h.settings.Elements() {
				doc = doc.Set(elem)
			}

			d.ProviderSettingsList[i] = doc
			// validate distro with new settings
			vErrors, err := validator.CheckDistro(ctx, &d, settings, false)
			if err != nil {
				catcher.Wrapf(err, "validating updated distro '%s'", d.Id)
				continue
			}
			if len(vErrors) != 0 {
				if len(originalErrors) != 0 {
					grip.Info(message.Fields{
						"message":         "not updating settings for invalid distro",
						"route":           "/distros/settings",
						"update_doc":      h.settings,
						"dry_run":         h.dryRun,
						"distro":          d.Id,
						"original_errors": originalErrors.String(),
						"new_errors":      vErrors.String(),
					})
					continue
				}
				catcher.Errorf("distro '%s' is not valid: %s", d.Id, vErrors.String())
				continue
			}
			modifiedDistros = append(modifiedDistros, d)
		}
		if h.region == evergreen.DefaultEC2Region && originalAMI != d.GetDefaultAMI() {
			modifiedAMIDistroIds = append(modifiedAMIDistroIds, d.Id)
		}
	}
	if catcher.HasErrors() {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(catcher.Resolve(), "no distros updated"))
	}

	modifiedIDs := []string{}
	for _, d := range modifiedDistros {
		if !h.dryRun {
			old, err := distro.FindOneId(ctx, d.Id)
			if err != nil {
				catcher.Wrapf(err, "finding existing distro '%s'", d.Id)
				continue

			}

			if err = d.ReplaceOne(ctx); err != nil {
				catcher.Wrapf(err, "updating distro '%s'", d.Id)
				continue
			}
			event.LogDistroModified(d.Id, u.Username(), old.DistroData(), d.DistroData())
		}

		modifiedIDs = append(modifiedIDs, d.Id)
	}
	if !h.dryRun {
		for _, distroId := range modifiedAMIDistroIds {
			event.LogDistroAMIModified(distroId, u.Username())
		}
	}

	if catcher.HasErrors() {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(catcher.Resolve(), "bulk updating distros"))
	}
	grip.Info(message.Fields{
		"message":              "updated distro provider settings",
		"route":                "/distros/settings",
		"update_doc":           h.settings,
		"dry_run":              h.dryRun,
		"distros":              modifiedIDs,
		"modified_ami_distros": modifiedAMIDistroIds,
	})
	return gimlet.NewJSONResponse(modifiedIDs)
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

///////////////////////////////////////////////////////////////////////
//
// PATCH /rest/v2/distros/{distro}/execute

type distroExecuteHandler struct {
	opts   model.APIDistroScriptOptions
	distro string
	env    evergreen.Environment
}

func makeDistroExecute(env evergreen.Environment) gimlet.RouteHandler {
	return &distroExecuteHandler{
		env: env,
	}
}

func (h *distroExecuteHandler) Factory() gimlet.RouteHandler {
	return &distroExecuteHandler{
		env: h.env,
	}
}

// Parse fetches the distro and JSON payload from the http request.
func (h *distroExecuteHandler) Parse(ctx context.Context, r *http.Request) error {
	h.distro = gimlet.GetVars(r)["distro_id"]
	body := utility.NewRequestReader(r)
	defer body.Close()

	if err := utility.ReadJSON(body, &h.opts); err != nil {
		return errors.Wrap(err, "reading request body")
	}

	if h.opts.Script == "" {
		return errors.New("cannot execute an empty script")
	}
	if !h.opts.IncludeTaskHosts && !h.opts.IncludeSpawnHosts {
		return errors.New("cannot exclude both spawn hosts and task hosts from script execution")
	}

	return nil
}

// Run enqueues a job to run a script on all selected hosts that are not down
// for the given given distro ID.
func (h *distroExecuteHandler) Run(ctx context.Context) gimlet.Responder {
	hosts, err := host.Find(ctx, host.ByDistroIDsOrAliasesRunning(h.distro))
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding hosts for the distro '%s'", h.distro))
	}

	var allHostIDs []string
	for _, host := range hosts {
		allHostIDs = append(allHostIDs, host.Id)
	}
	catcher := grip.NewBasicCatcher()
	var hostIDs []string
	for _, host := range hosts {
		ts := utility.RoundPartOfMinute(0).Format(units.TSFormat)
		if (host.StartedBy == evergreen.User && h.opts.IncludeTaskHosts) || (host.UserHost && h.opts.IncludeSpawnHosts) {
			if err = h.env.RemoteQueue().Put(ctx, units.NewHostExecuteJob(h.env, host, h.opts.Script, h.opts.Sudo, h.opts.SudoUser, ts)); err != nil {
				catcher.Wrapf(err, "enqueueing job to run script on host '%s'", host.Id)
				continue
			}
			hostIDs = append(hostIDs, host.Id)
		}
	}
	if catcher.HasErrors() {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "bulk enqueueing jobs to run script on distro hosts"))
	}

	return gimlet.NewJSONResponse(struct {
		HostIDs []string `json:"host_ids"`
	}{HostIDs: hostIDs})
}

///////////////////////////////////////////////////////////////////////
//
// PATCH /rest/v2/distros/{distro}/icecream_config

type distroIcecreamConfigHandler struct {
	distro string
	opts   model.APIDistroScriptOptions
	env    evergreen.Environment
}

func makeDistroIcecreamConfig(env evergreen.Environment) gimlet.RouteHandler {
	return &distroIcecreamConfigHandler{
		env: env,
	}
}

func (h *distroIcecreamConfigHandler) Factory() gimlet.RouteHandler {
	return &distroIcecreamConfigHandler{
		env: h.env,
	}
}

// Parse extracts the distro and JSON payload from the http request.
func (h *distroIcecreamConfigHandler) Parse(ctx context.Context, r *http.Request) error {
	h.distro = gimlet.GetVars(r)["distro_id"]
	body := utility.NewRequestReader(r)
	defer body.Close()

	if err := utility.ReadJSON(body, &h.opts); err != nil {
		return errors.Wrap(err, "reading request body")
	}

	return nil
}

// Run enqueues a job to run a script on all hosts that are not down for the
// given given distro ID.
func (h *distroIcecreamConfigHandler) Run(ctx context.Context) gimlet.Responder {
	hosts, err := host.Find(ctx, host.ByDistroIDsOrAliasesRunning(h.distro))
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding hosts for distro '%s'", h.distro))
	}

	dat, err := distro.NewDistroAliasesLookupTable(ctx)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting distro lookup table"))
	}

	catcher := grip.NewBasicCatcher()
	var hostIDs []string
	for _, host := range hosts {
		if host.StartedBy == evergreen.User || !host.IsVirtualWorkstation {
			continue
		}

		// If the distro exists, we use the settings directly from that distro;
		// if the distro in the host document is deleted, we make a best-effort
		// attempt to resolve it to a real distro by attempting to pick any
		// existing distro with an alias that matches the deleted distro.
		distroIDs := dat.Expand([]string{host.Distro.Id})
		if len(distroIDs) == 0 {
			catcher.Errorf("distro '%s' not found", host.Distro.Id)
			continue
		}
		var distros []distro.Distro
		distros, err = distro.Find(ctx, distro.ByIds(distroIDs))
		if err != nil {
			catcher.Wrapf(err, "finding distros '%s' for host '%s'", host.Distro.Id, host.Id)
			continue
		}
		var d distro.Distro
		var distroFound bool
		for _, d = range distros {
			if d.IceCreamSettings.Populated() {
				distroFound = true
				break
			}
		}
		if !distroFound {
			catcher.Errorf("could not find any distro '%s' for host '%s' with populated icecream settings", host.Distro.Id, host.Id)
			continue
		}

		script := d.IceCreamSettings.GetUpdateConfigScript()
		ts := utility.RoundPartOfMinute(0).Format(units.TSFormat)
		if err = h.env.RemoteQueue().Put(ctx, units.NewHostExecuteJob(h.env, host, script, true, "root", ts)); err != nil {
			catcher.Wrapf(err, "enqueueing job to update Icecream config file on host '%s'", host.Id)
			continue
		}
		hostIDs = append(hostIDs, host.Id)
	}

	if catcher.HasErrors() {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "bulk enqueueing jobs to update Icecream config on hosts"))
	}

	return gimlet.NewJSONResponse(struct {
		HostIDs []string `json:"host_ids"`
	}{HostIDs: hostIDs})
}

// GET /rest/v2/distros/{distro_id}/client_urls

type distroClientURLsGetHandler struct {
	env      evergreen.Environment
	distroID string
}

func makeGetDistroClientURLs(env evergreen.Environment) gimlet.RouteHandler {
	return &distroClientURLsGetHandler{
		env: env,
	}
}

// Factory creates an instance of the handler.
//
//	@Summary		Get Evergreen client URLs for a distro
//	@Description	Returns the URLs for downloading the Evergreen client for a distro.
//	@Tags			distros
//	@Router			/distros/{distro_id}/client_urls [get]
//	@Success		200	{array}	string	"The URLs for downloading the Evergreen client"
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

	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting admin settings"))
	}

	var urls []string
	settings := rh.env.Settings()
	if !flags.S3BinaryDownloadsDisabled && rh.env.ClientConfig().S3URLPrefix != "" {
		urls = append(urls, d.S3ClientURL(rh.env))
	}
	urls = append(urls, d.ClientURL(settings))

	return gimlet.NewJSONResponse(urls)
}
