package service

import (
	"fmt"
	"io"
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)


// setRevision sets the latest revision in the Repository
// database to the revision sent from the projects page.
func (uis *UIServer) setRevision(w http.ResponseWriter, r *http.Request) {
	MustHaveUser(r)

	project := gimlet.GetVars(r)["project_id"]

	body := utility.NewRequestReader(r)
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		uis.LoggedError(w, r, http.StatusNotFound, err)
		return
	}

	revision := string(data)
	if revision == "" {
		uis.LoggedError(w, r, http.StatusBadRequest, errors.Errorf("revision sent was empty"))
		return
	}

	projectRef, err := model.FindBranchProjectRef(r.Context(), project)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	// update the latest revision to be the revision id
	err = model.UpdateLastRevision(r.Context(), projectRef.Id, revision)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	if err := projectRef.SetRepotrackerError(r.Context(), &model.RepositoryErrorDetails{}); err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	// run the repotracker for the project
	ts := utility.RoundPartOfHour(1).Format(units.TSFormat)
	j := units.NewRepotrackerJob(fmt.Sprintf("catchup-%s", ts), projectRef.Id)
	if err := uis.queue.Put(r.Context(), j); err != nil {
		grip.Error(errors.Wrap(err, "problem creating catchup job from UI"))
	}
	gimlet.WriteJSON(w, nil)
}

func (uis *UIServer) projectEvents(w http.ResponseWriter, r *http.Request) {
	// Validate the project exists
	id := gimlet.GetVars(r)["project_id"]
	projectRef, err := model.FindBranchProjectRef(r.Context(), id)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	if projectRef == nil {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}

	DBUser := MustHaveUser(r)
	template := "not_admin.html"
	if isAdmin(DBUser, projectRef.Id) {
		template = "project_events.html"
	}

	spruceLink := fmt.Sprintf("%s/project/%s/settings/event-log", uis.Settings.Ui.UIv2Url, id)
	newUILink := ""
	if len(uis.Settings.Ui.UIv2Url) > 0 {
		newUILink = spruceLink
	}

	data := struct {
		Project string
		ViewData
		NewUILink string
	}{id, uis.GetCommonViewData(w, r, true, true), newUILink}
	uis.render.WriteResponse(w, http.StatusOK, data, "base", template, "base_angular.html", "menu.html")
}
