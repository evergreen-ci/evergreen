package route

import (
	"net/http"

	"github.com/evergreen-ci/evergreen"
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
		MethodType:        evergreen.MethodGet,
	}

	aph := &adminPostHandler{}
	adminPost := MethodHandler{
		PrefetchFunctions: []PrefetchFunc{PrefetchUser},
		Authenticator:     &SuperUserAuthenticator{},
		RequestHandler:    aph.Handler(),
		MethodType:        evergreen.MethodPost,
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
	settings, err := h.model.ToService()
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "API model error")
		}
		return ResponseData{}, err
	}
	err = sc.SetAdminSettings(settings.(admin.AdminSettings))
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
		MethodType:        evergreen.MethodPost,
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
	err := sc.SetAdminBanner(string(h.Banner))
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
		MethodType:        evergreen.MethodPost,
	}

	flagsRoute := RouteManager{
		Route:   route,
		Methods: []MethodHandler{flagsPost},
		Version: version,
	}
	return &flagsRoute
}

type flagsPostHandler struct {
	TaskDispatchDisabled  bool `json:"task_dispatch_disabled"`
	HostinitDisabled      bool `json:"hostinit_disabled"`
	MonitorDisabled       bool `json:"monitor_disabled"`
	NotificationsDisabled bool `json:"notifications_disabled"`
	AlertsDisabled        bool `json:"alerts_disabled"`
	TaskrunnerDisabled    bool `json:"taskrunner_disabled"`
	RepotrackerDisabled   bool `json:"repotracker_disabled"`
	SchedulerDisabled     bool `json:"scheduler_disabled"`
	model                 model.APIServiceFlags
}

func (h *flagsPostHandler) Handler() RequestHandler {
	return &flagsPostHandler{}
}

func (h *flagsPostHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	if err := util.ReadJSONInto(r.Body, h); err != nil {
		return err
	}
	h.model = model.APIServiceFlags{
		TaskDispatchDisabled:  h.TaskDispatchDisabled,
		HostinitDisabled:      h.HostinitDisabled,
		MonitorDisabled:       h.MonitorDisabled,
		NotificationsDisabled: h.NotificationsDisabled,
		AlertsDisabled:        h.AlertsDisabled,
		TaskrunnerDisabled:    h.TaskrunnerDisabled,
		RepotrackerDisabled:   h.RepotrackerDisabled,
		SchedulerDisabled:     h.SchedulerDisabled,
	}
	return nil
}

func (h *flagsPostHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	flags, err := h.model.ToService()
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "API model error")
		}
		return ResponseData{}, err
	}
	err = sc.SetServiceFlags(flags.(admin.ServiceFlags))
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
