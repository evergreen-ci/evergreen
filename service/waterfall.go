package service

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/evergreen-ci/evergreen/model"
)

const (
	InactiveStatus         = "inactive"
	WaterfallPerPageLimit  = 5
	WaterfallBVFilterParam = "bv_filter"
	WaterfallSkipParam     = "skip"
)

// Pull the skip value out of the http request
func skipValue(r *http.Request) (int, error) {
	// determine how many versions to skip
	toSkipStr := r.FormValue(WaterfallSkipParam)
	if toSkipStr == "" {
		toSkipStr = "0"
	}
	return strconv.Atoi(toSkipStr)
}

// Create and return the waterfall data we need to render the page.
// Http handler for the waterfall page
func (uis *UIServer) waterfallPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	project, err := projCtx.GetProject()
	if err != nil || project == nil {
		uis.ProjectNotFound(projCtx, w, r)
		return
	}

	skip, err := skipValue(r)
	if err != nil {
		skip = 0
	}

	variantQuery := strings.TrimSpace(r.URL.Query().Get(WaterfallBVFilterParam))

	// first, get all of the versions and variants we will need
	vvData, err := model.GetWaterfallVersionsAndVariants(
		skip, WaterfallPerPageLimit, project, variantQuery,
	)

	if err != nil {
		uis.LoggedError(w, r, http.StatusNotFound, err)
		return
	}

	finalData, err := model.WaterfallDataAdaptor(
		vvData, project, skip, WaterfallPerPageLimit, variantQuery,
	)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	uis.render.WriteResponse(w, http.StatusOK, struct {
		Data     model.WaterfallData
		JiraHost string
		ViewData
	}{finalData, uis.Settings.Jira.Host, uis.GetCommonViewData(w, r, false, true)}, "base", "waterfall.html", "base_angular.html", "menu.html")
}
