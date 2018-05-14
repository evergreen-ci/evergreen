package manifest

import (
	"encoding/json"

	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/plugin"
)

const (
	ManifestLoadCmd    = "load"
	ManifestAttachCmd  = "attach"
	ManifestPluginName = "manifest"

	ManifestLoadAPIEndpoint = "load"
)

// ManifestPlugin handles the creation of a Build Manifest associated with a version.
type ManifestPlugin struct{}

func init() {
	plugin.Publish(&ManifestPlugin{})
}

// Name returns the name of this plugin - satisfies 'Plugin' interface
func (m *ManifestPlugin) Name() string {
	return ManifestPluginName
}

func (m *ManifestPlugin) Configure(conf map[string]interface{}) error {
	return nil
}

// GetPanelConfig returns a pointer to a plugin's UI configuration.
// or an error, if an error occur while trying to generate the config
// A nil pointer represents a plugin without a UI presence, and is
// not an error.
// GetPanelConfig returns a plugin.PanelConfig struct representing panels
// that will be added to the Version page.
func (m *ManifestPlugin) GetPanelConfig() (*plugin.PanelConfig, error) {
	return &plugin.PanelConfig{
		Panels: []plugin.UIPanel{
			{
				Page:      plugin.VersionPage,
				Position:  plugin.PageRight,
				PanelHTML: "<div ng-include=\"'/plugin/manifest/static/partials/version_manifest_panel.html'\"></div>",
				DataFunc: func(context plugin.UIContext) (interface{}, error) {
					if context.Version == nil {
						return nil, nil
					}
					currentManifest, err := manifest.FindOne(manifest.ById(context.Version.Id))
					if err != nil {
						return nil, err
					}
					if currentManifest == nil {
						return nil, nil
					}
					prettyManifest, err := json.MarshalIndent(currentManifest, "", "  ")
					if err != nil {
						return nil, err
					}
					return string(prettyManifest), nil
				},
			},
		},
	}, nil
}
