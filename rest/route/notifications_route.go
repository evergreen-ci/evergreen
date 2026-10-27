package route

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/evergreen-ci/evergreen"
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

const (
	defaultFailedWebhookNotificationsLimit = 100
	maxFailedWebhookNotificationsLimit     = 500
)

func makeFetchNotifcationStatusRoute() gimlet.RouteHandler {
	return &notificationsStatusHandler{}
}

type notificationsStatusHandler struct{}

func (gh *notificationsStatusHandler) Factory() gimlet.RouteHandler {
	return &notificationsStatusHandler{}
}

func (gh *notificationsStatusHandler) Parse(_ context.Context, _ *http.Request) error { return nil }

func (gh *notificationsStatusHandler) Run(ctx context.Context) gimlet.Responder {
	stats, err := data.GetNotificationsStats(ctx)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting notification stats"))
	}

	return gimlet.NewJSONResponse(stats)
}

func makeFailedWebhookNotificationsRoute() gimlet.RouteHandler {
	return &failedWebhookNotificationsHandler{}
}

type failedWebhookNotificationsHandler struct {
	url      string
	projects []string
	limit    int
}

// Factory creates an instance of the handler.
//
//	@Summary		Get failed webhook notifications
//	@Description	List webhook notifications that failed to deliver for the given projects, optionally filtered by URL. Only failed deliveries within the last 3 days are retained.
//	@Tags			notifications
//	@Router			/failed_webhook_notifications [get]
//	@Security		Api-User || Api-Key
//	@Param			url			query	string	false	"only return failed deliveries to this URL"
//	@Param			projects	query	string	true	"comma-separated list of project IDs or identifiers"
//	@Param			limit		query	int		false	"number of failed deliveries to return (default 100, max 500)"
//	@Success		200	{array}	model.APIFailedWebhook
func (h *failedWebhookNotificationsHandler) Factory() gimlet.RouteHandler {
	return &failedWebhookNotificationsHandler{}
}

// Parse reads the url, projects, and limit query parameters, resolving each
// project to its ID.
func (h *failedWebhookNotificationsHandler) Parse(ctx context.Context, r *http.Request) error {
	vals := r.URL.Query()
	h.url = vals.Get("url")

	for _, project := range strings.Split(vals.Get("projects"), ",") {
		project = strings.TrimSpace(project)
		if project == "" {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "must provide at least one non-empty project",
			}
		}
		// Recorded failures are scoped by project ID, so resolve identifiers.
		id, err := dbModel.GetIdForProject(ctx, project)
		if err != nil {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusNotFound,
				Message:    err.Error(),
			}
		}
		h.projects = append(h.projects, id)
	}

	// No cursor pagination; 3-day TTL retention means the realistic result
	// set fits in the max limit. Add start_key pagination if volumes exceed
	// it.
	h.limit = defaultFailedWebhookNotificationsLimit
	if limitStr := vals.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit <= 0 {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "limit must be a positive integer",
			}
		}
		h.limit = min(limit, maxFailedWebhookNotificationsLimit)
	}

	return nil
}

func (h *failedWebhookNotificationsHandler) Run(ctx context.Context) gimlet.Responder {
	// The route is cross-project, so permission is checked per project here
	// rather than by project middleware.
	u := MustHaveUser(ctx)
	for _, project := range h.projects {
		if !u.HasPermission(ctx, gimlet.PermissionOpts{
			Resource:      project,
			ResourceType:  evergreen.ProjectResourceType,
			Permission:    evergreen.PermissionTasks,
			RequiredLevel: evergreen.TasksView.Value,
		}) {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusForbidden,
				Message:    fmt.Sprintf("no permission to view project '%s'", project),
			})
		}
	}

	failedWebhooks, err := data.FindFailedWebhooks(ctx, h.projects, h.url, h.limit)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	return gimlet.NewJSONResponse(failedWebhooks)
}
