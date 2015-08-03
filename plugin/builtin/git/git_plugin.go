package git

import (
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/gorilla/mux"
	"net/http"
)

func init() {
	plugin.Publish(&GitPlugin{})
}

const (
	GetProjectCmdName = "get_project"
	ApplyPatchCmdName = "apply_patch"
	GitPluginName     = "git"

	GitPatchPath     = "patch"
	GitPatchFilePath = "patchfile"
)

// GitPlugin handles fetching source code and applying patches
// using the git version control system.
type GitPlugin struct{}

// Name implements Plugin Interface.
func (self *GitPlugin) Name() string {
	return GitPluginName
}

func (self *GitPlugin) GetAPIHandler() http.Handler {
	r := mux.NewRouter()
	r.Path("/" + GitPatchFilePath + "/{patchfile_id}").Methods("GET").HandlerFunc(servePatchFile)
	r.HandleFunc("/"+GitPatchPath, servePatch) // GET
	r.HandleFunc("/", http.NotFound)
	return r
}

func (self *GitPlugin) GetUIHandler() http.Handler {
	return nil
}

func (self *GitPlugin) Configure(map[string]interface{}) error {
	return nil
}

// GetPanelConfig is required to fulfill the Plugin interface. This plugin
// does not have any UI hooks.
func (self *GitPlugin) GetPanelConfig() (*plugin.PanelConfig, error) {
	return nil, nil
}

// NewCommand returns requested commands by name. Fulfills the Plugin interface.
func (self *GitPlugin) NewCommand(cmdName string) (plugin.Command, error) {
	switch cmdName {
	case GetProjectCmdName:
		return &GitGetProjectCommand{}, nil
	case ApplyPatchCmdName:
		return &GitApplyPatchCommand{}, nil
	default:
		return nil, &plugin.ErrUnknownCommand{cmdName}
	}
}
