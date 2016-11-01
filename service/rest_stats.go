package service

import (
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
)

// restHostUtilizationBucket represents an aggregate view of the hosts and tasks Bucket for a given time frame.
type restHostUtilizationBucket struct {
	StaticHost  int       `json:"static_host" csv:"static_host"`
	DynamicHost int       `json:"dynamic_host" csv:"dynamic_host"`
	Task        int       `json:"task" csv:"task"`
	StartTime   time.Time `json:"start_time" csv:"start_time"`
	EndTime     time.Time `json:"end_time" csv:"end_time"`
}

// AvgBucket is one element in the results of a list of buckets that are created from the agg query.
type restAvgBucket struct {
	Id          int       `json:"index" csv:"index"`
	AverageTime int       `json:"avg" csv:"avg_time"`
	NumberTasks int       `json:"number_tasks" csv:"number_tasks"`
	Start       time.Time `json:"start_time" csv:"start_time"`
	End         time.Time `json:"end_time" csv:"end_time"`
}

func (restapi *restAPI) getHostUtilizationStats(w http.ResponseWriter, r *http.Request) {
	// get granularity (in seconds)
	granularity, err := util.GetIntValue(r, "granularity", 0)
	if err != nil {
		restapi.WriteJSON(w, http.StatusBadRequest, responseError{Message: err.Error()})
		return
	}
	if granularity == 0 {
		restapi.WriteJSON(w, http.StatusBadRequest, responseError{Message: "invalid granularity"})
		return
	}

	// get number of days back
	daysBack, err := util.GetIntValue(r, "numberDays", 0)
	if err != nil {
		restapi.WriteJSON(w, http.StatusBadRequest, responseError{Message: err.Error()})
		return
	}
	if daysBack == 0 {
		restapi.WriteJSON(w, http.StatusBadRequest, responseError{Message: "invalid days back"})
		return
	}

	isCSV, err := util.GetBoolValue(r, "csv", true)
	if err != nil {
		restapi.WriteJSON(w, http.StatusBadRequest, responseError{Message: err.Error()})
		return
	}
	buckets, err := model.CreateAllHostUtilizationBuckets(daysBack, granularity)
	if err != nil {
		restapi.WriteJSON(w, http.StatusInternalServerError, responseError{Message: fmt.Sprintf("error getting buckets: %v", err.Error())})
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
		util.WriteCSVResponse(w, restBuckets)
		return
	}
	restapi.WriteJSON(w, http.StatusOK, buckets)
}

func (restapi *restAPI) getAverageSchedulerStats(w http.ResponseWriter, r *http.Request) {
	// get granularity (in seconds)
	granularity, err := util.GetIntValue(r, "granularity", 0)
	if err != nil {
		restapi.WriteJSON(w, http.StatusBadRequest, responseError{Message: err.Error()})
		return
	}
	if granularity == 0 {
		restapi.WriteJSON(w, http.StatusBadRequest, responseError{Message: "invalid granularity"})
		return
	}

	// get number of days back
	daysBack, err := util.GetIntValue(r, "numberDays", 0)
	if err != nil {
		restapi.WriteJSON(w, http.StatusBadRequest, responseError{Message: err.Error()})
		return
	}
	if daysBack == 0 {
		restapi.WriteJSON(w, http.StatusBadRequest, responseError{Message: "invalid days back"})
		return
	}

	isCSV, err := util.GetBoolValue(r, "csv", true)
	if err != nil {
		restapi.WriteJSON(w, http.StatusBadRequest, responseError{Message: err.Error()})
		return
	}

	distroId := mux.Vars(r)["distro_id"]
	if distroId == "" {
		restapi.WriteJSON(w, http.StatusBadRequest, responseError{Message: "invalid distro id"})
		return
	}

	bounds := model.CalculateBounds(daysBack, granularity)

	buckets, err := model.AverageStatistics(distroId, bounds)
	if err != nil {
		restapi.WriteJSON(w, http.StatusInternalServerError, responseError{Message: fmt.Sprintf("error getting buckets: %v", err.Error())})
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
		util.WriteCSVResponse(w, restBuckets)
		return
	}
	restapi.WriteJSON(w, http.StatusOK, buckets)
}
