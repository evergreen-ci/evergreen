package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestAverageTaskLatencyLastMinuteByDistro(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)
	require.NoError(db.ClearCollections(task.Collection, distro.Collection))
	distroName := "sampleDistro"
	d := distro.Distro{Id: distroName}
	require.NoError(d.Insert())
	now := time.Now().Add(-50 * time.Second)

	tasks := []task.Task{
		{
			Id:                "task1",
			Requester:         evergreen.RepotrackerVersionRequester,
			ExecutionPlatform: task.ExecutionPlatformHost,
			ActivatedTime:     now,
			ScheduledTime:     now,
			StartTime:         now.Add(10 * time.Second),
			Status:            evergreen.TaskStarted,
			DistroId:          distroName,
			DisplayOnly:       false},
		{
			Id:                "task2",
			Requester:         evergreen.RepotrackerVersionRequester,
			ExecutionPlatform: task.ExecutionPlatformHost,
			ScheduledTime:     now,
			ActivatedTime:     now,
			StartTime:         now.Add(20 * time.Second),
			Status:            evergreen.TaskFailed,
			DistroId:          distroName},
		{
			Id:            "displaytask",
			Requester:     evergreen.RepotrackerVersionRequester,
			ActivatedTime: now,
			ScheduledTime: now,
			StartTime:     now.Add(40 * time.Second),
			Status:        evergreen.TaskFailed,
			DistroId:      distroName,
			DisplayOnly:   true},
		{
			Id:            "task3",
			Requester:     evergreen.RepotrackerVersionRequester,
			ActivatedTime: now.Add(10 * time.Second),
			ScheduledTime: now.Add(10 * time.Second),
			StartTime:     now.Add(40 * time.Second),
			Status:        evergreen.TaskSucceeded,
			DistroId:      distroName},
		{
			Id:            "task4",
			Requester:     evergreen.RepotrackerVersionRequester,
			ScheduledTime: now,
			ActivatedTime: now,
			StartTime:     now.Add(1000 * time.Second),
			Status:        evergreen.TaskUnstarted,
			DistroId:      distroName},
		{
			Id:            "task5",
			Requester:     evergreen.PatchVersionRequester,
			ScheduledTime: now,
			ActivatedTime: now,
			StartTime:     now.Add(5 * time.Second),
			Status:        evergreen.TaskSucceeded,
			DistroId:      distroName},
		{
			Id:            "task6",
			Requester:     evergreen.PatchVersionRequester,
			ActivatedTime: now,
			ScheduledTime: now,
			StartTime:     now.Add(15 * time.Second),
			Status:        evergreen.TaskSucceeded,
			DistroId:      distroName},
		{
			Id:            "task7",
			Requester:     evergreen.GithubPRRequester,
			ActivatedTime: now,
			ScheduledTime: now,
			StartTime:     now.Add(1 * time.Second),
			Status:        evergreen.TaskSucceeded,
			DistroId:      distroName},
	}
	for _, t := range tasks {
		require.NoError(t.Insert())
	}
	latencies, err := AverageHostTaskLatency(time.Minute)
	require.NoError(err)
	expected := []AverageTimeByDistroAndRequester{
		{
			Distro:      "sampleDistro",
			Requester:   evergreen.GithubPRRequester,
			AverageTime: time.Second,
		},
		{
			Distro:      "sampleDistro",
			Requester:   evergreen.PatchVersionRequester,
			AverageTime: 10 * time.Second,
		},
		{
			Distro:      "sampleDistro",
			Requester:   evergreen.RepotrackerVersionRequester,
			AverageTime: 20 * time.Second,
		},
	}
	for _, time := range expected {
		assert.Contains(latencies.Times, time)
	}
}
