package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"gopkg.in/yaml.v3"
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

// copyVersion copies the fields of a Version struct into a restVersion struct
func copyVersion(srcVersion *model.Version, destVersion *restVersion) {
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
	destVersion.Identifier = srcVersion.Identifier
	destVersion.Remote = srcVersion.Remote
	destVersion.RemotePath = srcVersion.RemotePath
	destVersion.Requester = srcVersion.Requester
}

// Returns a JSON response of an array with the NumRecentVersions
// most recent versions (sorted on commit order number descending).
func (restapi restAPI) getRecentVersions(w http.ResponseWriter, r *http.Request) {
	var err error
	projectIdentifier := gimlet.GetVars(r)["project_id"]
	limit := r.FormValue("limit")
	startStr := r.FormValue("start")
	start := 0
	if startStr != "" {
		start, err = strconv.Atoi(startStr)
		if err != nil {
			gimlet.WriteJSONError(w, responseError{Message: "'start' query parameter must be a valid integer"})
			return
		}
		if start < 0 {
			gimlet.WriteJSONError(w, responseError{Message: "'start' must be a non-negative integer"})
			return
		}
	}

	l := NumRecentVersions
	if limit != "" {
		l, err = strconv.Atoi(limit)
		if err != nil {
			msg := fmt.Sprintf("Error parsing %s as an integer", limit)
			gimlet.WriteJSONError(w, responseError{Message: msg})
			return
		}
	}
	var versions []model.Version
	projectId, err := model.GetIdForProject(projectIdentifier)
	// only look for versions if the project can be found, otherwise continue without error
	if err == nil {
		// add one to limit to determine if a new page is necessary
		versions, err = model.VersionFind(model.VersionBySystemRequesterOrdered(projectId, start).
			Limit(l + 1))
		if err != nil {
			msg := fmt.Sprintf("Error finding recent versions of project '%v'", projectIdentifier)
			grip.Error(errors.Wrap(err, msg))
			gimlet.WriteJSONInternalError(w, responseError{Message: msg})
			return
		}
	}

	nextPageStart := ""
	// save the ID for the next page version, and remove this version from results
	if len(versions) > l {
		nextPageStart = strconv.Itoa(versions[len(versions)-1].RevisionOrderNumber)
		versions = versions[:len(versions)-1]
	}
	// Create a slice of version ids to find all relevant builds
	versionIds := make([]string, 0, len(versions))

	// Cache the order of versions in a map for lookup by their id
	versionIdx := make(map[string]int, len(versions))
	for i, version := range versions {
		versionIds = append(versionIds, version.Id)
		versionIdx[version.Id] = i
	}

	result := recentVersionsContent{
		Project:  projectIdentifier,
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
	// Find all builds/tasks corresponding the set of version ids
	if err = result.populateBuildsAndTasks(versionIds, versionIdx); err != nil {
		msg := fmt.Sprintf("Error populating builds/tasks for recent versions of project '%v'", projectIdentifier)
		grip.Error(errors.Wrap(err, msg))
		gimlet.WriteJSONInternalError(w, responseError{Message: msg})
	}

	// create a page header
	if nextPageStart != "" {
		responder := gimlet.NewResponseBuilder()
		err = responder.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start",
				BaseURL:         restapi.GetSettings().ApiUrl,
				Key:             nextPageStart,
				Limit:           l,
			},
		})
		if err != nil {
			msg := "error setting pages"
			grip.Error(errors.Wrap(err, msg))
			gimlet.WriteJSONInternalError(w, responseError{Message: msg})
		}
		w.Header().Set("Link", responder.Pages().GetLinks(r.URL.String()))
	}
	gimlet.WriteJSON(w, result)
}

