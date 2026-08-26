package loaders

import (
	"context"
	"errors"

	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/vikstrous/dataloadgen"
)

type patchReader struct{}

func (r *patchReader) getPatches(ctx context.Context, patchIDs []string) (map[string]*patch.Patch, error) {
	patches, err := patch.Find(ctx, patch.ByStringIds(patchIDs))
	if err != nil {
		grip.Error(ctx, message.WrapError(err, message.Fields{
			"message": "error fetching patches in dataloader",
		}))
		return nil, &batchError{err: err}
	}

	patchMap := make(map[string]*patch.Patch, len(patches))
	for i := range patches {
		patchMap[patches[i].Id.Hex()] = &patches[i]
	}

	return patchMap, nil
}

// GetPatch returns a single patch by ID efficiently using the dataloader.
// Returns nil if the patch is not found.
func GetPatch(ctx context.Context, patchID string) (*patch.Patch, error) {
	l := For(ctx)
	result, err := l.PatchLoader.Load(ctx, patchID)
	if errors.Is(err, dataloadgen.ErrNotFound) {
		return nil, nil
	}
	return result, err
}

// PreloadPatches enqueues every patch ID into the dataloader's current batch in a
// single synchronous loop, guaranteeing that subsequent GetPatch calls for these
// IDs are served from the loader's thunk cache without any additional MongoDB
// queries. Use this when a resolver knows up front that it will need many patches
// whose loads would otherwise be split across multiple batches due to the
// wait-time window.
//
// Errors are intentionally discarded here; per-key errors are still surfaced to
// the individual GetPatch callers via the cached thunks.
func PreloadPatches(ctx context.Context, patchIDs []string) {
	if len(patchIDs) == 0 {
		return
	}
	l := For(ctx)
	_, _ = l.PatchLoader.LoadAll(ctx, patchIDs)
}
