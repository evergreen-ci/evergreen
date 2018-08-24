package plugin

import (
	"fmt"
	"html/template"
	"net/url"

	"github.com/evergreen-ci/evergreen"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	Publish(&BuildBaronPlugin{})
}

type bbPluginOptions struct {
	Projects map[string]evergreen.BuildBaronProject
}

type BuildBaronPlugin struct {
	opts *bbPluginOptions
}

func (bbp *BuildBaronPlugin) Name() string { return "buildbaron" }

func (bbp *BuildBaronPlugin) Configure(conf map[string]interface{}) error {
	// pull out options needed from config file (JIRA authentication info, and list of projects)
	bbpOptions := &bbPluginOptions{}

	err := mapstructure.Decode(conf, bbpOptions)
	if err != nil {
		return err
	}

	for projName, proj := range bbpOptions.Projects {
		if proj.TicketCreateProject == "" {
			return fmt.Errorf("ticket_create_project cannot be blank")
		}
		if len(proj.TicketSearchProjects) == 0 {
			return fmt.Errorf("ticket_search_projects cannot be empty")
		}
		if proj.BFSuggestionServer != "" {
			if _, err := url.Parse(proj.BFSuggestionServer); err != nil {
				return errors.Wrapf(err, `Failed to parse bf_suggestion_server for project "%s"`, projName)
			}
			if proj.BFSuggestionUsername == "" && proj.BFSuggestionPassword != "" {
				return errors.Errorf(`Failed validating configuration for project "%s": `+
					"bf_suggestion_password must be blank if bf_suggestion_username is blank", projName)
			}
			if proj.BFSuggestionTimeoutSecs <= 0 {
				return errors.Errorf(`Failed validating configuration for project "%s": `+
					"bf_suggestion_timeout_secs must be positive", projName)
			}
		} else if proj.BFSuggestionUsername != "" || proj.BFSuggestionPassword != "" {
			return errors.Errorf(`Failed validating configuration for project "%s": `+
				"bf_suggestion_username and bf_suggestion_password must be blank alt_endpoint_url is blank", projName)
		} else if proj.BFSuggestionTimeoutSecs != 0 {
			return errors.Errorf(`Failed validating configuration for project "%s": `+
				"bf_suggestion_timeout_secs must be zero when bf_suggestion_url is blank", projName)
		}
	}
	bbp.opts = bbpOptions

	return nil
}

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
					_, enabled := bbp.opts.Projects[context.ProjectRef.Identifier]
					return struct {
						Enabled bool `json:"enabled"`
					}{enabled}, nil
				},
			},
		},
	}, nil
}
