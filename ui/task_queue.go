package ui

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"10gen.com/mci/model/host"
	"10gen.com/mci/model/patch"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/gorilla/mux"
	"labix.org/v2/mgo/bson"
	"net/http"
	"strconv"
	"time"
)

// ui version of a task queue item
type uiTaskQueueItem struct {
	Id                  string `json:"_id"`
	DisplayName         string `json:"display_name"`
	BuildVariant        string `json:"build_variant"`
	RevisionOrderNumber int    `json:"order"`
	Requester           string `json:"requester"`
	Revision            string `json:"gitspec"`
	Project             string `json:"project"`
	Version             string `json:"version"`
	Build               string `json:"build"`

	// only if it's a patch request task
	User string `json:"user,omitempty"`
}

// ui version of a task queue, for wrapping the ui versions of task queue
// items
type uiTaskQueue struct {
	Distro string            `json:"distro"`
	Queue  []uiTaskQueueItem `json:"queue"`
}

// top-level ui struct for holding information on task
// queues and host usage
type uiResourceInfo struct {
	TaskQueues     []uiTaskQueue    `json:"task_queues"`
	HostStatistics uiHostStatistics `json:"host_stats"`
}

// information on host utilization
type uiHostStatistics struct {
	IdleHosts         int `json:"idle_hosts"`
	ActiveHosts       int `json:"active_hosts"`
	ActiveStaticHosts int `json:"active_static_hosts"`
	IdleStaticHosts   int `json:"idle_static_hosts"`
}

// simple struct for returning time aggregations to the ui
type uiTaskTimeStatistic struct {
	Id   string        `json:"id"`
	Time time.Duration `json:"time"`
}

// taskTimeStatisticsHandler is a handler for task time aggretations.
// it essentially acts as a wrapper for model.AverageTaskTimeDifference
func (uis *UIServer) taskTimeStatisticsHandler(w http.ResponseWriter, r *http.Request) {
	field1 := mux.Vars(r)["field1"]
	field2 := mux.Vars(r)["field2"]
	groupyBy := mux.Vars(r)["group_by"]
	cutoffDaysAsString := mux.Vars(r)["cutoff_days"]
	cutoffDays, err := strconv.Atoi(cutoffDaysAsString)
	if err != nil {
		uis.LoggedError(w, r, http.StatusBadRequest, fmt.Errorf("Error converting cutoff_days to integer: %v", err))
		return
	}

	var cutoff time.Time
	// -1 is passed to represent "All Time", otherwise the number
	// is an amount of days to include in the aggregation
	if cutoffDays < 0 {
		cutoff = time.Unix(1, 0) // 1 more than 0 time to ignore unset time fields
	} else {
		cutoff = time.Now().Add(time.Duration(-1*cutoffDays) * time.Hour * 24)
	}

	timeMap, err := model.AverageTaskTimeDifference(field1, field2, groupyBy, cutoff)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("Error computing time stats: %v", err))
		return
	}

	var timeList []uiTaskTimeStatistic
	for id, val := range timeMap {
		timeList = append(timeList, uiTaskTimeStatistic{id, val})
	}
	uis.WriteJSON(w, http.StatusOK, timeList)
}

