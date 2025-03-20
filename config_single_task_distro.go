package evergreen

import (
	"context"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

type ProjectTasksPair struct {
	ProjectID    string   `bson:"project_id" json:"project_id"`
	AllowedTasks []string `bson:"allowed_tasks" json:"allowed_tasks"`
	AllowedBVs   []string `bson:"allowed_bvs" json:"allowed_bvs"`
}

func (p *ProjectTasksPair) IsEmpty() bool {
	return len(p.AllowedTasks) == 0 && len(p.AllowedBVs) == 0
}

func (p *ProjectTasksPair) AllowAll() bool {
	for _, bv := range p.AllowedBVs {
		if bv == "all" {
			return true
		}
	}
	for _, task := range p.AllowedTasks {
		if task == "all" {
			return true
		}
	}
	return false
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
