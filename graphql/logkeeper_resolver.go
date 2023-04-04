package graphql

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/plank"
)

// Task is the resolver for the task field.
func (r *logkeeperBuildResolver) Task(ctx context.Context, obj *plank.Build) (*model.APITask, error) {
	task, err := getTask(ctx, obj.TaskID, &obj.TaskExecution, r.sc.GetURL())
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Finding task %s for buildId: %s: %s", obj.TaskID, obj.ID, err.Error()))
	}
	return task, nil
}

// LogkeeperBuild returns LogkeeperBuildResolver implementation.
func (r *Resolver) LogkeeperBuild() LogkeeperBuildResolver { return &logkeeperBuildResolver{r} }

type logkeeperBuildResolver struct{ *Resolver }

// !!! WARNING !!!
// The code below was going to be deleted when updating resolvers. It has been copied here so you have
// one last chance to move it out of harms way if you want. There are two reasons this happens:
//   - When renaming or deleting a resolver the old code will be put in here. You can safely delete
//     it when you're done.
//   - You have helper methods in this file. Move them out to keep these resolver files clean.
type logkeeperTestResolver struct{ *Resolver }
