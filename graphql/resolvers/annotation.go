package resolvers

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"

	gqlError "github.com/evergreen-ci/evergreen/graphql/errors"
	"github.com/evergreen-ci/evergreen/graphql/generated"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
)

func (r *annotationResolver) WebhookConfigured(ctx context.Context, obj *restModel.APITaskAnnotation) (bool, error) {
	t, err := task.FindOneId(*obj.TaskId)
	if err != nil {
		return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error finding task: %s", err.Error()))
	}
	if t == nil {
		return false, gqlError.ResourceNotFound.Send(ctx, "error finding task for the task annotation")
	}
	_, ok, _ := model.IsWebhookConfigured(t.Project, t.Version)
	return ok, nil
}

// Annotation returns generated.AnnotationResolver implementation.
func (r *Resolver) Annotation() generated.AnnotationResolver { return &annotationResolver{r} }

type annotationResolver struct{ *Resolver }
