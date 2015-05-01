package model

import (
	"fmt"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/bsonutil"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"time"
)

var EarliestDateToConsider time.Time

const (
	NotifyTimesCollection = "notify_times"
)

type ProjectNotificationTime struct {
	ProjectName               string    `bson:"_id"`
	LastNotificationEventTime time.Time `bson:"last_notification_event_time"`
}

var (
	PntProjectNameKey = bsonutil.MustHaveTag(ProjectNotificationTime{},
		"ProjectName")
	PntLastEventTime = bsonutil.MustHaveTag(ProjectNotificationTime{},
		"LastNotificationEventTime")
)

// Record the last-notification time for a given project.
func SetLastNotificationsEventTime(projectName string,
	timeOfEvent time.Time) error {
	_, err := db.Upsert(
		NotifyTimesCollection,
		bson.M{
			PntProjectNameKey: projectName,
		},
		bson.M{
			"$set": bson.M{
				PntLastEventTime: timeOfEvent,
			},
		},
	)
	return err
}

func LastNotificationsEventTime(projectName string) (time.Time,
	error) {

	nAnswers, err := db.Count(
		NotifyTimesCollection,
		bson.M{
			PntProjectNameKey: projectName,
		},
	)

	if err != nil {
		return EarliestDateToConsider, err
	}
	if nAnswers == 0 {
		return EarliestDateToConsider, nil
	}

	if nAnswers > 1 {
		return EarliestDateToConsider, fmt.Errorf("There are %v notification"+
			" times listed for having seen the NOTIFICATION_REPOSITORY “%v”;"+
			" there should be at most one.", nAnswers, projectName)
	}

	event := &ProjectNotificationTime{}
	err = db.FindOne(
		NotifyTimesCollection,
		bson.M{
			PntProjectNameKey: projectName,
		},
		db.NoProjection,
		db.NoSort,
		event,
	)
	if err != nil {
		return EarliestDateToConsider, err
	}
	if err == mgo.ErrNotFound {
		return EarliestDateToConsider, nil
	}

	return event.LastNotificationEventTime, nil
}
