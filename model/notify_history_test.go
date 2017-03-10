package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"gopkg.in/mgo.v2/bson"
)

var notifyHistoryTestConfig = testutil.TestConfig()

func init() {
	db.SetGlobalSessionProvider(
		db.SessionFactoryFromConfig(notifyHistoryTestConfig))
}

func TestGenericNotificationFinding(t *testing.T) {

	Convey("When finding notifications", t, func() {

		testutil.HandleTestingErr(db.Clear(NotifyHistoryCollection),
			t, "Error clearing '%v' collection", NotifyHistoryCollection)

		Convey("when finding one notification", func() {

			Convey("the matching notification should be returned", func() {

				nHistoryOne := &NotificationHistory{
					Id: bson.NewObjectId(),
				}
				So(nHistoryOne.Insert(), ShouldBeNil)

				nHistoryTwo := &NotificationHistory{
					Id: bson.NewObjectId(),
				}
				So(nHistoryTwo.Insert(), ShouldBeNil)

				found, err := FindOneNotification(
					bson.M{
						NHIdKey: nHistoryOne.Id,
					},
					db.NoProjection,
				)
				So(err, ShouldBeNil)
				So(found.Id, ShouldEqual, nHistoryOne.Id)

				found, err = FindOneNotification(
					bson.M{
						NHIdKey: nHistoryTwo.Id,
					},
					db.NoProjection,
				)
				So(err, ShouldBeNil)
				So(found.Id, ShouldEqual, nHistoryTwo.Id)
			})

		})

	})

}

func TestUpdatingNotifications(t *testing.T) {

	Convey("When updating notifications", t, func() {

		testutil.HandleTestingErr(db.Clear(NotifyHistoryCollection),
			t, "Error clearing '%v' collection", NotifyHistoryCollection)

		Convey("updating one notification should update the specified"+
			" notification in the database", func() {

			nHistory := &NotificationHistory{
				Id: bson.NewObjectId(),
			}
			So(nHistory.Insert(), ShouldBeNil)

			So(UpdateOneNotification(
				bson.M{
					NHIdKey: nHistory.Id,
				},
				bson.M{
					"$set": bson.M{
						NHPrevIdKey: "prevId",
					},
				},
			), ShouldBeNil)

			found, err := FindOneNotification(
				bson.M{
					NHIdKey: nHistory.Id,
				},
				db.NoProjection,
			)
			So(err, ShouldBeNil)
			So(found.PrevNotificationId, ShouldEqual, "prevId")

		})

	})

}
