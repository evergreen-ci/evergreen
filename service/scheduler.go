package service

import (
	"fmt"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"net/http"
	"time"
)

// UIBucket represents an aggregate view of the hosts and tasks Bucket for a given time frame.
type UIBucket struct {
	StaticHost  time.Duration `json:"static_host"`
	DynamicHost time.Duration `json:"dynamic_host"`
	Task        time.Duration `json:"task"`
	StartTime   time.Time     `json:"start_time"`
	EndTime     time.Time     `json:"end_time"`
}

func (uis *UIServer) getSchedulerLogs(w http.ResponseWriter, r *http.Request) {
	distroId := mux.Vars(r)["distro_id"]

	projCtx := MustHaveProjectContext(r)

	loggedEvents, err := event.Find(event.RecentSchedulerEvents(distroId, 500))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	uis.WriteHTML(w, http.StatusOK, struct {
		ProjectData projectContext
		User        *user.DBUser
		Data        []event.Event
		DistroId    string
	}{projCtx, GetUser(r), loggedEvents, distroId}, "base", "scheduler_events.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) schedulerStatsPage(w http.ResponseWriter, r *http.Request) {

	projCtx := MustHaveProjectContext(r)

	uis.WriteHTML(w, http.StatusOK, struct {
		ProjectData projectContext
		User        *user.DBUser
	}{projCtx, GetUser(r)}, "base", "scheduler_stats.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) schedulerHostUtilization(w http.ResponseWriter, r *http.Request) {

	// get granularity (in seconds)
	granularity, err := util.GetIntValue(r, "granularity", 0)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	if granularity == 0 {
		uis.LoggedError(w, r, http.StatusBadRequest, fmt.Errorf("Invalid granularity"))
		return
	}

	// get number of days back
	daysBack, err := util.GetIntValue(r, "numberDays", 0)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	if daysBack == 0 {
		uis.LoggedError(w, r, http.StatusBadRequest, fmt.Errorf("Invalid days back"))
		return
	}

	endTime := time.Now()
	totalTime := 24 * time.Hour * time.Duration(daysBack)
	startTime := endTime.Add(-1 * totalTime)

	numberBuckets := (time.Duration(daysBack) * time.Hour * 24) / (time.Duration(granularity) * time.Second)
	// find non-static hosts
	dynamicHosts, err := host.Find(host.ByDynamicWithinTime(startTime, endTime))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// find static hosts
	staticHosts, err := host.Find(host.AllStatic)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	bucketSize := time.Duration(granularity) * time.Second

	dynamicBuckets, _ := model.CreateHostBuckets(dynamicHosts, startTime, numberBuckets, bucketSize)
	staticBuckets, _ := model.CreateHostBuckets(staticHosts, startTime, numberBuckets, bucketSize)

	tasks, err := task.Find(task.ByTimeRun(startTime, endTime).WithFields(task.StartTimeKey, task.FinishTimeKey, task.HostIdKey))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	oldTasks, err := task.FindOld(task.ByTimeRun(startTime, endTime))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	taskBuckets, _ := model.CreateTaskBuckets(tasks, oldTasks, startTime, numberBuckets, bucketSize)
	bucketData := []UIBucket{}
	for i := 0; i < len(staticBuckets); i++ {
		b := UIBucket{
			StaticHost:  staticBuckets[i].TotalTime,
			DynamicHost: dynamicBuckets[i].TotalTime,
			Task:        taskBuckets[i].TotalTime,
			StartTime:   startTime.Add(time.Duration(i) * bucketSize),
			EndTime:     startTime.Add(time.Duration(i+1) * bucketSize),
		}
		bucketData = append(bucketData, b)

	}

	uis.WriteJSON(w, http.StatusOK, bucketData)
	return

}
