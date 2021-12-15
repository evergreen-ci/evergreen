package plugin

import (
	"html/template"

	"github.com/evergreen-ci/evergreen/model"
)

func init() {
	Publish(&BuildBaronPlugin{})
}

type BuildBaronPlugin struct{}

func (bbp *BuildBaronPlugin) Name() string { return "buildbaron" }

func (bbp *BuildBaronPlugin) Configure(map[string]interface{}) error { return nil }

func (bbp *BuildBaronPlugin) GetPanelConfig() (*PanelConfig, error) {
	return &PanelConfig{
		Panels: []UIPanel{
			{
				Page:      TaskPage,
				Position:  PageRight,
				PanelHTML: template.HTML(`<div ng-include="'/static/plugins/buildbaron/partials/task_build_baron.html'"></div>`),
				Includes: []template.HTML{
					template.HTML(`<link href="/static/plugins/buildbaron/css/task_build_baron.css" rel="stylesheet"/>`),
					template.HTML(`<script type="text/javascript" src="/static/plugins/buildbaron/js/task_build_baron.js"></script>`),
				},
				DataFunc: func(context UIContext) (interface{}, error) {
					bbSettings, ok := model.GetBuildBaronSettings(context.ProjectRef.Id, "")
					enabled := ok && len(bbSettings.TicketSearchProjects) > 0
					return struct {
						Enabled bool `json:"enabled"`
					}{enabled}, nil
				},
			},
		},
	}, nil
}
