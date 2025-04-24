package graphql

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
)

// WebhookConfigured is the resolver for the webhookConfigured field.
func (r *annotationResolver) WebhookConfigured(ctx context.Context, obj *restModel.APITaskAnnotation) (bool, error) {
	taskID := utility.FromStringPtr(obj.TaskId)
	t, err := task.FindOneId(ctx, taskID)
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("finding task '%s': %s", taskID, err.Error()))
	}
	if t == nil {
		return false, ResourceNotFound.Send(ctx, fmt.Sprintf("task '%s' not found", taskID))
	}
	_, ok, _ := model.IsWebhookConfigured(ctx, t.Project, t.Version)
	return ok, nil
}

// Annotation returns AnnotationResolver implementation.
func (r *Resolver) Annotation() AnnotationResolver { return &annotationResolver{r} }

type annotationResolver struct{ *Resolver }
