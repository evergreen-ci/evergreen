package plugin

import (
	"fmt"
	"html/template"
	"net/url"

	"github.com/evergreen-ci/evergreen"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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
		webhookConfigured := proj.TaskAnnotationSettings.FileTicketWebHook.Endpoint != ""
		if !webhookConfigured && proj.TicketCreateProject == "" {
			return fmt.Errorf("ticket_create_project and taskAnnotationSettings.FileTicketWebHook endpoint cannot both be blank")
		}
		if !webhookConfigured && len(proj.TicketSearchProjects) == 0 {
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
		// the webhook cannot be used if the default build baron creation and search is configurd
		if webhookConfigured {
			if len(proj.TicketCreateProject) != 0 {
				grip.Error(message.Fields{
					"message":      "The custom file ticket webhook and the build baron TicketCreateProject should not both be configured",
					"project_name": projName})
			}
			if _, err := url.Parse(proj.TaskAnnotationSettings.FileTicketWebHook.Endpoint); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message":      "Failed to parse webhook endpoint for project",
					"project_name": projName}))
			}
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
					enabled := len(bbp.opts.Projects[context.ProjectRef.Id].TicketSearchProjects) > 0
					if !enabled {
						enabled = len(bbp.opts.Projects[context.ProjectRef.Identifier].TicketSearchProjects) > 0
					}
					return struct {
						Enabled bool `json:"enabled"`
					}{enabled}, nil
				},
			},
		},
	}, nil
}
