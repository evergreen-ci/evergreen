package model

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

// ResourceInfo contains the meta data about a given resource
// This includes the id of the resource, the overall start and finish time and any
// extra data that should be included about the resource.
type ResourceInfo struct {
	Id    string      `json:"id"`
	Start time.Time   `json:"start"`
	End   time.Time   `json:"end"`
	Data  interface{} `json:"data"`
}

// Bucket stores the total amount of time in a given bucket and a list of the resources that are in that bucket
type Bucket struct {
	TotalTime time.Duration  `json:"total_time"`
	Resources []ResourceInfo `json:"resources"`
}

// AvgBucket is one element in the results of a list of buckets that are created from the agg query.
type AvgBucket struct {
	Id          int           `bson:"_id" json:"index" csv:"index"`
	AverageTime time.Duration `bson:"a" json:"avg" csv:"avg_time"`
	NumberTasks int           `bson:"n" json:"number_tasks" csv:"number_tasks"`
	Start       time.Time     `json:"start_time" csv:"start_time"`
	End         time.Time     `json:"end_time" csv:"end_time"`
}

// FrameBounds is a set of information about the inputs of buckets
type FrameBounds struct {
	StartTime     time.Time
	EndTime       time.Time
	BucketSize    time.Duration
	NumberBuckets int
}

// HostUtilizationBucket represents an aggregate view of the hosts and tasks Bucket for a given time frame.
type HostUtilizationBucket struct {
	StaticHost  time.Duration `json:"static_host" csv:"static_host"`
	DynamicHost time.Duration `json:"dynamic_host" csv:"dynamic_host"`
	Task        time.Duration `json:"task" csv:"task"`
	StartTime   time.Time     `json:"start_time" csv:"start_time"`
	EndTime     time.Time     `json:"end_time" csv:"end_time"`
}

type AvgBuckets []AvgBucket

// AverageTimeByRequester is the average time of a task.
type AverageTimeByRequester struct {
	Requester   string        `bson:"_id"`
	AverageTime time.Duration `bson:"average_time"`
}

// dependencyPath represents the path of tasks that can
// occur by taking one from each layer of the dependencies
// TotalTime is the sum of all task's time taken to run that are in Tasks.
type dependencyPath struct {
	TaskId    string
	TotalTime time.Duration
	Tasks     []string
}

func addBucketTime(duration time.Duration, resource ResourceInfo, bucket Bucket) Bucket {
	return Bucket{
		TotalTime: bucket.TotalTime + duration,
		Resources: append(bucket.Resources, resource),
	}
}

// CalculateBounds takes in a daysBack and granularity and returns the
// start time, end time, bucket size, and number of buckets
func CalculateBounds(daysBack, granularity int) FrameBounds {
	endTime := time.Now()
	totalTime := 24 * time.Hour * time.Duration(daysBack)
	startTime := endTime.Add(-1 * totalTime)

	bucketSize := time.Duration(granularity) * time.Second

	numberBuckets := (time.Duration(daysBack) * time.Hour * 24) / (time.Duration(granularity) * time.Second)

	return FrameBounds{
		StartTime:     startTime,
		EndTime:       endTime,
		BucketSize:    bucketSize,
		NumberBuckets: int(numberBuckets),
	}
}

