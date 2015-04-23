package rest

import (
	"10gen.com/mci"
	"10gen.com/mci/model"
	"10gen.com/mci/web"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/gorilla/mux"
	"net/http"
)

const (
	// Number of revisions to return in task history
	NoRevisions     = 0
	MaxNumRevisions = 10
)

func getTaskHistory(r *http.Request, mciSettings *mci.MCISettings) web.HTTPResponse {
	taskName := mux.Vars(r)["task_name"]
	projectName := r.FormValue("project")

	projectRef, err := model.FindOneProjectRef(projectName)
	if err != nil || projectRef == nil {
		msg := fmt.Sprintf("Error finding project '%v'", projectName)
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

	project, err := model.FindProject("", projectRef.Identifier, mciSettings.ConfigDir)
	if err != nil {
		msg := fmt.Sprintf("Error finding project '%v'", projectName)
		mci.Logger.Logf(slogger.ERROR, "%v: %v", msg, err)
		return web.JSONResponse{
			Data:       responseError{Message: msg},
			StatusCode: http.StatusInternalServerError,
		}
	}

	buildVariants := project.GetVariantsWithTask(taskName)
	iter := model.NewTaskHistoryIterator(taskName, buildVariants, project.Name())

	chunk, err := iter.GetChunk(nil, MaxNumRevisions, NoRevisions, false)
	if err != nil {
		msg := fmt.Sprintf("Error finding history for task '%v'", taskName)
		mci.Logger.Logf(slogger.ERROR, "%v: %v", msg, err)
		return web.JSONResponse{
			Data:       responseError{Message: msg},
			StatusCode: http.StatusInternalServerError,
		}
	}

	return web.JSONResponse{
		Data:       chunk,
		StatusCode: http.StatusOK,
	}
}
