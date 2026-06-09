package loaders

import (
	"context"
	"errors"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/vikstrous/dataloadgen"
)

type projectReader struct{}

func (p *projectReader) getProjects(ctx context.Context, projectIDs []string) (map[string]*model.ProjectRef, error) {
	projects, err := model.FindProjectRefsByIds(ctx, projectIDs...)
	if err != nil {
		grip.Error(ctx, message.WrapError(err, message.Fields{
			"message": "error fetching projects in dataloader",
		}))
		return nil, &batchError{err: err}
	}

	projectMap := make(map[string]*model.ProjectRef, len(projects))
	for i := range projects {
		projectMap[projects[i].Id] = &projects[i]
	}

	return projectMap, nil
}

// GetProject returns a single project by ID efficiently using the dataloader.
// Returns nil if the project is not found.
func GetProject(ctx context.Context, projectID string) (*model.ProjectRef, error) {
	l := For(ctx)
	result, err := l.ProjectLoader.Load(ctx, projectID)
	if errors.Is(err, dataloadgen.ErrNotFound) {
		return nil, nil
	}
	return result, err
}

// PreloadProjects enqueues every project ID into the dataloader's current batch
// in a single synchronous loop, guaranteeing that subsequent GetProject calls
// for these IDs are served from the loader's thunk cache without any additional
// MongoDB queries.
//
// Errors are intentionally discarded here; per-key errors are still surfaced to
// the individual GetProject callers via the cached thunks.
func PreloadProjects(ctx context.Context, projectIDs []string) {
	if len(projectIDs) == 0 {
		return
	}
	l := For(ctx)
	_, _ = l.ProjectLoader.LoadAll(ctx, projectIDs)
}