// bucketResource buckets amounts of time based on a number of buckets and the size of them.
// Given a resource with a start and end time, where the end >= start,
// a time frame with a frameStart and frameEnd, where frameEnd >= frameStart,
// a bucketSize that represents the amount of time each bucket holds
// and a list of buckets that may already have time in them,
// BucketResource will split the time and add the time the corresponds to a given buck to that bucket.
func bucketResource(resource ResourceInfo, frameStart, frameEnd time.Time, bucketSize time.Duration,
	currentBuckets []Bucket) ([]Bucket, error) {

	start := resource.Start
	end := resource.End
	// double check so that there are no panics
	if start.After(frameEnd) || start.Equal(frameEnd) {
		return currentBuckets, errors.Errorf("invalid resource start time %v that is after the time frame %v", start, frameEnd)
	}

	if util.IsZeroTime(start) {
		return currentBuckets, errors.New("start time is zero")
	}

	if !util.IsZeroTime(end) && (end.Before(frameStart) || end.Equal(frameStart)) {
		return currentBuckets, errors.Errorf("invalid resource end time, %v that is before the time frame, %v", end, frameStart)
	}

	if !util.IsZeroTime(end) && end.Before(start) {
		return currentBuckets, errors.Errorf("termination time, %v is before start time, %v and exists", end, start)
	}

	// if the times are equal then just return since nothing should be bucketed
	if end.Equal(start) {
		return currentBuckets, nil
	}

	// If the resource starts before the beginning of the frame,
	// the startBucket is the first one. The startOffset is the offset
	// of time from the beginning of the start bucket, so that is  0.
	startOffset := time.Duration(0)
	startBucket := time.Duration(0)
	if start.After(frameStart) {
		startOffset = start.Sub(frameStart)
		startBucket = startOffset / bucketSize
	}
	// If the resource ends after the end of the frame, the end bucket is the last bucket
	// the end offset is the entirety of that bucket.
	endBucket := time.Duration(len(currentBuckets) - 1)
	endOffset := bucketSize * (endBucket + 1)

	if !(util.IsZeroTime(end) || end.After(frameEnd) || end.Equal(frameEnd)) {
		endOffset = end.Sub(frameStart)
		endBucket = endOffset / bucketSize
	}

	// If the startBucket and the endBucket are the same, that means there is only one bucket.
	// The amount that goes in that bucket is the difference in the resources start time and end time.
	if startBucket == endBucket {
		currentBuckets[startBucket] = addBucketTime(endOffset-startOffset, resource, currentBuckets[startBucket])
		return currentBuckets, nil

	} else {
		// add the difference between the startOffset and the amount of time that has passed in the start and end bucket
		// to the start and end buckets.
		currentBuckets[startBucket] = addBucketTime((startBucket+1)*bucketSize-startOffset, resource, currentBuckets[startBucket])
		currentBuckets[endBucket] = addBucketTime(endOffset-endBucket*bucketSize, resource, currentBuckets[endBucket])
	}
	for i := startBucket + 1; i < endBucket; i++ {
		currentBuckets[i] = addBucketTime(bucketSize, resource, currentBuckets[i])
	}
	return currentBuckets, nil
}

// CreateHostBuckets takes in a list of hosts with their creation and termination times
// and returns durations bucketed based on a start time, number of buckets and the size of each bucket
func CreateHostBuckets(hosts []host.Host, bounds FrameBounds) ([]Bucket, []error) {
	hostBuckets := make([]Bucket, bounds.NumberBuckets)
	var err error
	errs := []error{}
	for _, h := range hosts {
		hostResource := ResourceInfo{
			Id:    h.Id,
			Start: h.CreationTime,
			End:   h.TerminationTime,
		}

		// static hosts
		if h.Provider == evergreen.HostTypeStatic {
			for i, b := range hostBuckets {
				hostBuckets[i] = addBucketTime(bounds.BucketSize, hostResource, b)
			}
			continue
		}
		hostBuckets, err = bucketResource(hostResource, bounds.StartTime, bounds.EndTime, bounds.BucketSize, hostBuckets)
		if err != nil {
			errs = append(errs, errors.Wrapf(err, "error bucketing host %s", h.Id))
		}
	}
	return hostBuckets, errs
}

// CreateTaskBuckets takes in a list of tasks with their start and finish times
// and returns durations bucketed based on  a start time, number of buckets and  the size of each bucket
func CreateTaskBuckets(tasks []task.Task, oldTasks []task.Task, bounds FrameBounds) ([]Bucket, []error) {
	taskBuckets := make([]Bucket, bounds.NumberBuckets)
	var err error
	errs := []error{}
	for _, t := range tasks {
		taskResource := ResourceInfo{
			Id:    t.Id,
			Start: t.StartTime,
			End:   t.FinishTime,
			Data:  t.HostId,
		}
		taskBuckets, err = bucketResource(taskResource, bounds.StartTime, bounds.EndTime, bounds.BucketSize, taskBuckets)
		if err != nil {
			errs = append(errs, errors.Wrapf(err, "error bucketing task %v", t.Id))
		}
	}

	for _, t := range oldTasks {
		taskResource := ResourceInfo{
			Id:    t.Id,
			Start: t.StartTime,
			End:   t.FinishTime,
			Data:  t.HostId,
		}
		taskBuckets, err = bucketResource(taskResource, bounds.StartTime, bounds.EndTime, bounds.BucketSize, taskBuckets)
		if err != nil {
			errs = append(errs, errors.Wrapf(err, "error bucketing task %v", t.Id))
		}
	}
	return taskBuckets, errs
}

