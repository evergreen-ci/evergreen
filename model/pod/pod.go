package pod

import (
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"gopkg.in/mgo.v2/bson"
)

// Pod represents a related collection of containers. This model holds metadata
// about containers running in a container orchestration system.
type Pod struct {
	// ID is the unique identifier for the metadata document. This is
	// semantically unrelated to the ExternalID.
	ID string `bson:"_id" json:"id"`
	// ExternalID is the unique external resource identifier for the collection
	// of containers running in the container orchestration service.
	ExternalID string `bson:"external_id,omitempty" json:"external_id,omitempty"`
	// TaskCreationOpts are options to configure how a task should be
	// containerized and run in a pod.
	TaskContainerCreationOpts TaskContainerCreationOptions `bson:"task_creation_opts,omitempty" json:"task_creation_opts,omitempty"`
	// TimeInfo contains timing information for the pod's lifecycle.
	TimeInfo TimeInfo `bson:"time_info,omitempty" json:"time_info,omitempty"`
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
	// IsWindows indicates whether or not the task will run in a Windows
	// container.
	IsWindows bool `bson:"is_windows" json:"is_windows"`
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
	return o.Image == "" && o.MemoryMB == 0 && o.CPU == 0 && !o.IsWindows && o.EnvVars == nil && o.EnvSecrets == nil
}

// TimeInfo represents timing information about the pod.
type TimeInfo struct {
	// Initialized is the time when this pod was first initialized.
	Initialized time.Time `bson:"initialized,omitempty" json:"initialized,omitempty"`
	// Started is the time when this pod was actually requested to start its
	// containers.
	Started time.Time `bson:"started,omitempty" json:"started,omitempty"`
	// Provisioned is the time when the pod was finished provisioning and
	// ready to do useful work.
	Provisioned time.Time `bson:"provisioned,omitempty" json:"provisioned,omitempty"`
}

// IsZero implements the bsoncodec.Zeroer interface for the sake of defining the
// zero value for BSON marshalling.
func (i TimeInfo) IsZero() bool {
	return i.Initialized.IsZero() && i.Started.IsZero() && i.Provisioned.IsZero()
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
