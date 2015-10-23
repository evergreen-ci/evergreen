package ui

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
	MaxRestNumRevisions = 10
)

func (restapi restAPI) getTaskHistory(w http.ResponseWriter, r *http.Request) {
	taskName := mux.Vars(r)["task_name"]
	projCtx := MustHaveProjectContext(r)
	project := projCtx.Project
	if project == nil {
		restapi.WriteJSON(w, http.StatusInternalServerError, responseError{Message: "error loading project"})
		return
	}

	buildVariants := project.GetVariantsWithTask(taskName)
	iter := model.NewTaskHistoryIterator(taskName, buildVariants, project.Identifier)

	chunk, err := iter.GetChunk(nil, MaxRestNumRevisions, NoRevisions, false)
	if err != nil {
		msg := fmt.Sprintf("Error finding history for task '%v'", taskName)
		evergreen.Logger.Logf(slogger.ERROR, "%v: %v", msg, err)
		restapi.WriteJSON(w, http.StatusInternalServerError, responseError{Message: msg})
		return
	}

	restapi.WriteJSON(w, http.StatusOK, chunk)
	return

}
