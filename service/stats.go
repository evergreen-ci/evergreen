package service

import (
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
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
	Host          string    `json:"host"`
	Distro        string    `json:"distro"`
}

//UIBuild has the fields that are necessary to send over the wire for builds
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

// UIStats is all of the data that the stats page might need.
type UIStats struct {
	Tasks    []*UITask         `json:"tasks"`
	Builds   []*UIBuild        `json:"builds"`
	Versions []version.Version `json:"versions"`
	Patches  []patch.Patch     `json:"patches"`
}

func (uis *UIServer) taskTimingPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	project, err := projCtx.GetProject()

	if err != nil || project == nil {
		uis.ProjectNotFound(projCtx, w, r)
		return
	}

	currentProject := UIProject{project.Identifier, []UIBuildVariant{}, []string{}}

	// populate buildVariants by iterating over the build variants tasks
	for _, bv := range project.BuildVariants {
		newBv := UIBuildVariant{bv.Name, []string{}}
		for _, task := range bv.TaskUnits {
			newBv.TaskNames = append(newBv.TaskNames, task.Name)
		}
		currentProject.BuildVariants = append(currentProject.BuildVariants, newBv)
	}
	for _, task := range project.Tasks {
		currentProject.TaskNames = append(currentProject.TaskNames, task.Name)
	}

	data := struct {
		Project UIProject
		ViewData
	}{currentProject, uis.GetCommonViewData(w, r, false, true)}

	uis.WriteHTML(w, http.StatusOK, data, "base", "task_timing.html", "base_angular.html", "menu.html")
}

// taskTimingJSON sends over the task data for a certain task of a certain build variant
func (uis *UIServer) taskTimingJSON(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	beforeTaskId := r.FormValue("before")
	onlySuccessful := r.FormValue("onlySuccessful")

	limit, err := getIntValue(r, "limit", 50)
	if err != nil {
		uis.LoggedError(w, r, http.StatusBadRequest, err)
		return
	}

	buildVariant := mux.Vars(r)["build_variant"]
	taskName := mux.Vars(r)["task_name"]
	request := mux.Vars(r)["request"]

	project, err := projCtx.GetProject()
	if err != nil || project == nil {
		uis.LoggedError(w, r, http.StatusNotFound, errors.New("not found"))
		return
	}

	bv := project.FindBuildVariant(buildVariant)
	if bv == nil {
		uis.LoggedError(w, r, http.StatusNotFound, errors.Errorf("build variant %v not found", buildVariant))
		return
	}
	var versionIds []string
	data := UIStats{}
	var statuses []string
	if onlySuccessful == "true" {
		statuses = []string{evergreen.TaskSucceeded}
	} else {
		statuses = evergreen.CompletedStatuses
	}

	// if its all tasks find the build
	if taskName == "" || taskName == "All Tasks" {
		// TODO: switch this to be a query on the builds TaskCache
		builds, err := build.Find(build.ByProjectAndVariant(project.Identifier, buildVariant, request, statuses).
			WithFields(build.IdKey, build.CreateTimeKey, build.VersionKey,
				build.TimeTakenKey, build.TasksKey, build.FinishTimeKey, build.StartTimeKey, build.StatusKey).
			Sort([]string{"-" + build.RevisionOrderNumberKey}).
			Limit(limit))
		if err != nil {
			uis.LoggedError(w, r, http.StatusBadRequest, err)
			return
		}
		versionIds = make([]string, 0, len(builds))
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
			versionIds = append(versionIds, b.Version)
		}

		data.Builds = uiBuilds

	} else {
		foundTask := false

		for _, t := range bv.TaskUnits {
			if t.Name == taskName {
				foundTask = true
				break
			}
		}

		if !foundTask {
			uis.LoggedError(w, r, http.StatusNotFound, errors.Errorf("no task named '%v'", taskName))
			return
		}
		var tasks []task.Task

		fields := []string{task.CreateTimeKey, task.DispatchTimeKey, task.PushTimeKey,
			task.ScheduledTimeKey, task.StartTimeKey, task.FinishTimeKey,
			task.VersionKey, task.HostIdKey, task.StatusKey, task.HostIdKey,
			task.DistroIdKey}

		if beforeTaskId != "" {
			t, err := task.FindOne(task.ById(beforeTaskId))
			if err != nil {
				uis.LoggedError(w, r, http.StatusNotFound, err)
				return
			}
			if t == nil {
				uis.LoggedError(w, r, http.StatusNotFound, errors.Errorf("Task %v not found", beforeTaskId))
				return
			}

			tasks, err = task.Find(task.ByBeforeRevisionWithStatusesAndRequester(t.RevisionOrderNumber, statuses,
				buildVariant, taskName, project.Identifier, request).Limit(limit).WithFields(fields...))
			if err != nil {
				uis.LoggedError(w, r, http.StatusNotFound, err)
				return
			}

		} else {
			tasks, err = task.Find(task.ByStatuses(statuses,
				buildVariant, taskName, project.Identifier, request).Limit(limit).WithFields(fields...).Sort([]string{"-" + task.RevisionOrderNumberKey}))
			if err != nil {
				uis.LoggedError(w, r, http.StatusNotFound, err)
				return
			}
		}

		uiTasks := []*UITask{}
		versionIds = make([]string, 0, len(tasks))
		// get the versions for every single task that was returned
		for _, t := range tasks {
			// create a UITask
			uiTask := &UITask{
				Id:            t.Id,
				CreateTime:    t.CreateTime,
				DispatchTime:  t.DispatchTime,
				PushTime:      t.PushTime,
				ScheduledTime: t.ScheduledTime,
				StartTime:     t.StartTime,
				FinishTime:    t.FinishTime,
				Version:       t.Version,
				Status:        t.Status,
				Host:          t.HostId,
				Distro:        t.DistroId,
			}
			uiTasks = append(uiTasks, uiTask)
			versionIds = append(versionIds, t.Version)
		}
		data.Tasks = uiTasks
	}

	// Populate the versions field if with commits, otherwise patches field
	if request == evergreen.RepotrackerVersionRequester {
		versions, err := version.Find(version.ByIds(versionIds).
			WithFields(version.IdKey, version.CreateTimeKey, version.MessageKey,
				version.AuthorKey, version.RevisionKey, version.RevisionOrderNumberKey).
			Sort([]string{"-" + version.RevisionOrderNumberKey}))
		if err != nil {
			uis.LoggedError(w, r, http.StatusBadRequest, err)
			return
		}
		data.Versions = versions
	} else {
		// patches
		patches, err := patch.Find(patch.ByVersions(versionIds).
			WithFields(patch.IdKey, patch.CreateTimeKey, patch.DescriptionKey, patch.AuthorKey))
		if err != nil {
			uis.LoggedError(w, r, http.StatusBadRequest, err)
			return
		}
		data.Patches = patches
	}

	uis.WriteJSON(w, http.StatusOK, data)
}
