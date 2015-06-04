package alert

import (
	"github.com/evergreen-ci/evergreen/db"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"time"
)

type QueueStatus string

const (
	Pending    QueueStatus = "pending"
	InProgress             = "in-progress"
	Delivered              = "delivered"
	Failed                 = "failed"
)

// AlertRequest represents the raw database record of an alert that has been queued into the DB
type AlertRequest struct {
	Id          bson.ObjectId `bson:"_id"`
	QueueStatus QueueStatus   `bson:"queue_status"`
	Trigger     string        `bson:"trigger"`
	TaskId      string        `bson:"task_id,omitempty"`
	HostId      string        `bson:"host_id,omitempty"`
	Execution   int           `bson:"execution,omitempty"`
	BuildId     string        `bson:"build_id,omitempty"`
	VersionId   string        `bson:"version_id,omitempty"`
	ProjectId   string        `bson:"project_id,omitempty"`
	PatchId     string        `bson:"patch_id,omitempty"`
	Display     string        `bson:"display"`
	CreatedAt   time.Time     `bson:"created_at"`
	ProcessedAt time.Time     `bson:"processed_at"`
}

func DequeueAlertRequest() (*AlertRequest, error) {
	out := AlertRequest{}
	_, err := db.FindAndModify(Collection,
		bson.M{QueueStatusKey: Pending},
		[]string{CreatedAtKey},
		mgo.Change{
			Update:    bson.M{"$set": bson.M{QueueStatusKey: InProgress}},
			Upsert:    false,
			Remove:    false,
			ReturnNew: true,
		}, &out)

	if err == mgo.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func EnqueueAlertRequest(a *AlertRequest) error {
	a.QueueStatus = Pending
	return db.Insert(Collection, a)
}
