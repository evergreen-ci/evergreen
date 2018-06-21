package route

import (
	"context"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	dataModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"github.com/mongodb/amboy"
	"github.com/pkg/errors"
)

// this route is to preserve backwards compatibility with old versions of the CLI
func getLegacyAdminSettingsManager(route string, version int) *RouteManager {
	agh := &legacyAdminGetHandler{}
	adminGet := MethodHandler{
		Authenticator:  &NoAuthAuthenticator{},
		RequestHandler: agh.Handler(),
		MethodType:     http.MethodGet,
	}

	adminRoute := RouteManager{
		Route:   route,
		Methods: []MethodHandler{adminGet},
		Version: version,
	}
	return &adminRoute
}

type legacyAdminGetHandler struct{}

func (h *legacyAdminGetHandler) Handler() RequestHandler {
	return &legacyAdminGetHandler{}
}

func (h *legacyAdminGetHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *legacyAdminGetHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
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
	modelCopy := model.NewConfigModel()
	modelCopy.Banner = settingsModel.Banner
	modelCopy.BannerTheme = settingsModel.BannerTheme
	return ResponseData{
		Result: []model.Model{modelCopy},
	}, nil
}

// this manages the /admin/settings route, which allows getting/setting the admin settings
func getAdminSettingsManager(route string, version int) *RouteManager {
	agh := &adminGetHandler{}
	adminGet := MethodHandler{
		Authenticator:  &SuperUserAuthenticator{},
		RequestHandler: agh.Handler(),
		MethodType:     http.MethodGet,
	}

	aph := &adminPostHandler{}
	adminPost := MethodHandler{
		Authenticator:  &SuperUserAuthenticator{},
		RequestHandler: aph.Handler(),
		MethodType:     http.MethodPost,
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
		Authenticator:  &SuperUserAuthenticator{},
		RequestHandler: bph.Handler(),
		MethodType:     http.MethodPost,
	}

	bgh := &bannerGetHandler{}
	bannerGet := MethodHandler{
		Authenticator:  &RequireUserAuthenticator{},
		RequestHandler: bgh.Handler(),
		MethodType:     http.MethodGet,
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
	if err := sc.SetAdminBanner(model.FromAPIString(h.Banner), u); err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}
	if err := sc.SetBannerTheme(model.FromAPIString(h.Theme), u); err != nil {
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
		Result: []model.Model{&model.APIBanner{Text: model.ToAPIString(banner), Theme: model.ToAPIString(theme)}},
	}, nil
}

// this manages the /admin/service_flags route, which allows setting the service flags
func getServiceFlagsRouteManager(route string, version int) *RouteManager {
	fph := &flagsPostHandler{}
	flagsPost := MethodHandler{
		Authenticator:  &SuperUserAuthenticator{},
		RequestHandler: fph.Handler(),
		MethodType:     http.MethodPost,
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
			Authenticator:  &SuperUserAuthenticator{},
			RequestHandler: rh.Handler(),
			MethodType:     http.MethodPost,
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
	if h.EndTime.Before(h.StartTime) {
		return rest.APIError{
			StatusCode: http.StatusBadRequest,
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

func getRevertRouteManager(route string, version int) *RouteManager {
	rh := revertHandler{}
	handler := MethodHandler{
		Authenticator:  &SuperUserAuthenticator{},
		RequestHandler: rh.Handler(),
		MethodType:     http.MethodPost,
	}

	return &RouteManager{
		Route:   route,
		Methods: []MethodHandler{handler},
		Version: version,
	}
}

type revertHandler struct {
	GUID string `json:"guid"`
}

func (h *revertHandler) Handler() RequestHandler {
	return &revertHandler{}
}

func (h *revertHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	if err := util.ReadJSONInto(r.Body, h); err != nil {
		return err
	}
	if h.GUID == "" {
		return rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "GUID to revert to must be specified",
		}
	}
	return nil
}

func (h *revertHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	u := MustHaveUser(ctx)
	err := sc.RevertConfigTo(h.GUID, u.Username())
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Error reverting")
		}
		return ResponseData{}, err
	}
	return ResponseData{}, nil
}

func getAdminEventRouteManager(route string, version int) *RouteManager {
	aeg := &adminEventsGet{}
	getHandler := MethodHandler{
		Authenticator:  &SuperUserAuthenticator{},
		RequestHandler: aeg.Handler(),
		MethodType:     http.MethodGet,
	}
	return &RouteManager{
		Route:   route,
		Methods: []MethodHandler{getHandler},
		Version: version,
	}
}

type adminEventsGet struct {
	*PaginationExecutor
}

func (h *adminEventsGet) Handler() RequestHandler {
	paginator := &PaginationExecutor{
		KeyQueryParam:   "ts",
		LimitQueryParam: "limit",
		Paginator:       adminEventPaginator,
	}
	return &adminEventsGet{paginator}
}

func adminEventPaginator(key string, limit int, args interface{}, sc data.Connector) ([]model.Model, *PageResult, error) {
	var ts time.Time
	var err error
	if key == "" {
		ts = time.Now()
	} else {
		ts, err = time.Parse(time.RFC3339, key)
		if err != nil {
			return []model.Model{}, nil, errors.Wrapf(err, "unable to parse '%s' in RFC-3339 format")
		}
	}
	if limit == 0 {
		limit = 10
	}

	events, err := sc.GetAdminEventLog(ts, limit)
	if err != nil {
		return []model.Model{}, nil, err
	}
	nextPage := makeNextEventsPage(events, limit)
	pageResults := &PageResult{
		Next: nextPage,
	}

	lastIndex := len(events)
	if nextPage != nil {
		lastIndex = limit
	}
	events = events[:lastIndex]
	results := make([]model.Model, len(events))
	for i := range events {
		results[i] = model.Model(&events[i])
	}

	return results, pageResults, nil
}

func makeNextEventsPage(events []model.APIAdminEvent, limit int) *Page {
	var nextPage *Page
	if len(events) == limit {
		nextPage = &Page{
			Relation: "next",
			Key:      events[limit-1].Timestamp.Format(time.RFC3339),
			Limit:    limit,
		}
	}
	return nextPage
}

func getClearTaskQueueRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route: route,
		Methods: []MethodHandler{
			MethodHandler{
				Authenticator:  &SuperUserAuthenticator{},
				RequestHandler: &clearTaskQueueHandler{},
				MethodType:     http.MethodDelete,
			},
		},
		Version: version,
	}
}

func (h *clearTaskQueueHandler) Handler() RequestHandler {
	return &clearTaskQueueHandler{}
}

func (h *clearTaskQueueHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	vars := mux.Vars(r)
	h.distro = vars["distro"]
	_, err := distro.FindOne(distro.ById(h.distro))
	if err != nil {
		return &rest.APIError{
			StatusCode: http.StatusNotFound,
			Message:    "unable to find distro",
		}
	}

	return nil
}

func (h *clearTaskQueueHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	return ResponseData{}, sc.ClearTaskQueue(h.distro)
}
