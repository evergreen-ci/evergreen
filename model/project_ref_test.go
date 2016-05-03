package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func TestFindOneProjectRef(t *testing.T) {
	Convey("With an existing repository ref", t, func() {
		testutil.HandleTestingErr(db.Clear(ProjectRefCollection), t,
			"Error clearing collection")
		projectRef := &ProjectRef{
			Owner:      "mongodb",
			Repo:       "mci",
			Branch:     "master",
			RepoKind:   "github",
			Enabled:    true,
			BatchTime:  10,
			Identifier: "ident",
		}
		Convey("all fields should be returned accurately for the "+
			"corresponding project ref", func() {
			So(projectRef.Insert(), ShouldBeNil)
			projectRefFromDB, err := FindOneProjectRef("ident")
			So(err, ShouldBeNil)
			So(projectRefFromDB, ShouldNotEqual, nil)
			So(projectRefFromDB.Owner, ShouldEqual, "mongodb")
			So(projectRefFromDB.Repo, ShouldEqual, "mci")
			So(projectRefFromDB.Branch, ShouldEqual, "master")
			So(projectRefFromDB.RepoKind, ShouldEqual, "github")
			So(projectRefFromDB.Enabled, ShouldEqual, true)
			So(projectRefFromDB.BatchTime, ShouldEqual, 10)
			So(projectRefFromDB.Identifier, ShouldEqual, "ident")
		})
	})
}
