package service

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/gimlet"
)

// GetPluginHandler returns a handler function given the template route and data to go to that page.
func (uis *UIServer) GetPluginHandler(uiPage *plugin.UIPage, pluginName string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		projCtx := MustHaveProjectContext(r)
		u := gimlet.GetUser(r.Context())
		pluginCtx := plugin.UIContext{
			Settings:   uis.Settings,
			User:       u,
			Task:       projCtx.Task,
			Build:      projCtx.Build,
			Version:    projCtx.Version,
			Patch:      projCtx.Patch,
			ProjectRef: projCtx.ProjectRef,
			Request:    r,
		}
		pluginData, err := uiPage.DataFunc(pluginCtx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data := struct {
			Data interface{}
			ViewData
		}{pluginData, uis.GetCommonViewData(w, r, false, true)}

		uis.render.WriteResponse(w, http.StatusOK, data, "base", "base_angular.html", "menu.html")
	}
}
