package archive

import (
	"10gen.com/mci/plugin"
	"net/http"
)

func init() {
	plugin.Publish(&ArchivePlugin{})
}

const (
	TarGzPackCmdName   = "targz_pack"
	TarGzUnpackCmdName = "targz_unpack"
	ArchivePluginName  = "archive"
)

// ArchivePlugin holds commands for creating archives and extracting
// their contents during a task.
type ArchivePlugin struct{}

// Name returns the name of the plugin. Fulfills the Plugin interface.
func (self *ArchivePlugin) Name() string {
	return ArchivePluginName
}

// GetRoutes is needed to fulfill the Plugin interface. ArchivePlugin
// does not register any routes.
func (self *ArchivePlugin) GetAPIHandler() http.Handler {
	return nil
}

func (self *ArchivePlugin) GetUIHandler() http.Handler {
	return nil
}

func (self *ArchivePlugin) Configure(map[string]interface{}) error {
	return nil
}

// GetPanelConfig is needed to fulfill the Plugin interface, it
// does nothing here.
func (self *ArchivePlugin) GetPanelConfig() (*plugin.PanelConfig, error) {
	return nil, nil
}

// NewCommand takes a command name as a string and returns the requested command,
// or an error if the command does not exist. Fulfills the Plugin interface.
func (self *ArchivePlugin) NewCommand(cmdName string) (plugin.Command, error) {
	if cmdName == TarGzPackCmdName {
		return &TarGzPackCommand{}, nil
	}
	if cmdName == TarGzUnpackCmdName {
		return &TarGzUnpackCommand{}, nil
	}
	return nil, &plugin.ErrUnknownCommand{cmdName}
}
