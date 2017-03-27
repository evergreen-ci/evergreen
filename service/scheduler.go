package service

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

func (uis *UIServer) getSchedulerPage(w http.ResponseWriter, r *http.Request) {
	distroId := mux.Vars(r)["distro_id"]
	projCtx := MustHaveProjectContext(r)

	uis.WriteHTML(w, http.StatusOK, struct {
		ProjectData projectContext
		User        *user.DBUser
		DistroId    string
	}{projCtx, GetUser(r), distroId}, "base", "scheduler_events.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) getSchedulerLogs(w http.ResponseWriter, r *http.Request) {
	distroId := mux.Vars(r)["distro_id"]

	loggedEvents, err := event.Find(event.AllLogCollection, event.RecentSchedulerEvents(distroId, 500))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	uis.WriteJSON(w, http.StatusOK, loggedEvents)
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
		uis.LoggedError(w, r, http.StatusBadRequest, errors.New("Invalid granularity"))
		return
	}

	// get number of days back
	daysBack, err := util.GetIntValue(r, "numberDays", 0)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	if daysBack == 0 {
		uis.LoggedError(w, r, http.StatusBadRequest, errors.New("Invalid days back"))
		return
	}

	bucketData, err := model.CreateAllHostUtilizationBuckets(daysBack, granularity)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	uis.WriteJSON(w, http.StatusOK, bucketData)

	return

}

func (uis *UIServer) averageSchedulerStats(w http.ResponseWriter, r *http.Request) {
	// get granularity (in seconds)
	granularity, err := util.GetIntValue(r, "granularity", 0)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	if granularity == 0 {
		uis.LoggedError(w, r, http.StatusBadRequest, errors.New("Invalid granularity"))
		return
	}

	// get number of days back
	daysBack, err := util.GetIntValue(r, "numberDays", 0)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	if daysBack == 0 {
		uis.LoggedError(w, r, http.StatusBadRequest, errors.New("Invalid days back"))
		return
	}

	distroId := mux.Vars(r)["distro_id"]

	bounds := model.CalculateBounds(daysBack, granularity)

	avgBuckets, err := model.AverageStatistics(distroId, bounds)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	uis.WriteJSON(w, http.StatusOK, avgBuckets)
}
