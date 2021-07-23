package pod

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"gopkg.in/mgo.v2/bson"
)

// Pod represents a related collection of containers. This model holds metadata
// about containers running in a container orchestration system.
type Pod struct {
	// ID is the unique identifier for the metadata document. This is
	// semantically unrelated to the ExternalID.
	ID string `bson:"_id" json:"id"`
	// Status is the current state of the pod.
	Status Status `bson:"pod_status"`
	// Secret is the shared secret between the server and the pod for
	// authentication when the host is provisioned.
	Secret string `bson:"secret" json:"secret"`
	// TaskCreationOpts are options to configure how a task should be
	// containerized and run in a pod.
	TaskContainerCreationOpts TaskContainerCreationOptions `bson:"task_creation_opts,omitempty" json:"task_creation_opts,omitempty"`
	// Resources are external resources that are managed by this pod.
	Resources ResourceInfo `bson:"ecs_info" json:"ecs_info"`
}

// Status represents a possible state that a pod can be in.
type Status string

const (
	// StatusInitializing indicates that a pod is waiting to be created.
	StatusInitializing Status = "initializing"
	// StatusStarting indicates that a pod's containers are starting.
	StatusStarting Status = "starting"
	// StatusRunning indicates that the pod's containers are running.
	StatusRunning Status = "running"
	// StatusTerminated indicates that the pod's containers have been
	// deleted.
	StatusTerminated Status = "terminated"
)

// ResourceInfo represents information about external resources associated with
// a pod.
type ResourceInfo struct {
	// ID is the unique resource identifier for the collection of containers
	// running.
	ID string `bson:"external_id,omitempty" json:"external_id,omitempty"`
	// DefinitionID is the resource identifier for the pod definition template.
	DefinitionID string `bson:"definition_id" json:"definition_id"`
	// Cluster is the name of the cluster where the containers are running.
	Cluster string `bson:"cluster" json:"cluster"`
	// SecretIDs are the resource identifiers for the secrets owned by this pod
	// in Secrets Manager.
	SecretIDs []string `bson:"secret_ids" json:"secret_ids"`
}

// TaskCreationOptions are options to apply to the task's container when
// creating a pod in the container orchestration service.
type TaskContainerCreationOptions struct {
	// Image is the image that the task's container will run.
	Image string `bson:"image" json:"image"`
	// MemoryMB is the memory (in MB) that the task's container will be
	// allocated.
	MemoryMB int `bson:"memory_mb" json:"memory_mb"`
	// CPU is the CPU units that the task will be allocated. 1024 CPU units is
	// equivalent to 1vCPU.
	CPU int `bson:"cpu" json:"cpu"`
	// Platform indicates which particular platform the pod's containers run on.
	Platform evergreen.PodPlatform `bson:"platform" json:"platform"`
	// EnvVars is a mapping of the non-secret environment variables to expose in
	// the task's container environment.
	EnvVars map[string]string `bson:"env_vars,omitempty" json:"env_vars,omitempty"`
	// EnvSecrets is a mapping of secret names to secret values to expose in the
	// task's container environment variables. The secret name is the
	// environment variable name.
	EnvSecrets map[string]string `bson:"secrets,omitempty" json:"secrets,omitempty"`
}

// IsZero implements the bsoncodec.Zeroer interface for the sake of defining the
// zero value for BSON marshalling.
func (o TaskContainerCreationOptions) IsZero() bool {
	return o.Image == "" && o.MemoryMB == 0 && o.CPU == 0 && o.Platform == "" && o.EnvVars == nil && o.EnvSecrets == nil
}

// Insert inserts a new pod into the collection.
func (p *Pod) Insert() error {
	return db.Insert(Collection, p)
}

// Remove removes the pod from the collection.
func (p *Pod) Remove() error {
	return db.Remove(
		Collection,
		bson.M{
			IDKey: p.ID,
		},
	)
}

// UpdateStatus updates the pod status.
func (p *Pod) UpdateStatus(s Status) error {
	byIDAndStatus := ByID(p.ID)
	byIDAndStatus[StatusKey] = p.Status

	if err := UpdateOne(byIDAndStatus, bson.M{
		"$set": bson.M{StatusKey: s},
	}); err != nil {
		return err
	}

	event.LogPodStatusChanged(p.ID, string(p.Status), string(s))

	p.Status = s

	return nil
}