func (uis *UIServer) allTaskQueues(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

	taskQueues, err := model.FindAllTaskQueues()
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError,
			fmt.Errorf("Error finding task queues: %v", err))
		return
	}

	// cached map of version id to relevant patch
	cachedPatches := map[string]*patch.Patch{}

	// convert the task queues to the ui versions
	uiTaskQueues := []uiTaskQueue{}
	for _, tQ := range taskQueues {
		asUI := uiTaskQueue{
			Distro: tQ.Distro,
			Queue:  []uiTaskQueueItem{},
		}

		if len(tQ.Queue) == 0 {
			uiTaskQueues = append(uiTaskQueues, asUI)
			continue
		}

		// convert the individual task queue items
		taskIds := []string{}
		for _, item := range tQ.Queue {

			// cache the ids, for fetching the tasks from the db
			taskIds = append(taskIds, item.Id)
			queueItemAsUI := uiTaskQueueItem{
				Id:                  item.Id,
				DisplayName:         item.DisplayName,
				BuildVariant:        item.BuildVariant,
				RevisionOrderNumber: item.RevisionOrderNumber,
				Requester:           item.Requester,
				Revision:            item.Revision,
				Project:             item.Project,
			}
			asUI.Queue = append(asUI.Queue, queueItemAsUI)
		}

		// find all the relevant tasks
		tasks, err := model.FindAllTasks(
			bson.M{
				model.TaskIdKey: bson.M{
					"$in": taskIds,
				},
			},
			bson.M{
				model.TaskVersionKey: 1,
				model.TaskBuildIdKey: 1,
			},
			db.NoSort,
			db.NoSkip,
			db.NoLimit,
		)

		if err != nil {
			msg := fmt.Sprintf("Error finding tasks: %v", err)
			mci.Logger.Errorf(slogger.ERROR, msg)
			http.Error(w, msg, http.StatusInternalServerError)
			return
		}

		// store all of the version and build ids in the relevant task queue
		// items
		for _, task := range tasks {
			// this sucks, but it's because we're not guaranteed the order out
			// of the db
			for idx, queueItemAsUI := range asUI.Queue {
				if queueItemAsUI.Id == task.Id {
					queueItemAsUI.Version = task.Version
					queueItemAsUI.Build = task.BuildId
					asUI.Queue[idx] = queueItemAsUI
				}
			}
		}

		// add all of the necessary patch info into the relevant task queue
		// items
		for idx, queueItemAsUI := range asUI.Queue {
			if queueItemAsUI.Requester == mci.PatchVersionRequester {
				// fetch the patch, if necessary
				var p *patch.Patch
				var ok bool
				if p, ok = cachedPatches[queueItemAsUI.Version]; ok {
					queueItemAsUI.User = p.Author
					asUI.Queue[idx] = queueItemAsUI
				} else {
					p, err = patch.FindOne(
						patch.ByVersion(queueItemAsUI.Version).WithFields(patch.AuthorKey),
					)
					if err != nil {
						msg := fmt.Sprintf("Error finding patch: %v", err)
						mci.Logger.Errorf(slogger.ERROR, msg)
						http.Error(w, msg, http.StatusInternalServerError)
						return
					}
					if p == nil {
						msg := fmt.Sprintf("Couldn't find patch for version %v", queueItemAsUI.Version)
						mci.Logger.Errorf(slogger.ERROR, msg)
						http.Error(w, msg, http.StatusInternalServerError)
						return
					}
					cachedPatches[queueItemAsUI.Version] = p
				}
				queueItemAsUI.User = p.Author
				asUI.Queue[idx] = queueItemAsUI

			}
		}

		uiTaskQueues = append(uiTaskQueues, asUI)

	}

	// add other useful statistics to view alongside queue
	idleHosts, err := host.Find(host.IsIdle)
	if err != nil {
		msg := fmt.Sprintf("Error finding idle hosts: %v", err)
		mci.Logger.Errorf(slogger.ERROR, msg)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
	activeHosts, err := host.Find(host.IsLive)
	if err != nil {
		msg := fmt.Sprintf("Error finding active hosts: %v", err)
		mci.Logger.Errorf(slogger.ERROR, msg)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
	idleStaticHostsCount := 0
	for _, host := range idleHosts {
		if host.Provider == mci.HostTypeStatic {
			idleStaticHostsCount++
		}
	}
	activeStaticHostsCount := 0
	for _, host := range activeHosts {
		if host.Provider == mci.HostTypeStatic {
			activeStaticHostsCount++
		}
	}
	hostStats := uiHostStatistics{
		ActiveHosts:       len(activeHosts),
		ActiveStaticHosts: activeStaticHostsCount,
		IdleHosts:         len(idleHosts),
		IdleStaticHosts:   idleStaticHostsCount,
	}

	uis.WriteHTML(w, http.StatusOK, struct {
		ProjectData projectContext
		User        *model.DBUser
		Flashes     []interface{}
		Data        uiResourceInfo
	}{projCtx, GetUser(r), []interface{}{}, uiResourceInfo{uiTaskQueues, hostStats}},
		"base", "task_queues.html", "base_angular.html", "menu.html")
}
