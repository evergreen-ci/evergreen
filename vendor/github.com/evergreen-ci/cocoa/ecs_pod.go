package cocoa

import (
	"context"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// ECSPod provides an abstraction of a pod backed by AWS ECS.
type ECSPod interface {
	// Resources returns information about the current resources being used by
	// the pod.
	Resources() ECSPodResources
	// StatusInfo returns the current cached status information for the pod.
	StatusInfo() ECSPodStatusInfo
	// LatestStatusInfo returns the latest non-cached status information for the
	// pod. Implementations should query ECS directly for its most up-to-date
	// status.
	LatestStatusInfo(ctx context.Context) (*ECSPodStatusInfo, error)
	// Stop stops the running pod without cleaning up any of its underlying
	// resources.
	Stop(ctx context.Context) error
	// Delete deletes the pod and its owned resources.
	Delete(ctx context.Context) error
}

// ECSPodStatusInfo represents the current status of a pod and its containers in
// ECS.
type ECSPodStatusInfo struct {
	// Status is the status of the pod as a whole.
	Status ECSStatus `bson:"-" json:"-" yaml:"-"`
	// Containers represent the status information of the individual containers
	// within the pod.
	Containers []ECSContainerStatusInfo `bson:"-" json:"-" yaml:"-"`
}

// NewECSPodStatusInfo returns a new uninitialized set of status information for
// a pod.
func NewECSPodStatusInfo() *ECSPodStatusInfo {
	return &ECSPodStatusInfo{}
}

// SetStatus sets the status of the pod as a whole.
func (i *ECSPodStatusInfo) SetStatus(status ECSStatus) *ECSPodStatusInfo {
	i.Status = status
	return i
}

// SetContainers sets the status information of the individual containers
// associated with the pod. This overwrites any existing container status
// information.
func (i *ECSPodStatusInfo) SetContainers(containers []ECSContainerStatusInfo) *ECSPodStatusInfo {
	i.Containers = containers
	return i
}

// AddContainers adds new container status information to the existing ones
// associated with the pod.
func (i *ECSPodStatusInfo) AddContainers(containers ...ECSContainerStatusInfo) *ECSPodStatusInfo {
	i.Containers = append(i.Containers, containers...)
	return i
}

// Validate checks that the required pod status information is populated and the
// pod status is valid.
func (i *ECSPodStatusInfo) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.Wrap(i.Status.Validate(), "invalid pod status")
	catcher.NewWhen(len(i.Containers) == 0, "missing container statuses")
	for _, c := range i.Containers {
		catcher.Wrapf(c.Validate(), "container '%s'", utility.FromStringPtr(c.Name))
	}
	return catcher.Resolve()
}

// ECSContainerStatusInfo represents the current status of a container in ECS.
type ECSContainerStatusInfo struct {
	// ContainerID is the resource identifier for the container.
	ContainerID *string
	// Name is the friendly name of the container.
	Name *string
	// Status is the current status of the container.
	Status ECSStatus
}

// NewECSContainerStatusInfo returns a new uninitialized set of status
// information for a container.
func NewECSContainerStatusInfo() *ECSContainerStatusInfo {
	return &ECSContainerStatusInfo{}
}

// SetContainerID sets the ECS container ID.
func (i *ECSContainerStatusInfo) SetContainerID(id string) *ECSContainerStatusInfo {
	i.ContainerID = &id
	return i
}

// SetName sets the friendly name for the container.
func (i *ECSContainerStatusInfo) SetName(name string) *ECSContainerStatusInfo {
	i.Name = &name
	return i
}

// SetStatus sets the status of the container.
func (i *ECSContainerStatusInfo) SetStatus(status ECSStatus) *ECSContainerStatusInfo {
	i.Status = status
	return i
}

// Validate checks that the required container status information is populated
// and the container status is valid.
func (i *ECSContainerStatusInfo) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(utility.FromStringPtr(i.ContainerID) == "", "missing container ID")
	catcher.NewWhen(utility.FromStringPtr(i.Name) == "", "missing container name")
	catcher.Wrap(i.Status.Validate(), "invalid status")
	return catcher.Resolve()
}

// ECSPodResources are ECS-specific resources associated with a pod.
type ECSPodResources struct {
	// TaskID is the resource identifier for the pod.
	TaskID *string `bson:"-" json:"-" yaml:"-"`
	// TaskDefinition is the resource identifier for the definition template
	// that created the pod.
	TaskDefinition *ECSTaskDefinition `bson:"-" json:"-" yaml:"-"`
	// Cluster is the name of the cluster namespace in which the pod is running.
	Cluster *string `bson:"-" json:"-" yaml:"-"`
	// Containers represent the resources associated with each individual
	// container in the pod.
	Containers []ECSContainerResources `bson:"-" json:"-" yaml:"-"`
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

// SetContainers sets the containers associated with the pod. This overwrites
// any existing containers.
func (r *ECSPodResources) SetContainers(containers []ECSContainerResources) *ECSPodResources {
	r.Containers = containers
	return r
}

// AddContainers adds new containers to the existing ones associated with the
// pod.
func (r *ECSPodResources) AddContainers(containers ...ECSContainerResources) *ECSPodResources {
	r.Containers = append(r.Containers, containers...)
	return r
}

// Validate checks that the task ID is set, the task definition is valid, and
// all container resources are valid.
func (r *ECSPodResources) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(r.TaskID == nil, "must specify task ID of the pod")
	if r.TaskDefinition != nil {
		catcher.Wrapf(r.TaskDefinition.Validate(), "invalid task definition")
	}
	for _, c := range r.Containers {
		catcher.Wrapf(c.Validate(), "container '%s'", utility.FromStringPtr(c.Name))
	}
	return catcher.Resolve()
}

