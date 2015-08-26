package manifest

import (
	"fmt"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/mitchellh/mapstructure"
	"net/http"
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
		fmt.Errorf("GitHub token is empty")
	}
	m.OAuthCredentials = mp.GithubToken
	return nil
}
func (m *ManifestPlugin) GetUIHandler() http.Handler {
	return nil
}

func (m *ManifestPlugin) GetAPIHandler() http.Handler {
	r := http.NewServeMux()
	r.HandleFunc(fmt.Sprintf("/%v", ManifestLoadAPIEndpoint), m.ManifestLoadHandler)
	r.HandleFunc("/", http.NotFound) // 4                    04 any request not routable to these endpoints
	return r
}

// NewCommand returns requested commands by name. Fulfills the Plugin interface.
func (m *ManifestPlugin) NewCommand(cmdName string) (plugin.Command, error) {
	switch cmdName {
	case ManifestLoadCmd:
		return &ManifestLoadCommand{}, nil
	default:
		return nil, fmt.Errorf("No such %v command: %v", m.Name(), cmdName)
	}
}
