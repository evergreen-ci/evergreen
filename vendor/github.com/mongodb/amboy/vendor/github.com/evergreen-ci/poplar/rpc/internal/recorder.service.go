package internal

import (
	"context"

	"github.com/evergreen-ci/poplar"
	"github.com/golang/protobuf/ptypes"
	"github.com/pkg/errors"
)

type recorderService struct {
	registry *poplar.RecorderRegistry
}

func (s *recorderService) CreateRecorder(ctx context.Context, info *CreateOptions) (*PoplarResponse, error) {
	_, err := s.registry.Create(info.Name, info.Export())
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &PoplarResponse{Name: info.Name, Status: true}, nil
}

func (s *recorderService) CloseRecorder(ctx context.Context, id *PoplarID) (*PoplarResponse, error) {
	if err := s.registry.Close(id.Name); err != nil {
		return nil, errors.WithStack(err)
	}

	return &PoplarResponse{Name: id.Name, Status: true}, nil
}

func (s *recorderService) BeginEvent(ctx context.Context, id *PoplarID) (*PoplarResponse, error) {
	rec, ok := s.registry.GetRecorder(id.Name)
	if !ok {
		return nil, errors.Errorf("could not find recorder '%s'", id.Name)
	}

	rec.BeginIteration()

	return &PoplarResponse{Name: id.Name, Status: true}, nil
}

func (s *recorderService) ResetEvent(ctx context.Context, id *PoplarID) (*PoplarResponse, error) {
	rec, ok := s.registry.GetRecorder(id.Name)
	if !ok {
		return nil, errors.Errorf("could not find recorder '%s'", id.Name)
	}

	rec.Reset()

	return &PoplarResponse{Name: id.Name, Status: true}, nil
}

func (s *recorderService) EndEvent(ctx context.Context, val *EventSendDuration) (*PoplarResponse, error) {
	rec, ok := s.registry.GetRecorder(val.Name)
	if !ok {
		return nil, errors.Errorf("could not find recorder '%s'", val.Name)
	}

	dur, err := ptypes.Duration(val.Duration)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert duration value")
	}

	rec.EndIteration(dur)

	return &PoplarResponse{Name: val.Name, Status: true}, nil
}

func (s *recorderService) SetID(ctx context.Context, val *EventSendInt) (*PoplarResponse, error) {
	rec, ok := s.registry.GetRecorder(val.Name)
	if !ok {
		return nil, errors.Errorf("could not find recorder '%s'", val.Name)
	}

	rec.SetID(val.Value)

	return &PoplarResponse{Name: val.Name, Status: true}, nil
}

func (s *recorderService) SetTime(ctx context.Context, t *EventSendTime) (*PoplarResponse, error) {
	rec, ok := s.registry.GetRecorder(t.Name)
	if !ok {
		return nil, errors.Errorf("could not find recorder '%s'", t.Name)
	}

	ts, err := ptypes.Timestamp(t.Time)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert timestamp value")
	}

	rec.SetTime(ts)

	return &PoplarResponse{Name: t.Name, Status: true}, nil
}

func (s *recorderService) SetDuration(ctx context.Context, val *EventSendDuration) (*PoplarResponse, error) {
	rec, ok := s.registry.GetRecorder(val.Name)
	if !ok {
		return nil, errors.Errorf("could not find recorder '%s'", val.Name)
	}

	dur, err := ptypes.Duration(val.Duration)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert duration value")
	}

	rec.SetDuration(dur)

	return &PoplarResponse{Name: val.Name, Status: true}, nil
}

func (s *recorderService) SetTotalDuration(ctx context.Context, val *EventSendDuration) (*PoplarResponse, error) {
	rec, ok := s.registry.GetRecorder(val.Name)
	if !ok {
		return nil, errors.Errorf("could not find recorder '%s'", val.Name)
	}

	dur, err := ptypes.Duration(val.Duration)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert duration value")
	}

	rec.SetTotalDuration(dur)

	return &PoplarResponse{Name: val.Name, Status: true}, nil
}

func (s *recorderService) SetState(ctx context.Context, val *EventSendInt) (*PoplarResponse, error) {
	rec, ok := s.registry.GetRecorder(val.Name)
	if !ok {
		return nil, errors.Errorf("could not find recorder '%s'", val.Name)
	}

	rec.SetState(val.Value)

	return &PoplarResponse{Name: val.Name, Status: true}, nil
}

func (s *recorderService) SetWorkers(ctx context.Context, val *EventSendInt) (*PoplarResponse, error) {
	rec, ok := s.registry.GetRecorder(val.Name)
	if !ok {
		return nil, errors.Errorf("could not find recorder '%s'", val.Name)
	}

	rec.SetWorkers(val.Value)

	return &PoplarResponse{Name: val.Name, Status: true}, nil
}

func (s *recorderService) SetFailed(ctx context.Context, val *EventSendBool) (*PoplarResponse, error) {
	rec, ok := s.registry.GetRecorder(val.Name)
	if !ok {
		return nil, errors.Errorf("could not find recorder '%s'", val.Name)
	}

	rec.SetFailed(val.Value)

	return &PoplarResponse{Name: val.Name, Status: true}, nil
}

func (s *recorderService) IncOps(ctx context.Context, val *EventSendInt) (*PoplarResponse, error) {
	rec, ok := s.registry.GetRecorder(val.Name)
	if !ok {
		return nil, errors.Errorf("could not find recorder '%s'", val.Name)
	}

	rec.IncOperations(val.Value)

	return &PoplarResponse{Name: val.Name, Status: true}, nil
}

func (s *recorderService) IncSize(ctx context.Context, val *EventSendInt) (*PoplarResponse, error) {
	rec, ok := s.registry.GetRecorder(val.Name)
	if !ok {
		return nil, errors.Errorf("could not find recorder '%s'", val.Name)
	}

	rec.IncSize(val.Value)

	return &PoplarResponse{Name: val.Name, Status: true}, nil
}

func (s *recorderService) IncError(ctx context.Context, val *EventSendInt) (*PoplarResponse, error) {
	rec, ok := s.registry.GetRecorder(val.Name)
	if !ok {
		return nil, errors.Errorf("could not find recorder '%s'", val.Name)
	}

	rec.IncError(val.Value)

	return &PoplarResponse{Name: val.Name, Status: true}, nil
}

func (s *recorderService) IncIterations(ctx context.Context, val *EventSendInt) (*PoplarResponse, error) {
	rec, ok := s.registry.GetRecorder(val.Name)
	if !ok {
		return nil, errors.Errorf("could not find recorder '%s'", val.Name)
	}

	rec.IncIterations(val.Value)

	return &PoplarResponse{Name: val.Name, Status: true}, nil
}
