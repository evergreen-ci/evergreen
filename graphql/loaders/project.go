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

func (p *projectReader) getProjects(ctx context.Context, idsOrIdentifiers []string) (map[string]*model.ProjectRef, error) {
	projects, err := model.FindMergedProjectRefsByIdsOrIdentifiers(ctx, idsOrIdentifiers...)
	if err != nil {
		grip.Error(ctx, message.WrapError(err, message.Fields{
			"message": "error fetching projects in dataloader",
		}))
		return nil, &batchError{err: err}
	}

	// A project ref can be requested by either its id or its identifier, so index
	// each result under both so callers get a hit regardless of which they passed.
	projectMap := make(map[string]*model.ProjectRef, len(projects))
	for i := range projects {
		projectMap[projects[i].Id] = &projects[i]
		if projects[i].Identifier != "" {
			projectMap[projects[i].Identifier] = &projects[i]
		}
	}

	return projectMap, nil
}

// GetProject returns a single merged project ref by its id or identifier efficiently
// using the dataloader. Returns nil if the project is not found.
func GetProject(ctx context.Context, idOrIdentifier string) (*model.ProjectRef, error) {
	l := For(ctx)
	result, err := l.ProjectLoader.Load(ctx, idOrIdentifier)
	if errors.Is(err, dataloadgen.ErrNotFound) {
		return nil, nil
	}
	return result, err
}

// PreloadProjects enqueues every project id or identifier into the dataloader's current
// batch in a single synchronous loop, guaranteeing that subsequent GetProject calls for
// these values are served from the loader's thunk cache without any additional MongoDB
// queries. Use this when a resolver knows up front that it will need many projects whose
// loads would otherwise be split across multiple batches due to the wait-time window.
//
// Errors are intentionally discarded here; per-key errors are still surfaced to the
// individual GetProject callers via the cached thunks.
func PreloadProjects(ctx context.Context, idsOrIdentifiers []string) {
	if len(idsOrIdentifiers) == 0 {
		return
	}
	l := For(ctx)
	_, _ = l.ProjectLoader.LoadAll(ctx, idsOrIdentifiers)
}
