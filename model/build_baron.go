package model

import (
	"context"
	"net/url"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const jiraSource = "JIRA"

func (js *JiraSuggest) GetTimeout() time.Duration {
	// This function is never called because we are willing to wait forever for the fallback handler
	// to return JIRA ticket results.
	return 0
}

// Suggest returns JIRA ticket results based on the test and/or task name.
func (js *JiraSuggest) Suggest(ctx context.Context, t *task.Task) ([]thirdparty.JiraTicket, error) {
	jql := t.GetJQL(js.BbProj.TicketSearchProjects)

	results, err := js.JiraHandler.JQLSearch(ctx, jql, 0, 50)
	if err != nil {
		return nil, err
	}

	return results.Issues, nil
}

type Suggester interface {
	Suggest(context.Context, *task.Task) ([]thirdparty.JiraTicket, error)
	GetTimeout() time.Duration
}

type MultiSourceSuggest struct {
	JiraSuggester Suggester
}

type JiraSuggest struct {
	BbProj      evergreen.BuildBaronSettings
	JiraHandler thirdparty.JiraHandler
}

func (mss *MultiSourceSuggest) Suggest(ctx context.Context, t *task.Task) ([]thirdparty.JiraTicket, string, error) {
	tickets, err := mss.JiraSuggester.Suggest(ctx, t)
	return tickets, jiraSource, err
}

// GetBuildBaronSettings retrieves build baron settings from project settings.
// Project page settings takes precedence, otherwise fallback to project config yaml.
// Returns build baron settings and ok if found.
func GetBuildBaronSettings(ctx context.Context, projectId string, version string) (evergreen.BuildBaronSettings, bool) {
	projectRef, err := FindMergedProjectRefSecondary(ctx, projectId, version, true)
	if err != nil || projectRef == nil {
		return evergreen.BuildBaronSettings{}, false
	}
	return projectRef.BuildBaronSettings, true
}

func ValidateBbProject(ctx context.Context, projName string, proj evergreen.BuildBaronSettings, webhook *evergreen.WebHook) error {
	catcher := grip.NewBasicCatcher()
	var err error
	var webhookConfigured bool
	if webhook == nil {
		pRefWebHook, _, err := IsWebhookConfigured(ctx, projName, "")
		if err != nil {
			return errors.Wrapf(err, "retrieving webhook config for project '%s'", projName)
		}
		webhook = &pRefWebHook
		webhookConfigured = webhook != nil && webhook.Endpoint != ""
	}

	if !webhookConfigured && proj.TicketCreateProject == "" && len(proj.TicketSearchProjects) == 0 {
		return nil
	}
	if !webhookConfigured && len(proj.TicketSearchProjects) == 0 {
		catcher.New("Must provide projects to search")
	}
	if !webhookConfigured && proj.TicketCreateProject == "" {
		catcher.Errorf("Must provide project to create tickets for")
	}
	// the webhook cannot be used if the default build baron creation and search is configured
	if webhookConfigured {
		if len(proj.TicketCreateProject) != 0 {
			catcher.Errorf("The custom file ticket webhook and the build baron should not both be configured")
		}
		if _, err = url.Parse(webhook.Endpoint); err != nil {
			catcher.Errorf("Failed to parse webhook endpoint for project")
		}
	}
	return catcher.Resolve()
}

// BuildBaronConfig describes the Build Baron features configured for a project.
type BuildBaronConfig struct {
	SearchConfigured      bool
	TicketCreationDefined bool
}

// GetBuildBaron returns Build Baron configuration and Jira suggestions for a task execution.
func GetBuildBaron(ctx context.Context, taskID string, execution int) (*thirdparty.SearchReturnInfo, BuildBaronConfig, error) {
	bbConfig := BuildBaronConfig{}
	t, err := task.FindOneIdAndExecution(ctx, taskID, execution)
	if err != nil {
		return nil, bbConfig, errors.Wrap(err, "finding task")
	}
	if t == nil {
		return nil, bbConfig, errors.Errorf("no task found for task '%s' and execution %d", taskID, execution)
	}

	bbProj, ok := GetBuildBaronSettings(ctx, t.Project, t.Version)
	if !ok {
		return nil, bbConfig, nil
	}
	bbConfig.SearchConfigured = len(bbProj.TicketSearchProjects) > 0
	bbConfig.TicketCreationDefined = bbProj.TicketCreateProject != ""
	if !bbConfig.SearchConfigured {
		return nil, bbConfig, nil
	}

	if err = t.PopulateTestResults(ctx); err != nil {
		return nil, bbConfig, errors.Wrap(err, "populating test results")
	}

	settings := evergreen.GetEnvironment().Settings()
	jiraHandler, err := thirdparty.NewJiraHandler(*settings.Jira.Export())
	if err != nil {
		return nil, bbConfig, errors.Wrap(err, "creating jira handler")
	}
	jira := &JiraSuggest{bbProj, jiraHandler}
	multiSource := &MultiSourceSuggest{jira}

	jql := t.GetJQL(bbProj.TicketSearchProjects)
	tickets, source, err := multiSource.Suggest(ctx, t)
	if err != nil {
		return nil, bbConfig, errors.Wrap(err, "searching for tickets")
	}

	return &thirdparty.SearchReturnInfo{
		Issues: tickets,
		Search: jql,
		Source: source,
	}, bbConfig, nil
}
