package plugin

import (
	"encoding/json"

	"github.com/evergreen-ci/evergreen/model/manifest"
)

// ManifestPlugin handles the creation of a Build Manifest associated with a version.
type ManifestPlugin struct{}

func init() {
	Publish(&ManifestPlugin{})
}

// Name returns the name of this plugin - satisfies 'Plugin' interface
func (m *ManifestPlugin) Name() string                           { return "manifest" }
func (m *ManifestPlugin) Configure(map[string]interface{}) error { return nil }

// GetPanelConfig returns a pointer to a plugin's UI configuration.
// or an error, if an error occur while trying to generate the config
// A nil pointer represents a plugin without a UI presence, and is
// not an error.
// GetPanelConfig returns a plugin.PanelConfig struct representing panels
// that will be added to the Version page.
func (m *ManifestPlugin) GetPanelConfig() (*PanelConfig, error) {
	return &PanelConfig{
		Panels: []UIPanel{
			{
				Page:      VersionPage,
				Position:  PageRight,
				PanelHTML: "<div ng-include=\"'/static/plugins/manifest/partials/version_manifest_panel.html'\"></div>",
				DataFunc: func(context UIContext) (interface{}, error) {
					if context.Version == nil {
						return nil, nil
					}
					currentManifest, err := manifest.FindFromVersion(context.Version.Id, context.Version.Identifier, context.Version.Revision, context.Version.Requester)
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
