package service

import (
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

// Returns a JSON response with the marshaled output of the build
// specified in the request.
func (restapi *restAPI) getBuildInfo(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveRESTContext(r)
	b, _ := projCtx.GetBuild()
	if b == nil {
		restapi.WriteJSON(w, http.StatusNotFound, responseError{Message: "error finding build"})
		return
	}

	destBuild := &restBuild{}
	destBuild.Id = b.Id
	destBuild.CreateTime = b.CreateTime
	destBuild.StartTime = b.StartTime
	destBuild.FinishTime = b.FinishTime
	destBuild.PushTime = b.PushTime
	destBuild.Version = b.Version
	destBuild.Project = b.Project
	destBuild.Revision = b.Revision
	destBuild.BuildVariant = b.BuildVariant
	destBuild.BuildNumber = b.BuildNumber
	destBuild.Status = b.Status
	destBuild.Activated = b.Activated
	destBuild.ActivatedTime = b.ActivatedTime
	destBuild.RevisionOrderNumber = b.RevisionOrderNumber
	destBuild.TimeTaken = b.TimeTaken
	destBuild.DisplayName = b.DisplayName
	destBuild.Requester = b.Requester

	destBuild.Tasks = make(buildStatusByTask, len(b.Tasks))
	for _, task := range b.Tasks {
		status := buildStatus{
			Id:        task.Id,
			Status:    task.Status,
			TimeTaken: task.TimeTaken,
		}
		destBuild.Tasks[task.DisplayName] = status
	}

	restapi.WriteJSON(w, http.StatusOK, destBuild)
}

// Returns a JSON response with the status of the specified build.
// The keys of the object are the task names.
func (restapi restAPI) getBuildStatus(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveRESTContext(r)
	b, _ := projCtx.GetBuild()
	if b == nil {
		restapi.WriteJSON(w, http.StatusNotFound, responseError{Message: "error finding build"})
		return
	}

	result := buildStatusContent{
		Id:           b.Id,
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
}
