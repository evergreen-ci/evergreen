package resolvers

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/graphql/generated"
	gqlModel "github.com/evergreen-ci/evergreen/graphql/model"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
)

func (r *taskQueueItemResolver) Requester(ctx context.Context, obj *restModel.APITaskQueueItem) (gqlModel.TaskQueueItemType, error) {
	if *obj.Requester != evergreen.RepotrackerVersionRequester {
		return gqlModel.TaskQueueItemTypePatch, nil
	}
	return gqlModel.TaskQueueItemTypeCommit, nil
}

// TaskQueueItem returns generated.TaskQueueItemResolver implementation.
func (r *Resolver) TaskQueueItem() generated.TaskQueueItemResolver { return &taskQueueItemResolver{r} }

type taskQueueItemResolver struct{ *Resolver }
