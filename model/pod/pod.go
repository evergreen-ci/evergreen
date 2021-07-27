package pod

import (
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

// Pod represents a related collection of containers. This model holds metadata
// about containers running in a container orchestration system.
type Pod struct {
	// ID is the unique identifier for the metadata document.
	ID string `bson:"_id" json:"id"`
	// Status is the current state of the pod.
	Status Status `bson:"pod_status"`
	// Secret is the shared secret between the server and the pod for
	// authentication when the host is provisioned.
	Secret string `bson:"secret" json:"secret"`
	// TaskCreationOpts are options to configure how a task should be
	// containerized and run in a pod.
	TaskContainerCreationOpts TaskContainerCreationOptions `bson:"task_creation_opts,omitempty" json:"task_creation_opts,omitempty"`
	// TimeInfo contains timing information for the pod's lifecycle.
	TimeInfo TimeInfo `bson:"time_info,omitempty" json:"time_info,omitempty"`
	// Resources are external resources that are owned and managed by this pod.
	Resources ResourceInfo `bson:"resource_info,omitempty" json:"resource_info,omitempty"`
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

// Validate checks that the pod status is recognized.
func (s Status) Validate() error {
	switch s {
	case StatusInitializing, StatusStarting, StatusRunning, StatusTerminated:
		return nil
	default:
		return errors.Errorf("unrecognized pod status '%s'", s)
	}
}

// TimeInfo represents timing information about the pod.
type TimeInfo struct {
	// Initializing is the time when this pod was initialized and is waiting to
	// be created in the container orchestration service. This should correspond
	// with the time when the pod transitions to the "initializing" status.
	Initializing time.Time `bson:"initializing,omitempty" json:"initializing,omitempty"`
	// Starting is the time when this pod was actually requested to start its
	// containers. This should correspond with the time when the pod transitions
	// to the "starting" status.
	Starting time.Time `bson:"starting,omitempty" json:"starting,omitempty"`
	// LastCommunicated is the last time that the pod connected to the
	// application server or the application server connected to the pod. This
	// is used as one indicator of liveliness.
	LastCommunicated time.Time `bson:"last_communicated,omitempty" json:"last_communicated,omitempty"`
}

// IsZero implements the bsoncodec.Zeroer interface for the sake of defining the
// zero value for BSON marshalling.
func (i TimeInfo) IsZero() bool {
	return i.Initializing.IsZero() && i.Starting.IsZero() && i.LastCommunicated.IsZero()
}

// ResourceInfo represents information about external resources associated with
// a pod.
type ResourceInfo struct {
	// ExternalID is the unique resource identifier for the aggregate collection
	// of containers running for the pod in the container service.
	ExternalID string `bson:"external_id,omitempty" json:"external_id,omitempty"`
	// DefinitionID is the resource identifier for the pod definition template.
	DefinitionID string `bson:"definition_id,omitempty" json:"definition_id,omitempty"`
	// Cluster is the namespace where the containers are running.
	Cluster string `bson:"cluster,omitempty" json:"cluster,omitempty"`
	// SecretIDs are the resource identifiers for the secrets owned by this pod.
	SecretIDs []string `bson:"secret_ids,omitempty" json:"secret_ids,omitempty"`
}

// IsZero implements the bsoncodec.Zeroer interface for the sake of defining the
// zero value for BSON marshalling.
func (o ResourceInfo) IsZero() bool {
	return o.ExternalID == "" && o.DefinitionID == "" && o.Cluster == "" && len(o.SecretIDs) == 0
}

// TaskContainerCreationOptions are options to apply to the task's container
// when creating a pod in the container orchestration service.
type TaskContainerCreationOptions struct {
	// Image is the image that the task's container will run.
	Image string `bson:"image" json:"image"`
	// MemoryMB is the memory (in MB) that the task's container will be
	// allocated.
	MemoryMB int `bson:"memory_mb" json:"memory_mb"`
	// CPU is the CPU units that the task will be allocated. 1024 CPU units is
	// equivalent to 1vCPU.
	CPU int `bson:"cpu" json:"cpu"`
	// OS indicates which particular operating system the pod's containers run
	// on.
	OS OS `bson:"os" json:"os"`
	// Arch indicates the particular architecture that the pod's containers run
	// on.
	Arch Arch ` bson:"arch" json:"arch"`
	// EnvVars is a mapping of the non-secret environment variables to expose in
	// the task's container environment.
	EnvVars map[string]string `bson:"env_vars,omitempty" json:"env_vars,omitempty"`
	// EnvSecrets is a mapping of secret names to secret values to expose in the
	// task's container environment variables. The secret name is the
	// environment variable name.
	EnvSecrets map[string]string `bson:"env_secrets,omitempty" json:"env_secrets,omitempty"`
}

// OS represents recognized operating systems for pods.
type OS string

const (
	// OSLinux indicates that the pods will run with Linux containers.
	OSLinux OS = "linux"
	// OSWindows indicates that the pods will run with Windows containers.
	OSWindows OS = "windows"
)

// Validate checks that the pod OS is recognized.
func (os OS) Validate() error {
	switch os {
	case OSLinux, OSWindows:
		return nil
	default:
		return errors.Errorf("unrecognized pod OS '%s'", os)
	}
}

// Arch represents recognized architectures for pods.
type Arch string

const (
	ArchAMD64 = "amd64"
	ArchARM64 = "arm64"
)

// Validate checks that the pod architecture is recognized.
func (a Arch) Validate() error {
	switch a {
	case ArchAMD64, ArchARM64:
		return nil
	default:
		return errors.Errorf("unrecognized pod architecture '%s'", a)
	}
}

// IsZero implements the bsoncodec.Zeroer interface for the sake of defining the
// zero value for BSON marshalling.
func (o TaskContainerCreationOptions) IsZero() bool {
	return o.Image == "" && o.MemoryMB == 0 && o.CPU == 0 && o.OS == "" && o.Arch == "" && len(o.EnvVars) == 0 && len(o.EnvSecrets) == 0
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

// UpdateStatus updates the pod status. If the new status is identical to the
// current one, this is a no-op.
func (p *Pod) UpdateStatus(s Status) error {
	if p.Status == s {
		return nil
	}

	byIDAndStatus := ByID(p.ID)
	byIDAndStatus[StatusKey] = p.Status

	updated := utility.BSONTime(time.Now())
	setFields := bson.M{StatusKey: s}
	switch s {
	case StatusInitializing:
		setFields[bsonutil.GetDottedKeyName(TimeInfoKey, TimeInfoInitializingKey)] = updated
	case StatusStarting:
		setFields[bsonutil.GetDottedKeyName(TimeInfoKey, TimeInfoStartingKey)] = updated
	}

	if err := UpdateOne(byIDAndStatus, bson.M{
		"$set": setFields,
	}); err != nil {
		return err
	}

	event.LogPodStatusChanged(p.ID, string(p.Status), string(s))

	p.Status = s
	switch s {
	case StatusInitializing:
		p.TimeInfo.Initializing = updated
	case StatusStarting:
		p.TimeInfo.Starting = updated
	}

	return nil
}
