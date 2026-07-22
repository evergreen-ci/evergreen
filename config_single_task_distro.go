package evergreen

import (
	"context"
	"regexp"
	"slices"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

type ProjectTasksPair struct {
	ProjectID string `bson:"project_id" json:"project_id"`
	// IsRegex indicates that ProjectID is a regular expression matching project
	// identifiers rather than an exact project ID or identifier.
	IsRegex      bool     `bson:"is_regex" json:"is_regex"`
	AllowedTasks []string `bson:"allowed_tasks" json:"allowed_tasks"`
	AllowedBVs   []string `bson:"allowed_bvs" json:"allowed_bvs"`
}

// AllowAll returns true if all tasks or build variants are allowed.
func (p *ProjectTasksPair) AllowAll() bool {
	return slices.Contains(p.AllowedBVs, "all") || slices.Contains(p.AllowedTasks, "all")
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
	catcher := grip.NewBasicCatcher()
	for _, pair := range c.ProjectTasksPairs {
		if pair.ProjectID == "" {
			catcher.New("project ID cannot be empty")
			continue
		}
		if pair.IsRegex {
			if _, err := regexp.Compile(pair.ProjectID); err != nil {
				catcher.Wrapf(err, "project '%s' is not a valid regular expression", pair.ProjectID)
			}
		}
	}
	return catcher.Resolve()
}
