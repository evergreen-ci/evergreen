package graphql

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	jiraSource = "JIRA"
)

func BbGetConfig(settings *evergreen.Settings) map[string]evergreen.BuildBaronProject {
	bbconf, ok := settings.Plugins["buildbaron"]
	if !ok {
		return nil
	}

	projectConfig, ok := bbconf["projects"]
	if !ok {
		grip.Error("no build baron projects configured")
		return nil
	}

	projects := map[string]evergreen.BuildBaronProject{}
	err := mapstructure.Decode(projectConfig, &projects)
	if err != nil {
		grip.Critical(errors.Wrap(err, "unable to parse bb project config"))
	}

	return projects
}

func BbGetTask(taskId string, execution string) (*task.Task, error) {
	oldId := fmt.Sprintf("%v_%v", taskId, execution)
	t, err := task.FindOneOld(task.ById(oldId))
	if err != nil {
		return t, errors.Wrap(err, "Failed to find task with old Id")
	}
	// if the archived task was not found, we must be looking for the most recent exec
	if t == nil {
		t, err = task.FindOne(task.ById(taskId))
		if err != nil {
			return nil, errors.Wrap(err, "Failed to find task")
		}
	}
	if t == nil {
		return nil, errors.Errorf("No task found for taskId: %s and execution: %s", taskId, execution)
	}
	if t.DisplayOnly {
		t.LocalTestResults, err = t.GetTestResultsForDisplayTask()
		if err != nil {
			return nil, errors.Wrapf(err, "Problem finding test results for display task '%s'", t.Id)
		}
	}
	return t, nil
}
func (js *jiraSuggest) GetTimeout() time.Duration {
	// This function is never called because we are willing to wait forever for the fallback handler
	// to return JIRA ticket results.
	return 0
}

// Suggest returns JIRA ticket results based on the test and/or task name.
func (js *jiraSuggest) Suggest(ctx context.Context, t *task.Task) ([]thirdparty.JiraTicket, error) {
	jql := t.GetJQL(js.bbProj.TicketSearchProjects)

	results, err := js.jiraHandler.JQLSearch(jql, 0, -1)
	if err != nil {
		return nil, err
	}

	return results.Issues, nil
}

type suggester interface {
	Suggest(context.Context, *task.Task) ([]thirdparty.JiraTicket, error)
	GetTimeout() time.Duration
}

type multiSourceSuggest struct {
	jiraSuggester suggester
}

type jiraSuggest struct {
	bbProj      evergreen.BuildBaronProject
	jiraHandler thirdparty.JiraHandler
}

func (mss *multiSourceSuggest) Suggest(t *task.Task) ([]thirdparty.JiraTicket, string, error) {
	tickets, err := mss.jiraSuggester.Suggest(context.TODO(), t)
	return tickets, jiraSource, err
}
