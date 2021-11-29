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
					bbSettings, ok := BbGetProject(context.ProjectRef.Id, "")
					enabled := ok && len(bbSettings.TicketSearchProjects) > 0
					return struct {
						Enabled bool `json:"enabled"`
					}{enabled}, nil
				},
			},
		},
	}, nil
}

// IsWebhookConfigured retrieves webhook configuration from the project settings.
func IsWebhookConfigured(project string, version string) (evergreen.WebHook, bool, error) {
	projectRef, err := model.FindMergedProjectRef(project, version, true)
	if err != nil || projectRef == nil {
		return evergreen.WebHook{}, false, errors.Errorf("Unable to find merged project ref for project %s", project)
	}
	webHook := projectRef.TaskAnnotationSettings.FileTicketWebhook
	if webHook.Endpoint != "" {
		return webHook, true, nil
	} else {
		return evergreen.WebHook{}, false, nil
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

// BbGetProject retrieves build baron settings from project settings.
// Project page settings takes precedence, otherwise fallback to project config yaml.
// Returns build baron settings and ok if found.
func BbGetProject(projectId string, version string) (evergreen.BuildBaronSettings, bool) {
	projectRef, err := model.FindMergedProjectRef(projectId, version, true)
	if err != nil || projectRef == nil {
		return evergreen.BuildBaronSettings{}, false
	}
	isValid := validateBbProject(projectId, projectRef.BuildBaronSettings)
	if !isValid {
		return evergreen.BuildBaronSettings{}, false
	}
	return projectRef.BuildBaronSettings, true
}

func validateBbProject(projName string, proj evergreen.BuildBaronSettings) bool {
	hasValidationError := false
	webHook, _, _ := IsWebhookConfigured(projName, "")

	webhookConfigured := webHook.Endpoint != ""
	if !webhookConfigured && proj.TicketCreateProject == "" {
		grip.Critical(message.Fields{
			"message":      "ticket_create_project and taskAnnotationSettings.FileTicketWebhook endpoint cannot both be blank",
			"project_name": projName,
		})
		hasValidationError = true
	}
	if !webhookConfigured && len(proj.TicketSearchProjects) == 0 {
		grip.Critical(message.Fields{
			"message":      "ticket_search_projects cannot be empty",
			"project_name": projName,
		})
		hasValidationError = true
	}
	if proj.BFSuggestionServer != "" {
		if _, err := url.Parse(proj.BFSuggestionServer); err != nil {
			grip.Critical(message.WrapError(err, message.Fields{
				"message":      fmt.Sprintf(`Failed to parse bf_suggestion_server for project "%s"`, projName),
				"project_name": projName,
			}))
			hasValidationError = true
		}
		if proj.BFSuggestionUsername == "" && proj.BFSuggestionPassword != "" {
			grip.Critical(message.Fields{
				"message": fmt.Sprintf(`Failed validating configuration for project "%s": `+
					"bf_suggestion_password must be blank if bf_suggestion_username is blank", projName),
				"project_name": projName,
			})
			hasValidationError = true
		}
		if proj.BFSuggestionTimeoutSecs <= 0 {
			grip.Critical(message.Fields{
				"message": fmt.Sprintf(`Failed validating configuration for project "%s": `+
					"bf_suggestion_timeout_secs must be positive", projName),
				"project_name": projName,
			})
			hasValidationError = true
		}
	} else if proj.BFSuggestionUsername != "" || proj.BFSuggestionPassword != "" {
		grip.Critical(message.Fields{
			"message": fmt.Sprintf(`Failed validating configuration for project "%s": `+
				"bf_suggestion_username and bf_suggestion_password must be blank alt_endpoint_url is blank", projName),
			"project_name": projName,
		})
		hasValidationError = true
	} else if proj.BFSuggestionTimeoutSecs != 0 {
		grip.Critical(message.Fields{
			"message": fmt.Sprintf(`Failed validating configuration for project "%s": `+
				"bf_suggestion_timeout_secs must be zero when bf_suggestion_url is blank", projName),
			"project_name": projName,
		})
		hasValidationError = true
	}
	// the webhook cannot be used if the default build baron creation and search is configured
	if webhookConfigured {
		if len(proj.TicketCreateProject) != 0 {
			grip.Critical(message.Fields{
				"message":      "The custom file ticket webhook and the build baron TicketCreateProject should not both be configured",
				"project_name": projName})
			hasValidationError = true
		}
		if _, err := url.Parse(webHook.Endpoint); err != nil {
			grip.Critical(message.WrapError(err, message.Fields{
				"message":      "Failed to parse webhook endpoint for project",
				"project_name": projName}))
			hasValidationError = true
		}
	}
	return !hasValidationError
}
