package rest

import (
	"10gen.com/mci"
	"10gen.com/mci/model/build"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/gorilla/mux"
	"github.com/shelman/angier"
	"net/http"
	"time"
)

type buildStatusContent struct {
	Id           string            `json:"build_id"`
	BuildVariant string            `json:"build_variant"`
	Tasks        buildStatusByTask `json:"tasks"`
}

type restBuild struct {
	Id                  string            `json:"id"`
	CreateTime          time.Time         `json:"create_time"`
	StartTime           time.Time         `json:"start_time"`
	FinishTime          time.Time         `json:"finish_time"`
	PushTime            time.Time         `json:"push_time"`
	Version             string            `json:"version"`
	Project             string            `json:"project"`
	Revision            string            `json:"revision"`
	BuildVariant        string            `json:"variant"`
	BuildNumber         string            `json:"number"`
	Status              string            `json:"status"`
	Activated           bool              `json:"activated"`
	ActivatedTime       time.Time         `json:"activated_time"`
	RevisionOrderNumber int               `json:"order"`
	Tasks               buildStatusByTask `json:"tasks"`
	TimeTaken           time.Duration     `json:"time_taken"`
	DisplayName         string            `json:"name"`
	Requester           string            `json:"requester"`
}

type buildStatus struct {
	Id        string        `json:"task_id"`
	Status    string        `json:"status"`
	TimeTaken time.Duration `json:"time_taken"`
}

type buildStatusByTask map[string]buildStatus

// Returns a JSON response with the marshalled output of the build
// specified in the request.
func (restapi *restAPI) getBuildInfo(w http.ResponseWriter, r *http.Request) {
	buildId := mux.Vars(r)["build_id"]

	srcBuild, err := build.FindOne(build.ById(buildId))
	if err != nil || srcBuild == nil {
		msg := fmt.Sprintf("Error finding build '%v'", buildId)
		statusCode := http.StatusNotFound

		if err != nil {
			mci.Logger.Logf(slogger.ERROR, "%v: %v", msg, err)
			statusCode = http.StatusInternalServerError
		}

		restapi.WriteJSON(w, statusCode, responseError{Message: msg})
		return

	}

	destBuild := &restBuild{}
	// Copy the contents from the database into our local build type
	err = angier.TransferByFieldNames(srcBuild, destBuild)
	if err != nil {
		msg := fmt.Sprintf("Error finding build '%v'", buildId)
		mci.Logger.Logf(slogger.ERROR, "%v: %v", msg, err)
		restapi.WriteJSON(w, http.StatusInternalServerError, responseError{Message: msg})
		return

	}

	destBuild.Tasks = make(buildStatusByTask, len(srcBuild.Tasks))
	for _, task := range srcBuild.Tasks {
		status := buildStatus{
			Id:        task.Id,
			Status:    task.Status,
			TimeTaken: task.TimeTaken,
		}
		destBuild.Tasks[task.DisplayName] = status
	}

	restapi.WriteJSON(w, http.StatusOK, destBuild)
	return

}

// Returns a JSON response with the status of the specified build.
// The keys of the object are the task names.
func (restapi restAPI) getBuildStatus(w http.ResponseWriter, r *http.Request) {
	buildId := mux.Vars(r)["build_id"]

	b, err := build.FindOne(build.ById(buildId))
	if err != nil || b == nil {
		msg := fmt.Sprintf("Error finding build '%v'", buildId)
		statusCode := http.StatusNotFound

		if err != nil {
			mci.Logger.Logf(slogger.ERROR, "%v: %v", msg, err)
			statusCode = http.StatusInternalServerError
		}

		restapi.WriteJSON(w, statusCode, responseError{Message: msg})
		return

	}

	result := buildStatusContent{
		Id:           buildId,
		BuildVariant: b.BuildVariant,
		Tasks:        make(buildStatusByTask, len(b.Tasks)),
	}

	for _, task := range b.Tasks {
		status := buildStatus{
			Id:        task.Id,
			Status:    task.Status,
			TimeTaken: task.TimeTaken,
		}
		result.Tasks[task.DisplayName] = status
	}

	restapi.WriteJSON(w, http.StatusOK, result)
	return

}
