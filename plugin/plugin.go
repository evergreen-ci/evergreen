package plugin

import (
	"fmt"
	"net/http"
)

var (
	// These are slices of all plugins that have made themselves
	// visible to the Evergreen system. A Plugin can add itself by appending an instance
	// of itself to these slices on init, i.e. by adding the following to its
	// source file:
	//  func init(){
	//	plugin.Publish(&MyCoolPlugin{})
	//  }
	// This list is then used by Agent, API, and UI Server code to register
	// the published plugins.
	UIPlugins []UIPlugin
)

type pluginTaskContext int

const pluginTaskContextKey pluginTaskContext = 0

// Plugin defines the interface that all evergreen plugins must implement in order
// to register themselves with Evergreen. A plugin must also implement one of the
// PluginCommand or UIPlugin interfaces in order to do useful work.
type Plugin interface {
	// Returns the name to identify this plugin when registered.
	Name() string
}

type UIPlugin interface {
	Plugin

	// Install any server-side handlers needed by this plugin in the UI server
	GetUIHandler() http.Handler

	// GetPanelConfig returns a pointer to a plugin's UI configuration.
	// or an error, if an error occur while trying to generate the config
	// A nil pointer represents a plugin without a UI presence, and is
	// not an error.
	GetPanelConfig() (*PanelConfig, error)

	// Configure reads in a settings map from the Evergreen config file.
	Configure(conf map[string]interface{}) error
}

// AppUIPlugin represents a UIPlugin that also has a page route.
type AppUIPlugin interface {
	UIPlugin

	// GetAppPluginInfo returns all the information
	// needed for the UI server to render a page from the navigation bar.
	GetAppPluginInfo() *UIPage
}

// Publish is called in a plugin's "init" func to
// announce that plugin's presence to the entire plugin package.
// This architecture is designed to make it easy to add
// new external plugin code to Evergreen by simply importing the
// new plugin's package in plugin/config/installed_plugins.go
//
// Packages implementing the Plugin interface MUST call Publish in their
// init code in order for Evergreen to detect and use them. A plugin must
// also implement one of CommandPlugin or UIPlugin in order to
// be useable.
//
// See the documentation of the 10gen.com/mci/plugin/config package for more
func Publish(plugin Plugin) {
	published := false

	if asUI, ok := plugin.(UIPlugin); ok {
		UIPlugins = append(UIPlugins, asUI)
		published = true
	}
	if !published {
		panic(fmt.Sprintf("Plugin '%v' does not implement any of CommandPlugin, or UIPlugin", plugin.Name()))
	}
}
