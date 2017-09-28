package route

import (
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/gorilla/mux"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

type taskMetricsArgs struct {
	task string
}

////////////////////////////////////////////////////////////////////////
//
// Handler for the system information for a task
//
//    /task/{task_id}/metrics/system

func getTaskSystemMetricsManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route:   route,
		Version: version,
		Methods: []MethodHandler{
			{
				MethodType:     http.MethodGet,
				Authenticator:  &NoAuthAuthenticator{},
				RequestHandler: &taskSystemMetricsHandler{},
			},
		},
	}
}

type taskSystemMetricsHandler struct {
	PaginationExecutor
}

func (p *taskSystemMetricsHandler) Handler() RequestHandler {
	return &taskSystemMetricsHandler{PaginationExecutor{
		KeyQueryParam:   "start_at",
		LimitQueryParam: "limit",
		Paginator:       taskSystemMetricsPaginator,
	}}
}

func (p *taskSystemMetricsHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	p.Args = taskMetricsArgs{task: mux.Vars(r)["task_id"]}

	return p.PaginationExecutor.ParseAndValidate(ctx, r)
}

func taskSystemMetricsPaginator(key string, limit int, args interface{}, sc data.Connector) ([]model.Model, *PageResult, error) {
	task := args.(taskMetricsArgs).task
	grip.Debugln("getting results for task:", task)

	ts, err := time.ParseInLocation(model.APITimeFormat, key, time.UTC)
	if err != nil {
		return []model.Model{}, nil, rest.APIError{
			Message:    fmt.Sprintf("problem parsing time from '%s' (%s)", key, err.Error()),
			StatusCode: http.StatusBadRequest,
		}
	}

	// fetch required data from the service layer
	metrics, err := sc.FindTaskSystemMetrics(task, ts, limit*2, 1)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "database error")
		}
		return []model.Model{}, nil, err

	}
	prevData, err := sc.FindTaskSystemMetrics(task, ts, limit, -1)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return []model.Model{}, nil, err
	}

	// populate the page info structure
	pages := &PageResult{}
	if len(metrics) > limit {
		pages.Next = &Page{
			Relation: "next",
			Key:      model.NewTime(metrics[limit].Base.Time).String(),
			Limit:    len(metrics) - limit,
		}
	}
	if len(prevData) > 1 {
		pages.Prev = &Page{
			Relation: "prev",
			Key:      model.NewTime(metrics[0].Base.Time).String(),
			Limit:    len(prevData),
		}
	}

	// truncate results data if there's a next page.
	if pages.Next != nil {
		metrics = metrics[:limit]
	}

	models := make([]model.Model, len(metrics))
	for idx, info := range metrics {
		sysinfoModel := &model.APISystemMetrics{}
		if err = sysinfoModel.BuildFromService(info); err != nil {
			return []model.Model{}, nil, rest.APIError{
				Message:    "problem converting metrics document",
				StatusCode: http.StatusInternalServerError,
			}
		}

		models[idx] = sysinfoModel
	}

	return models, pages, nil
}

////////////////////////////////////////////////////////////////////////
//
// Handler for the process tree for a task
//
//    /task/{task_id}/metrics/process

func getTaskProcessMetricsManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route:   route,
		Version: version,
		Methods: []MethodHandler{
			{
				MethodType:     http.MethodGet,
				Authenticator:  &NoAuthAuthenticator{},
				RequestHandler: &taskProcessMetricsHandler{},
			},
		},
	}
}

type taskProcessMetricsHandler struct {
	PaginationExecutor
}

func (p *taskProcessMetricsHandler) Handler() RequestHandler {
	return &taskProcessMetricsHandler{PaginationExecutor{
		KeyQueryParam:   "start_at",
		LimitQueryParam: "limit",
		Paginator:       taskProcessMetricsPaginator,
	}}
}

func (p *taskProcessMetricsHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	p.Args = taskMetricsArgs{task: mux.Vars(r)["task_id"]}

	return p.PaginationExecutor.ParseAndValidate(ctx, r)
}

func taskProcessMetricsPaginator(key string, limit int, args interface{}, sc data.Connector) ([]model.Model, *PageResult, error) {
	task := args.(taskMetricsArgs).task
	grip.Debugln("getting results for task:", task)

	ts, err := time.ParseInLocation(model.APITimeFormat, key, time.FixedZone("", 0))
	if err != nil {
		return []model.Model{}, nil, rest.APIError{
			Message:    fmt.Sprintf("problem parsing time from '%s' (%s)", key, err.Error()),
			StatusCode: http.StatusBadRequest,
		}
	}

	// fetch required data from the service layer
	data, err := sc.FindTaskProcessMetrics(task, ts, limit*2, 1)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "database error")
		}
		return []model.Model{}, nil, err

	}
	prevData, err := sc.FindTaskProcessMetrics(task, ts, limit, -1)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "database error")
		}
		return []model.Model{}, nil, err
	}

	// populate the page info structure
	pages := &PageResult{}
	if len(data) > limit && len(data[limit]) > 1 {
		pages.Next = &Page{
			Relation: "next",
			Key:      model.NewTime(data[limit][0].Base.Time).String(),
			Limit:    len(data) - limit,
		}
	}
	if len(prevData) > 1 && len(data[0]) > 1 {
		pages.Prev = &Page{
			Relation: "prev",
			Key:      model.NewTime(data[0][0].Base.Time).String(),
			Limit:    len(prevData),
		}
	}

	// truncate results data if there's a next page.
	if pages.Next != nil {
		data = data[:limit]
	}

	models := make([]model.Model, len(data))
	for idx, info := range data {
		procModel := &model.APIProcessMetrics{}
		if err = procModel.BuildFromService(info); err != nil {
			return []model.Model{}, nil, rest.APIError{
				Message:    "problem converting metrics document",
				StatusCode: http.StatusInternalServerError,
			}
		}

		models[idx] = procModel
	}

	return models, pages, nil
}
