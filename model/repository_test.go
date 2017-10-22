package model

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	. "github.com/smartystreets/goconvey/convey"
)

var (
	_           fmt.Stringer = nil
	projectName              = "mongodb-mongo-testing"
)

func init() {
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
}

func TestGetNewRevisionOrderNumber(t *testing.T) {

	Convey("When requesting a new commit order number...", t, func() {

		Convey("The returned commit order number should be 1 for a new"+
			" project", func() {
			ron, err := GetNewRevisionOrderNumber(projectName)
			So(err, ShouldBeNil)
			So(ron, ShouldEqual, 1)
		})

		Convey("The returned commit order number should be 1 for monotonically"+
			" incremental on a new project", func() {
			ron, err := GetNewRevisionOrderNumber(projectName)
			So(err, ShouldBeNil)
			So(ron, ShouldEqual, 1)
			ron, err = GetNewRevisionOrderNumber(projectName)
			So(err, ShouldBeNil)
			So(ron, ShouldEqual, 2)
		})

		Convey("The returned commit order number should be 1 for monotonically"+
			" incremental within (but not across) projects", func() {
			ron, err := GetNewRevisionOrderNumber(projectName)
			So(err, ShouldBeNil)
			So(ron, ShouldEqual, 1)
			ron, err = GetNewRevisionOrderNumber(projectName)
			So(err, ShouldBeNil)
			So(ron, ShouldEqual, 2)
			ron, err = GetNewRevisionOrderNumber(projectName + "-12")
			So(err, ShouldBeNil)
			So(ron, ShouldEqual, 1)
			ron, err = GetNewRevisionOrderNumber(projectName + "-12")
			So(err, ShouldBeNil)
			So(ron, ShouldEqual, 2)
		})

		Reset(func() {
			So(db.Clear(RepositoriesCollection), ShouldBeNil)
		})

	})
}
