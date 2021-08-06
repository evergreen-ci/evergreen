package mock

import (
	"context"

	"github.com/evergreen-ci/cocoa"
)

// ECSPod provides a mock implementation of a cocoa.ECSPod backed by another ECS
// pod implementation.
type ECSPod struct {
	cocoa.ECSPod

	ResourcesOutput *cocoa.ECSPodResources

	StatusOutput *cocoa.ECSPodStatusInfo

	StopError error

	DeleteError error
}

// NewECSPod creates a mock ECS Pod backed by the given ECSPod.
func NewECSPod(p cocoa.ECSPod) *ECSPod {
	return &ECSPod{
		ECSPod: p,
	}
}

// StatusInfo returns mock cached status information about the pod. The mock
// output can be customized. By default, it will return the result of the
// backing ECS pod.
func (p *ECSPod) StatusInfo() cocoa.ECSPodStatusInfo {
	if p.StatusOutput != nil {
		return *p.StatusOutput
	}

	return p.ECSPod.StatusInfo()
}

// Resources returns mock resource information about the pod. The mock output
// can be customized. By default, it will return the result of the backing ECS
// pod.
func (p *ECSPod) Resources() cocoa.ECSPodResources {
	if p.ResourcesOutput != nil {
		return *p.ResourcesOutput
	}

	return p.ECSPod.Resources()
}

// Stop stops the mock pod. The mock output can be customized. By default, it
// will set the cached status to stopped.
func (p *ECSPod) Stop(ctx context.Context) error {
	if p.StopError != nil {
		return p.StopError
	}

	return p.ECSPod.Stop(ctx)
}

// Delete deletes the mock pod and all of its underlying resources. The mock
// output can be customized. By default, it will return the result of the
// backing ECS pod.
func (p *ECSPod) Delete(ctx context.Context) error {
	if p.DeleteError != nil {
		return p.DeleteError
	}

	return p.ECSPod.Delete(ctx)
}
