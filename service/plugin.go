package service

import (
	"bytes"
	"io"
	"net/http"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/plugin"
)

// pluginHTML the given set of template files and executes the template named entryPoint against
// the context data, writing the result to out. Returns error if the template could not be
// loaded or if executing the template failed.
func (uis *UIServer) pluginHTML(out io.Writer, data interface{}, entryPoint, pluginName, pluginFile string, files ...string) error {

	// check if the template is cached and execute cached template if so.
	if uis.Settings.Ui.CacheTemplates {
		if templ, ok := uis.PluginTemplates[pluginName]; ok {
			return templ.ExecuteTemplate(out, entryPoint, data)
		}
	}

	t, err := uis.GetHTMLTemplate(files...)
	if err != nil {
		return err
	}
	newTemplate, err := t.Clone()
	if err != nil {
		return err
	}

	newTemplate, err = newTemplate.ParseFiles(pluginFile)
	if err != nil {
		return err
	}

	// cache the template if necessary
	if uis.Settings.Ui.CacheTemplates {
		uis.PluginTemplates[pluginName] = newTemplate
	}
	err = newTemplate.ExecuteTemplate(out, entryPoint, data)
	return err
}

// PluginWriteHTML calls HTML() on its args and writes the output to the response with the given status.
// If the template can't be loaded or executed, the status is set to 500 and error details
// are written to the response body.
func (uis *UIServer) PluginWriteHTML(w http.ResponseWriter, status int, data interface{}, entryPoint, pluginName, pluginFile string, files ...string) {
	out := &bytes.Buffer{}
	err := uis.pluginHTML(out, data, entryPoint, pluginName, pluginFile, files...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	w.WriteHeader(status)
	w.Write(out.Bytes())
}

// GetPluginHandler returns a handler function given the template route and data to go to that page.
func (uis *UIServer) GetPluginHandler(uiPage *plugin.UIPage, pluginName string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		projCtx := MustHaveProjectContext(r)
		u := GetUser(r)
		pluginCtx := plugin.UIContext{
			Settings:   uis.Settings,
			User:       u,
			Task:       projCtx.Task,
			Build:      projCtx.Build,
			Version:    projCtx.Version,
			Patch:      projCtx.Patch,
			Project:    projCtx.Project,
			ProjectRef: projCtx.ProjectRef,
		}
		pluginData, err := uiPage.DataFunc(pluginCtx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data := struct {
			Data        interface{}
			User        *user.DBUser
			ProjectData projectContext
		}{pluginData, u, projCtx}

		pluginTemplatePath := filepath.Join(plugin.TemplateRoot(pluginName), uiPage.TemplatePath)
		uis.PluginWriteHTML(w, http.StatusOK, data, "base", pluginName, pluginTemplatePath, "base_angular.html", "menu.html")
	}
}
