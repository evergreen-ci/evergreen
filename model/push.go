package model

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"go.mongodb.org/mongo-driver/bson"
)

const PushlogCollection = "pushes"

type PushLog struct {
	Id mgobson.ObjectId `bson:"_id,omitempty"`

	//the permanent location of the pushed file.
	Location string `bson:"location"`

	//the task id of the push stage
	TaskId string `bson:"task_id"`

	CreateTime time.Time `bson:"create_time"`
	Revision   string    `bson:"githash"`
	Status     string    `bson:"status"`

	//copied from version for the task
	RevisionOrderNumber int `bson:"order"`
}

var (
	// bson fields for the push log struct
	PushLogIdKey         = bsonutil.MustHaveTag(PushLog{}, "Id")
	PushLogLocationKey   = bsonutil.MustHaveTag(PushLog{}, "Location")
	PushLogTaskIdKey     = bsonutil.MustHaveTag(PushLog{}, "TaskId")
	PushLogCreateTimeKey = bsonutil.MustHaveTag(PushLog{}, "CreateTime")
	PushLogRevisionKey   = bsonutil.MustHaveTag(PushLog{}, "Revision")
	PushLogStatusKey     = bsonutil.MustHaveTag(PushLog{}, "Status")
	PushLogRonKey        = bsonutil.MustHaveTag(PushLog{}, "RevisionOrderNumber")
)

func NewPushLog(v *Version, task *task.Task, location string) *PushLog {
	return &PushLog{
		Id:                  mgobson.NewObjectId(),
		Location:            location,
		TaskId:              task.Id,
		CreateTime:          time.Now(),
		Revision:            v.Revision,
		RevisionOrderNumber: v.RevisionOrderNumber,
		Status:              evergreen.PushLogPushing,
	}
}

func (pl *PushLog) Insert(ctx context.Context) error {
	return db.Insert(ctx, PushlogCollection, pl)
}

func (pl *PushLog) UpdateStatus(ctx context.Context, newStatus string) error {
	return db.UpdateContext(
		ctx,
		PushlogCollection,
		bson.M{
			PushLogIdKey: pl.Id,
		},
		bson.M{
			"$set": bson.M{
				PushLogStatusKey: newStatus,
			},
		},
	)
}

func FindOnePushLog(ctx context.Context, query any, projection any,
	sort []string) (*PushLog, error) {
	pushLog := &PushLog{}
	q := db.Query(query).Project(projection).Sort(sort)
	err := db.FindOneQContext(ctx, PushlogCollection, q, pushLog)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return pushLog, err
}

// FindNewerPushLog returns a PushLog item if there is a file pushed from
// this version or a newer one, or one already in progress.
func FindPushLogAfter(ctx context.Context, fileLoc string, revisionOrderNumber int) (*PushLog, error) {
	query := bson.M{
		PushLogStatusKey: bson.M{
			"$in": []string{
				evergreen.PushLogPushing, evergreen.PushLogSuccess,
			},
		},
		PushLogLocationKey: fileLoc,
		PushLogRonKey: bson.M{
			"$gte": revisionOrderNumber,
		},
	}
	existingPushLog, err := FindOnePushLog(
		ctx,
		query,
		db.NoProjection,
		[]string{"-" + PushLogRonKey},
	)
	if err != nil {
		return nil, err
	}
	return existingPushLog, nil
}
