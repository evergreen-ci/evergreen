package dispatcher

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	adb "github.com/mongodb/anser/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// PodDispatcher represents a set of tasks that are dispatched to a set of pods
// that can run those tasks.
type PodDispatcher struct {
	// ID is the unique identifier for this dispatcher.
	ID string `bson:"_id" json:"id"`
	// GroupID is the unique identifier for the set of tasks that should run in
	// this dispatcher.
	GroupID string `bson:"group_id" json:"group_id"`
	// PodIDs are the identifiers for the pods that run the tasks.
	PodIDs []string `bson:"pod_ids" json:"pod_ids"`
	// TaskIDs is the identifiers for the set of tasks to run.
	TaskIDs []string `bson:"task_ids" json:"task_ids"`
	// ModificationCount is an incrementing lock used to resolve conflicting
	// updates to the dispatcher.
	ModificationCount int `bson:"modification_count" json:"modification_count"`
}

// NewPodDispatcher returns a new pod dispatcher.
func NewPodDispatcher(groupID string, taskIDs, podIDs []string) PodDispatcher {
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

func (pd *PodDispatcher) atomicUpsertQuery() bson.M {
	return bson.M{
		IDKey:                pd.ID,
		GroupIDKey:           pd.GroupID,
		ModificationCountKey: pd.ModificationCount,
	}
}

func (pd *PodDispatcher) atomicUpsertUpdate() bson.M {
	return bson.M{
		"$setOnInsert": bson.M{
			IDKey:      pd.ID,
			GroupIDKey: pd.GroupID,
		},
		"$set": bson.M{
			PodIDsKey:  pd.PodIDs,
			TaskIDsKey: pd.TaskIDs,
		},
		"$inc": bson.M{
			ModificationCountKey: 1,
		},
	}
}

// UpsertAtomically inserts/updates the pod dispatcher depending on whether the
// document already exists.
func (pd *PodDispatcher) UpsertAtomically() (*adb.ChangeInfo, error) {
	change, err := UpsertOne(pd.atomicUpsertQuery(), pd.atomicUpsertUpdate())
	if err != nil {
		return change, err
	}
	pd.ModificationCount++
	return change, nil
}