func (r *recentVersionsContent) populateBuildsAndTasks(versionIds []string, versionIdx map[string]int) error {
	builds, err := build.FindBuildsByVersions(versionIds)
	if err != nil {
		return errors.Wrap(err, "Error finding recent versions")
	}
	tasks, err := task.FindTasksFromVersions(versionIds)
	if err != nil {
		return errors.Wrap(err, "Error finding recent tasks for recent versions")
	}

	for _, b := range builds {
		buildInfo := versionBuildInfo{
			Id:    b.Id,
			Name:  b.DisplayName,
			Tasks: make(versionByBuildByTask),
		}
		versionInfo := r.Versions[versionIdx[b.Version]]
		versionInfo.Builds[b.BuildVariant] = buildInfo
	}
	for _, t := range tasks {
		taskInfo := versionStatus{
			Id:        t.Id,
			Status:    t.Status,
			TimeTaken: t.TimeTaken,
		}
		// save task with the corresponding build for the corresponding version
		versionInfo := r.Versions[versionIdx[t.Version]]
		buildInfo := versionInfo.Builds[t.BuildVariant]
		if buildInfo.Tasks == nil {
			buildInfo.Tasks = make(versionByBuildByTask)
			buildInfo.Id = t.BuildId
		}
		buildInfo.Tasks[t.DisplayName] = taskInfo
	}
	return nil
}

// Returns a JSON response with the marshaled output of the version
// specified in the request.
func (restapi restAPI) getVersionInfo(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveRESTContext(r)
	srcVersion := projCtx.Version
	if srcVersion == nil {
		gimlet.WriteJSONResponse(w, http.StatusNotFound, responseError{Message: "error finding version"})
		return
	}

	destVersion := &restVersion{}
	copyVersion(srcVersion, destVersion)
	for _, buildStatus := range srcVersion.BuildVariants {
		destVersion.BuildVariants = append(destVersion.BuildVariants, buildStatus.BuildVariant)
		grip.Infof("adding BuildVariant %s", buildStatus.BuildVariant)
	}

	gimlet.WriteJSON(w, destVersion)
}

