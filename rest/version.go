package rest

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"encoding/json"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/gorilla/mux"
	"github.com/shelman/angier"
	"labix.org/v2/mgo/bson"
	"net/http"
	"time"
)

const NumRecentVersions = 10

type recentVersionsContent struct {
	Project  string            `json:"project"`
	Versions []versionLessInfo `json:"versions"`
}

type versionStatusByTaskContent struct {
	Id    string                         `json:"version_id"`
	Tasks map[string]versionStatusByTask `json:"tasks"`
}

type versionStatusByBuildContent struct {
	Id     string                          `json:"version_id"`
	Builds map[string]versionStatusByBuild `json:"builds"`
}

type version struct {
	Id                  string    `json:"id"`
	CreateTime          time.Time `json:"create_time"`
	StartTime           time.Time `json:"start_time"`
	FinishTime          time.Time `json:"finish_time"`
	Project             string    `json:"project"`
	Revision            string    `json:"revision"`
	Author              string    `json:"author"`
	AuthorEmail         string    `json:"author_email"`
	Message             string    `json:"message"`
	Status              string    `json:"status"`
	Activated           bool      `json:"activated"`
	BuildIds            []string  `json:"builds"`
	BuildVariants       []string  `json:"build_variants"`
	RevisionOrderNumber int       `json:"order"`
	Owner               string    `json:"owner_name"`
	Repo                string    `json:"repo_name"`
	Branch              string    `json:"branch_name"`
	RepoKind            string    `json:"repo_kind"`
	BatchTime           int       `json:"batch_time"`
	Identifier          string    `json:"identifier"`
	Remote              bool      `json:"remote"`
	RemotePath          string    `json:"remote_path"`
	Requester           string    `json:"requester"`
}

type versionLessInfo struct {
	Id       string         `json:"version_id"`
	Author   string         `json:"author"`
	Revision string         `json:"revision"`
	Message  string         `json:"message"`
	Builds   versionByBuild `json:"builds"`
}

type versionStatus struct {
	Id        string        `json:"task_id"`
	Status    string        `json:"status"`
	TimeTaken time.Duration `json:"time_taken"`
}

type versionByBuild map[string]versionBuildInfo

type versionBuildInfo struct {
	Id    string               `json:"build_id"`
	Name  string               `json:"name"`
	Tasks versionByBuildByTask `json:"tasks"`
}

type versionByBuildByTask map[string]versionStatus

type versionStatusByTask map[string]versionStatus

type versionStatusByBuild map[string]versionStatus

// Returns a JSON response of an array with the NumRecentVersions
// most recent versions (sorted on commit order number descending).
func (restapi RESTAPI) getRecentVersions(w http.ResponseWriter, r *http.Request) {
	projectName := mux.Vars(r)["project_id"]

	versions, err := model.FindMostRecentVersions(projectName,
		mci.RepotrackerVersionRequester, db.NoSkip, NumRecentVersions)
	if err != nil {
		msg := fmt.Sprintf("Error finding recent versions of project '%v'", projectName)
		mci.Logger.Logf(slogger.ERROR, "%v: %v", msg, err)
		restapi.WriteJSON(w, http.StatusInternalServerError, responseError{Message: msg})
		return

	}

	// Create a slice of version ids to find all relevant builds
	versionIds := make([]string, 0, len(versions))

	// Cache the order of versions in a map for lookup by their id
	versionIdx := make(map[string]int, len(versions))

	for i, version := range versions {
		versionIds = append(versionIds, version.Id)
		versionIdx[version.Id] = i
	}

	// Find all builds corresponding the set of version ids
	builds, err := model.FindAllBuilds(
		bson.M{
			model.BuildVersionKey: bson.M{
				"$in": versionIds,
			},
		},
		bson.M{
			model.BuildBuildVariantKey: 1,
			model.BuildDisplayNameKey:  1,
			model.BuildTasksKey:        1,
			model.BuildVersionKey:      1,
		},
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)
	if err != nil {
		msg := fmt.Sprintf("Error finding recent versions of project '%v'", projectName)
		mci.Logger.Logf(slogger.ERROR, "%v: %v", msg, err)
		restapi.WriteJSON(w, http.StatusInternalServerError, responseError{Message: msg})
		return

	}

	result := recentVersionsContent{
		Project:  projectName,
		Versions: make([]versionLessInfo, 0, len(versions)),
	}

	for _, version := range versions {
		versionInfo := versionLessInfo{
			Id:       version.Id,
			Author:   version.Author,
			Revision: version.Revision,
			Message:  version.Message,
			Builds:   make(versionByBuild),
		}

		result.Versions = append(result.Versions, versionInfo)
	}

	for _, build := range builds {
		buildInfo := versionBuildInfo{
			Id:    build.Id,
			Name:  build.DisplayName,
			Tasks: make(versionByBuildByTask, len(build.Tasks)),
		}

		for _, task := range build.Tasks {
			buildInfo.Tasks[task.DisplayName] = versionStatus{
				Id:        task.Id,
				Status:    task.Status,
				TimeTaken: task.TimeTaken,
			}
		}

		versionInfo := result.Versions[versionIdx[build.Version]]
		versionInfo.Builds[build.BuildVariant] = buildInfo
	}

	restapi.WriteJSON(w, http.StatusOK, result)
	return

}

