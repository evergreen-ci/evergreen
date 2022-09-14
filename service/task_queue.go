package service

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/gimlet"
)

func (uis *UIServer) taskQueue(w http.ResponseWriter, r *http.Request) {
	distro := gimlet.GetVars(r)["distro"]
	taskId := gimlet.GetVars(r)["task_id"]
	newUILink := ""
	if len(uis.Settings.Ui.UIv2Url) > 0 {
		newUILink = fmt.Sprintf("%s/task-queue/%s/%s", uis.Settings.Ui.UIv2Url, distro, taskId)
	}

	http.Redirect(w, r, newUILink, http.StatusTemporaryRedirect)
}
