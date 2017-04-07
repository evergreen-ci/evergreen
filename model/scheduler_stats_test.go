package model

import (
	"math"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
)

var projectTestConfig = testutil.TestConfig()

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(projectTestConfig))
}

func TestBucketResource(t *testing.T) {
	Convey("With a start time and a bucket size of 10 and 10 buckets", t, func() {
		frameStart := time.Now()
		// 10 buckets * 10 bucket size = 100
		frameEnd := frameStart.Add(time.Duration(100))
		bucketSize := time.Duration(10)
		Convey("when resource start time is equal to end time should error", func() {
			buckets := make([]Bucket, 10)
			resourceStart := frameEnd
			resourceEnd := frameEnd.Add(time.Duration(10))
			resource := ResourceInfo{
				Start: resourceStart,
				End:   resourceEnd,
			}
			_, err := bucketResource(resource, frameStart, frameEnd, bucketSize, buckets)
			So(err, ShouldNotBeNil)
		})
		Convey("when resource start time is greater than end time should error", func() {
			buckets := make([]Bucket, 10)
			resourceStart := frameEnd.Add(time.Duration(10))
			resourceEnd := frameEnd.Add(time.Duration(20))
			resource := ResourceInfo{
				Start: resourceStart,
				End:   resourceEnd,
			}
			_, err := bucketResource(resource, frameStart, frameEnd, bucketSize, buckets)
			So(err, ShouldNotBeNil)
		})
		Convey("when resource end time is equal to start time should error", func() {
			buckets := make([]Bucket, 10)
			resourceStart := frameStart.Add(time.Duration(-10))
			resourceEnd := frameStart
			resource := ResourceInfo{
				Start: resourceStart,
				End:   resourceEnd,
			}
			_, err := bucketResource(resource, frameStart, frameEnd, bucketSize, buckets)
			So(err, ShouldNotBeNil)
		})
		Convey("when resource end time is less than start time should error", func() {
			buckets := make([]Bucket, 10)
			resourceStart := frameStart.Add(time.Duration(-30))
			resourceEnd := frameStart.Add(time.Duration(-10))
			resource := ResourceInfo{
				Start: resourceStart,
				End:   resourceEnd,
			}
			_, err := bucketResource(resource, frameStart, frameEnd, bucketSize, buckets)
			So(err, ShouldNotBeNil)
		})
		Convey("when resource end time is less than resource start time, should error", func() {
			buckets := make([]Bucket, 10)
			resourceStart := frameStart.Add(time.Duration(10))
			resourceEnd := frameStart
			resource := ResourceInfo{
				Start: resourceStart,
				End:   resourceEnd,
			}
			_, err := bucketResource(resource, frameStart, frameEnd, bucketSize, buckets)
			So(err, ShouldNotBeNil)
		})
		Convey("when resource start is zero, errors out", func() {
			buckets := make([]Bucket, 10)
			resourceStart := time.Time{}
			resourceEnd := frameStart.Add(time.Duration(1))
			resource := ResourceInfo{
				Start: resourceStart,
				End:   resourceEnd,
			}
			_, err := bucketResource(resource, frameStart, frameEnd, bucketSize, buckets)
			So(err, ShouldNotBeNil)
		})
		Convey("when the resource start and end time are in the same bucket, only one bucket has the difference", func() {
			buckets := make([]Bucket, 10)
			resourceStart := frameStart.Add(time.Duration(1))
			resourceEnd := frameStart.Add(time.Duration(5))
			resource := ResourceInfo{
				Start: resourceStart,
				End:   resourceEnd,
			}
			_, err := bucketResource(resource, frameStart, frameEnd, bucketSize, buckets)
			So(err, ShouldBeNil)
			So(buckets[0].TotalTime, ShouldEqual, time.Duration(4))
			for i := 1; i < 10; i++ {
				So(buckets[i].TotalTime, ShouldEqual, 0)
			}
		})
		Convey("when the resourceEnd is zero, there is no error", func() {
			buckets := make([]Bucket, 10)
			resourceStart := frameStart.Add(time.Duration(10))
			resourceEnd := util.ZeroTime
			So(util.IsZeroTime(resourceEnd), ShouldBeTrue)
			resource := ResourceInfo{
				Start: resourceStart,
				End:   resourceEnd,
			}
			_, err := bucketResource(resource, frameStart, frameEnd, bucketSize, buckets)
			So(err, ShouldBeNil)
			So(buckets[0].TotalTime, ShouldEqual, 0)
			for i := 1; i < 10; i++ {
				So(buckets[i].TotalTime, ShouldEqual, 10)
			}
		})

	})
}

