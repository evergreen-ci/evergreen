package manifest

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
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
					ver, err := context.Metadata.GetVersion()
					if err != nil || ver == nil {
						return nil, nil
					}

					currentManifest, err := manifest.FindOne(manifest.ById(ver.Id))
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

func (m *ManifestPlugin) GetUIHandler() http.Handler {
	r := mux.NewRouter()
	r.Path("/get/{project_id}/{revision}").HandlerFunc(m.GetManifest)
	return r
}

func (m *ManifestPlugin) GetManifest(w http.ResponseWriter, r *http.Request) {
	project := mux.Vars(r)["project_id"]
	revision := mux.Vars(r)["revision"]

	version, err := version.FindOne(version.ByProjectIdAndRevision(project, revision))
	if err != nil {
		http.Error(w, fmt.Sprintf("error getting version for project %v with revision %v: %v",
			project, revision, err), http.StatusBadRequest)
		return
	}
	if version == nil {
		http.Error(w, fmt.Sprintf("version not found for project %v, with revision %v", project, revision),
			http.StatusNotFound)
		return
	}

	foundManifest, err := manifest.FindOne(manifest.ById(version.Id))
	if err != nil {
		http.Error(w, fmt.Sprintf("error getting manifest with version id %v: %v",
			version.Id, err), http.StatusBadRequest)
		return
	}
	if foundManifest == nil {
		http.Error(w, fmt.Sprintf("manifest not found for version %v", version.Id), http.StatusNotFound)
		return
	}
	util.WriteJSON(w, http.StatusOK, foundManifest)
}
