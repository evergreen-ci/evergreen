package route

import (
	"context"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

////////////////////////////////////////////////////////////////////////
//
// Handler for the system information for a task
//
//    /task/{task_id}/metrics/system

func makeFetchTaskSystmMetrics(sc data.Connector) gimlet.RouteHandler {
	return &taskSystemMetricsHandler{
		sc: sc,
	}
}

type taskSystemMetricsHandler struct {
	taskID string
	limit  int
	key    time.Time
	sc     data.Connector
}

func (p *taskSystemMetricsHandler) Factory() gimlet.RouteHandler {
	return &taskSystemMetricsHandler{
		sc: p.sc,
	}
}

func (p *taskSystemMetricsHandler) Parse(ctx context.Context, r *http.Request) error {
	p.taskID = gimlet.GetVars(r)["task_id"]
	vals := r.URL.Query()

	var err error
	p.key, err = time.ParseInLocation(model.APITimeFormat, vals.Get("start_at"), time.FixedZone("", 0))
	if err != nil {
		return errors.WithStack(err)
	}

	p.limit, err = getLimit(vals)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (p *taskSystemMetricsHandler) Run(ctx context.Context) gimlet.Responder {
	metrics, err := p.sc.FindTaskSystemMetrics(p.taskID, p.key, p.limit+1)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "database error"))
	}

	resp := gimlet.NewResponseBuilder()
	if err = resp.SetFormat(gimlet.JSON); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	lastIndex := len(metrics)
	if len(metrics) > p.limit {
		lastIndex = p.limit
		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start_at",
				BaseURL:         p.sc.GetURL(),
				Key:             model.NewTime(metrics[p.limit].Base.Time).String(),
				Limit:           p.limit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err,
				"problem paginating response"))
		}
	}

	metrics = metrics[:lastIndex]

	for _, info := range metrics {
		sysinfoModel := &model.APISystemMetrics{}

		if err = sysinfoModel.BuildFromService(info); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "problem converting metrics document"))
		}

		err = resp.AddData(sysinfoModel)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
	}

	return resp
}

////////////////////////////////////////////////////////////////////////
//
// Handler for the process tree for a task
//
//    /task/{task_id}/metrics/process

func makeFetchTaskProcessMetrics(sc data.Connector) gimlet.RouteHandler {
	return &taskProcessMetricsHandler{
		sc: sc,
	}
}

type taskProcessMetricsHandler struct {
	taskID string
	limit  int
	key    time.Time
	sc     data.Connector
}

func (p *taskProcessMetricsHandler) Factory() gimlet.RouteHandler {
	return &taskProcessMetricsHandler{
		sc: p.sc,
	}
}

func (p *taskProcessMetricsHandler) Parse(ctx context.Context, r *http.Request) error {
	p.taskID = gimlet.GetVars(r)["task_id"]
	vals := r.URL.Query()
	key := vals.Get("start_at")

	var err error
	p.key, err = time.ParseInLocation(model.APITimeFormat, key, time.FixedZone("", 0))
	if err != nil {
		return errors.WithStack(err)
	}

	p.limit, err = getLimit(vals)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (p *taskProcessMetricsHandler) Run(ctx context.Context) gimlet.Responder {
	// fetch required data from the service layer
	data, err := p.sc.FindTaskProcessMetrics(p.taskID, p.key, p.limit+1)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "database error"))
	}

	resp := gimlet.NewResponseBuilder()
	if err = resp.SetFormat(gimlet.JSON); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	lastIndex := len(data)
	if len(data) > p.limit {
		lastIndex = p.limit
		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start_at",
				BaseURL:         p.sc.GetURL(),
				Key:             model.NewTime(data[p.limit][0].Base.Time).String(),
				Limit:           p.limit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err,
				"problem paginating response"))
		}
	}

	data = data[:lastIndex]

	for _, info := range data {
		procModel := &model.APIProcessMetrics{}
		if err = procModel.BuildFromService(info); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "problem converting metrics document"))
		}

		err = resp.AddData(procModel)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
	}

	return resp
}
