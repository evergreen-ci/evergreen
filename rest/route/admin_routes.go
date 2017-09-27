package route

import (
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/model/admin"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// this manages the /admin route, which allows getting/setting the admin settings
func getAdminSettingsManager(route string, version int) *RouteManager {
	agh := &adminGetHandler{}
	adminGet := MethodHandler{
		PrefetchFunctions: []PrefetchFunc{PrefetchUser},
		Authenticator:     &SuperUserAuthenticator{},
		RequestHandler:    agh.Handler(),
		MethodType:        http.MethodGet,
	}

	aph := &adminPostHandler{}
	adminPost := MethodHandler{
		PrefetchFunctions: []PrefetchFunc{PrefetchUser},
		Authenticator:     &SuperUserAuthenticator{},
		RequestHandler:    aph.Handler(),
		MethodType:        http.MethodPost,
	}

	adminRoute := RouteManager{
		Route:   route,
		Methods: []MethodHandler{adminGet, adminPost},
		Version: version,
	}
	return &adminRoute
}

type adminGetHandler struct{}

func (h *adminGetHandler) Handler() RequestHandler {
	return &adminGetHandler{}
}

func (h *adminGetHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *adminGetHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	settings, err := sc.GetAdminSettings()
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}
	settingsModel := &model.APIAdminSettings{}
	err = settingsModel.BuildFromService(settings)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "API model error")
		}
		return ResponseData{}, err
	}
	return ResponseData{
		Result: []model.Model{settingsModel},
	}, nil
}

type adminPostHandler struct {
	Banner       model.APIString       `json:"banner"`
	ServiceFlags model.APIServiceFlags `json:"service_flags"`
	model        model.APIAdminSettings
}

func (h *adminPostHandler) Handler() RequestHandler {
	return &adminPostHandler{}
}

func (h *adminPostHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	if err := util.ReadJSONInto(r.Body, h); err != nil {
		return err
	}
	h.model = model.APIAdminSettings{
		Banner:       h.Banner,
		ServiceFlags: h.ServiceFlags,
	}
	return nil
}

func (h *adminPostHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	u := MustHaveUser(ctx)
	settingsModel, err := h.model.ToService()
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "API model error")
		}
		return ResponseData{}, err
	}
	settings := settingsModel.(admin.AdminSettings)
	err = sc.SetAdminSettings(&settings, u)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}
	return ResponseData{
		Result: []model.Model{&h.model},
	}, nil
}

// this manages the /admin/banner route, which allows setting the banner
func getBannerRouteManager(route string, version int) *RouteManager {
	bph := &bannerPostHandler{}
	bannerPost := MethodHandler{
		PrefetchFunctions: []PrefetchFunc{PrefetchUser},
		Authenticator:     &SuperUserAuthenticator{},
		RequestHandler:    bph.Handler(),
		MethodType:        http.MethodPost,
	}

	bannerRoute := RouteManager{
		Route:   route,
		Methods: []MethodHandler{bannerPost},
		Version: version,
	}
	return &bannerRoute
}

type bannerPostHandler struct {
	Banner model.APIString `json:"banner"`
	model  model.APIBanner
}

func (h *bannerPostHandler) Handler() RequestHandler {
	return &bannerPostHandler{}
}

func (h *bannerPostHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	if err := util.ReadJSONInto(r.Body, h); err != nil {
		return err
	}
	h.model = model.APIBanner{
		Text: h.Banner,
	}
	return nil
}

func (h *bannerPostHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	u := MustHaveUser(ctx)
	err := sc.SetAdminBanner(string(h.Banner), u)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}
	return ResponseData{
		Result: []model.Model{&h.model},
	}, nil
}

// this manages the /admin/service_flags route, which allows setting the service flags
func getServiceFlagsRouteManager(route string, version int) *RouteManager {
	fph := &flagsPostHandler{}
	flagsPost := MethodHandler{
		PrefetchFunctions: []PrefetchFunc{PrefetchUser},
		Authenticator:     &SuperUserAuthenticator{},
		RequestHandler:    fph.Handler(),
		MethodType:        http.MethodPost,
	}

	flagsRoute := RouteManager{
		Route:   route,
		Methods: []MethodHandler{flagsPost},
		Version: version,
	}
	return &flagsRoute
}

type flagsPostHandler struct {
	Flags model.APIServiceFlags `json:"service_flags"`
}

func (h *flagsPostHandler) Handler() RequestHandler {
	return &flagsPostHandler{}
}

func (h *flagsPostHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	if err := util.ReadJSONInto(r.Body, h); err != nil {
		return err
	}
	return nil
}

func (h *flagsPostHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	u := MustHaveUser(ctx)
	flags, err := h.Flags.ToService()
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "API model error")
		}
		return ResponseData{}, err
	}

	err = sc.SetServiceFlags(flags.(admin.ServiceFlags), u)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}
	return ResponseData{
		Result: []model.Model{&h.Flags},
	}, nil
}

// this manages the /admin/restart route, which restarts failed tasks
func getRestartRouteManager(route string, version int) *RouteManager {
	rh := &restartHandler{}
	restartHandler := MethodHandler{
		PrefetchFunctions: []PrefetchFunc{PrefetchUser},
		Authenticator:     &SuperUserAuthenticator{},
		RequestHandler:    rh.Handler(),
		MethodType:        http.MethodPost,
	}

	restartRoute := RouteManager{
		Route:   route,
		Methods: []MethodHandler{restartHandler},
		Version: version,
	}
	return &restartRoute
}

type restartHandler struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	DryRun    bool      `json:"dry_run"`
}

func (h *restartHandler) Handler() RequestHandler {
	return &restartHandler{}
}

func (h *restartHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	if err := util.ReadJSONInto(r.Body, h); err != nil {
		return err
	}
	defer r.Body.Close()
	if h.EndTime.Before(h.StartTime) {
		return rest.APIError{
			StatusCode: 400,
			Message:    "End time cannot be before start time",
		}
	}
	return nil
}

func (h *restartHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	u := MustHaveUser(ctx)
	resp, err := sc.RestartFailedTasks(h.StartTime, h.EndTime, u.Username(), h.DryRun)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Error restarting tasks")
		}
		return ResponseData{}, err
	}
	restartModel := &model.RestartTasksResponse{}
	if err = restartModel.BuildFromService(resp); err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "API model error")
		}
		return ResponseData{}, err
	}
	return ResponseData{
		Result: []model.Model{restartModel},
	}, nil
}