// CreateAllHostUtilizationBuckets aggregates each bucket by creating a time frame given the number of days back
// and the granularity wanted (ie. days, minutes, seconds, hours) all in seconds. It returns a list of Host utilization
// information for each bucket.
func CreateAllHostUtilizationBuckets(daysBack, granularity int) ([]HostUtilizationBucket, error) {
	bounds := CalculateBounds(daysBack, granularity)
	// find non-static hosts
	dynamicHosts, err := host.Find(host.ByDynamicWithinTime(bounds.StartTime, bounds.EndTime))
	if err != nil {
		return nil, err
	}
	// find static hosts
	staticHosts, err := host.Find(host.AllStatic)
	if err != nil {
		return nil, err
	}

	dynamicBuckets, _ := CreateHostBuckets(dynamicHosts, bounds)
	staticBuckets, _ := CreateHostBuckets(staticHosts, bounds)

	tasks, err := task.Find(task.ByTimeRun(bounds.StartTime, bounds.EndTime).WithFields(task.StartTimeKey, task.FinishTimeKey, task.HostIdKey))
	if err != nil {
		return nil, err
	}

	oldTasks, err := task.FindOld(task.ByTimeRun(bounds.StartTime, bounds.EndTime))
	if err != nil {
		return nil, err
	}

	taskBuckets, _ := CreateTaskBuckets(tasks, oldTasks, bounds)
	bucketData := []HostUtilizationBucket{}
	for i, staticBucket := range staticBuckets {
		b := HostUtilizationBucket{
			StaticHost:  staticBucket.TotalTime,
			DynamicHost: dynamicBuckets[i].TotalTime,
			Task:        taskBuckets[i].TotalTime,
			StartTime:   bounds.StartTime.Add(time.Duration(i) * bounds.BucketSize),
			EndTime:     bounds.StartTime.Add(time.Duration(i+1) * bounds.BucketSize),
		}
		bucketData = append(bucketData, b)

	}
	return bucketData, nil
}

// AverageStatistics uses an agg pipeline that creates buckets given a time frame and finds the average scheduled ->
// start time for that time frame.
// One thing to note is that the average time is in milliseconds, not nanoseconds and must be converted.
func AverageStatistics(distroId string, bounds FrameBounds) (AvgBuckets, error) {

	// error out if the distro does not exist
	_, err := distro.FindOne(distro.ById(distroId))
	if err != nil {
		return nil, err
	}
	intBucketSize := util.FromNanoseconds(bounds.BucketSize)
	buckets := AvgBuckets{}
	pipeline := []bson.M{
		// find all tasks that have started within the time frame for a given distro and only valid statuses.
		{"$match": bson.M{
			task.StartTimeKey: bson.M{
				"$gte": bounds.StartTime,
				"$lte": bounds.EndTime,
			},
			// only need tasks that have already started or those that have finished,
			// not looking for tasks that have been scheduled but not started.
			task.StatusKey: bson.M{
				"$in": []string{evergreen.TaskStarted,
					evergreen.TaskFailed, evergreen.TaskSucceeded},
			},
			task.DistroIdKey: distroId,
		}},
		// project the difference in scheduled -> start, as well as the bucket
		{"$project": bson.M{
			"diff": bson.M{
				"$subtract": []interface{}{"$" + task.StartTimeKey, "$" + task.ScheduledTimeKey},
			},
			"b": bson.M{
				"$floor": bson.M{
					"$divide": []interface{}{
						bson.M{"$subtract": []interface{}{"$" + task.StartTimeKey, bounds.StartTime}},
						intBucketSize},
				},
			},
		}},
		{"$group": bson.M{
			"_id": "$b",
			"a":   bson.M{"$avg": "$diff"},
			"n":   bson.M{"$sum": 1},
		}},

		{"$sort": bson.M{
			"_id": 1,
		}},
	}

	if err := db.Aggregate(task.Collection, pipeline, &buckets); err != nil {
		return nil, err
	}
	return convertBucketsToNanoseconds(buckets, bounds), nil
}

