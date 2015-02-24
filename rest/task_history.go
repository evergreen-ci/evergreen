package rest

import (
	"10gen.com/mci"
	"10gen.com/mci/model"
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

func (restapi RESTAPI) getTaskHistory(w http.ResponseWriter, r *http.Request) {
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

		restapi.WriteJSON(w, statusCode, responseError{Message: msg})
		return

	}

	project, err := model.FindProject("", projectRef.Identifier, restapi.MCISettings.ConfigDir)
	if err != nil {
		msg := fmt.Sprintf("Error finding project '%v'", projectName)
		mci.Logger.Logf(slogger.ERROR, "%v: %v", msg, err)
		restapi.WriteJSON(w, http.StatusInternalServerError, responseError{Message: msg})
		return

	}

	buildVariants := project.GetVariantsWithTask(taskName)
	iter := model.NewTaskHistoryIterator(taskName, buildVariants, project.Name())

	chunk, err := iter.GetChunk(nil, MaxNumRevisions, NoRevisions, false)
	if err != nil {
		msg := fmt.Sprintf("Error finding history for task '%v'", taskName)
		mci.Logger.Logf(slogger.ERROR, "%v: %v", msg, err)
		restapi.WriteJSON(w, http.StatusInternalServerError, responseError{Message: msg})
		return

	}

	restapi.WriteJSON(w, http.StatusOK, chunk)
	return

}
