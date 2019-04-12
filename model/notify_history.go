package model

import (
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const (
	NotifyHistoryCollection = "notify_history"
)

type NotificationHistory struct {
	Id                    bson.ObjectId `bson:"_id,omitempty"`
	PrevNotificationId    string        `bson:"p_nid"`
	CurrNotificationId    string        `bson:"c_nid"`
	NotificationName      string        `bson:"n_name"`
	NotificationType      string        `bson:"n_type"`
	NotificationTime      time.Time     `bson:"n_time"`
	NotificationProject   string        `bson:"n_branch"`
	NotificationRequester string        `bson:"n_requester"`
}

var (
	// bson fields for the notification history struct
	NHIdKey     = bsonutil.MustHaveTag(NotificationHistory{}, "Id")
	NHPrevIdKey = bsonutil.MustHaveTag(NotificationHistory{},
		"PrevNotificationId")
	NHCurrIdKey = bsonutil.MustHaveTag(NotificationHistory{},
		"CurrNotificationId")
	NHNameKey = bsonutil.MustHaveTag(NotificationHistory{},
		"NotificationName")
	NHTypeKey = bsonutil.MustHaveTag(NotificationHistory{},
		"NotificationType")
	NHTimeKey = bsonutil.MustHaveTag(NotificationHistory{},
		"NotificationTime")
	NHProjectKey = bsonutil.MustHaveTag(NotificationHistory{},
		"NotificationProject")
	NHRequesterKey = bsonutil.MustHaveTag(NotificationHistory{},
		"NotificationRequester")
)

func FindNotificationRecord(notificationId, notificationName, notificationType,
	notificationProject, notificationRequester string) (*NotificationHistory,
	error) {
	return FindOneNotification(
		bson.M{
			NHPrevIdKey:    notificationId,
			NHNameKey:      notificationName,
			NHTypeKey:      notificationType,
			NHProjectKey:   notificationProject,
			NHRequesterKey: notificationRequester,
		},
		bson.M{
			NHIdKey:     1,
			NHPrevIdKey: 1,
		},
	)
}

func InsertNotificationRecord(prevNotification, currNotification,
	notificationName, notificationType, notificationProject,
	notificationRequester string) error {
	nh := &NotificationHistory{
		PrevNotificationId:    prevNotification,
		CurrNotificationId:    currNotification,
		NotificationName:      notificationName,
		NotificationType:      notificationType,
		NotificationTime:      time.Now(),
		NotificationProject:   notificationProject,
		NotificationRequester: notificationRequester,
	}
	return nh.Insert()
}

func (self *NotificationHistory) Insert() error {
	return db.Insert(NotifyHistoryCollection, self)
}

func FindOneNotification(query interface{},
	projection interface{}) (*NotificationHistory, error) {
	notificationHistory := &NotificationHistory{}
	err := db.FindOne(
		NotifyHistoryCollection,
		query,
		projection,
		db.NoSort,
		notificationHistory,
	)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	return notificationHistory, err
}

func UpdateOneNotification(query interface{}, update interface{}) error {
	return db.Update(
		NotifyHistoryCollection,
		query,
		update,
	)
}
