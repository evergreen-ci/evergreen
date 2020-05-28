package internal

import (
	"context"
	"io"

	"github.com/evergreen-ci/poplar"
	"github.com/mongodb/ftdc/events"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type collectorService struct {
	registry *poplar.RecorderRegistry
}

func (s *collectorService) CreateCollector(ctx context.Context, opts *CreateOptions) (*PoplarResponse, error) {
	if _, ok := s.registry.GetCollector(opts.Name); !ok {
		_, err := s.registry.Create(opts.Name, opts.Export())
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	return &PoplarResponse{Name: opts.Name, Status: true}, nil
}

func (s *collectorService) CloseCollector(ctx context.Context, id *PoplarID) (*PoplarResponse, error) {
	err := s.registry.Close(id.Name)

	grip.Error(message.WrapError(err, message.Fields{
		"message":  "problem closing recorder",
		"recorder": id.Name,
	}))

	return &PoplarResponse{Name: id.Name, Status: err == nil}, nil

}

func (s *collectorService) SendEvent(ctx context.Context, event *EventMetrics) (*PoplarResponse, error) {
	collector, ok := s.registry.GetEventsCollector(event.Name)

	if !ok {
		return nil, status.Errorf(codes.NotFound, "no registry named %s", event.Name)
	}

	err := collector.AddEvent(event.Export())

	return &PoplarResponse{Name: event.Name, Status: err == nil}, nil

}

func (s *collectorService) StreamEvents(srv PoplarEventCollector_StreamEventsServer) error {
	ctx := srv.Context()

	var (
		collector events.Collector
		eventName string
	)

	for {
		event, err := srv.Recv()
		if err == io.EOF {
			return srv.SendAndClose(&PoplarResponse{
				Name:   eventName,
				Status: true,
			})
		} else if err != nil {
			return srv.SendAndClose(&PoplarResponse{
				Name:   eventName,
				Status: false,
			})
		}
		if collector == nil {
			if event.Name == "" {
				return status.Error(codes.InvalidArgument, "registries must be named")
			}

			eventName = event.Name
			var ok bool
			collector, ok = s.registry.GetEventsCollector(eventName)
			if !ok {
				return status.Errorf(codes.NotFound, "no registry named %s", eventName)
			}
		}

		if err := collector.AddEvent(event.Export()); err != nil {
			return status.Errorf(codes.Internal, "problem persisting argument %s", err.Error())
		}

		if ctx.Err() != nil {
			return status.Errorf(codes.Canceled, "operation canceled for %s", eventName)
		}
	}
}