// Returns a JSON response with the marshalled output of the version
// specified in the request.
func (restapi RESTAPI) getVersionInfo(w http.ResponseWriter, r *http.Request) {
	versionId := mux.Vars(r)["version_id"]

	srcVersion, err := model.FindVersion(versionId)
	if err != nil || srcVersion == nil {
		msg := fmt.Sprintf("Error finding version '%v'", versionId)
		statusCode := http.StatusNotFound

		if err != nil {
			mci.Logger.Logf(slogger.ERROR, "%v: %v", msg, err)
			statusCode = http.StatusInternalServerError
		}

		restapi.WriteJSON(w, statusCode, responseError{Message: msg})
		return

	}

	destVersion := &version{}
	// Copy the contents from the database into our local version type
	err = angier.TransferByFieldNames(srcVersion, destVersion)
	if err != nil {
		msg := fmt.Sprintf("Error finding version '%v'", versionId)
		mci.Logger.Logf(slogger.ERROR, "%v: %v", msg, err)
		restapi.WriteJSON(w, http.StatusInternalServerError, responseError{Message: msg})
		return

	}
	for _, buildStatus := range srcVersion.BuildVariants {
		destVersion.BuildVariants = append(destVersion.BuildVariants, buildStatus.BuildVariant)
		mci.Logger.Logf(slogger.ERROR, "adding BuildVariant %v", buildStatus.BuildVariant)
	}

	restapi.WriteJSON(w, http.StatusOK, destVersion)
	return

}

// Returns a JSON response with the marshalled output of the version
// specified by its revision and project name in the request.
func (restapi RESTAPI) getVersionInfoViaRevision(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	projectName := vars["project_id"]
	revision := vars["revision"]

	srcVersion, err := model.FindVersionByIdAndRevision(projectName, revision)
	if err != nil || srcVersion == nil {
		msg := fmt.Sprintf("Error finding revision '%v' for project '%v'",
			revision, projectName)
		statusCode := http.StatusNotFound

		if err != nil {
			mci.Logger.Logf(slogger.ERROR, "%v: %v", msg, err)
			statusCode = http.StatusInternalServerError
		}

		restapi.WriteJSON(w, statusCode, responseError{Message: msg})
		return

	}

	destVersion := &version{}
	// Copy the contents from the database into our local version type
	err = angier.TransferByFieldNames(srcVersion, destVersion)
	if err != nil {
		msg := fmt.Sprintf("Error finding revision '%v' for project '%v'",
			revision, projectName)
		mci.Logger.Logf(slogger.ERROR, "%v: %v", msg, err)
		restapi.WriteJSON(w, http.StatusInternalServerError, responseError{Message: msg})
		return

	}
	for _, buildStatus := range srcVersion.BuildVariants {
		destVersion.BuildVariants = append(destVersion.BuildVariants, buildStatus.BuildVariant)
		mci.Logger.Logf(slogger.ERROR, "adding BuildVariant %v", buildStatus.BuildVariant)
	}

	restapi.WriteJSON(w, http.StatusOK, destVersion)
	return

}

// Modifies part of the version specified in the request, and returns a
// JSON response with the marshalled output of its new state.
func (restapi RESTAPI) modifyVersionInfo(w http.ResponseWriter, r *http.Request) {
	versionId := mux.Vars(r)["version_id"]

	var input struct {
		Activated *bool `json:"activated"`
	}
	json.NewDecoder(r.Body).Decode(&input)

	version, err := model.FindVersion(versionId)
	if err != nil || version == nil {
		msg := fmt.Sprintf("Error finding version '%v'", versionId)
		statusCode := http.StatusNotFound

		if err != nil {
			mci.Logger.Logf(slogger.ERROR, "%v: %v", msg, err)
			statusCode = http.StatusInternalServerError
		}

		restapi.WriteJSON(w, statusCode, responseError{Message: msg})
		return

	}

	if input.Activated != nil {
		if err = version.SetActivated(*input.Activated); err != nil {
			state := "inactive"
			if *input.Activated {
				state = "active"
			}

			msg := fmt.Sprintf("Error marking version '%v' as %v", versionId, state)
			restapi.WriteJSON(w, http.StatusInternalServerError, responseError{Message: msg})
			return

		}
	}

	restapi.getVersionInfo(w, r)
}

// Returns a JSON response with the status of the specified version
// either grouped by the task names or the build variant names depending
// on the "groupby" query parameter.
func (restapi *RESTAPI) getVersionStatus(w http.ResponseWriter, r *http.Request) {
	versionId := mux.Vars(r)["version_id"]
	groupBy := r.FormValue("groupby")

	switch groupBy {
	case "": // default to group by tasks
		fallthrough
	case "tasks":
		restapi.getVersionStatusByTask(versionId, w, r)
		return
	case "builds":
		restapi.getVersionStatusByBuild(versionId, w, r)
		return
	default:
		msg := fmt.Sprintf("Invalid groupby parameter '%v'", groupBy)
		restapi.WriteJSON(w, http.StatusBadRequest, responseError{Message: msg})
		return

	}
}