// Returns a JSON response with the marshaled output of the version
// specified in the request (we still want this in yaml).
func (restapi restAPI) getVersionConfig(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveRESTContext(r)
	srcVersion := projCtx.Version
	if srcVersion == nil {
		gimlet.WriteJSONResponse(w, http.StatusNotFound, responseError{Message: "version not found"})
		return
	}
	w.Header().Set("Content-Type", "application/x-yaml; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	settings := restapi.GetSettings()
	pp, err := model.ParserProjectFindOneByID(r.Context(), &settings, projCtx.Version.ProjectStorageMethod, projCtx.Version.Id)
	if err != nil {
		gimlet.WriteJSONResponse(w, http.StatusInternalServerError, responseError{Message: "problem finding parser project"})
		return
	}
	var config []byte
	config, err = yaml.Marshal(pp)
	if err != nil {
		gimlet.WriteJSONResponse(w, http.StatusInternalServerError, responseError{Message: "problem marshalling project"})
		return
	}
	_, err = w.Write(config)
	grip.Warning(message.WrapError(err, message.Fields{
		"message":    "could not write parser project to response",
		"version_id": projCtx.Version.Id,
		"route":      "/versions/{version_id}/config",
	}))
}

// Returns a JSON response with the marshaled output of the version
// specified in the request.
func (restapi restAPI) getVersionProject(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveRESTContext(r)
	if projCtx.Version == nil {
		gimlet.WriteJSONResponse(w, http.StatusNotFound, responseError{Message: "version not found"})
		return
	}

	env := evergreen.GetEnvironment()
	pp, err := model.ParserProjectFindOneByID(r.Context(), env.Settings(), projCtx.Version.ProjectStorageMethod, projCtx.Version.Id)
	if err != nil {
		gimlet.WriteJSONResponse(w, http.StatusInternalServerError, responseError{Message: "problem finding parser project"})
		return
	}
	if pp == nil {
		gimlet.WriteJSONResponse(w, http.StatusNotFound, responseError{Message: fmt.Sprintf("parser project '%s' not found", projCtx.Version.Id)})
		return
	}

	bytes, err := bson.Marshal(pp)
	if err != nil {
		gimlet.WriteJSONResponse(w, http.StatusInternalServerError, responseError{Message: "problem reading to bson"})
		return
	}
	gimlet.WriteBinary(w, bytes)
}

// Returns a JSON response with the marshaled output of the version
// specified by its revision and project name in the request.
func (restapi restAPI) getVersionInfoViaRevision(w http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	projectName := vars["project_id"]
	revision := vars["revision"]

	projectId, err := model.GetIdForProject(projectName)
	if err != nil {
		gimlet.WriteJSONError(w, responseError{Message: "project doesn't exist"})
	}
	srcVersion, err := model.VersionFindOne(model.BaseVersionByProjectIdAndRevision(projectId, revision))
	if err != nil || srcVersion == nil {
		msg := fmt.Sprintf("Error finding revision '%v' for project '%v'", revision, projectId)
		statusCode := http.StatusNotFound

		if err != nil {
			grip.Errorf("%v: %+v", msg, err)
			statusCode = http.StatusInternalServerError
		}

		gimlet.WriteJSONResponse(w, statusCode, responseError{Message: msg})
		return
	}

	destVersion := &restVersion{}
	copyVersion(srcVersion, destVersion)

	for _, buildStatus := range srcVersion.BuildVariants {
		destVersion.BuildVariants = append(destVersion.BuildVariants, buildStatus.BuildVariant)
		grip.Infof("adding BuildVariant %s", buildStatus.BuildVariant)
	}

	gimlet.WriteJSON(w, destVersion)
}

// Modifies part of the version specified in the request, and returns a
// JSON response with the marshaled output of its new state.
func (restapi restAPI) modifyVersionInfo(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveRESTContext(r)
	user := MustHaveUser(r)
	v := projCtx.Version
	if v == nil {
		gimlet.WriteJSONResponse(w, http.StatusNotFound, responseError{Message: "error finding version"})
		return
	}

	input := struct {
		Activated *bool `json:"activated"`
	}{}

	body := utility.NewRequestReader(r)
	defer body.Close()

	if err := json.NewDecoder(body).Decode(&input); err != nil {
		http.Error(w, fmt.Sprintf("problem parsing input: %v", err.Error()),
			http.StatusInternalServerError)
	}

	if input.Activated != nil {
		if err := model.SetVersionActivation(r.Context(), v.Id, *input.Activated, user.Id); err != nil {
			state := "inactive"
			if *input.Activated {
				state = "active"
			}

			msg := fmt.Sprintf("Error marking version '%v' as %v", v.Id, state)
			gimlet.WriteJSONInternalError(w, responseError{Message: msg})
			return
		}
	}

	restapi.getVersionInfo(w, r)
}

// Returns a JSON response with the status of the specified version
// either grouped by the task names or the build variant names depending
// on the "groupby" query parameter.
func (restapi *restAPI) getVersionStatus(w http.ResponseWriter, r *http.Request) {
	versionId := gimlet.GetVars(r)["version_id"]
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
		gimlet.WriteJSONError(w, responseError{Message: msg})
		return
	}
}

