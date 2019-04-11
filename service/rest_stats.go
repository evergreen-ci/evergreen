package service

import (
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
)

// restHostUtilizationBucket represents an aggregate view of the hosts and tasks Bucket for a given time frame.
type restHostUtilizationBucket struct {
	StaticHost  int       `json:"static_host" csv:"static_host"`
	DynamicHost int       `json:"dynamic_host" csv:"dynamic_host"`
	Task        int       `json:"task" csv:"task"`
	StartTime   time.Time `json:"start_time" csv:"start_time"`
	EndTime     time.Time `json:"end_time" csv:"end_time"`
}

// restAvgBucketW is one element in the results of a list of buckets that are created from the agg query.
type restAvgBucket struct {
	Id          int       `json:"index" csv:"index"`
	AverageTime int       `json:"avg" csv:"avg_time"`
	NumberTasks int       `json:"number_tasks" csv:"number_tasks"`
	Start       time.Time `json:"start_time" csv:"start_time"`
	End         time.Time `json:"end_time" csv:"end_time"`
}

// restMakespanStats represents the actual and predicted makespan for a given build
type restMakespanStats struct {
	ActualMakespan    int    `json:"actual" csv:"actual"`
	PredictedMakespan int    `json:"predicted" csv:"predicted"`
	BuildId           string `json:"build_id" csv:"build_id"`
}

// getMakespanRatios returns a list of MakespanRatio structs that contain
// the actual and predicted makespans for a certain number of recent builds.
func getMakespanRatios(numberBuilds int) ([]restMakespanStats, error) {
	builds, err := build.Find(build.ByRecentlyFinishedWithMakespans(numberBuilds))
	if err != nil {
		return nil, err
	}
	makespanRatios := []restMakespanStats{}
	for _, b := range builds {
		makespanRatios = append(makespanRatios, restMakespanStats{
			BuildId:           b.Id,
			PredictedMakespan: int(b.PredictedMakespan),
			ActualMakespan:    int(b.ActualMakespan),
		})
	}
	return makespanRatios, nil
}

func (restapi *restAPI) getHostUtilizationStats(w http.ResponseWriter, r *http.Request) {
	// get granularity (in seconds)
	granularity, err := util.GetIntValue(r, "granularity", 0)
	if err != nil {
		gimlet.WriteJSONError(w, responseError{Message: err.Error()})
		return
	}
	if granularity == 0 {
		gimlet.WriteJSONError(w, responseError{Message: "invalid granularity"})
		return
	}

	// get number of days back
	daysBack, err := util.GetIntValue(r, "numberDays", 0)
	if err != nil {
		gimlet.WriteJSONError(w, responseError{Message: err.Error()})
		return
	}
	if daysBack == 0 {
		gimlet.WriteJSONError(w, responseError{Message: "invalid days back"})
		return
	}

	isCSV, err := util.GetBoolValue(r, "csv", true)
	if err != nil {
		gimlet.WriteJSONError(w, responseError{Message: err.Error()})
		return
	}
	buckets, err := model.CreateAllHostUtilizationBuckets(daysBack, granularity)
	if err != nil {
		gimlet.WriteJSONInternalError(w, responseError{Message: fmt.Sprintf("error getting buckets: %v", err.Error())})
		return
	}
	restBuckets := []restHostUtilizationBucket{}
	// convert the time.Durations into integers
	for _, b := range buckets {
		r := restHostUtilizationBucket{
			StaticHost:  int(b.StaticHost),
			DynamicHost: int(b.DynamicHost),
			Task:        int(b.Task),
			StartTime:   b.StartTime,
			EndTime:     b.EndTime,
		}
		restBuckets = append(restBuckets, r)
	}

	if isCSV {
		util.WriteCSVResponse(w, http.StatusOK, restBuckets)
		return
	}
	gimlet.WriteJSON(w, buckets)
}

func (restapi *restAPI) getAverageSchedulerStats(w http.ResponseWriter, r *http.Request) {
	// get granularity (in seconds)
	granularity, err := util.GetIntValue(r, "granularity", 0)
	if err != nil {
		gimlet.WriteJSONError(w, responseError{Message: err.Error()})
		return
	}
	if granularity == 0 {
		gimlet.WriteJSONError(w, responseError{Message: "invalid granularity"})
		return
	}

	// get number of days back
	daysBack, err := util.GetIntValue(r, "numberDays", 0)
	if err != nil {
		gimlet.WriteJSONError(w, responseError{Message: err.Error()})
		return
	}
	if daysBack == 0 {
		gimlet.WriteJSONError(w, responseError{Message: "invalid days back"})
		return
	}
	isCSV, err := util.GetBoolValue(r, "csv", true)
	if err != nil {
		gimlet.WriteJSONError(w, responseError{Message: err.Error()})
		return
	}

	distroId := gimlet.GetVars(r)["distro_id"]
	if distroId == "" {
		gimlet.WriteJSONError(w, responseError{Message: "invalid distro id"})
		return
	}

	bounds := model.CalculateBounds(daysBack, granularity)

	buckets, err := model.AverageStatistics(distroId, bounds)
	if err != nil {
		gimlet.WriteJSONInternalError(w, responseError{Message: fmt.Sprintf("error getting buckets: %v", err.Error())})
		return
	}
	restBuckets := []restAvgBucket{}
	// convert the time.Durations into integers
	for _, b := range buckets {
		r := restAvgBucket{
			Id:          b.Id,
			AverageTime: int(b.AverageTime),
			NumberTasks: b.NumberTasks,
			Start:       b.Start,
			End:         b.End,
		}
		restBuckets = append(restBuckets, r)
	}

	if isCSV {
		util.WriteCSVResponse(w, http.StatusOK, restBuckets)
		return
	}
	gimlet.WriteJSON(w, buckets)

}

func (restapi *restAPI) getOptimalAndActualMakespans(w http.ResponseWriter, r *http.Request) {
	// get number of days back
	numberBuilds, err := util.GetIntValue(r, "number", 0)
	if err != nil {
		gimlet.WriteJSONError(w, responseError{Message: err.Error()})
		return
	}
	if numberBuilds == 0 {
		gimlet.WriteJSONError(w, responseError{Message: "invalid number builds"})
		return
	}

	isCSV, err := util.GetBoolValue(r, "csv", true)
	if err != nil {
		gimlet.WriteJSONError(w, responseError{Message: err.Error()})
		return
	}

	makespanData, err := getMakespanRatios(numberBuilds)
	if err != nil {
		gimlet.WriteJSONError(w, responseError{Message: err.Error()})
		return
	}

	if isCSV {
		util.WriteCSVResponse(w, http.StatusOK, makespanData)
		return
	}
	gimlet.WriteJSON(w, makespanData)

}
