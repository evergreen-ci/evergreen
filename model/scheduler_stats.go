package model

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
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
	Id          int           `bson:"_id" json:"index"`
	AverageTime time.Duration `bson:"a" json:"avg"`
	NumberTasks int           `bson:"n" json:"number_tasks"`
	Start       time.Time     `json:"start_time"`
	End         time.Time     `json:"end_time"`
}

type AvgBuckets []AvgBucket

func addBucketTime(duration time.Duration, resource ResourceInfo, bucket Bucket) Bucket {
	return Bucket{
		TotalTime: bucket.TotalTime + duration,
		Resources: append(bucket.Resources, resource),
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
		return currentBuckets, fmt.Errorf("invalid resource start time %v that is after the time frame %v", start, frameEnd)
	}

	if util.IsZeroTime(start) {
		return currentBuckets, fmt.Errorf("start time is zero")
	}

	if !util.IsZeroTime(end) && (end.Before(frameStart) || end.Equal(frameStart)) {
		return currentBuckets, fmt.Errorf("invalid resource end time, %v that is before the time frame, %v", end, frameStart)
	}

	if !util.IsZeroTime(end) && end.Before(start) {
		return currentBuckets, fmt.Errorf("termination time, %v is before start time, %v and exists", end, start)
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
func CreateHostBuckets(hosts []host.Host, startTime time.Time, numberBuckets, bucketSize time.Duration) ([]Bucket, []error) {
	hostBuckets := make([]Bucket, numberBuckets)
	var err error
	endTime := startTime.Add(bucketSize * numberBuckets)
	errors := []error{}
	for _, h := range hosts {
		hostResource := ResourceInfo{
			Id:    h.Id,
			Start: h.CreationTime,
			End:   h.TerminationTime,
		}

		// static hosts
		if h.Provider == evergreen.HostTypeStatic {
			for i, b := range hostBuckets {
				hostBuckets[i] = addBucketTime(bucketSize, hostResource, b)
			}
			continue
		}
		hostBuckets, err = bucketResource(hostResource, startTime, endTime, bucketSize, hostBuckets)
		if err != nil {
			errors = append(errors, fmt.Errorf("error bucketing host %v : %v", h.Id, err))
		}
	}
	return hostBuckets, errors
}

// CreateTaskBuckets takes in a list of tasks with their start and finish times
// and returns durations bucketed based on  a start time, number of buckets and  the size of each bucket
func CreateTaskBuckets(tasks []task.Task, oldTasks []task.Task, startTime time.Time, numberBuckets, bucketSize time.Duration) ([]Bucket, []error) {
	taskBuckets := make([]Bucket, numberBuckets)
	endTime := startTime.Add(bucketSize * numberBuckets)
	var err error
	errors := []error{}
	for _, t := range tasks {
		taskResource := ResourceInfo{
			Id:    t.Id,
			Start: t.StartTime,
			End:   t.FinishTime,
			Data:  t.HostId,
		}
		taskBuckets, err = bucketResource(taskResource, startTime, endTime, bucketSize, taskBuckets)
		if err != nil {
			errors = append(errors, fmt.Errorf("error bucketing task %v : %v", t.Id, err))
		}
	}

	for _, t := range oldTasks {
		taskResource := ResourceInfo{
			Id:    t.Id,
			Start: t.StartTime,
			End:   t.FinishTime,
			Data:  t.HostId,
		}
		taskBuckets, err = bucketResource(taskResource, startTime, endTime, bucketSize, taskBuckets)
		if err != nil {
			errors = append(errors, fmt.Errorf("error bucketing task %v : %v", t.Id, err))
		}
	}
	return taskBuckets, errors
}

// AverageStatistics uses an agg pipeline that creates buckets given a time frame and finds the average scheduled ->
// start time for that time frame.
// One thing to note is that the average time is in milliseconds, not nanoseconds and must be converted.
func AverageStatistics(startTime time.Time, numberBuckets int, bucketSize time.Duration, distroId string) (AvgBuckets, error) {
	numBucketsDuration := time.Duration(numberBuckets)
	endTime := startTime.Add(numBucketsDuration * bucketSize)
	intBucketSize := util.FromNanoseconds(bucketSize)
	buckets := AvgBuckets{}
	pipeline := []bson.M{
		// find all tasks that have started within the time frame for a given distro and only valid statuses.
		{"$match": bson.M{
			task.StartTimeKey: bson.M{
				"$gte": startTime,
				"$lte": endTime,
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
						bson.M{"$subtract": []interface{}{"$" + task.StartTimeKey, startTime}},
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
	return convertBucketsToNanoseconds(buckets, numberBuckets, bucketSize, startTime), nil
}

// convertBucketsToNanoseconds fills in 0 time buckets to the list of Average Buckets
// and it converts the average times to nanoseconds.
func convertBucketsToNanoseconds(buckets AvgBuckets, numberBuckets int, bucketSize time.Duration, frameStart time.Time) AvgBuckets {
	allBuckets := AvgBuckets{}
	for i := 0; i < numberBuckets; i++ {
		startTime := frameStart.Add(time.Duration(i) * bucketSize)
		endTime := startTime.Add(bucketSize)
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
