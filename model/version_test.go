package model

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func TestLastKnownGoodConfig(t *testing.T) {
	Convey("When calling LastKnownGoodConfig..", t, func() {
		identifier := "identifier"
		Convey("no versions should be returned if there're no good "+
			"last known configurations", func() {
			v := &Version{
				Identifier: identifier,
				Requester:  evergreen.RepotrackerVersionRequester,
				Errors:     []string{"error 1", "error 2"},
			}
			testutil.HandleTestingErr(v.Insert(), t, "Error inserting test version: %s", v.Id)
			lastGood, err := VersionFindOne(VersionByLastKnownGoodConfig(identifier))
			testutil.HandleTestingErr(err, t, "error finding last known good")
			So(lastGood, ShouldBeNil)
		})
		Convey("a version should be returned if there is a last known good configuration", func() {
			v := &Version{
				Identifier: identifier,
				Requester:  evergreen.RepotrackerVersionRequester,
			}
			testutil.HandleTestingErr(v.Insert(), t, "Error inserting test version: %s", v.Id)
			lastGood, err := VersionFindOne(VersionByLastKnownGoodConfig(identifier))
			testutil.HandleTestingErr(err, t, "error finding last known good: %s", lastGood.Id)
			So(lastGood, ShouldNotBeNil)
		})
		Convey("most recent version should be found if there are several recent good configs", func() {
			v := &Version{
				Id:                  "1",
				Identifier:          identifier,
				Requester:           evergreen.RepotrackerVersionRequester,
				RevisionOrderNumber: 1,
				Config:              "1",
			}
			testutil.HandleTestingErr(v.Insert(), t, "Error inserting test version: %s", v.Id)
			v.Id = "5"
			v.RevisionOrderNumber = 5
			v.Config = "5"
			testutil.HandleTestingErr(v.Insert(), t, "Error inserting test version: %s", v.Id)
			v.Id = "2"
			v.RevisionOrderNumber = 2
			v.Config = "2"
			testutil.HandleTestingErr(v.Insert(), t, "Error inserting test version: %s", v.Id)
			lastGood, err := VersionFindOne(VersionByLastKnownGoodConfig(identifier))
			testutil.HandleTestingErr(err, t, "error finding last known good: %s", v.Id)
			So(lastGood, ShouldNotBeNil)
			So(lastGood.Config, ShouldEqual, "5")
		})
		Reset(func() {
			So(db.Clear(VersionCollection), ShouldBeNil)
		})
	})
}

func TestVersionSortByOrder(t *testing.T) {
	assert := assert.New(t)
	versions := VersionsByOrder{
		{Id: "5", RevisionOrderNumber: 5},
		{Id: "3", RevisionOrderNumber: 3},
		{Id: "1", RevisionOrderNumber: 1},
		{Id: "4", RevisionOrderNumber: 4},
		{Id: "100", RevisionOrderNumber: 100},
	}
	sort.Sort(versions)
	assert.Equal("1", versions[0].Id)
	assert.Equal("3", versions[1].Id)
	assert.Equal("4", versions[2].Id)
	assert.Equal("5", versions[3].Id)
	assert.Equal("100", versions[4].Id)
}
