package plugin

import (
	"fmt"
	"html/template"
	"net/url"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

func init() {
	Publish(&BuildBaronPlugin{})
}

type bbPluginOptions struct {
	Projects map[string]evergreen.BuildBaronSettings
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
		webHook := proj.TaskAnnotationSettings.FileTicketWebHook
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.Wrap(err, "error getting service flags")
		}
		if flags.PluginAdminPageDisabled {
			webHook, _ = IsWebhookConfigured(projName, "")
		}
		webhookConfigured := webHook.Endpoint != ""
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
			if _, err := url.Parse(webHook.Endpoint); err != nil {
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

// IsWebhookConfigured webhook will can be retrieved from project or admin config depending on PluginAdminPageDisabled flag
// if deriving from project config, we first try to retrieve webhook config prom project parser config, otherwise we fallback to project page settings
// version is needed to retrieve last good project config, if version is not available/empty when calling this function we must first retrieve it
func IsWebhookConfigured(project string, version string) (evergreen.WebHook, bool) {
	var webHook evergreen.WebHook
	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		return evergreen.WebHook{}, false
	}
	if flags.PluginAdminPageDisabled {
		if version == "" {
			lastGoodVersion, err := model.FindVersionByLastKnownGoodConfig(project, -1)
			if err == nil && lastGoodVersion == nil {
				version = lastGoodVersion.Id
			}
		}
		parserProject, err := model.ParserProjectFindOneById(version)
		if err != nil {
			return evergreen.WebHook{}, false
		}
		if parserProject != nil && parserProject.TaskAnnotationSettings != nil {
			webHook = parserProject.TaskAnnotationSettings.FileTicketWebHook
		} else {
			projectRef, err := model.FindMergedProjectRef(project)
			if err != nil || projectRef == nil {
				return evergreen.WebHook{}, false
			}
			webHook = projectRef.TaskAnnotationSettings.FileTicketWebHook
		}
	} else {
		bbProject, _ := BbGetProject(evergreen.GetEnvironment().Settings(), project, "")
		webHook = bbProject.TaskAnnotationSettings.FileTicketWebHook
	}
	if webHook.Endpoint != "" {
		return webHook, true
	} else {
		return evergreen.WebHook{}, false
	}
}

func BbGetConfig(settings *evergreen.Settings) map[string]evergreen.BuildBaronSettings {
	bbconf, ok := settings.Plugins["buildbaron"]
	if !ok {
		return nil
	}

	projectConfig, ok := bbconf["projects"]
	if !ok {
		grip.Error("no build baron projects configured")
		return nil
	}

	projects := map[string]evergreen.BuildBaronSettings{}
	err := mapstructure.Decode(projectConfig, &projects)
	if err != nil {
		grip.Critical(errors.Wrap(err, "unable to parse bb project config"))
	}

	return projects
}

// BbGetProject build baron settings will be retrieved from either project or admin config depending on PluginAdminPageDisabled flag
// Project parser config takes precedence, otherwise we fallback to project page settings
// Version is needed to retrieve last good project config, if version is not available/empty when calling this function we must first retrieve it
// Secondary boolean return value signifies if we were able to retrieve build baron config for error handling in places where this function is called.
func BbGetProject(settings *evergreen.Settings, projectId string, version string) (evergreen.BuildBaronSettings, bool) {
	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		return evergreen.BuildBaronSettings{}, false
	}
	if flags.PluginAdminPageDisabled {
		if version == "" {
			lastGoodVersion, err := model.FindVersionByLastKnownGoodConfig(projectId, -1)
			if err == nil && lastGoodVersion != nil {
				version = lastGoodVersion.Id
			}
		}
		parserProject, err := model.ParserProjectFindOneById(version)
		if err != nil {
			return evergreen.BuildBaronSettings{}, false
		}
		if parserProject != nil && parserProject.BuildBaronSettings != nil {
			return *parserProject.BuildBaronSettings, true
		}
		projectRef, err := model.FindMergedProjectRef(projectId)
		if err != nil || projectRef == nil {
			return evergreen.BuildBaronSettings{}, false
		}
		return projectRef.BuildBaronSettings, true
	}
	buildBaronProjects := BbGetConfig(settings)
	bbProject, ok := buildBaronProjects[projectId]
	if !ok {
		// project may be stored under the identifier rather than the ID
		identifier, err := model.GetIdentifierForProject(projectId)
		if err == nil && identifier != "" {
			bbProject, ok = buildBaronProjects[identifier]
		}
	}
	return bbProject, ok
}
