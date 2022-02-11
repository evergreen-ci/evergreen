package dispatcher

// PodDispatcher represents a queue of tasks that are dispatched to a set of
// pods that can run those tasks.
type PodDispatcher struct {
	// ID is the unique identifier for this dispatch queue.
	ID string `bson:"_id" json:"id"`
	// GroupID is the unique identifier for the set of tasks that should run in
	// this dispatch queue.
	GroupID string `bson:"group_id" json:"group_id"`
	// PodIDs are the identifiers for the pods that run the tasks.
	PodIDs []string `bson:"pod_ids" json:"pod_ids"`
	// Tasks is the queue of tasks to run.
	Tasks []string `bson:"tasks" json:"tasks"`
}
