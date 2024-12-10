package model

import (
	"context"
	"net/url"
	"strconv"
	"strings"
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

	results, err := js.JiraHandler.JQLSearch(jql, 0, 50)
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

func (mss *MultiSourceSuggest) Suggest(t *task.Task) ([]thirdparty.JiraTicket, string, error) {
	tickets, err := mss.JiraSuggester.Suggest(context.TODO(), t)
	return tickets, jiraSource, err
}

// GetBuildBaronSettings retrieves build baron settings from project settings.
// Project page settings takes precedence, otherwise fallback to project config yaml.
// Returns build baron settings and ok if found.
func GetBuildBaronSettings(projectId string, version string) (evergreen.BuildBaronSettings, bool) {
	projectRef, err := FindMergedProjectRef(projectId, version, true)
	if err != nil || projectRef == nil {
		return evergreen.BuildBaronSettings{}, false
	}
	return projectRef.BuildBaronSettings, true
}

func ValidateBbProject(projName string, proj evergreen.BuildBaronSettings, webhook *evergreen.WebHook) error {
	catcher := grip.NewBasicCatcher()
	var err error
	var webhookConfigured bool
	if webhook == nil {
		pRefWebHook, _, err := IsWebhookConfigured(projName, "")
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
	if proj.BFSuggestionServer != "" {
		if _, err = url.Parse(proj.BFSuggestionServer); err != nil {
			catcher.Errorf("Failed to parse bf_suggestion_server for project '%s'", projName)
		}
		if proj.BFSuggestionUsername == "" && proj.BFSuggestionPassword != "" {
			catcher.Errorf("Failed validating configuration for project '%s': "+
				"bf_suggestion_password must be blank if bf_suggestion_username is blank", projName)
		}
		if proj.BFSuggestionTimeoutSecs <= 0 {
			catcher.Errorf("Failed validating configuration for project '%s': "+
				"bf_suggestion_timeout_secs must be positive", projName)
		}
	} else if proj.BFSuggestionUsername != "" || proj.BFSuggestionPassword != "" {
		catcher.Errorf("Failed validating configuration for project '%s': "+
			"bf_suggestion_username and bf_suggestion_password must be blank when alt_endpoint_url is blank", projName)
	} else if proj.BFSuggestionTimeoutSecs != 0 {
		catcher.Errorf("Failed validating configuration for project '%s': "+
			"bf_suggestion_timeout_secs must be zero when bf_suggestion_url is blank", projName)
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

type buildBaronConfig struct {
	ProjectFound bool
	// if search project is configured, then that's an
	// indication that the build baron is configured
	SearchConfigured      bool
	TicketCreationDefined bool
}

func GetSearchReturnInfo(ctx context.Context, taskId string, exec string) (*thirdparty.SearchReturnInfo, buildBaronConfig, error) {
	bbConfig := buildBaronConfig{}
	t, err := BbGetTask(ctx, taskId, exec)
	if err != nil {
		return nil, bbConfig, err
	}
	settings := evergreen.GetEnvironment().Settings()
	bbProj, ok := GetBuildBaronSettings(t.Project, t.Version)
	if !ok {
		// build baron project not found, meaning it's not configured for
		// either regular build baron or for a custom ticket filing webhook
		return nil, bbConfig, nil
	}

	createProject := bbProj.TicketCreateProject
	if createProject != "" {
		bbConfig.TicketCreationDefined = true
	}
	bbConfig.TicketCreationDefined = false

	// the build baron is configured if the jira search is configured
	if len(bbProj.TicketSearchProjects) <= 0 {
		bbConfig.SearchConfigured = false
		return nil, bbConfig, nil
	}
	bbConfig.SearchConfigured = true

	jiraHandler := thirdparty.NewJiraHandler(*settings.Jira.Export())
	jira := &JiraSuggest{bbProj, jiraHandler}
	multiSource := &MultiSourceSuggest{jira}

	var tickets []thirdparty.JiraTicket
	var source string

	jql := t.GetJQL(bbProj.TicketSearchProjects)
	tickets, source, err = multiSource.Suggest(t)
	if err != nil {
		return nil, bbConfig, errors.Wrap(err, "searching for tickets")
	}

	var featuresURL string
	if bbProj.BFSuggestionFeaturesURL != "" {
		featuresURL = bbProj.BFSuggestionFeaturesURL
		featuresURL = strings.Replace(featuresURL, "{task_id}", taskId, -1)
		featuresURL = strings.Replace(featuresURL, "{execution}", exec, -1)
	} else {
		featuresURL = ""
	}
	bbConfig.ProjectFound = true
	return &thirdparty.SearchReturnInfo{Issues: tickets, Search: jql, Source: source, FeaturesURL: featuresURL}, bbConfig, nil
}

func BbGetTask(ctx context.Context, taskId string, executionString string) (*task.Task, error) {
	execution, err := strconv.Atoi(executionString)
	if err != nil {
		return nil, errors.Wrap(err, "invalid execution number")
	}

	t, err := task.FindOneIdOldOrNew(taskId, execution)
	if err != nil {
		return nil, errors.Wrap(err, "finding task")
	}
	if t == nil {
		return nil, errors.Errorf("no task found for task '%s' and execution %d", taskId, execution)
	}

	if err = t.PopulateTestResults(ctx); err != nil {
		return nil, errors.Wrap(err, "populating test results")
	}

	return t, nil
}
