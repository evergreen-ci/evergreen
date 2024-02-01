package graphql

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
)

// WebhookConfigured is the resolver for the webhookConfigured field.
func (r *annotationResolver) WebhookConfigured(ctx context.Context, obj *restModel.APITaskAnnotation) (bool, error) {
	t, err := task.FindOneId(*obj.TaskId)
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("error finding task: %s", err.Error()))
	}
	if t == nil {
		return false, ResourceNotFound.Send(ctx, "error finding task for the task annotation")
	}
	_, ok, _ := model.IsWebhookConfigured(t.Project, t.Version)
	return ok, nil
}

// Annotation returns AnnotationResolver implementation.
func (r *Resolver) Annotation() AnnotationResolver { return &annotationResolver{r} }

type annotationResolver struct{ *Resolver }
