package dispatcher

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	adb "github.com/mongodb/anser/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// PodDispatcher represents a queue of tasks that are dispatched to a set of
// pods that can run those tasks.
// kim: TODO: should this be called a PodDispatchQueue or should it be renamed
// to DispatchQueue??? Maybe just reword the documentaiton to be "pod
// dispatcher" instead of "dispatch queue".
type PodDispatcher struct {
	// ID is the unique identifier for this dispatch queue.
	ID string `bson:"_id" json:"id"`
	// GroupID is the unique identifier for the set of tasks that should run in
	// this dispatch queue.
	GroupID string `bson:"group_id" json:"group_id"`
	// PodIDs are the identifiers for the pods that run the tasks.
	PodIDs []string `bson:"pod_ids" json:"pod_ids"`
	// TaskIDs is the queue of tasks to run.
	TaskIDs []string `bson:"task_ids" json:"task_ids"`
	// ModificationCount is an incrementing lock used to resolve conflicting
	// updates to the dispatcher queue.
	ModificationCount int `bson:"modification_count" json:"modification_count"`
}

// NewPodDispatcher returns a new pod dispatcher.
func NewPodDispatcher(groupID string, podIDs, taskIDs []string) PodDispatcher {
	return PodDispatcher{
		ID:                primitive.NewObjectID().Hex(),
		GroupID:           groupID,
		PodIDs:            podIDs,
		TaskIDs:           taskIDs,
		ModificationCount: 0,
	}
}

// Insert inserts the pod dispatcher into the DB.
func (pd *PodDispatcher) Insert() error {
	return db.Insert(Collection, pd)
}

// UpsertAtomically inserts/updates the pod dispatcher depending on whether the
// document already exists.
// kim: TODO: test
func (pd *PodDispatcher) UpsertAtomically() (*adb.ChangeInfo, error) {
	atomicQuery := bson.M{
		// kim: TODO: since ID could vary even if GroupID is constant, should
		// this query for unique GroupID instead of ID?
		IDKey:                pd.ID,
		GroupIDKey:           pd.GroupID,
		ModificationCountKey: pd.ModificationCount,
	}
	update := bson.M{
		"$setOnInsert": bson.M{
			IDKey:      pd.ID,
			GroupIDKey: pd.GroupID,
		},
		"$set": bson.M{
			PodIDsKey:            pd.PodIDs,
			TaskIDsKey:           pd.TaskIDs,
			ModificationCountKey: pd.ModificationCount + 1,
		},
	}
	return UpsertOne(atomicQuery, update)
}
