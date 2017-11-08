package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
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

type restVersion struct {
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
	Config              string    `json:"config,omitempty"`
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

// copyVersion copies the fields of a Version struct into a restVersion struct
func copyVersion(srcVersion *version.Version, destVersion *restVersion) {
	destVersion.Id = srcVersion.Id
	destVersion.CreateTime = srcVersion.CreateTime
	destVersion.StartTime = srcVersion.StartTime
	destVersion.FinishTime = srcVersion.FinishTime
	destVersion.Project = srcVersion.Identifier
	destVersion.Revision = srcVersion.Revision
	destVersion.Author = srcVersion.Author
	destVersion.AuthorEmail = srcVersion.AuthorEmail
	destVersion.Message = srcVersion.Message
	destVersion.Status = srcVersion.Status
	destVersion.BuildIds = srcVersion.BuildIds
	destVersion.RevisionOrderNumber = srcVersion.RevisionOrderNumber
	destVersion.Owner = srcVersion.Owner
	destVersion.Repo = srcVersion.Repo
	destVersion.Branch = srcVersion.Branch
	destVersion.RepoKind = srcVersion.RepoKind
	destVersion.Identifier = srcVersion.Identifier
	destVersion.Remote = srcVersion.Remote
	destVersion.RemotePath = srcVersion.RemotePath
	destVersion.Requester = srcVersion.Requester
	destVersion.Config = srcVersion.Config
}

// Returns a JSON response of an array with the NumRecentVersions
// most recent versions (sorted on commit order number descending).
func (restapi restAPI) getRecentVersions(w http.ResponseWriter, r *http.Request) {
	projectId := mux.Vars(r)["project_id"]

	versions, err := version.Find(version.ByMostRecentForRequester(projectId, evergreen.RepotrackerVersionRequester).Limit(10))
	if err != nil {
		msg := fmt.Sprintf("Error finding recent versions of project '%v'", projectId)
		grip.Errorf("%v: %+v", msg, err)
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
	builds, err := build.Find(
		build.ByVersions(versionIds).
			WithFields(build.BuildVariantKey, build.DisplayNameKey, build.TasksKey, build.VersionKey))
	if err != nil {
		msg := fmt.Sprintf("Error finding recent versions of project '%v'", projectId)
		grip.Errorf("%v: %+v", msg, err)
		restapi.WriteJSON(w, http.StatusInternalServerError, responseError{Message: msg})
		return
	}

	result := recentVersionsContent{
		Project:  projectId,
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
}

// Returns a JSON response with the marshaled output of the version
// specified in the request.
func (restapi restAPI) getVersionInfo(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveRESTContext(r)
	srcVersion, _ := projCtx.GetVersion()
	if srcVersion == nil {
		restapi.WriteJSON(w, http.StatusNotFound, responseError{Message: "error finding version"})
		return
	}

	destVersion := &restVersion{}
	copyVersion(srcVersion, destVersion)
	for _, buildStatus := range srcVersion.BuildVariants {
		destVersion.BuildVariants = append(destVersion.BuildVariants, buildStatus.BuildVariant)
		grip.Errorln("adding BuildVariant", buildStatus.BuildVariant)
	}

	restapi.WriteJSON(w, http.StatusOK, destVersion)
}

// Returns a JSON response with the marshaled output of the version
// specified in the request.
func (restapi restAPI) getVersionConfig(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveRESTContext(r)
	srcVersion, _ := projCtx.GetVersion()
	if srcVersion == nil {
		restapi.WriteJSON(w, http.StatusNotFound, responseError{Message: "version not found"})
		return
	}
	w.Header().Set("Content-Type", "application/x-yaml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(srcVersion.Config))
	grip.Warning(errors.Wrap(err, "problem writing response"))
}

// Returns a JSON response with the marshaled output of the version
// specified by its revision and project name in the request.
func (restapi restAPI) getVersionInfoViaRevision(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	projectId := vars["project_id"]
	revision := vars["revision"]

	srcVersion, err := version.FindOne(version.ByProjectIdAndRevision(projectId, revision))
	if err != nil || srcVersion == nil {
		msg := fmt.Sprintf("Error finding revision '%v' for project '%v'", revision, projectId)
		statusCode := http.StatusNotFound

		if err != nil {
			grip.Errorf("%v: %+v", msg, err)
			statusCode = http.StatusInternalServerError
		}

		restapi.WriteJSON(w, statusCode, responseError{Message: msg})
		return
	}

	destVersion := &restVersion{}
	copyVersion(srcVersion, destVersion)

	for _, buildStatus := range srcVersion.BuildVariants {
		destVersion.BuildVariants = append(destVersion.BuildVariants, buildStatus.BuildVariant)
		grip.Errorln("adding BuildVariant", buildStatus.BuildVariant)
	}

	restapi.WriteJSON(w, http.StatusOK, destVersion)
}

// Modifies part of the version specified in the request, and returns a
// JSON response with the marshaled output of its new state.
func (restapi restAPI) modifyVersionInfo(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveRESTContext(r)
	user := MustHaveUser(r)
	v, _ := projCtx.GetVersion()
	if v == nil {
		restapi.WriteJSON(w, http.StatusNotFound, responseError{Message: "error finding version"})
		return
	}

	input := struct {
		Activated *bool `json:"activated"`
	}{}

	body := util.NewRequestReader(r)
	defer body.Close()

	if err := json.NewDecoder(body).Decode(&input); err != nil {
		http.Error(w, fmt.Sprintf("problem parsing input: %v", err.Error()),
			http.StatusInternalServerError)
	}

	if input.Activated != nil {
		if err := model.SetVersionActivation(v.Id, *input.Activated, user.Id); err != nil {
			state := "inactive"
			if *input.Activated {
				state = "active"
			}

			msg := fmt.Sprintf("Error marking version '%v' as %v", v.Id, state)
			restapi.WriteJSON(w, http.StatusInternalServerError, responseError{Message: msg})
			return
		}
	}

	restapi.getVersionInfo(w, r)
}

// Returns a JSON response with the status of the specified version
// either grouped by the task names or the build variant names depending
// on the "groupby" query parameter.
func (restapi *restAPI) getVersionStatus(w http.ResponseWriter, r *http.Request) {
	versionId := mux.Vars(r)["version_id"]
	groupBy := r.FormValue("groupby")

	switch groupBy {
	case "": // default to group by tasks
		fallthrough
	case "tasks":
		restapi.getVersionStatusByTask(versionId, w)
		return
	case "builds":
		restapi.getVersionStatusByBuild(versionId, w)
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
func (restapi *restAPI) getVersionStatusByTask(versionId string, w http.ResponseWriter) {
	id := "_id"
	taskName := "task_name"
	statuses := "statuses"

	pipeline := []bson.M{
		// 1. Find only builds corresponding to the specified version
		{
			"$match": bson.M{
				build.VersionKey: versionId,
			},
		},
		// 2. Loop through each task run on a particular build variant
		{
			"$unwind": fmt.Sprintf("$%v", build.TasksKey),
		},
		// 3. Group on the task name and construct a new document containing
		//    all of the relevant info about the task status
		{
			"$group": bson.M{
				id: fmt.Sprintf("$%v.%v", build.TasksKey, build.TaskCacheDisplayNameKey),
				statuses: bson.M{
					"$push": bson.M{
						build.BuildVariantKey:       fmt.Sprintf("$%v", build.BuildVariantKey),
						build.TaskCacheIdKey:        fmt.Sprintf("$%v.%v", build.TasksKey, build.TaskCacheIdKey),
						build.TaskCacheStatusKey:    fmt.Sprintf("$%v.%v", build.TasksKey, build.TaskCacheStatusKey),
						build.TaskCacheStartTimeKey: fmt.Sprintf("$%v.%v", build.TasksKey, build.TaskCacheStartTimeKey),
						build.TaskCacheTimeTakenKey: fmt.Sprintf("$%v.%v", build.TasksKey, build.TaskCacheTimeTakenKey),
						build.TaskCacheActivatedKey: fmt.Sprintf("$%v.%v", build.TasksKey, build.TaskCacheActivatedKey),
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
			build.TaskCache `bson:",inline"`
		} `bson:"statuses"`
	}

	err := db.Aggregate(build.Collection, pipeline, &tasks)
	if err != nil {
		msg := fmt.Sprintf("Error finding status for version '%v'", versionId)
		grip.Errorf("%v: %+v", msg, err)
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
}

// Returns a JSON response with the status of the specified version
// grouped on the build variants. The keys of the object are the build
// variant name, with each key in the nested object representing a
// particular task.
func (restapi restAPI) getVersionStatusByBuild(versionId string, w http.ResponseWriter) {
	// Get all of the builds corresponding to this version
	builds, err := build.Find(
		build.ByVersion(versionId).WithFields(build.BuildVariantKey, build.TasksKey),
	)
	if err != nil {
		msg := fmt.Sprintf("Error finding status for version '%v'", versionId)
		grip.Errorf("%v: %+v", msg, err)
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
}

// lastGreen returns the most recent version for which the supplied variants completely pass.
func (ra *restAPI) lastGreen(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveRESTContext(r)
	project, err := projCtx.GetProject()
	if err != nil || project == nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	// queryParams should list build variants, example:
	// GET /rest/v1/projects/mongodb-mongo-master/last_green?linux-64=1&windows-64=1
	queryParams := r.URL.Query()

	// Make sure all query params are valid variants and put them in an array
	var bvs []string
	for key := range queryParams {
		if project.FindBuildVariant(key) != nil {
			bvs = append(bvs, key)
		} else {
			msg := fmt.Sprintf("build variant '%v' does not exist", key)
			http.Error(w, msg, http.StatusNotFound)
			return
		}
	}

	// Get latest version for which all the given build variants passed.
	version, err := model.FindLastPassingVersionForBuildVariants(project, bvs)
	if err != nil {
		ra.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	if version == nil {
		msg := fmt.Sprintf("Couldn't find latest green version for %v", strings.Join(bvs, ", "))
		http.Error(w, msg, http.StatusNotFound)
		return
	}

	ra.WriteJSON(w, http.StatusOK, version)
}
