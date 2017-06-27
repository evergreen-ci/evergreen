package manifest

import (
	"encoding/json"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	ManifestLoadCmd    = "load"
	ManifestAttachCmd  = "attach"
	ManifestPluginName = "manifest"

	ManifestLoadAPIEndpoint = "load"
)

type manifestParams struct {
	GithubToken string `mapstructure:"github_token"`
}

// ManifestPlugin handles the creation of a Build Manifest associated with a version.
type ManifestPlugin struct {
	OAuthCredentials string
}

func init() {
	plugin.Publish(&ManifestPlugin{})
}

// Name returns the name of this plugin - satisfies 'Plugin' interface
func (m *ManifestPlugin) Name() string {
	return ManifestPluginName
}

func (m *ManifestPlugin) Configure(conf map[string]interface{}) error {
	mp := &manifestParams{}
	err := mapstructure.Decode(conf, mp)
	if err != nil {
		return err
	}
	if mp.GithubToken == "" {
		// this should return an error, but never has before
		// (caught an lint error.) Tests break if this returns early.
		grip.Warning(errors.New("GitHub token is empty"))
	}
	m.OAuthCredentials = mp.GithubToken
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

// required to satisfy UI interface, but the route required by this
// plugin is mounted directly as part of the service package.
func (m *ManifestPlugin) GetUIHandler() http.Handler { return nil }

// NewCommand returns requested commands by name. Fulfills the Plugin interface.
func (m *ManifestPlugin) NewCommand(cmdName string) (plugin.Command, error) {
	switch cmdName {
	case ManifestLoadCmd:
		return &ManifestLoadCommand{}, nil
	default:
		return nil, errors.Errorf("No such %v command: %v", m.Name(), cmdName)
	}
}
