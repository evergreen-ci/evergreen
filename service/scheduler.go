package service

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

func (uis *UIServer) getSchedulerPage(w http.ResponseWriter, r *http.Request) {
	distroId := gimlet.GetVars(r)["distro_id"]

	uis.render.WriteResponse(w, http.StatusOK, struct {
		DistroId string
		ViewData
	}{distroId, uis.GetCommonViewData(w, r, false, true)}, "base", "scheduler_events.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) getSchedulerLogs(w http.ResponseWriter, r *http.Request) {
	distroId := gimlet.GetVars(r)["distro_id"]

	loggedEvents, err := event.Find(event.AllLogCollection, event.RecentSchedulerEvents(distroId, 500))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	gimlet.WriteJSON(w, loggedEvents)
}

func (uis *UIServer) schedulerStatsPage(w http.ResponseWriter, r *http.Request) {

	uis.render.WriteResponse(w, http.StatusOK, uis.GetCommonViewData(w, r, false, true), "base", "scheduler_stats.html", "base_angular.html", "menu.html")
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

	gimlet.WriteJSON(w, bucketData)
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

	distroId := gimlet.GetVars(r)["distro_id"]

	bounds := model.CalculateBounds(daysBack, granularity)

	avgBuckets, err := model.AverageStatistics(distroId, bounds)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	gimlet.WriteJSON(w, avgBuckets)
}
