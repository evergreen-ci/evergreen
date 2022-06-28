package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	. "github.com/smartystreets/goconvey/convey"
)

func TestFindPredictedMakespan(t *testing.T) {
	Convey("With a simple set of tasks that are dependent on each other and different times taken", t, func() {

		a := task.Task{Id: "a", TimeTaken: time.Duration(5) * time.Second, DependsOn: []task.Dependency{}}
		b := task.Task{Id: "b", TimeTaken: time.Duration(3) * time.Second, DependsOn: []task.Dependency{{TaskId: "a", Status: evergreen.TaskFailed}}}
		c := task.Task{Id: "c", TimeTaken: time.Duration(4) * time.Second, DependsOn: []task.Dependency{{TaskId: "a", Status: evergreen.TaskFailed}}}
		f := task.Task{Id: "f", TimeTaken: time.Duration(40) * time.Second, DependsOn: []task.Dependency{{TaskId: "b", Status: evergreen.TaskFailed}}}

		d := task.Task{Id: "d", TimeTaken: time.Duration(10) * time.Second}
		e := task.Task{Id: "e", TimeTaken: time.Duration(5) * time.Second, DependsOn: []task.Dependency{{TaskId: "d", Status: evergreen.TaskFailed}}}

		Convey("with one tree of dependencies", func() {
			allTasks := []task.Task{a, b, c}
			depPath := FindPredictedMakespan(allTasks)
			So(depPath.TotalTime, ShouldEqual, time.Duration(9)*time.Second)
			So(len(depPath.Tasks), ShouldEqual, 2)
		})
		Convey("with one tree and one singular longer task", func() {
			allTasks := []task.Task{a, b, c, d}
			depPath := FindPredictedMakespan(allTasks)
			So(depPath.TotalTime, ShouldEqual, time.Duration(10)*time.Second)
			So(len(depPath.Tasks), ShouldEqual, 1)
			So(depPath.Tasks[0], ShouldEqual, "d")
		})
		Convey("with two trees", func() {
			allTasks := []task.Task{a, b, c, d, e}
			depPath := FindPredictedMakespan(allTasks)
			So(depPath.TotalTime, ShouldEqual, time.Duration(15)*time.Second)
			So(len(depPath.Tasks), ShouldEqual, 2)

		})
		Convey("with a tree with varying times taken", func() {
			allTasks := []task.Task{a, b, c, f}
			depPath := FindPredictedMakespan(allTasks)
			So(depPath.TotalTime, ShouldEqual, time.Duration(48)*time.Second)
		})

	})
}

func TestCalculateActualMakespan(t *testing.T) {
	Convey("With a simple set of tasks that are dependent on each other and different times taken", t, func() {
		now := time.Now()
		a := task.Task{Id: "a", StartTime: now.Add(time.Duration(-10) * time.Second), FinishTime: now.Add(10 * time.Second)}
		b := task.Task{Id: "b", StartTime: now.Add(time.Duration(-20) * time.Second), FinishTime: now.Add(20 * time.Second)}
		c := task.Task{Id: "c", StartTime: now, FinishTime: now.Add(10 * time.Second)}
		d := task.Task{Id: "d", StartTime: now.Add(time.Duration(10) * time.Second), FinishTime: now.Add(40 * time.Second)}

		Convey("with one tree of dependencies", func() {
			allTasks := []task.Task{a, b, c, d}
			makespan := CalculateActualMakespan(allTasks)
			So(makespan, ShouldEqual, time.Duration(60)*time.Second)
		})

	})
}
