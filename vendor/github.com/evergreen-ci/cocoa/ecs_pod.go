package cocoa

import (
	"context"

	"github.com/pkg/errors"
)

// ECSPod provides an abstraction of a pod backed by AWS ECS.
type ECSPod interface {
	// Info returns information about the current state of the pod.
	Info(ctx context.Context) (*ECSPodInfo, error)
	// Stop stops the running pod without cleaning up any of its underlying
	// resources.
	Stop(ctx context.Context) error
	// Delete deletes the pod and its owned resources.
	Delete(ctx context.Context) error
}

// ECSPodInfo provides information about the current status of the pod.
type ECSPodInfo struct {
	// Status is the current status of the pod.
	Status ECSPodStatus `bson:"-" json:"-" yaml:"-"`
	// Resources provides information about the underlying ECS resources being
	// used by the pod.
	Resources ECSPodResources `bson:"-" json:"-" yaml:"-"`
}

// ECSPodResources are ECS-specific resources that a pod uses.
type ECSPodResources struct {
	TaskID         *string            `bson:"-" json:"-" yaml:"-"`
	TaskDefinition *ECSTaskDefinition `bson:"-" json:"-" yaml:"-"`
	Cluster        *string            `bson:"-" json:"-" yaml:"-"`
	Secrets        []PodSecret        `bson:"-" json:"-" yaml:"-"`
}

// NewECSPodResources returns a new uninitialized set of resources used by a
// pod.
func NewECSPodResources() *ECSPodResources {
	return &ECSPodResources{}
}

// SetTaskID sets the ECS task ID associated with the pod.
func (r *ECSPodResources) SetTaskID(id string) *ECSPodResources {
	r.TaskID = &id
	return r
}

// SetTaskDefinition sets the ECS task definition associated with the pod.
func (r *ECSPodResources) SetTaskDefinition(def ECSTaskDefinition) *ECSPodResources {
	r.TaskDefinition = &def
	return r
}

// SetCluster sets the cluster associated with the pod.
func (r *ECSPodResources) SetCluster(cluster string) *ECSPodResources {
	r.Cluster = &cluster
	return r
}

// SetSecrets sets the secrets associated with the pod. This overwrites any
// existing secrets.
func (r *ECSPodResources) SetSecrets(secrets []PodSecret) *ECSPodResources {
	r.Secrets = secrets
	return r
}

// AddSecrets adds new secrets to the existing ones associated with the pod.
func (r *ECSPodResources) AddSecrets(secrets ...PodSecret) *ECSPodResources {
	r.Secrets = append(r.Secrets, secrets...)
	return r
}

// PodSecret is a named secret that may or may not be owned by its pod.
type PodSecret struct {
	NamedSecret
	// Owned determines whether or not the secret is owned by its pod or not.
	Owned *bool
}

// NewPodSecret creates a new uninitialized pod secret.
func NewPodSecret() *PodSecret {
	return &PodSecret{}
}

// SetName sets the secret's name.
func (s *PodSecret) SetName(name string) *PodSecret {
	s.Name = &name
	return s
}

// SetValue sets the secret's value.
func (s *PodSecret) SetValue(val string) *PodSecret {
	s.Value = &val
	return s
}

// SetOwned sets if the secret should be owned by its pod.
func (s *PodSecret) SetOwned(owned bool) *PodSecret {
	s.Owned = &owned
	return s
}

// ECSPodStatus represents the different statuses possible for an ECS pod.
type ECSPodStatus string

const (
	// StatusStarting indicates that the ECS pod is being prepared to run.
	StatusStarting ECSPodStatus = "starting"
	// StatusRunning indicates that the ECS pod is actively running.
	StatusRunning ECSPodStatus = "running"
	// StatusStopped indicates the that ECS pod is stopped, but all of its
	// resources are still available.
	StatusStopped ECSPodStatus = "stopped"
	// StatusDeleted indicates that the ECS pod has been cleaned up completely,
	// including all of its resources.
	StatusDeleted ECSPodStatus = "deleted"
)

// Validate checks that the ECS pod status is one of the recognized statuses.
func (s ECSPodStatus) Validate() error {
	switch s {
	case StatusStarting, StatusRunning, StatusStopped, StatusDeleted:
		return nil
	default:
		return errors.Errorf("unrecognized status '%s'", s)
	}
}
