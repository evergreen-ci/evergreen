package data

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	mgo "gopkg.in/mgo.v2"
)

type GenerateConnector struct{}

// GenerateTasks parses JSON files for `generate.tasks` and creates the new builds and tasks.
func (gc *GenerateConnector) GenerateTasks(ctx context.Context, taskID string, jsonBytes []json.RawMessage) error {
	projects, err := ParseProjects(jsonBytes)
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Wrap(err, "error parsing JSON from `generate.tasks`").Error(),
		}
	}
	g := model.MergeGeneratedProjects(projects)
	g.TaskID = taskID

	_, err = util.Retry(
		ctx,
		func() (bool, error) {
			p, v, t, pm, prevConfig, err := g.NewVersion() // nolint
			if err != nil {
				return false, err
			}
			if err = validator.CheckProjectConfigurationIsValid(p); err != nil {
				return false, err
			}
			err = g.Save(p, v, t, pm, prevConfig)
			if err != nil && errors.Cause(err) == mgo.ErrNotFound {
				return true, gimlet.ErrorResponse{
					StatusCode: http.StatusInternalServerError,
					Message:    errors.Wrap(err, "error updating config in `generate.tasks`").Error(),
				}
			}
			return false, nil
		}, 10, 100*time.Millisecond)
	return err
}

func ParseProjects(jsonBytes []json.RawMessage) ([]model.GeneratedProject, error) {
	catcher := grip.NewBasicCatcher()
	var projects []model.GeneratedProject
	for _, f := range jsonBytes {
		p, err := model.ParseProjectFromJSON(f)
		if err != nil {
			catcher.Add(err)
		}
		projects = append(projects, p)
	}
	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}
	return projects, nil
}

type MockGenerateConnector struct{}

func (gc *MockGenerateConnector) GenerateTasks(ctx context.Context, taskID string, jsonBytes []json.RawMessage) error {
	return nil
}
