package git

import (
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
)

func init() {
	plugin.Publish(&PerfPlugin{})
}

var includes = []template.HTML{
	`<script type="text/javascript" src="/plugin/perf/static/js/trend_chart.js"></script>`,
	`<script type="text/javascript" src="/plugin/perf/static/js/perf.js"></script>`,
}

// PerfPlugin displays performance statistics in the UI.
type PerfPlugin struct {
	Projects []string `yaml:"string"`
}

// Name implements Plugin Interface.
func (pp *PerfPlugin) Name() string {
	return "perf"
}

func (pp *PerfPlugin) GetUIHandler() http.Handler { return nil }
func (pp *PerfPlugin) Configure(params map[string]interface{}) error {
	err := mapstructure.Decode(params, pp)
	if err != nil {
		return fmt.Errorf("error decoding %v params: %v", pp.Name(), err)
	}
	return nil
}

func (pp *PerfPlugin) GetPanelConfig() (*plugin.PanelConfig, error) {
	panelHTML, err := ioutil.ReadFile(filepath.Join(plugin.TemplateRoot(pp.Name()), "task_perf_data.html"))
	if err != nil {
		return nil, fmt.Errorf("Can't load panel html file: %v", err)
	}

	return &plugin.PanelConfig{
		Panels: []plugin.UIPanel{
			{
				Includes:  includes,
				Page:      plugin.TaskPage,
				Position:  plugin.PageCenter,
				PanelHTML: template.HTML(panelHTML),
				DataFunc: func(context plugin.UIContext) (interface{}, error) {
					return struct {
						Enabled bool `json:"enabled"`
					}{util.StringSliceContains(pp.Projects, context.ProjectRef.Identifier)}, nil
				},
			},
		},
	}, nil
	return nil, nil
}
