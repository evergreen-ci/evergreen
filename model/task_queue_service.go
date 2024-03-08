package model

import (
	"context"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/pkg/errors"
)

type TaskQueueItemDispatcher interface {
	FindNextTask(context.Context, string, TaskSpec, time.Time) (*TaskQueueItem, error)
	Refresh(context.Context, string) error
	RefreshFindNextTask(context.Context, string, TaskSpec, time.Time) (*TaskQueueItem, error)
}

type CachedDispatcher interface {
	Refresh() error
	FindNextTask(context.Context, TaskSpec, time.Time) *TaskQueueItem
	Type() string
	CreatedAt() time.Time
}

type taskDispatchService struct {
	cachedDispatchers map[string]CachedDispatcher
	mu                sync.RWMutex
	ttl               time.Duration
	useAliases        bool
}

func NewTaskDispatchService(ttl time.Duration) TaskQueueItemDispatcher {
	return &taskDispatchService{
		ttl:               ttl,
		cachedDispatchers: map[string]CachedDispatcher{},
	}
}

func NewTaskDispatchAliasService(ttl time.Duration) TaskQueueItemDispatcher {
	return &taskDispatchService{
		ttl:               ttl,
		useAliases:        true,
		cachedDispatchers: map[string]CachedDispatcher{},
	}
}

func (s *taskDispatchService) FindNextTask(ctx context.Context, distroID string, spec TaskSpec, amiUpdatedTime time.Time) (*TaskQueueItem, error) {
	distroDispatchService, err := s.ensureQueue(ctx, distroID)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return distroDispatchService.FindNextTask(ctx, spec, amiUpdatedTime), nil
}

func (s *taskDispatchService) RefreshFindNextTask(ctx context.Context, distroID string, spec TaskSpec, amiUpdatedTime time.Time) (*TaskQueueItem, error) {
	distroDispatchService, err := s.ensureQueue(ctx, distroID)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if err := distroDispatchService.Refresh(); err != nil {
		return nil, errors.WithStack(err)
	}

	return distroDispatchService.FindNextTask(ctx, spec, amiUpdatedTime), nil
}

func (s *taskDispatchService) Refresh(ctx context.Context, distroID string) error {
	distroDispatchService, err := s.ensureQueue(ctx, distroID)
	if err != nil {
		return errors.WithStack(err)
	}

	if err := distroDispatchService.Refresh(); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (s *taskDispatchService) ensureQueue(ctx context.Context, distroID string) (CachedDispatcher, error) {
	d := distro.Distro{}
	foundDistro, err := distro.FindOneId(ctx, distroID)
	if err != nil {
		return nil, errors.Wrapf(err, "finding distro '%s'", distroID)
	}
	if foundDistro != nil {
		d = *foundDistro
	}
	// If there is a "distro": *basicCachedDispatcherImpl in the cachedDispatchers map, return that.
	// Otherwise, get the "distro"'s taskQueue from the database; seed its cachedDispatcher; put that in the map and return it.
	s.mu.Lock()
	defer s.mu.Unlock()

	distroDispatchService, ok := s.cachedDispatchers[distroID]
	if ok && distroDispatchService.Type() == d.DispatcherSettings.Version {
		return distroDispatchService, nil
	}

	var taskQueue TaskQueue
	if s.useAliases {
		taskQueue, err = FindDistroSecondaryTaskQueue(distroID)
	} else {
		taskQueue, err = FindDistroTaskQueue(distroID)
	}

	if err != nil {
		return nil, errors.WithStack(err)
	}

	switch d.DispatcherSettings.Version {
	case evergreen.DispatcherVersionRevisedWithDependencies:
		distroDispatchService, err = newDistroTaskDAGDispatchService(taskQueue, s.ttl)
		if err != nil {
			return nil, err
		}
	default:
		// TODO: Error?
	}

	s.cachedDispatchers[distroID] = distroDispatchService
	return distroDispatchService, nil
}



