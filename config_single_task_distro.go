package evergreen

import (
	"context"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type ProjectTasksPair struct {
	ProjectID    string   `bson:"project_id" json:"project_id"`
	AllowedTasks []string `bson:"allowed_tasks" json:"allowed_tasks"`
}

type SingleTaskDistroConfig struct {
	ProjectTasksPairs []ProjectTasksPair `bson:"project_tasks_pairs" json:"project_tasks_pairs"`
}

func (c *SingleTaskDistroConfig) SectionId() string { return "single_task_distro" }

func (c *SingleTaskDistroConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *SingleTaskDistroConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			ProjectTasksPairsKey: c.ProjectTasksPairs,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *SingleTaskDistroConfig) ValidateAndDefault() error {
	return nil
}