func TestCreateHostBuckets(t *testing.T) {
	testutil.HandleTestingErr(db.ClearCollections(host.Collection), t, "couldnt reset host")
	Convey("With a starting time and a minute bucket size and inserting dynamic hosts with different time frames", t, func() {
		now := time.Now()
		bucketSize := time.Duration(10) * time.Second

		// -20 -> 20
		beforeStartHost := host.Host{Id: "beforeStartHost", CreationTime: now.Add(time.Duration(-20) * time.Second), TerminationTime: now.Add(time.Duration(20) * time.Second), Provider: "ec2"}
		So(beforeStartHost.Insert(), ShouldBeNil)

		// 80 -> 120
		afterEndHost := host.Host{Id: "afterEndHost", CreationTime: now.Add(time.Duration(80) * time.Second), TerminationTime: now.Add(time.Duration(120) * time.Second), Provider: "ec2"}
		So(afterEndHost.Insert(), ShouldBeNil)

		// 20 -> 40
		h1 := host.Host{Id: "h1", CreationTime: now.Add(time.Duration(20) * time.Second), TerminationTime: now.Add(time.Duration(40) * time.Second), Provider: "ec2"}
		So(h1.Insert(), ShouldBeNil)

		// 10 -> 80
		h2 := host.Host{Id: "h2", CreationTime: now.Add(time.Duration(10) * time.Second), TerminationTime: now.Add(time.Duration(80) * time.Second), Provider: "ec2"}
		So(h2.Insert(), ShouldBeNil)

		// 20 ->
		h3 := host.Host{Id: "h3", CreationTime: now.Add(time.Duration(20) * time.Second), TerminationTime: util.ZeroTime, Provider: "ec2", Status: evergreen.HostRunning}
		So(h3.Insert(), ShouldBeNil)

		// 5 -> 7
		sameBucket := host.Host{Id: "sameBucket", CreationTime: now.Add(time.Duration(5) * time.Second), TerminationTime: now.Add(time.Duration(7) * time.Second), Provider: "ec2"}
		So(sameBucket.Insert(), ShouldBeNil)

		// 5 -> 30
		h4 := host.Host{Id: "h4", CreationTime: now.Add(time.Duration(5) * time.Second), TerminationTime: now.Add(time.Duration(30) * time.Second), Provider: "ec2"}
		So(h4.Insert(), ShouldBeNil)

		Convey("for three buckets of 10 seconds, should only retrieve pertinent host docs", func() {

			endTime := now.Add(time.Duration(30) * time.Second)
			hosts, err := host.Find(host.ByDynamicWithinTime(now, endTime))
			So(err, ShouldBeNil)
			So(len(hosts), ShouldEqual, 6)
			frameBounds := FrameBounds{
				StartTime:     now,
				EndTime:       endTime,
				BucketSize:    bucketSize,
				NumberBuckets: 3,
			}
			Convey("should create the correct buckets and bucket time accordingly", func() {
				buckets, errors := CreateHostBuckets(hosts, frameBounds)
				So(errors, ShouldBeEmpty)
				So(len(buckets), ShouldEqual, 3)
				So(int(buckets[0].TotalTime.Seconds()), ShouldEqual, 17)
				So(int(buckets[1].TotalTime.Seconds()), ShouldEqual, 30)
				So(int(math.Ceil(buckets[2].TotalTime.Seconds())), ShouldEqual, 40)
			})
		})

	})
}

func TestCreateTaskBuckets(t *testing.T) {
	testutil.HandleTestingErr(db.ClearCollections(task.Collection), t, "couldnt reset host")
	Convey("With a starting time and a minute bucket size and inserting tasks with different start and finish", t, func() {
		now := time.Now()
		bucketSize := time.Duration(10) * time.Second

		// -20 -> 20
		beforeStartHost := task.Task{Id: "beforeStartTask", StartTime: now.Add(time.Duration(-20) * time.Second), FinishTime: now.Add(time.Duration(20) * time.Second), Status: evergreen.TaskSucceeded}
		So(beforeStartHost.Insert(), ShouldBeNil)

		// 80 -> 120
		afterEndHost := task.Task{Id: "afterStartTask", StartTime: now.Add(time.Duration(80) * time.Second), FinishTime: now.Add(time.Duration(120) * time.Second), Status: evergreen.TaskFailed}
		So(afterEndHost.Insert(), ShouldBeNil)

		// 20 -> 40: shouldnt be added
		h1 := task.Task{Id: "h1", StartTime: now.Add(time.Duration(20) * time.Second), FinishTime: now.Add(time.Duration(40) * time.Second), Status: evergreen.TaskUndispatched}
		So(h1.Insert(), ShouldBeNil)

		// 10 -> 80
		h2 := task.Task{Id: "h2", StartTime: now.Add(time.Duration(10) * time.Second), FinishTime: now.Add(time.Duration(80) * time.Second), Status: evergreen.TaskSucceeded}
		So(h2.Insert(), ShouldBeNil)

		// 20 -> shouldnt be added
		neverEnding := task.Task{Id: "neverEnding", StartTime: now.Add(time.Duration(20) * time.Second), Status: evergreen.TaskSucceeded}
		So(neverEnding.Insert(), ShouldBeNil)

		// 5 -> 7
		sameBucket := task.Task{Id: "sameBucket", StartTime: now.Add(time.Duration(5) * time.Second), FinishTime: now.Add(time.Duration(7) * time.Second), Status: evergreen.TaskFailed}
		So(sameBucket.Insert(), ShouldBeNil)

		// 5 -> 30
		h4 := task.Task{Id: "h4", StartTime: now.Add(time.Duration(5) * time.Second), FinishTime: now.Add(time.Duration(30) * time.Second), Status: evergreen.TaskFailed}
		So(h4.Insert(), ShouldBeNil)

		endTime := now.Add(time.Duration(40) * time.Second)
		frameBounds := FrameBounds{
			StartTime:     now,
			EndTime:       endTime,
			NumberBuckets: 4,
			BucketSize:    bucketSize,
		}
		Convey("for four buckets of 10 seconds", func() {
			tasks, err := task.Find(task.ByTimeRun(now, endTime))
			So(err, ShouldBeNil)
			So(len(tasks), ShouldEqual, 4)

			buckets, errors := CreateTaskBuckets(tasks, []task.Task{}, frameBounds)
			So(errors, ShouldBeEmpty)
			So(len(buckets), ShouldEqual, 4)
			So(int(buckets[0].TotalTime.Seconds()), ShouldEqual, 17)
			So(int(math.Ceil(buckets[1].TotalTime.Seconds())), ShouldEqual, 30)
			So(int(math.Ceil(buckets[2].TotalTime.Seconds())), ShouldEqual, 20)
		})

	})
}

