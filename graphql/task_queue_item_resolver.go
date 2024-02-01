package graphql

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
)

// Requester is the resolver for the requester field.
func (r *taskQueueItemResolver) Requester(ctx context.Context, obj *restModel.APITaskQueueItem) (TaskQueueItemType, error) {
	if *obj.Requester != evergreen.RepotrackerVersionRequester {
		return TaskQueueItemTypePatch, nil
	}
	return TaskQueueItemTypeCommit, nil
}

// TaskQueueItem returns TaskQueueItemResolver implementation.
func (r *Resolver) TaskQueueItem() TaskQueueItemResolver { return &taskQueueItemResolver{r} }

type taskQueueItemResolver struct{ *Resolver }
