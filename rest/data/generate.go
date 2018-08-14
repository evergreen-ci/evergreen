package data

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type GenerateConnector struct{}

// GenerateTasks parses JSON files for `generate.tasks` and creates the new builds and tasks.
func (gc *GenerateConnector) GenerateTasks(taskID string, jsonBytes []json.RawMessage) error {
	projects, err := ParseProjects(jsonBytes)
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Wrap(err, "error parsing JSON from `generate.tasks`").Error(),
		}
	}
	g := model.MergeGeneratedProjects(projects)
	g.TaskID = taskID
	p, v, t, pm, err := g.NewVersion()
	if err != nil {
		return err
	}
	syntaxErrs, err := validator.CheckProjectSyntax(p)
	if err != nil {
		return err
	}
	if len(syntaxErrs) > 0 {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("project syntax is invalid: %s", validator.ValidationErrorsToString(syntaxErrs)),
		}
	}
	semanticErrs := validator.CheckProjectSemantics(p)
	if len(semanticErrs) > 0 {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("project semantics is invalid: %s", validator.ValidationErrorsToString(semanticErrs)),
		}
	}
	return g.Save(p, v, t, pm)
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

func (gc *MockGenerateConnector) GenerateTasks(taskID string, jsonBytes []json.RawMessage) error {
	return nil
}
