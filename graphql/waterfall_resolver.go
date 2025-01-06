package graphql

import (
	"context"

	"github.com/evergreen-ci/evergreen/model"
)

// DisplayStatus is the resolver for the displayStatus field.
func (r *waterfallTaskResolver) DisplayStatus(ctx context.Context, obj *model.WaterfallTask) (string, error) {
	return obj.DisplayStatusCache, nil
}

// WaterfallTask returns WaterfallTaskResolver implementation.
func (r *Resolver) WaterfallTask() WaterfallTaskResolver { return &waterfallTaskResolver{r} }

type waterfallTaskResolver struct{ *Resolver }
