package git

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/render"
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

func (hwp *HelloWorldPlugin) GetUIHandler() http.Handler {
	renderer := render.New(render.Options{})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// this handler returns the Id of the current user, as a sanity check that
		// proves plugins have access to user information.
		userId := "Not Logged In"
		u := plugin.GetUser(r)
		if u != nil {
			userId = u.Id
		}
		renderer.WriteJSON(w, http.StatusOK, struct {
			UserId string
		}{userId})
	})
}

func (self *HelloWorldPlugin) Configure(map[string]interface{}) error {
	return nil
}

// GetPanelConfig is required to fulfill the Plugin interface. This plugin
// does not have any UI hooks.
func (self *HelloWorldPlugin) GetPanelConfig() (*plugin.PanelConfig, error) {
	return &plugin.PanelConfig{
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
