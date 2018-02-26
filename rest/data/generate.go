package data

import (
	"encoding/json"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type GenerateConnector struct{}

// GenerateTasks parses JSON files for `generate.tasks` and creates the new builds and tasks.
func (gc *GenerateConnector) GenerateTasks(taskID string, jsonBytes []json.RawMessage) error {
	projects, err := ParseProjects(jsonBytes)
	if err != nil {
		return errors.Wrap(err, "error parsing JSON from `generate.tasks`")
	}
	g := model.MergeGeneratedProjects(projects)
	g.TaskID = taskID
	return g.AddGeneratedProjectToVersion()
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
