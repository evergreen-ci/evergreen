package taskdata

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/gorilla/mux"
)

func init() {
	plugin.Publish(&TaskJSONPlugin{})
}

const (
	TaskJSONPluginName = "json"
	TaskJSONSend       = "send"
	TaskJSONGet        = "get"
	TaskJSONGetHistory = "get_history"
	TaskJSONHistory    = "history"
)

// TaskJSONPlugin handles thet
type TaskJSONPlugin struct{}

// Name implements Plugin Interface.
func (jsp *TaskJSONPlugin) Name() string {
	return TaskJSONPluginName
}

func (hwp *TaskJSONPlugin) GetUIHandler() http.Handler {
	r := mux.NewRouter()

	// version routes
	r.HandleFunc("/version", getVersion)
	r.HandleFunc("/version/{version_id}/{name}/", uiGetTasksForVersion)
	r.HandleFunc("/version/latest/{project_id}/{name}", uiGetTasksForLatestVersion)

	// task routes
	r.HandleFunc("/task/{task_id}/{name}/", uiGetTaskById)
	r.HandleFunc("/task/{task_id}/{name}/tags", uiGetTags)
	r.HandleFunc("/task/{task_id}/{name}/tag", uiHandleTaskTag).Methods("POST", "DELETE")

	r.HandleFunc("/tag/{project_id}/{tag}/{variant}/{task_name}/{name}", uiGetTaskJSONByTag)
	r.HandleFunc("/commit/{project_id}/{revision}/{variant}/{task_name}/{name}", uiGetCommit)
	r.HandleFunc("/history/{task_id}/{name}", uiGetTaskHistory)
	return r
}

func (jsp *TaskJSONPlugin) Configure(map[string]interface{}) error {
	return nil
}

// GetPanelConfig is required to fulfill the Plugin interface. This plugin
// does not have any UI hooks.
func (jsp *TaskJSONPlugin) GetPanelConfig() (*plugin.PanelConfig, error) {
	return &plugin.PanelConfig{}, nil
}