// Returns a JSON response with the status of the specified version
// grouped on the tasks. The keys of the object are the task names,
// with each key in the nested object representing a particular build
// variant.
func (restapi *RESTAPI) getVersionStatusByTask(versionId string, w http.ResponseWriter, r *http.Request) {
	id := "_id"
	taskName := "task_name"
	statuses := "statuses"

	pipeline := []bson.M{
		// 1. Find only builds corresponding to the specified version
		{
			"$match": bson.M{
				model.BuildVersionKey: versionId,
			},
		},
		// 2. Loop through each task run on a particular build variant
		{
			"$unwind": fmt.Sprintf("$%v", model.BuildTasksKey),
		},
		// 3. Group on the task name and construct a new document containing
		//    all of the relevant info about the task status
		{
			"$group": bson.M{
				id: fmt.Sprintf("$%v.%v", model.BuildTasksKey, model.TaskCacheDisplayNameKey),
				statuses: bson.M{
					"$push": bson.M{
						model.BuildBuildVariantKey:  fmt.Sprintf("$%v", model.BuildBuildVariantKey),
						model.TaskCacheIdKey:        fmt.Sprintf("$%v.%v", model.BuildTasksKey, model.TaskCacheIdKey),
						model.TaskCacheStatusKey:    fmt.Sprintf("$%v.%v", model.BuildTasksKey, model.TaskCacheStatusKey),
						model.TaskCacheStartTimeKey: fmt.Sprintf("$%v.%v", model.BuildTasksKey, model.TaskCacheStartTimeKey),
						model.TaskCacheTimeTakenKey: fmt.Sprintf("$%v.%v", model.BuildTasksKey, model.TaskCacheTimeTakenKey),
						model.TaskCacheActivatedKey: fmt.Sprintf("$%v.%v", model.BuildTasksKey, model.TaskCacheActivatedKey),
					},
				},
			},
		},
		// 4. Rename the "_id" field to "task_name"
		{
			"$project": bson.M{
				id:       0,
				taskName: fmt.Sprintf("$%v", id),
				statuses: 1,
			},
		},
	}

	// Anonymous struct used to unmarshal output from the aggregation pipeline
	var tasks []struct {
		DisplayName string `bson:"task_name"`
		Statuses    []struct {
			BuildVariant string `bson:"build_variant"`
			// Use an anonyous field to make the semantics of inlining
			model.TaskCache `bson:",inline"`
		} `bson:"statuses"`
	}

	err := db.Aggregate(model.BuildsCollection, pipeline, &tasks)
	if err != nil {
		msg := fmt.Sprintf("Error finding status for version '%v'", versionId)
		mci.Logger.Logf(slogger.ERROR, "%v: %v", msg, err)
		restapi.WriteJSON(w, http.StatusInternalServerError, responseError{Message: msg})
		return

	}

	result := versionStatusByTaskContent{
		Id:    versionId,
		Tasks: make(map[string]versionStatusByTask, len(tasks)),
	}

	for _, task := range tasks {
		statuses := make(versionStatusByTask, len(task.Statuses))
		for _, task := range task.Statuses {
			status := versionStatus{
				Id:        task.Id,
				Status:    task.Status,
				TimeTaken: task.TimeTaken,
			}
			statuses[task.BuildVariant] = status
		}
		result.Tasks[task.DisplayName] = statuses
	}

	restapi.WriteJSON(w, http.StatusOK, result)
	return

}

// Returns a JSON response with the status of the specified version
// grouped on the build variants. The keys of the object are the build
// variant name, with each key in the nested object representing a
// particular task.
func (restapi RESTAPI) getVersionStatusByBuild(versionId string, w http.ResponseWriter, r *http.Request) {
	// Get all of the builds corresponding to this version
	builds, err := model.FindAllBuilds(
		bson.M{
			model.BuildVersionKey: versionId,
		},
		bson.M{
			model.BuildBuildVariantKey: 1,
			model.BuildTasksKey:        1,
		},
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)
	if err != nil {
		msg := fmt.Sprintf("Error finding status for version '%v'", versionId)
		mci.Logger.Logf(slogger.ERROR, "%v: %v", msg, err)
		restapi.WriteJSON(w, http.StatusInternalServerError, responseError{Message: msg})
		return

	}

	result := versionStatusByBuildContent{
		Id:     versionId,
		Builds: make(map[string]versionStatusByBuild, len(builds)),
	}

	for _, build := range builds {
		statuses := make(versionStatusByBuild, len(build.Tasks))
		for _, task := range build.Tasks {
			status := versionStatus{
				Id:        task.Id,
				Status:    task.Status,
				TimeTaken: task.TimeTaken,
			}
			statuses[task.DisplayName] = status
		}
		result.Builds[build.BuildVariant] = statuses
	}

	restapi.WriteJSON(w, http.StatusOK, result)
	return

}
