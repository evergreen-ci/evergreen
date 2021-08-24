package mock

import (
	"context"

	"github.com/evergreen-ci/cocoa"
	"github.com/pkg/errors"
)

// ECSPodCreator provides a mock implementation of a cocoa.ECSPodCreator
// that produces mock ECS pods. It can also be mocked to produce a pre-defined
// cocoa.ECSPod.
type ECSPodCreator struct {
	cocoa.ECSPodCreator

	CreatePodInput  []cocoa.ECSPodCreationOptions
	CreatePodOutput *cocoa.ECSPod
	CreatePodError  error
}

// NewECSPodCreator creates a mock ECS Pod Creator backed by the given Pod Creator.
func NewECSPodCreator(c cocoa.ECSPodCreator) *ECSPodCreator {
	return &ECSPodCreator{
		ECSPodCreator: c,
	}
}

// CreatePod saves the input and returns a new mock pod. The mock output can be
// customized. By default, it will create a new pod based on the input that is
// backed by a mock ECSClient.
func (m *ECSPodCreator) CreatePod(ctx context.Context, opts ...cocoa.ECSPodCreationOptions) (cocoa.ECSPod, error) {
	m.CreatePodInput = opts

	if m.CreatePodOutput != nil {
		return *m.CreatePodOutput, m.CreatePodError
	} else if m.CreatePodError != nil {
		return nil, m.CreatePodError
	}

	return m.ECSPodCreator.CreatePod(ctx, opts...)
}

// CreatePodFromExistingDefinition saves the input and returns a new mock pod.
// The mock output can be customized. By default, it will create a new pod from
// the existing task definition that is backed by a mock ECSClient.
func (m *ECSPodCreator) CreatePodFromExistingDefinition(ctx context.Context, def cocoa.ECSTaskDefinition, opts ...cocoa.ECSPodExecutionOptions) (cocoa.ECSPod, error) {
	return nil, errors.New("TODO: implement")
}
