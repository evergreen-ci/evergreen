package service

import (
	"net/http"
	"sort"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

const (
	allTasks = "All Tasks"
)

type UIDisplayTask struct {
	Name           string   `json:"name"`
	ExecutionTasks []string `json:"execution_tasks"`
}

// UIBuildVariant contains the name of the build variant and the tasks associated with that build variant.
type UIBuildVariant struct {
	Name         string          `json:"name"`
	TaskNames    []string        `json:"task_names"`
	DisplayTasks []UIDisplayTask `json:"display_tasks"`
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
	ScheduledTime time.Time `json:"scheduled_time"`
	StartTime     time.Time `json:"start_time"`
	FinishTime    time.Time `json:"finish_time"`
	Version       string    `json:"version"`
	Status        string    `json:"status"`
	Host          string    `json:"host"`
	Distro        string    `json:"distro"`
	IsDisplay     bool      `json:"is_display"`
}

//UIBuild has the fields that are necessary to send over the wire for builds
type UIBuild struct {
	Id         string    `json:"id"`
	CreateTime time.Time `json:"create_time"`
	StartTime  time.Time `json:"start_time"`
	FinishTime time.Time `json:"finish_time"`
	Version    string    `json:"version"`
	Status     string    `json:"status"`
	TimeTaken  int64     `json:"time_taken"`
}

// UIStats is all of the data that the stats page might need.
type UIStats struct {
	Tasks    []*UITask       `json:"tasks"`
	Builds   []*UIBuild      `json:"builds"`
	Versions []model.Version `json:"versions"`
	Patches  []patch.Patch   `json:"patches"`
}

func (uis *UIServer) taskTimingPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	project, err := projCtx.GetProject()

	if err != nil || project == nil {
		uis.ProjectNotFound(w, r)
		return
	}

	currentProject := UIProject{project.Identifier, []UIBuildVariant{}, []string{}}

	// populate buildVariants by iterating over the build variants tasks
	for _, bv := range project.BuildVariants {
		newBv := UIBuildVariant{bv.Name, []string{}, []UIDisplayTask{}}
		for _, task := range bv.Tasks {
			if task.IsGroup {
				tg := project.FindTaskGroup(task.Name)
				if tg != nil {
					for _, groupTask := range tg.Tasks {
						newBv.TaskNames = append(newBv.TaskNames, groupTask)
					}
				}
			} else {
				newBv.TaskNames = append(newBv.TaskNames, task.Name)
			}
		}

		// Copy display and execution tasks to UIBuildVariant ui-model
		for _, dispTask := range bv.DisplayTasks {
			executionTasks := dispTask.ExecTasks
			newBv.DisplayTasks = append(newBv.DisplayTasks, UIDisplayTask{
				Name:           dispTask.Name,
				ExecutionTasks: executionTasks,
			})
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

	uis.render.WriteResponse(w, http.StatusOK, data, "base", "task_timing.html", "base_angular.html", "menu.html")
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

	vars := gimlet.GetVars(r)
	buildVariant := vars["build_variant"]
	taskName := vars["task_name"]
	request := vars["request"]

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
	if taskName == "" || taskName == allTasks {
		// TODO: switch this to be a query on the builds TaskCache
		var builds []build.Build

		builds, err = build.Find(build.ByProjectAndVariant(project.Identifier, buildVariant, request, statuses).
			WithFields(build.IdKey, build.CreateTimeKey, build.VersionKey,
				build.TimeTakenKey, build.FinishTimeKey, build.StartTimeKey, build.StatusKey).
			Sort([]string{"-" + build.CreateTimeKey}).
			Limit(limit))
		if err != nil {
			uis.LoggedError(w, r, http.StatusBadRequest, err)
			return
		}
		versionIds = make([]string, 0, len(builds))
		uiBuilds := []*UIBuild{}
		// get the versions for every single task that was returned
		for _, build := range builds {
			// create a UIBuild
			b := &UIBuild{
				Id:         build.Id,
				CreateTime: build.CreateTime,
				StartTime:  build.StartTime,
				FinishTime: build.FinishTime,
				Version:    build.Version,
				Status:     build.Status,
				TimeTaken:  int64(build.TimeTaken),
			}

			uiBuilds = append(uiBuilds, b)
			versionIds = append(versionIds, b.Version)
		}

		data.Builds = uiBuilds

	} else {
		var tasks []task.Task

		fields := []string{task.CreateTimeKey, task.DispatchTimeKey,
			task.ScheduledTimeKey, task.StartTimeKey, task.FinishTimeKey,
			task.VersionKey, task.HostIdKey, task.StatusKey, task.HostIdKey,
			task.DistroIdKey, task.DisplayOnlyKey}

		if beforeTaskId != "" {
			var t *task.Task
			t, err = task.FindOneId(beforeTaskId)
			if err != nil {
				uis.LoggedError(w, r, http.StatusNotFound, err)
				return
			}
			if t == nil {
				uis.LoggedError(w, r, http.StatusNotFound, errors.Errorf("Task %v not found", beforeTaskId))
				return
			}

			tasks, err = task.FindAll(task.ByBeforeRevisionWithStatusesAndRequesters(t.RevisionOrderNumber, statuses,
				buildVariant, taskName, project.Identifier, []string{request}).Limit(limit).Sort([]string{"-" + task.CreateTimeKey}).WithFields(fields...))
			if err != nil {
				uis.LoggedError(w, r, http.StatusNotFound, err)
				return
			}

		} else {
			tasks, err = task.FindAll(task.ByStatuses(statuses,
				buildVariant, taskName, project.Identifier, request).Limit(limit).WithFields(fields...).Sort([]string{"-" + task.CreateTimeKey}))

			if err != nil {
				uis.LoggedError(w, r, http.StatusNotFound, err)
				return
			}
		}
		if len(tasks) == 0 {
			uis.LoggedError(w, r, http.StatusNotFound, errors.New("no task data found"))
			return
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
				ScheduledTime: t.ScheduledTime,
				StartTime:     t.StartTime,
				FinishTime:    t.FinishTime,
				Version:       t.Version,
				Status:        t.Status,
				Host:          t.HostId,
				Distro:        t.DistroId,
				IsDisplay:     t.DisplayOnly,
			}
			uiTasks = append(uiTasks, uiTask)
			versionIds = append(versionIds, t.Version)
		}
		data.Tasks = uiTasks
	}

	orderedVersionIDs := make([]string, 0, len(versionIds))
	// Populate the versions field if with commits, otherwise patches field
	if utility.StringSliceContains(evergreen.SystemVersionRequesterTypes, request) {
		versions, err := model.VersionFind(model.VersionByIds(versionIds).
			WithFields(model.VersionIdKey, model.VersionCreateTimeKey, model.VersionMessageKey,
				model.VersionAuthorKey, model.VersionRevisionKey))
		if err != nil {
			uis.LoggedError(w, r, http.StatusNotFound, errors.Wrap(err, "error finding past versions"))
			return
		}

		sort.Sort(sort.Reverse(model.VersionsByCreateTime(versions)))
		for _, v := range versions {
			orderedVersionIDs = append(orderedVersionIDs, v.Id)
		}

		data.Versions = versions
	} else {
		// patches
		patches, err := patch.Find(patch.ByVersions(versionIds).
			WithFields(patch.IdKey, patch.CreateTimeKey, patch.DescriptionKey, patch.AuthorKey, patch.VersionKey, patch.GithashKey))
		if err != nil {
			uis.LoggedError(w, r, http.StatusNotFound, errors.Wrap(err, "error finding past patches"))
			return
		}
		sort.Sort(sort.Reverse(patch.PatchesByCreateTime(patches)))
		for _, p := range patches {
			orderedVersionIDs = append(orderedVersionIDs, p.Id.Hex())
		}

		data.Patches = patches
	}

	if len(data.Builds) > 0 {
		data.Builds = alignBuilds(orderedVersionIDs, data.Builds)
	}
	if len(data.Tasks) > 0 {
		data.Tasks = alignTasks(orderedVersionIDs, data.Tasks)
	}

	gimlet.WriteJSON(w, data)
}

func alignBuilds(versionIDs []string, builds []*UIBuild) []*UIBuild {
	buildMap := make(map[string]*UIBuild)
	for _, b := range builds {
		buildMap[b.Version] = b
	}

	orderedBuilds := make([]*UIBuild, 0, len(builds))
	for _, id := range versionIDs {
		orderedBuilds = append(orderedBuilds, buildMap[id])
	}

	return orderedBuilds
}

func alignTasks(versionIDs []string, tasks []*UITask) []*UITask {
	taskMap := make(map[string]*UITask)
	for _, t := range tasks {
		taskMap[t.Version] = t
	}

	orderedTasks := make([]*UITask, 0, len(tasks))
	for _, id := range versionIDs {
		orderedTasks = append(orderedTasks, taskMap[id])
	}

	return orderedTasks
}
