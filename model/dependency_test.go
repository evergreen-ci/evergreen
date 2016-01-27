package model

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"gopkg.in/mgo.v2/bson"
	"testing"
)

// We have to define a wrapper for the dependencies,
// since you can't marshal BSON straight into a slice.
type depTask struct {
	DependsOn []task.Dependency `bson:"depends_on"`
}

func TestDependencyBSON(t *testing.T) {
	Convey("With BSON bytes", t, func() {
		Convey("representing legacy dependency format (i.e. just strings)", func() {
			bytes, err := bson.Marshal(map[string]interface{}{
				"depends_on": []string{"t1", "t2", "t3"},
			})
			testutil.HandleTestingErr(err, t, "failed to marshal test BSON")

			Convey("unmarshalling the BSON into a Dependency slice should succeed", func() {
				var deps depTask
				So(bson.Unmarshal(bytes, &deps), ShouldBeNil)
				So(len(deps.DependsOn), ShouldEqual, 3)

				Convey("with the proper tasks", func() {
					So(deps.DependsOn[0].TaskId, ShouldEqual, "t1")
					So(deps.DependsOn[0].Status, ShouldEqual, evergreen.TaskSucceeded)
					So(deps.DependsOn[1].TaskId, ShouldEqual, "t2")
					So(deps.DependsOn[1].Status, ShouldEqual, evergreen.TaskSucceeded)
					So(deps.DependsOn[2].TaskId, ShouldEqual, "t3")
					So(deps.DependsOn[2].Status, ShouldEqual, evergreen.TaskSucceeded)
				})
			})
		})

		Convey("representing the current dependency format", func() {
			inputDeps := depTask{[]task.Dependency{
				{TaskId: "t1", Status: evergreen.TaskSucceeded},
				{TaskId: "t2", Status: "*"},
				{TaskId: "t3", Status: evergreen.TaskFailed},
			}}
			bytes, err := bson.Marshal(inputDeps)
			testutil.HandleTestingErr(err, t, "failed to marshal test BSON")

			Convey("unmarshalling the BSON into a Dependency slice should succeed", func() {
				var deps depTask
				So(bson.Unmarshal(bytes, &deps), ShouldBeNil)

				Convey("with the proper tasks", func() {
					So(deps.DependsOn, ShouldResemble, inputDeps.DependsOn)
				})
			})
		})
	})
}
