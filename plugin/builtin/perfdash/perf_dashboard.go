package perfdash

import (
	"fmt"
	"html/template"
	"io/ioutil"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
)

const (
	perfDashboardPluginName = "dashboard"
)

func init() {
	plugin.Publish(&PerfDashboardPlugin{})
}

var includes = []template.HTML{
	`<script type="text/javascript" src="/plugin/dashboard/static/js/dashboard.js"></script>`,
	`<link href="/plugin/dashboard/static/css/dashboard.css" rel="stylesheet"/>`,
}

// PerfDashboardPlugin displays performance statistics in the UI.
// Branches is a map of the branch name to the list of project names
// associated with that branch.
type PerfDashboardPlugin struct {
	Branches map[string][]string `yaml:"branches"`
}

// DashboardAppData is the data that is returned from calling the app level data function
// Branches is a mapping from a branch name to the projects in that branch
// DefaultBranch is the branch that should show up if it exists.
// DefaultBaselines is a mapping from project to baseline that is default
type DashboardAppData struct {
	Branches         map[string][]string `json:"branches"`
	DefaultBranch    string              `json:"default_branch"`
	DefaultBaselines map[string]string   `json:"default_baselines"`
}

// Name implements Plugin Interface.
func (pdp *PerfDashboardPlugin) Name() string { return perfDashboardPluginName }

func (pdp *PerfDashboardPlugin) GetAppPluginInfo() *plugin.UIPage {
	data := func(context plugin.UIContext) (interface{}, error) {
		defaultBranch := context.Request.FormValue("branch")
		defaultBaselines := map[string]string{}
		for _, projects := range pdp.Branches {
			for _, projectName := range projects {
				defaultBaselines[projectName] = context.Request.FormValue(projectName)
			}
		}
		dashboardData := DashboardAppData{
			DefaultBranch:    defaultBranch,
			DefaultBaselines: defaultBaselines,
			Branches:         pdp.Branches,
		}

		return dashboardData, nil
	}
	return &plugin.UIPage{"perf_dashboard.html", data}
}

func (pdp *PerfDashboardPlugin) Configure(params map[string]interface{}) error {
	err := mapstructure.Decode(params, pdp)
	if err != nil {
		return fmt.Errorf("error decoding %v params: %v", pdp.Name(), err)
	}
	return nil
}

func (pdp *PerfDashboardPlugin) GetPanelConfig() (*plugin.PanelConfig, error) {
	dashboardHTML, err := ioutil.ReadFile(filepath.Join(plugin.TemplateRoot(pdp.Name()), "version_perf_dashboard.html"))
	if err != nil {
		return nil, fmt.Errorf("Can't load version panel file html %v", err)
	}
	return &plugin.PanelConfig{
		Panels: []plugin.UIPanel{
			{
				Includes:  includes,
				Page:      plugin.VersionPage,
				Position:  plugin.PageCenter,
				PanelHTML: template.HTML(dashboardHTML),
				DataFunc: func(context plugin.UIContext) (interface{}, error) {
					exists := false
					for _, projects := range pdp.Branches {
						if util.StringSliceContains(projects, context.ProjectRef.Identifier) {
							exists = true
							break
						}
					}
					return struct {
						Enabled bool `json:"enabled"`
					}{exists}, nil
				},
			},
		},
	}, nil
}
