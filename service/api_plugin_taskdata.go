package service

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
)

func (as *APIServer) insertTaskJSON(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)

	name := gimlet.GetVars(r)["name"]
	rawData := map[string]interface{}{}

	if err := utility.ReadJSON(utility.NewRequestReader(r), &rawData); err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	if err := model.InsertTaskJSON(t, name, rawData); err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	gimlet.WriteJSON(w, "ok")
}