// ECSContainerResources are ECS-specific resources associated with a container.
type ECSContainerResources struct {
	// ContainerID is the resource identifier for the container.
	ContainerID *string `bson:"-" json:"-" yaml:"-"`
	// Name is the friendly name of the container.
	Name *string `bson:"-" json:"-" yaml:"-"`
	// Secrets are the secrets associated with the container.
	Secrets []ContainerSecret `bson:"-" json:"-" yaml:"-"`
}

// NewECSContainerResources returns a new uninitialized set of resources used by
// a container.
func NewECSContainerResources() *ECSContainerResources {
	return &ECSContainerResources{}
}

// SetContainerID sets the ECS container ID associated with the container.
func (r *ECSContainerResources) SetContainerID(id string) *ECSContainerResources {
	r.ContainerID = &id
	return r
}

// SetName sets the friendly name for the container.
func (r *ECSContainerResources) SetName(name string) *ECSContainerResources {
	r.Name = &name
	return r
}

// SetSecrets sets the secrets associated with the container. This overwrites
// any existing secrets.
func (r *ECSContainerResources) SetSecrets(secrets []ContainerSecret) *ECSContainerResources {
	r.Secrets = secrets
	return r
}

// AddSecrets adds new secrets to the existing ones associated with the
// container.
func (r *ECSContainerResources) AddSecrets(secrets ...ContainerSecret) *ECSContainerResources {
	r.Secrets = append(r.Secrets, secrets...)
	return r
}

// Validate checks that the container ID is given and that all given container
// secrets are valid.
func (r *ECSContainerResources) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(r.ContainerID == nil, "must specify a container ID")
	catcher.NewWhen(utility.FromStringPtr(r.ContainerID) == "", "must specify a non-empty container ID")
	for _, s := range r.Secrets {
		catcher.Wrapf(s.Validate(), "invalid secret")
	}
	return catcher.Resolve()
}

// ContainerSecret is a named secret that may or may not be owned by its container.
type ContainerSecret struct {
	// ID is the unique resource identifier for the secret.
	ID *string
	// Name is the friendly name of the secret.
	Name *string
	// Owned determines whether or not the secret is owned by its container or
	// not.
	Owned *bool
}

// NewContainerSecret creates a new uninitialized container secret.
func NewContainerSecret() *ContainerSecret {
	return &ContainerSecret{}
}

// SetID sets the secret's unique resource identifier.
func (s *ContainerSecret) SetID(id string) *ContainerSecret {
	s.ID = &id
	return s
}

// SetName sets the secret's friendly name.
func (s *ContainerSecret) SetName(name string) *ContainerSecret {
	s.Name = &name
	return s
}

// SetOwned sets if the secret should be owned by its container.
func (s *ContainerSecret) SetOwned(owned bool) *ContainerSecret {
	s.Owned = &owned
	return s
}

// Validate checks that the secret has either a name or ID
func (s *ContainerSecret) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(s.ID == nil, "missing ID")
	catcher.NewWhen(s.ID != nil && utility.FromStringPtr(s.ID) == "", "cannot have an empty ID")
	catcher.NewWhen(s.Name != nil && utility.FromStringPtr(s.Name) == "", "cannot have an empty name")
	return catcher.Resolve()
}

// ECSStatus represents the different statuses possible for an ECS pod or
// container.
type ECSStatus string

const (
	// StatusUnknown indicates that the ECS pod or container status cannot be
	// determined.
	StatusUnknown ECSStatus = "unknown"
	// StatusStarting indicates that the ECS pod or container is being prepared
	// to run.
	StatusStarting ECSStatus = "starting"
	// StatusRunning indicates that the ECS pod or container is actively
	// running.
	StatusRunning ECSStatus = "running"
	// StatusStopped indicates the that ECS pod or container is stopped. For a
	// pod, all of its resources are still available even if it's stopped.
	StatusStopped ECSStatus = "stopped"
	// StatusDeleted indicates that the ECS pod or container has been cleaned up
	// completely, including all of its resources.
	StatusDeleted ECSStatus = "deleted"
)

// Validate checks that the ECS status is one of the recognized statuses.
func (s ECSStatus) Validate() error {
	switch s {
	case StatusStarting, StatusRunning, StatusStopped, StatusDeleted, StatusUnknown:
		return nil
	default:
		return errors.Errorf("unrecognized status '%s'", s)
	}
}
