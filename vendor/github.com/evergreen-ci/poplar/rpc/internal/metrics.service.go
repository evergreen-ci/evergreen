package internal

import (
	"context"

	"github.com/evergreen-ci/poplar"
	"github.com/pkg/errors"
)

type metricsService struct {
	registry *poplar.RecorderRegistry
}

func (s *metricsService) CreateCollector(ctx context.Context, opts *CreateOptions) (*PoplarResponse, error) {
	_, err := s.registry.Create(opts.Name, opts.Export())
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &PoplarResponse{Name: opts.Name, Status: true}, nil
}

func (s *metricsService) CloseCollector(ctx context.Context, id *PoplarID) (*PoplarResponse, error) {
	if err := s.registry.Close(id.Name); err != nil {
		return nil, errors.WithStack(err)
	}

	return &PoplarResponse{Name: id.Name, Status: true}, nil
}

func (s *metricsService) ResetSample(ctx context.Context, id *PoplarID) (*PoplarResponse, error) {
	metrics, ok := s.registry.GetCustomCollector(id.Name)
	if !ok {
		return nil, errors.Errorf("could not find metrics aggregator '%s'", id.Name)
	}

	metrics.Reset()

	return &PoplarResponse{Name: id.Name, Status: true}, nil
}

func (s *metricsService) FlushSample(ctx context.Context, id *PoplarID) (*PoplarResponse, error) {
	metrics, ok := s.registry.GetCustomCollector(id.Name)
	if !ok {
		return nil, errors.Errorf("could not find metrics aggregator '%s'", id.Name)
	}

	collector, ok := s.registry.GetCollector(id.Name)
	if !ok {
		return nil, errors.Errorf("could not find collector '%s'", id.Name)
	}

	if err := collector.Add(metrics.Dump()); err != nil {
		return nil, errors.Wrap(err, "problem dumping service")
	}

	metrics.Reset()

	return &PoplarResponse{Name: id.Name, Status: true}, nil
}

func (s *metricsService) Add(ctx context.Context, payload *IntervalSummary) (*PoplarResponse, error) {
	metrics, ok := s.registry.GetCustomCollector(payload.Collector)
	if !ok {
		return nil, errors.Errorf("could not find metrics aggregator '%s'", payload.Collector)
	}

	var err error
	switch summary := payload.Value.(type) {
	case *IntervalSummary_Number:
		err = metrics.Add(summary.Number.Name, summary.Number.Value)
	case *IntervalSummary_NumberValues:
		err = metrics.Add(summary.NumberValues.Name, summary.NumberValues.Value)
	case *IntervalSummary_Point:
		err = metrics.Add(summary.Point.Name, summary.Point.Value)
	case *IntervalSummary_PointValues:
		err = metrics.Add(summary.PointValues.Name, summary.PointValues.Value)
	default:
		return nil, errors.Errorf("invalid type %T for summary value", summary)
	}

	if err != nil {
		return nil, errors.Wrap(err, "problem adding metric")
	}

	return &PoplarResponse{Name: payload.Collector, Status: true}, nil
}
