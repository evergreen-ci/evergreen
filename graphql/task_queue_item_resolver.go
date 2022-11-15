package graphql

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
)

func (r *taskQueueItemResolver) Requester(ctx context.Context, obj *restModel.APITaskQueueItem) (TaskQueueItemType, error) {
	if *obj.Requester != evergreen.RepotrackerVersionRequester {
		return TaskQueueItemTypePatch, nil
	}
	return TaskQueueItemTypeCommit, nil
}

// TaskQueueItem returns TaskQueueItemResolver implementation.
func (r *Resolver) TaskQueueItem() TaskQueueItemResolver { return &taskQueueItemResolver{r} }

type taskQueueItemResolver struct{ *Resolver }
