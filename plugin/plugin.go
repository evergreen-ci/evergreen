package plugin

var registered []Plugin

// Plugin defines the interface that all evergreen plugins must implement in order
// to register themselves with Evergreen. A plugin must also implement one of the
// PluginCommand or UIPlugin interfaces in order to do useful work.
//
// The Plugin interface is deprecated.
type Plugin interface {
	// Returns the name to identify this plugin when registered.
	Name() string

	// GetPanelConfig returns a pointer to a plugin's UI configuration.
	// or an error, if an error occur while trying to generate the config
	// A nil pointer represents a plugin without a UI presence, and is
	// not an error.
	GetPanelConfig() (*PanelConfig, error)

	// Configure reads in a settings map from the Evergreen config file.
	Configure(conf map[string]interface{}) error
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
func Publish(p Plugin) { registered = append(registered, p) }

func GetPublished() []Plugin { return registered }