func TestAverageStatistics(t *testing.T) {
	testutil.HandleTestingErr(db.ClearCollections(task.Collection), t, "couldnt reset host")
	Convey("With a distro sampleDistro inserted", t, func() {
		d := distro.Distro{
			Id: "sampleDistro",
		}
		err := d.Insert()
		So(err, ShouldBeNil)
		distroId := d.Id
		Convey("With a set of tasks that have different scheduled -> start times over a given time period", func() {
			now := time.Now()
			bucketSize := 10 * time.Second
			numberBuckets := 3

			task1 := task.Task{Id: "task1", ScheduledTime: now,
				StartTime: now.Add(time.Duration(5) * time.Second), Status: evergreen.TaskStarted, DistroId: distroId}

			So(task1.Insert(), ShouldBeNil)

			task2 := task.Task{Id: "task2", ScheduledTime: now,
				StartTime: now.Add(time.Duration(20) * time.Second), Status: evergreen.TaskStarted, DistroId: distroId}

			So(task2.Insert(), ShouldBeNil)

			task3 := task.Task{Id: "task3", ScheduledTime: now.Add(time.Duration(10) * time.Second),
				StartTime: now.Add(time.Duration(20) * time.Second), Status: evergreen.TaskStarted, DistroId: distroId}
			So(task3.Insert(), ShouldBeNil)

			frameBounds := FrameBounds{
				StartTime:     now,
				EndTime:       now.Add(time.Duration(numberBuckets) * bucketSize),
				NumberBuckets: numberBuckets,
				BucketSize:    bucketSize,
			}
			avgBuckets, err := AverageStatistics(distroId, frameBounds)
			So(err, ShouldBeNil)

			So(avgBuckets[0].AverageTime, ShouldEqual, 5*time.Second)
			So(avgBuckets[1].AverageTime, ShouldEqual, 0)
			So(avgBuckets[2].AverageTime, ShouldEqual, 15*time.Second)

			Convey("if the distro id given does not exist, it shoud return an empty list", func() {
				_, err := AverageStatistics("noId", frameBounds)
				So(err, ShouldNotBeNil)
			})
		})
	})

}

func TestFindPredictedMakespan(t *testing.T) {
	Convey("With a simple set of tasks that are dependent on each other and different times taken", t, func() {

		a := task.Task{Id: "a", TimeTaken: time.Duration(5) * time.Second, DependsOn: []task.Dependency{}}
		b := task.Task{Id: "b", TimeTaken: time.Duration(3) * time.Second, DependsOn: []task.Dependency{{"a", evergreen.TaskFailed}}}
		c := task.Task{Id: "c", TimeTaken: time.Duration(4) * time.Second, DependsOn: []task.Dependency{{"a", evergreen.TaskFailed}}}
		f := task.Task{Id: "f", TimeTaken: time.Duration(40) * time.Second, DependsOn: []task.Dependency{{"b", evergreen.TaskFailed}}}

		d := task.Task{Id: "d", TimeTaken: time.Duration(10) * time.Second}
		e := task.Task{Id: "e", TimeTaken: time.Duration(5) * time.Second, DependsOn: []task.Dependency{{"d", evergreen.TaskFailed}}}

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
