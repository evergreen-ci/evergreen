package git

import (
	"fmt"
	"github.com/evergreen-ci/evergreen/plugin"
	"net/http"
)

func init() {
	plugin.Publish(&HelloWorldPlugin{})
}

// GitPlugin handles fetching source code and applying patches
// using the git version control system.
type HelloWorldPlugin struct{}

// Name implements Plugin Interface.
func (self *HelloWorldPlugin) Name() string {
	return "helloworld"
}

// GetRoutes returns an API route for serving patch data.
func (self *HelloWorldPlugin) GetAPIHandler() http.Handler {
	r := http.NewServeMux()
	r.HandleFunc("/cmd1", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("cmd1 got called with path:", r.URL.Path)
	})
	r.HandleFunc("/cmd2", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("cmd2 got called with path:", r.URL.Path)
	})
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("hello world: command not found!")
		http.Error(w, "not found", http.StatusNotFound)
	})
	return r

}

func (hwp *HelloWorldPlugin) GetUIHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
}

func (self *HelloWorldPlugin) Configure(map[string]interface{}) error {
	return nil
}

// GetPanelConfig is required to fulfill the Plugin interface. This plugin
// does not have any UI hooks.
func (self *HelloWorldPlugin) GetPanelConfig() (*plugin.PanelConfig, error) {
	return &plugin.PanelConfig{
		StaticRoot: plugin.StaticWebRootFromSourceFile(),
		Panels: []plugin.UIPanel{
			{
				Page:      plugin.TaskPage,
				Position:  plugin.PageCenter,
				PanelHTML: "<!--hello world!-->",
				DataFunc: func(context plugin.UIContext) (interface{}, error) {
					return map[string]interface{}{}, nil
				},
			},
		},
	}, nil

	return nil, nil
}

// NewCommand returns requested commands by name. Fulfills the Plugin interface.
func (self *HelloWorldPlugin) NewCommand(cmdName string) (plugin.Command, error) {
	return nil, &plugin.ErrUnknownCommand{cmdName}
}
