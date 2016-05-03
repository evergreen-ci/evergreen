package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(evergreen.TestConfig()))
	evergreen.SetLogger("/tmp/version_test.log")
}

func TestLastKnownGoodConfig(t *testing.T) {
	Convey("When calling LastKnownGoodConfig..", t, func() {
		identifier := "identifier"
		Convey("no versions should be returned if there're no good "+
			"last known configurations", func() {
			v := &version.Version{
				Identifier: identifier,
				Requester:  evergreen.RepotrackerVersionRequester,
				Errors:     []string{"error 1", "error 2"},
			}
			testutil.HandleTestingErr(v.Insert(), t, "Error inserting test version: %v")
			lastGood, err := version.FindOne(version.ByLastKnownGoodConfig(identifier))
			testutil.HandleTestingErr(err, t, "error finding last known good: %v")
			So(lastGood, ShouldBeNil)
		})
		Convey("a version should be returned if there is a last known good configuration", func() {
			v := &version.Version{
				Identifier: identifier,
				Requester:  evergreen.RepotrackerVersionRequester,
			}
			testutil.HandleTestingErr(v.Insert(), t, "Error inserting test version: %v")
			lastGood, err := version.FindOne(version.ByLastKnownGoodConfig(identifier))
			testutil.HandleTestingErr(err, t, "error finding last known good: %v")
			So(lastGood, ShouldNotBeNil)
		})
		Convey("most recent version should be found if there are several recent good configs", func() {
			v := &version.Version{
				Id:                  "1",
				Identifier:          identifier,
				Requester:           evergreen.RepotrackerVersionRequester,
				RevisionOrderNumber: 1,
				Config:              "1",
			}
			testutil.HandleTestingErr(v.Insert(), t, "Error inserting test version: %v")
			v.Id = "5"
			v.RevisionOrderNumber = 5
			v.Config = "5"
			testutil.HandleTestingErr(v.Insert(), t, "Error inserting test version: %v")
			v.Id = "2"
			v.RevisionOrderNumber = 2
			v.Config = "2"
			testutil.HandleTestingErr(v.Insert(), t, "Error inserting test version: %v")
			lastGood, err := version.FindOne(version.ByLastKnownGoodConfig(identifier))
			testutil.HandleTestingErr(err, t, "error finding last known good: %v")
			So(lastGood, ShouldNotBeNil)
			So(lastGood.Config, ShouldEqual, "5")
		})
		Reset(func() {
			db.Clear(version.Collection)
		})
	})
}
