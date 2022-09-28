package service

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/gimlet"
)

func (uis *UIServer) podPage(w http.ResponseWriter, r *http.Request) {
	id := gimlet.GetVars(r)["pod_id"]

	p, err := pod.FindOneByID(id)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	if p == nil {
		http.Error(w, "pod not found", http.StatusNotFound)
		return
	}

	if RedirectSpruceUsers(w, r, fmt.Sprintf("%s/pod/%s", uis.Settings.Ui.UIv2Url, id)) {
		return
	}

	http.Error(w, "user is not opted in to automatic redirects to Spruce and pod page is not supported in legacy UI", http.StatusNotImplemented)
}
