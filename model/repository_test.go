package model

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

var (
	_           fmt.Stringer = nil
	projectName              = "mongodb-mongo-testing"
)

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

func TestUpdateLastRevision(t *testing.T) {
	for name, test := range map[string]func(*testing.T, string){
		"InvalidProject": func(t *testing.T, revision string) {
			assert.Error(t, UpdateLastRevision("not-a-project", revision))
		},
		"ValidProject": func(t *testing.T, revision string) {
			_, err := GetNewRevisionOrderNumber("my-project")
			assert.NoError(t, err)
			assert.NoError(t, UpdateLastRevision("my-project", revision))
		},
	} {
		t.Run(name, func(t *testing.T) {
			test(t, "my-revision")
		})
	}
}
