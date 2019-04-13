package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	mgobson "gopkg.in/mgo.v2/bson"
)

var notifyHistoryTestConfig = testutil.TestConfig()

func TestGenericNotificationFinding(t *testing.T) {

	Convey("When finding notifications", t, func() {

		require.NoError(t, db.Clear(NotifyHistoryCollection), "Error clearing '%v' collection", NotifyHistoryCollection)

		Convey("when finding one notification", func() {

			Convey("the matching notification should be returned", func() {

				nHistoryOne := &NotificationHistory{
					Id: mgobson.NewObjectId(),
				}
				So(nHistoryOne.Insert(), ShouldBeNil)

				nHistoryTwo := &NotificationHistory{
					Id: mgobson.NewObjectId(),
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

		require.NoError(t, db.Clear(NotifyHistoryCollection), "Error clearing '%v' collection", NotifyHistoryCollection)

		Convey("updating one notification should update the specified"+
			" notification in the database", func() {

			nHistory := &NotificationHistory{
				Id: mgobson.NewObjectId(),
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
