package rest

import (
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/gorilla/mux"
	"net/http"
)

const (
	// Number of revisions to return in task history
	NoRevisions     = 0
	MaxNumRevisions = 10
)

func (restapi restAPI) getTaskHistory(w http.ResponseWriter, r *http.Request) {
	taskName := mux.Vars(r)["task_name"]
	projectName := r.FormValue("project")

	projectRef, err := model.FindOneProjectRef(projectName)
	if err != nil || projectRef == nil {
		msg := fmt.Sprintf("Error finding project '%v'", projectName)
		statusCode := http.StatusNotFound

		if err != nil {
			evergreen.Logger.Logf(slogger.ERROR, "%v: %v", msg, err)
			statusCode = http.StatusInternalServerError
		}

		restapi.WriteJSON(w, statusCode, responseError{Message: msg})
		return
	}

	project, err := model.FindProject("", projectRef)
	if err != nil {
		msg := fmt.Sprintf("Error finding project '%v'", projectName)
		evergreen.Logger.Logf(slogger.ERROR, "%v: %v", msg, err)
		restapi.WriteJSON(w, http.StatusInternalServerError, responseError{Message: msg})
		return
	}

	buildVariants := project.GetVariantsWithTask(taskName)
	iter := model.NewTaskHistoryIterator(taskName, buildVariants, project.Identifier)

	chunk, err := iter.GetChunk(nil, MaxNumRevisions, NoRevisions, false)
	if err != nil {
		msg := fmt.Sprintf("Error finding history for task '%v'", taskName)
		evergreen.Logger.Logf(slogger.ERROR, "%v: %v", msg, err)
		restapi.WriteJSON(w, http.StatusInternalServerError, responseError{Message: msg})
		return

	}

	restapi.WriteJSON(w, http.StatusOK, chunk)
	return

}
