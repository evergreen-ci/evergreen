package model

import (
	"10gen.com/mci/db"
	"10gen.com/mci/util"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestFindOneProjectVar(t *testing.T) {
	Convey("With an existing repository var", t, func() {
		util.HandleTestingErr(db.Clear(ProjectVarsCollection), t,
			"Error clearing collection")
		vars := map[string]string{
			"a": "b",
			"c": "d",
		}
		projectVars := ProjectVars{
			Id:   "mongodb",
			Vars: vars,
		}

		Convey("all fields should be returned accurately for the "+
			"corresponding project vars", func() {
			_, err := projectVars.Upsert()
			So(err, ShouldBeNil)
			projectVarsFromDB, err := FindOneProjectVars("mongodb")
			So(err, ShouldBeNil)
			So(projectVarsFromDB.Id, ShouldEqual, "mongodb")
			So(projectVarsFromDB.Vars, ShouldResemble, vars)
		})
	})
}
