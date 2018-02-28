package route

import (
	"context"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	dataModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/amboy"
	"github.com/pkg/errors"
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
	settings, err := sc.GetEvergreenSettings()
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}
	settingsModel := model.NewConfigModel()
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
	model *model.APIAdminSettings
}

func (h *adminPostHandler) Handler() RequestHandler {
	return &adminPostHandler{}
}

func (h *adminPostHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	return errors.Wrap(util.ReadJSONInto(r.Body, &h.model), "error parsing request body")
}

func (h *adminPostHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	u := MustHaveUser(ctx)
	oldSettings, err := sc.GetEvergreenSettings()
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "error retrieving existing settings")
	}

	// validate the changes
	newSettings, err := sc.SetEvergreenSettings(h.model, oldSettings, u, false)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "error applying new settings")
	}
	err = newSettings.Validate()
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "Validation error")
	}

	_, err = sc.SetEvergreenSettings(h.model, oldSettings, u, true)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}
	err = h.model.BuildFromService(newSettings)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "error building API model")
	}
	return ResponseData{
		Result: []model.Model{h.model},
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

	bgh := &bannerGetHandler{}
	bannerGet := MethodHandler{
		PrefetchFunctions: []PrefetchFunc{PrefetchUser},
		Authenticator:     &RequireUserAuthenticator{},
		RequestHandler:    bgh.Handler(),
		MethodType:        http.MethodGet,
	}

	bannerRoute := RouteManager{
		Route:   route,
		Methods: []MethodHandler{bannerPost, bannerGet},
		Version: version,
	}
	return &bannerRoute
}

type bannerPostHandler struct {
	Banner model.APIString `json:"banner"`
	Theme  model.APIString `json:"theme"`
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
		Text:  h.Banner,
		Theme: h.Theme,
	}
	return nil
}

func (h *bannerPostHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	u := MustHaveUser(ctx)
	if err := sc.SetAdminBanner(string(h.Banner), u); err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}
	if err := sc.SetBannerTheme(string(h.Theme), u); err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}
	return ResponseData{
		Result: []model.Model{&h.model},
	}, nil
}

type bannerGetHandler struct{}

func (h *bannerGetHandler) Handler() RequestHandler {
	return &bannerGetHandler{}
}

func (h *bannerGetHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *bannerGetHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	banner, theme, err := sc.GetBanner()
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}
	return ResponseData{
		Result: []model.Model{&model.APIBanner{Text: model.APIString(banner), Theme: model.APIString(theme)}},
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
	return errors.WithStack(util.ReadJSONInto(r.Body, h))
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

	err = sc.SetServiceFlags(flags.(evergreen.ServiceFlags), u)
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
func getRestartRouteManager(queue amboy.Queue) routeManagerFactory {
	return func(route string, version int) *RouteManager {
		rh := &restartHandler{
			queue: queue,
		}

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
}

type restartHandler struct {
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	DryRun     bool      `json:"dry_run"`
	OnlyRed    bool      `json:"only_red"`
	OnlyPurple bool      `json:"only_purple"`

	queue amboy.Queue
}

func (h *restartHandler) Handler() RequestHandler {
	return &restartHandler{
		queue: h.queue,
	}
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
	opts := dataModel.RestartTaskOptions{
		DryRun:     h.DryRun,
		OnlyRed:    h.OnlyRed,
		OnlyPurple: h.OnlyPurple,
		StartTime:  h.StartTime,
		EndTime:    h.EndTime,
		User:       u.Username(),
	}
	resp, err := sc.RestartFailedTasks(h.queue, opts)
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