// AverageTaskLatencyByDistro finds the average task latency grouped by requester.
func AverageTaskLatencyByDistro(distroId string, since time.Duration) (map[string]time.Duration, error) {
	now := time.Now()
	pipeline := []bson.M{
		{"$match": bson.M{
			task.StartTimeKey: bson.M{
				"$gte": now.Add(-since),
				"$lte": now,
			},
			task.StatusKey: bson.M{
				"$in": []string{
					evergreen.TaskStarted,
					evergreen.TaskFailed,
					evergreen.TaskSucceeded},
			},
			task.DistroIdKey: distroId,
		}},
		{"$group": bson.M{
			"_id": "$" + task.RequesterKey,
			"average_time": bson.M{
				"$avg": bson.M{
					"$subtract": []interface{}{"$" + task.StartTimeKey, "$" + task.ScheduledTimeKey},
				},
			},
		}},
	}

	stats := []AverageTimeByRequester{}
	if err := db.Aggregate(task.Collection, pipeline, &stats); err != nil {
		return nil, errors.Wrap(err, "error running average task latency aggregation")
	}
	m := map[string]time.Duration{}
	for _, t := range stats {
		// multiply by time.Millisecond/time.Nanosecond to convert from MongoDB to Golang times
		m[t.Requester] = t.AverageTime * (time.Millisecond / time.Nanosecond)
	}
	return m, nil
}

// convertBucketsToNanoseconds fills in 0 time buckets to the list of Average Buckets
// and it converts the average times to nanoseconds.
func convertBucketsToNanoseconds(buckets AvgBuckets, bounds FrameBounds) AvgBuckets {
	allBuckets := AvgBuckets{}
	for i := 0; i < bounds.NumberBuckets; i++ {
		startTime := bounds.StartTime.Add(time.Duration(i) * bounds.BucketSize)
		endTime := bounds.StartTime.Add(bounds.BucketSize)
		currentBucket := AvgBucket{
			Id:          i,
			AverageTime: 0,
			NumberTasks: 0,
			Start:       startTime,
			End:         endTime,
		}
		for j := 0; j < len(buckets); j++ {
			if buckets[j].Id == i {
				currentBucket.AverageTime = util.ToNanoseconds(buckets[j].AverageTime)
				currentBucket.NumberTasks = buckets[j].NumberTasks
				break
			}
		}
		allBuckets = append(allBuckets, currentBucket)
	}
	return allBuckets
}

// CalculateActualMakespan finds the amount of time it took for the build to complete from
// the first task start to the last task finishing.
func CalculateActualMakespan(tasks []task.Task) time.Duration {
	// find the minimum start time and the maximum finish time and take the difference
	if len(tasks) == 0 {
		return time.Duration(0)
	}

	minStart := tasks[0].StartTime
	maxFinish := tasks[0].FinishTime

	for _, t := range tasks {
		if t.StartTime.Before(minStart) {
			minStart = t.StartTime
		}
		if t.FinishTime.After(maxFinish) {
			maxFinish = t.FinishTime
		}
	}
	return maxFinish.Sub(minStart)
}

// hasTaskId returns true if the dependency list has the task
func hasTaskId(taskId string, dependsOn []task.Dependency) bool {
	for _, d := range dependsOn {
		if d.TaskId == taskId {
			return true
		}
	}
	return false
}

// getMaxDependencyPath recursively traverses a task's dependencies to get the dependency path object with the maximum
// total time.
func getMaxDependencyPath(tasks []task.Task, depPath dependencyPath) dependencyPath {
	maxDepPath := depPath
	maxTime := time.Duration(0)
	// find tasks that depend on the current task in the depPath
	for _, t := range tasks {
		if hasTaskId(depPath.TaskId, t.DependsOn) {
			newDepPath := dependencyPath{
				TaskId:    t.Id,
				Tasks:     append(depPath.Tasks, t.Id),
				TotalTime: depPath.TotalTime + t.TimeTaken,
			}
			newDepPath = getMaxDependencyPath(tasks, newDepPath)
			if newDepPath.TotalTime > maxTime {
				maxTime = newDepPath.TotalTime
				maxDepPath = newDepPath
			}
		}
	}
	return maxDepPath
}

// FindPredictedMakespan, given a list of tasks that have been completed, finds the optimal makespan of that build.
func FindPredictedMakespan(tasks []task.Task) dependencyPath {
	maxTime := time.Duration(0)
	var maxDepPath dependencyPath

	for _, t := range tasks {
		if len(t.DependsOn) == 0 {
			depPath := dependencyPath{
				TaskId:    t.Id,
				Tasks:     []string{t.Id},
				TotalTime: t.TimeTaken,
			}
			fullDepPath := getMaxDependencyPath(tasks, depPath)
			if fullDepPath.TotalTime > maxTime {
				maxTime = fullDepPath.TotalTime
				maxDepPath = fullDepPath
			}
		}
	}
	return maxDepPath
}
