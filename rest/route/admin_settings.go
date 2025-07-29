package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// GET rest/v2/admin/settings

func makeFetchAdminSettings() gimlet.RouteHandler {
	return &adminGetHandler{}
}

type adminGetHandler struct{}

func (h *adminGetHandler) Factory() gimlet.RouteHandler {
	return &adminGetHandler{}
}

func (h *adminGetHandler) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *adminGetHandler) Run(ctx context.Context) gimlet.Responder {
	var settings *evergreen.Settings
	var err error
	if evergreen.GetEnvironment().SharedDB() != nil {
		settings, err = evergreen.GetRawConfig(ctx)
	} else {
		settings, err = evergreen.GetConfig(ctx)
	}
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting admin settings"))
	}
	settingsModel := model.NewConfigModel()

	err = settingsModel.BuildFromService(settings)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "converting admin settings to API model"))
	}

	return gimlet.NewJSONResponse(settingsModel)
}

func makeFetchAdminUIV2Url() gimlet.RouteHandler {
	return &uiV2URLGetHandler{}
}

type uiV2URLGetHandler struct{}

func (h *uiV2URLGetHandler) Factory() gimlet.RouteHandler {
	return &uiV2URLGetHandler{}
}

func (h *uiV2URLGetHandler) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *uiV2URLGetHandler) Run(ctx context.Context) gimlet.Responder {
	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting admin settings"))
	}

	return gimlet.NewJSONResponse(&model.APIUiV2URL{
		UIv2Url: utility.ToStringPtr(settings.Ui.UIv2Url),
	})
}

// POST rest/v2/admin/settings

func makeSetAdminSettings() gimlet.RouteHandler {
	return &adminPostHandler{}
}

type adminPostHandler struct {
	model *model.APIAdminSettings
}

func (h *adminPostHandler) Factory() gimlet.RouteHandler {
	return &adminPostHandler{}
}

func (h *adminPostHandler) Parse(ctx context.Context, r *http.Request) error {
	return errors.Wrap(gimlet.GetJSON(r.Body, &h.model), "parsing request body")
}

func (h *adminPostHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)
	oldSettings, err := evergreen.GetRawConfig(ctx)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting existing admin settings"))
	}

	// validate the changes
	newSettings, err := data.SetEvergreenSettings(ctx, h.model, oldSettings, u, false)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "dry run applying new settings"))
	}
	if err = newSettings.Validate(); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "new settings are invalid"))
	}

	err = distro.ValidateContainerPoolDistros(ctx, newSettings)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "container pool distros are invalid"))
	}

	_, err = data.SetEvergreenSettings(ctx, h.model, oldSettings, u, true)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "applying new settings"))
	}

	err = h.model.BuildFromService(newSettings)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "converting new admin settings to API model"))
	}

	return gimlet.NewJSONResponse(h.model)
}

func makeFetchTaskLimits() gimlet.RouteHandler {
	return &taskLimitsGetHandler{}
}

type taskLimitsGetHandler struct{}

// Factory creates an instance of the handler.
//
//	@Summary		Get Evergreen's task limits
//	@Description	Returns all of Evergreen's task-related limitations.
//	@Tags			admin
//	@Router			/admin/task_limits [get]
//	@Security		Api-User || Api-Key
//	@Success		200	{object}	model.APITaskLimitsConfig
func (h *taskLimitsGetHandler) Factory() gimlet.RouteHandler {
	return &taskLimitsGetHandler{}
}

func (h *taskLimitsGetHandler) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *taskLimitsGetHandler) Run(ctx context.Context) gimlet.Responder {
	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting admin settings"))
	}
	taskLimits := &model.APITaskLimitsConfig{}
	if err = taskLimits.BuildFromService(settings.TaskLimits); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "converting task limits to API model"))
	}
	return gimlet.NewJSONResponse(taskLimits)
}
