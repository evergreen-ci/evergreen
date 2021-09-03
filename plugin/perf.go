package plugin

import (
	"fmt"
	"html/template"
	"io/ioutil"
	"path/filepath"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
)

func init() {
	Publish(&PerfPlugin{})
}

// PerfPlugin displays performance statistics in the UI.
type PerfPlugin struct {
	Projects []string `yaml:"string"`
}

// Name implements Plugin Interface.
func (pp *PerfPlugin) Name() string { return "perf" }

func (pp *PerfPlugin) Configure(params map[string]interface{}) error {
	err := mapstructure.Decode(params, pp)
	if err != nil {
		return fmt.Errorf("error decoding %v params: %v", pp.Name(), err)
	}
	return nil
}

func (pp *PerfPlugin) GetPanelConfig() (*PanelConfig, error) {
	panelHTML, err := ioutil.ReadFile(filepath.Join(TemplateRoot(pp.Name()), "task_perf_data.html"))
	if err != nil {
		return nil, fmt.Errorf("Can't load panel html file: %v", err)
	}

	return &PanelConfig{
		Panels: []UIPanel{
			{
				Includes: []template.HTML{
					`<script type="text/javascript" src="/static/app/perf/trend_chart.js"></script>`,
					`<script type="text/javascript" src="/static/app/perf/perf.js"></script>`,
					`<script type="text/javascript" src="/static/app/common/ApiUtil.js"></script>`,
					`<script type="text/javascript" src="/static/app/common/ApiTaskdata.js"></script>`,
					`<script type="text/javascript" src="/static/app/perf/PerfChartService.js"></script>`,
					`<script type="text/javascript" src="/static/app/perf/TrendSamples.js"></script>`,
					`<script type="text/javascript" src="/static/app/perf/TestSample.js"></script>`,
					`<script type="text/javascript" src="/static/thirdparty/numeral.js"></script>`,
				},
				Page:      TaskPage,
				Position:  PageCenter,
				PanelHTML: template.HTML(panelHTML),
				DataFunc: func(context UIContext) (interface{}, error) {
					enabled := isPerfEnabled(*context.ProjectRef, pp.Projects)
					return struct {
						Enabled bool `json:"enabled"`
					}{Enabled: enabled}, nil
				},
			},
		},
	}, nil
}

func isPerfEnabled(projectRef model.ProjectRef, projects []string) bool {
	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		return false
	}
	if flags.PluginAdminPageDisabled {
		return model.IsPerfEnabledForProject(projectRef.Id)
	} else {
		return utility.StringSliceContains(projects, projectRef.Id) || utility.StringSliceContains(projects, projectRef.Identifier)
	}
}
