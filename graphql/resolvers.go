package graphql

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/pkg/errors"
)

type Resolver struct {
	sc data.Connector
}

func (r *Resolver) Patch() PatchResolver {
	return &patchResolver{r}
}
func (r *Resolver) Query() QueryResolver {
	return &queryResolver{r}
}

type patchResolver struct{ *Resolver }

func (r *patchResolver) ID(ctx context.Context, obj *patch.Patch) (string, error) {
	return obj.Id.Hex(), nil
}
func (r *patchResolver) Variants(ctx context.Context, obj *patch.Patch) ([]*string, error) {
	builds := make([]*string, 0)
	for _, b := range obj.BuildVariants {
		builds = append(builds, &b)
	}
	return builds, nil
}
func (r *patchResolver) VariantTasks(ctx context.Context, obj *patch.Patch) ([]*VariantTask, error) {
	variantTasks := []*VariantTask{}
	for _, vt := range obj.VariantsTasks {
		vtasks := make([]*string, 0)
		for _, task := range vt.Tasks {
			vtasks = append(vtasks, &task)
		}
		variantTasks = append(variantTasks, &VariantTask{
			DisplayName: vt.Variant,
			Tasks:       vtasks,
		})
	}
	return variantTasks, nil
}

type queryResolver struct{ *Resolver }

func (r *queryResolver) UserPatches(ctx context.Context, userID string) ([]*patch.Patch, error) {
	patches, err := r.sc.FindPatchesByUser(userID, time.Now(), 10)
	if err != nil {
		return make([]*patch.Patch, 0), errors.New("Error retrieving patches")
	}

	// must convert patches to array of pointers to satisfy generated code return values
	patchPointers := make([]*patch.Patch, len(patches))
	for _, p := range patches {
		patchPointers = append(patchPointers, &p)
	}

	return patchPointers, nil
}

func (r *queryResolver) Task(ctx context.Context, taskID string) (*task.Task, error) {
	task, err := r.sc.FindTaskById(taskID)
	if err != nil {
		return nil, errors.New("Error retreiving Task")
	}
	return task, err
}

// New injects resources into the resolvers, such as the data connector
func New() Config {
	return Config{
		Resolvers: &Resolver{
			sc: &data.DBConnector{},
		},
	}
}