// Returns a JSON response with the status of the specified version
// grouped on the tasks. The keys of the object are the task names,
// with each key in the nested object representing a particular build
// variant.
func (restapi *restAPI) getVersionStatusByTask(versionId string, w http.ResponseWriter) {
	id := "_id"

	pipeline := []bson.M{
		// Find only tasks corresponding to the specified version
		{
			"$match": bson.M{
				task.VersionKey: versionId,
			},
		},
		// Group on the task display name and construct a new task document containing
		// all of the relevant info about the task status
		{
			"$group": bson.M{
				id: fmt.Sprintf("$%s", task.DisplayNameKey),
				"tasks": bson.M{
					"$push": bson.M{
						task.BuildVariantKey: fmt.Sprintf("$%s", task.BuildVariantKey),
						task.IdKey:           fmt.Sprintf("$%s", task.IdKey),
						task.StatusKey:       fmt.Sprintf("$%s", task.StatusKey),
						task.TimeTakenKey:    fmt.Sprintf("$%s", task.TimeTakenKey),
					},
				},
			},
		},
		// Rename the "_id" field to "task_name"
		{
			"$project": bson.M{
				id:          0,
				"task_name": fmt.Sprintf("$%s", id),
				"tasks":     1,
			},
		},
	}

	// Anonymous struct used to unmarshal output from the aggregation pipeline
	var groupedTasks []struct {
		DisplayName string      `bson:"task_name"`
		Tasks       []task.Task `bson:"tasks"`
	}

	err := db.Aggregate(task.Collection, pipeline, &groupedTasks)
	if err != nil {
		msg := fmt.Sprintf("Error finding status for version '%v'", versionId)
		grip.Errorf("%v: %+v", msg, err)
		gimlet.WriteJSONInternalError(w, responseError{Message: msg})
		return
	}

	result := versionStatusByTaskContent{
		Id:    versionId,
		Tasks: make(map[string]versionStatusByTask, len(groupedTasks)),
	}

	for _, t := range groupedTasks {
		statuses := make(versionStatusByTask, len(t.Tasks))
		for _, task := range t.Tasks {
			status := versionStatus{
				Id:        task.Id,
				Status:    task.Status,
				TimeTaken: task.TimeTaken,
			}
			statuses[task.BuildVariant] = status
		}
		result.Tasks[t.DisplayName] = statuses
	}

	gimlet.WriteJSON(w, result)
}

// Returns a JSON response with the status of the specified version
// grouped on the build variants. The keys of the object are the build
// variant name, with each key in the nested object representing a
// particular task.
func (restapi restAPI) getVersionStatusByBuild(versionId string, w http.ResponseWriter) {
	// Get all of the builds corresponding to this version
	builds, err := build.Find(
		build.ByVersion(versionId).WithFields(build.BuildVariantKey, bsonutil.GetDottedKeyName(build.TasksKey, build.TaskCacheIdKey)),
	)
	if err != nil {
		msg := fmt.Sprintf("Error finding builds for version '%v'", versionId)
		grip.Errorf("%v: %+v", msg, err)
		gimlet.WriteJSONInternalError(w, responseError{Message: msg})
		return
	}

	query := db.Query(task.ByVersion(versionId)).WithFields(task.StatusKey, task.TimeTakenKey, task.DisplayNameKey)
	tasks, err := task.FindAll(query)
	if err != nil {
		msg := fmt.Sprintf("Error finding tasks for version '%v'", versionId)
		grip.Errorf("%s: %+v", msg, err)
		gimlet.WriteJSONInternalError(w, responseError{Message: msg})
		return
	}
	taskMap := task.TaskSliceToMap(tasks)

	result := versionStatusByBuildContent{
		Id:     versionId,
		Builds: make(map[string]versionStatusByBuild, len(builds)),
	}

	for _, build := range builds {
		statuses := make(versionStatusByBuild, len(build.Tasks))
		for _, task := range build.Tasks {
			t, ok := taskMap[task.Id]
			if !ok {
				continue
			}
			status := versionStatus{
				Id:        t.Id,
				Status:    t.Status,
				TimeTaken: t.TimeTaken,
			}
			statuses[t.DisplayName] = status
		}
		result.Builds[build.BuildVariant] = statuses
	}

	gimlet.WriteJSON(w, result)
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

	// Don't validate build variants are in project, since they may be generated
	// variants that don't yet exist for the latest parser project.
	var bvs []string
	for key := range queryParams {
		bvs = append(bvs, key)
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

	gimlet.WriteJSON(w, version)
}
