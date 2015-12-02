package ui

import (
	"fmt"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/gorilla/mux"
	"net/http"
	"time"
)

// UIBuildVariant contains the name of the build variant and the tasks associated with that build variant.
type UIBuildVariant struct {
	Name      string   `json:"name"`
	TaskNames []string `json:"task_names"`
}

// UIProject has all the tasks that are in a project and all the BuildVariants.
type UIProject struct {
	Name          string           `json:"name"`
	BuildVariants []UIBuildVariant `json:"build_variants"`
	TaskNames     []string         `json:"task_names"`
}

//UITask has the fields that are necessary to send over the wire for tasks
type UITask struct {
	Id            string    `json:"id"`
	CreateTime    time.Time `json:"create_time"`
	DispatchTime  time.Time `json:"dispatch_time"`
	PushTime      time.Time `json:"push_time"`
	ScheduledTime time.Time `json:"scheduled_time"`
	StartTime     time.Time `json:"start_time"`
	FinishTime    time.Time `json:"finish_time"`
	Version       string    `json:"version"`
	Status        string    `json:"status"`
}

//UITask has the fields that are necessary to send over the wire for tasks
type UIBuild struct {
	Id         string            `json:"id"`
	CreateTime time.Time         `json:"create_time"`
	StartTime  time.Time         `json:"start_time"`
	FinishTime time.Time         `json:"finish_time"`
	Version    string            `json:"version"`
	Status     string            `json:"status"`
	Tasks      []build.TaskCache `json:"tasks"`
	TimeTaken  int64             `json:"time_taken"`
}

func (uis *UIServer) taskTimingPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

	if projCtx.Project == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	currentProject := UIProject{projCtx.Project.Identifier, []UIBuildVariant{}, []string{}}

	// populate buildVariants by iterating over the build variants tasks
	for _, bv := range projCtx.Project.BuildVariants {
		newBv := UIBuildVariant{bv.Name, []string{}}
		for _, task := range bv.Tasks {
			newBv.TaskNames = append(newBv.TaskNames, task.Name)
		}
		currentProject.BuildVariants = append(currentProject.BuildVariants, newBv)
	}
	for _, task := range projCtx.Project.Tasks {
		currentProject.TaskNames = append(currentProject.TaskNames, task.Name)
	}

	data := struct {
		ProjectData projectContext
		User        *user.DBUser
		Project     UIProject
	}{projCtx, GetUser(r), currentProject}

	uis.WriteHTML(w, http.StatusOK, data, "base", "task_timing.html", "base_angular.html", "menu.html")
}

// taskTimingJSON sends over the task data for a certain task of a certain build variant
func (uis *UIServer) taskTimingJSON(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	beforeTaskId := r.FormValue("before")

	limit, err := getIntValue(r, "limit", 50)
	if err != nil {
		uis.LoggedError(w, r, http.StatusBadRequest, err)
		return
	}

	buildVariant := mux.Vars(r)["build_variant"]
	taskName := mux.Vars(r)["task_name"]
	request := mux.Vars(r)["request"]

	if projCtx.Project == nil {
		uis.LoggedError(w, r, http.StatusNotFound, fmt.Errorf("not found"))
		return
	}

	bv := projCtx.Project.FindBuildVariant(buildVariant)
	if bv == nil {
		uis.LoggedError(w, r, http.StatusNotFound, fmt.Errorf("build variant %v not found", buildVariant))
		return
	}

	foundTask := false
	if taskName == "compile" || taskName == "push" {
		foundTask = true
	} else {
		for _, task := range bv.Tasks {
			if task.Name == taskName {
				foundTask = true
				break
			}
		}
	}
	if !foundTask {
		uis.LoggedError(w, r, http.StatusNotFound, fmt.Errorf("no task named '%v'", taskName))
		return
	}

	tasks, err := model.FindCompletedTasksByVariantAndName(projCtx.Project.Identifier,
		buildVariant, taskName, request, limit, beforeTaskId)
	if err != nil {
		uis.LoggedError(w, r, http.StatusNotFound, err)
		return
	}
	versions := []*version.Version{}
	uiTasks := []*UITask{}
	fmt.Println(len(tasks))
	// get the versions for every single task that was returned
	for _, task := range tasks {
		// create a UITask
		t := &UITask{
			Id:            task.Id,
			CreateTime:    task.CreateTime,
			DispatchTime:  task.DispatchTime,
			PushTime:      task.PushTime,
			ScheduledTime: task.ScheduledTime,
			StartTime:     task.StartTime,
			FinishTime:    task.FinishTime,
			Version:       task.Version,
			Status:        task.Status,
		}
		uiTasks = append(uiTasks, t)

		// find the version and append it as well
		v, err := version.FindOne(version.ById(task.Version))
		if err != nil {
			uis.LoggedError(w, r, http.StatusBadRequest, err)
			return
		}
		if v == nil {
			uis.LoggedError(w, r, http.StatusNotFound, fmt.Errorf("No version found for task %v", task.Id))
		}

		versions = append(versions, v)

	}
	data := struct {
		Tasks    []*UITask          `json:"tasks"`
		Versions []*version.Version `json:"versions"`
	}{uiTasks, versions}
	uis.WriteJSON(w, http.StatusOK, data)
}

// allTasksJSON sends over the total times for a certain buildvariant
func (uis *UIServer) buildJSON(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

	limit, err := getIntValue(r, "limit", 50)
	if err != nil {
		uis.LoggedError(w, r, http.StatusBadRequest, err)
		return
	}

	buildVariant := mux.Vars(r)["build_variant"]
	request := mux.Vars(r)["request"]

	if projCtx.Project == nil {
		uis.LoggedError(w, r, http.StatusNotFound, fmt.Errorf("not found"))
		return
	}

	// TODO: switch this to be a query on the builds TaskCache
	builds, err := build.Find(build.ByProjectAndVariant(projCtx.Project.Identifier, buildVariant, request).
		WithFields(build.IdKey, build.CreateTimeKey, build.VersionKey,
		build.TimeTakenKey, build.TasksKey, build.FinishTimeKey, build.StartTimeKey).
		Limit(limit))
	if err != nil {
		uis.LoggedError(w, r, http.StatusBadRequest, err)
		return
	}

	versions := []*version.Version{}
	uiBuilds := []*UIBuild{}
	// get the versions for every single task that was returned
	for _, build := range builds {

		// create a UITask
		b := &UIBuild{
			Id:         build.Id,
			CreateTime: build.CreateTime,
			StartTime:  build.StartTime,
			FinishTime: build.FinishTime,
			Version:    build.Version,
			Status:     build.Status,
			TimeTaken:  int64(build.TimeTaken),
			Tasks:      build.Tasks,
		}
		uiBuilds = append(uiBuilds, b)

		v, err := version.FindOne(version.ById(build.Version))
		if err != nil {
			uis.LoggedError(w, r, http.StatusBadRequest, err)
			return
		}
		if v == nil {
			uis.LoggedError(w, r, http.StatusNotFound, fmt.Errorf("No version found for build %v", build.Id))
		}

		versions = append(versions, v)

	}
	data := struct {
		Builds   []*UIBuild         `json:"tasks"`
		Versions []*version.Version `json:"versions"`
	}{uiBuilds, versions}
	uis.WriteJSON(w, http.StatusOK, data)
}
