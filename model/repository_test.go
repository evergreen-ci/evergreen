package model

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	_           fmt.Stringer = nil
	projectName              = "mongodb-mongo-testing"
)

func TestGetNewRevisionOrderNumber(t *testing.T) {
	Convey("When requesting a new commit order number...", t, func() {

		Convey("The returned commit order number should be 1 for a new"+
			" project", func() {
			ron, err := GetNewRevisionOrderNumber(t.Context(), projectName)
			So(err, ShouldBeNil)
			So(ron, ShouldEqual, 1)
		})

		Convey("The returned commit order number should be 1 for monotonically"+
			" incremental on a new project", func() {
			ron, err := GetNewRevisionOrderNumber(t.Context(), projectName)
			So(err, ShouldBeNil)
			So(ron, ShouldEqual, 1)
			ron, err = GetNewRevisionOrderNumber(t.Context(), projectName)
			So(err, ShouldBeNil)
			So(ron, ShouldEqual, 2)
		})

		Convey("The returned commit order number should be 1 for monotonically"+
			" incremental within (but not across) projects", func() {
			ron, err := GetNewRevisionOrderNumber(t.Context(), projectName)
			So(err, ShouldBeNil)
			So(ron, ShouldEqual, 1)
			ron, err = GetNewRevisionOrderNumber(t.Context(), projectName)
			So(err, ShouldBeNil)
			So(ron, ShouldEqual, 2)
			ron, err = GetNewRevisionOrderNumber(t.Context(), projectName+"-12")
			So(err, ShouldBeNil)
			So(ron, ShouldEqual, 1)
			ron, err = GetNewRevisionOrderNumber(t.Context(), projectName+"-12")
			So(err, ShouldBeNil)
			So(ron, ShouldEqual, 2)
		})

		Reset(func() {
			So(db.Clear(RepositoriesCollection), ShouldBeNil)
		})

	})
}

func TestUpdateLastRevision(t *testing.T) {
	for name, test := range map[string]func(*testing.T, string, string){
		"InvalidProject": func(t *testing.T, project string, revision string) {
			assert.Error(t, UpdateLastRevision(t.Context(), project, revision))
		},
		"ValidProject": func(t *testing.T, project string, revision string) {
			_, err := GetNewRevisionOrderNumber(t.Context(), project)
			assert.NoError(t, err)
			assert.NoError(t, UpdateLastRevision(t.Context(), project, revision))
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, db.Clear(RepositoriesCollection))
			defer func() {
				assert.NoError(t, db.Clear(RepositoriesCollection))
			}()
			test(t, "my-project", "my-revision")
		})
	}
}
