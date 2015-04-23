package rest

import (
	"10gen.com/mci"
	"10gen.com/mci/model"
	"10gen.com/mci/web"
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

type build struct {
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
func getBuildInfo(r *http.Request) web.HTTPResponse {
	buildId := mux.Vars(r)["build_id"]

	srcBuild, err := model.FindBuild(buildId)
	if err != nil || srcBuild == nil {
		msg := fmt.Sprintf("Error finding build '%v'", buildId)
		statusCode := http.StatusNotFound

		if err != nil {
			mci.Logger.Logf(slogger.ERROR, "%v: %v", msg, err)
			statusCode = http.StatusInternalServerError
		}

		return web.JSONResponse{
			Data:       responseError{Message: msg},
			StatusCode: statusCode,
		}
	}

	destBuild := &build{}
	// Copy the contents from the database into our local build type
	err = angier.TransferByFieldNames(srcBuild, destBuild)
	if err != nil {
		msg := fmt.Sprintf("Error finding build '%v'", buildId)
		mci.Logger.Logf(slogger.ERROR, "%v: %v", msg, err)
		return web.JSONResponse{
			Data:       responseError{Message: msg},
			StatusCode: http.StatusInternalServerError,
		}
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

	return web.JSONResponse{
		Data:       destBuild,
		StatusCode: http.StatusOK,
	}
}

// Returns a JSON response with the status of the specified build.
// The keys of the object are the task names.
func getBuildStatus(r *http.Request) web.HTTPResponse {
	buildId := mux.Vars(r)["build_id"]

	build, err := model.FindBuild(buildId)
	if err != nil || build == nil {
		msg := fmt.Sprintf("Error finding build '%v'", buildId)
		statusCode := http.StatusNotFound

		if err != nil {
			mci.Logger.Logf(slogger.ERROR, "%v: %v", msg, err)
			statusCode = http.StatusInternalServerError
		}

		return web.JSONResponse{
			Data:       responseError{Message: msg},
			StatusCode: statusCode,
		}
	}

	result := buildStatusContent{
		Id:           buildId,
		BuildVariant: build.BuildVariant,
		Tasks:        make(buildStatusByTask, len(build.Tasks)),
	}

	for _, task := range build.Tasks {
		status := buildStatus{
			Id:        task.Id,
			Status:    task.Status,
			TimeTaken: task.TimeTaken,
		}
		result.Tasks[task.DisplayName] = status
	}

	return web.JSONResponse{
		Data:       result,
		StatusCode: http.StatusOK,
	}
}
