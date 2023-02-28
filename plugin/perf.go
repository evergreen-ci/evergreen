package plugin

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/model"
)

func init() {
	Publish(&PerfPlugin{})
}

// PerfPlugin displays performance statistics in the UI.
type PerfPlugin struct{}

// Name implements Plugin Interface.
func (pp *PerfPlugin) Name() string { return "perf" }

func (pp *PerfPlugin) Configure(map[string]interface{}) error { return nil }

func (pp *PerfPlugin) GetPanelConfig() (*PanelConfig, error) {
	panelHTML, err := os.ReadFile(filepath.Join(TemplateRoot(pp.Name()), "task_perf_data.html"))
	if err != nil {
		return nil, fmt.Errorf("Can't load panel html file: %v", err)
	}

	return &PanelConfig{
		Panels: []UIPanel{
			{
				Includes: []template.HTML{
					`<script type="text/javascript" src="/static/app/perf/perf.js"></script>`,
					`<script type="text/javascript" src="/static/app/common/ApiUtil.js"></script>`,
					`<script type="text/javascript" src="/static/app/common/ApiTaskdata.js"></script>`,
					`<script type="text/javascript" src="/static/thirdparty/numeral.js"></script>`,
				},
				Page:      TaskPage,
				Position:  PageCenter,
				PanelHTML: template.HTML(panelHTML),
				DataFunc: func(context UIContext) (interface{}, error) {
					enabled := model.IsPerfEnabledForProject(context.ProjectRef.Id)
					return struct {
						Enabled bool `json:"enabled"`
					}{Enabled: enabled}, nil
				},
			},
		},
	}, nil
}
