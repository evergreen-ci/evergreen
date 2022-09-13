package service

import (
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// ui version of a task queue item
type uiTaskQueueItem struct {
	Id                  string        `json:"_id"`
	DisplayName         string        `json:"display_name"`
	BuildVariant        string        `json:"build_variant"`
	RevisionOrderNumber int           `json:"order"`
	Requester           string        `json:"requester"`
	Revision            string        `json:"gitspec"`
	Project             string        `json:"project"`
	Version             string        `json:"version"`
	Build               string        `json:"build"`
	ExpectedDuration    time.Duration `json:"exp_dur"`
	Priority            int64         `json:"priority"`

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
	Distros        []string         `json:"distros"`
}

// information on host utilization
type uiHostStatistics struct {
	IdleHosts         int `json:"idle_hosts"`
	ActiveHosts       int `json:"active_hosts"`
	ActiveStaticHosts int `json:"active_static_hosts"`
	IdleStaticHosts   int `json:"idle_static_hosts"`
}

func (uis *UIServer) taskQueue(w http.ResponseWriter, r *http.Request) {
	distro := gimlet.GetVars(r)["distro"]
	taskId := gimlet.GetVars(r)["task_id"]
	newUILink := ""
	if len(uis.Settings.Ui.UIv2Url) > 0 {
		newUILink = fmt.Sprintf("%s/task-queue/%s/%s", uis.Settings.Ui.UIv2Url, distro, taskId)
	}

	http.Redirect(w, r, newUILink, http.StatusTemporaryRedirect)
}

func (uis *UIServer) allTaskQueues(w http.ResponseWriter, r *http.Request) {
	begin := time.Now()
	taskQueues, err := model.FindAllTaskQueues()
	grip.DebugWhen(time.Since(begin) > 100*time.Millisecond, message.Fields{
		"operation":     "task queue page",
		"message":       "found all task queues",
		"duration_secs": time.Since(begin).Seconds(),
	})
	begin = time.Now()
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError,
			errors.Wrap(err, "Error finding task queues"))
		return
	}

	// find all distros so that we only display task queues of distros that exist.
	allDistros, err := distro.Find(distro.All.WithFields(distro.IdKey))
	if err != nil {
		message := fmt.Sprintf("error fetching distros: %v", err)
		http.Error(w, message, http.StatusInternalServerError)
		return
	}
	grip.DebugWhen(time.Since(begin) > 100*time.Millisecond, message.Fields{
		"operation":     "task queue page",
		"message":       "found all distros",
		"duration_secs": time.Since(begin).Seconds(),
	})
	begin = time.Now()
	distroIds := []string{}
	for _, d := range allDistros {
		distroIds = append(distroIds, d.Id)
	}

	// cached map of version id to relevant patch
	cachedPatches := map[string]*patch.Patch{}

	// convert the task queues to the ui versions
	uiTaskQueues := []uiTaskQueue{}
	var tasks []task.Task

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
				ExpectedDuration:    item.ExpectedDuration,
				Priority:            item.Priority,
			}

			asUI.Queue = append(asUI.Queue, queueItemAsUI)
		}

		// find all the relevant tasks
		tasks, err = task.FindWithFields(task.ByIds(taskIds), task.VersionKey, task.BuildIdKey)
		if err != nil {
			msg := fmt.Sprintf("Error finding tasks: %v", err)
			grip.Error(msg)
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
			if evergreen.IsPatchRequester(queueItemAsUI.Requester) {
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
						grip.Error(msg)
						http.Error(w, msg, http.StatusInternalServerError)
						return
					}
					if p == nil {
						msg := fmt.Sprintf("Couldn't find patch for version %v", queueItemAsUI.Version)
						grip.Error(msg)
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
	grip.DebugWhen(time.Since(begin) > 100*time.Millisecond, message.Fields{
		"operation":     "task queue page",
		"message":       "converted task queues to UI versions",
		"duration_secs": time.Since(begin).Seconds(),
	})
	begin = time.Now()

	// add other useful statistics to view alongside queue
	idleHosts, err := host.Find(host.IsIdle)
	if err != nil {
		msg := fmt.Sprintf("Error finding idle hosts: %v", err)
		grip.Error(msg)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
	grip.DebugWhen(time.Since(begin) > 100*time.Millisecond, message.Fields{
		"operation":     "task queue page",
		"message":       "found idle hosts",
		"duration_secs": time.Since(begin).Seconds(),
	})
	begin = time.Now()
	activeHosts, err := host.Find(db.Query(host.IsLive()))
	if err != nil {
		msg := fmt.Sprintf("Error finding active hosts: %v", err)
		grip.Error(msg)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
	grip.DebugWhen(time.Since(begin) > 100*time.Millisecond, message.Fields{
		"operation":     "task queue page",
		"message":       "found live hosts",
		"duration_secs": time.Since(begin).Seconds(),
	})
	idleStaticHostsCount := 0
	for _, host := range idleHosts {
		if host.Provider == evergreen.HostTypeStatic {
			idleStaticHostsCount++
		}
	}
	activeStaticHostsCount := 0
	for _, host := range activeHosts {
		if host.Provider == evergreen.HostTypeStatic {
			activeStaticHostsCount++
		}
	}
	hostStats := uiHostStatistics{
		ActiveHosts:       len(activeHosts),
		ActiveStaticHosts: activeStaticHostsCount,
		IdleHosts:         len(idleHosts),
		IdleStaticHosts:   idleStaticHostsCount,
	}

	uis.render.WriteResponse(w, http.StatusOK, struct {
		Data uiResourceInfo
		ViewData
	}{uiResourceInfo{uiTaskQueues, hostStats, distroIds}, uis.GetCommonViewData(w, r, false, true)},
		"base", "task_queues.html", "base_angular.html", "menu.html")
}
