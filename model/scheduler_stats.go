package model

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
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
