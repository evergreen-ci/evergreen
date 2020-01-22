package graphql

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/pkg/errors"
)

type Resolver struct {
	sc data.Connector
}

func (r *Resolver) Query() QueryResolver {
	return &queryResolver{r}
}

type patchResolver struct{ *Resolver }

func (r *patchResolver) ID(ctx context.Context, obj *model.APIPatch) (string, error) {
	return *obj.Id, nil
}

type queryResolver struct{ *Resolver }

func (r *queryResolver) UserPatches(ctx context.Context, userID string) ([]*model.APIPatch, error) {
	patchPointers := []*model.APIPatch{}
	patches, err := r.sc.FindPatchesByUser(userID, time.Now(), 10)
	if err != nil {
		return patchPointers, errors.Wrap(err, "error retrieving patches")
	}

	for _, p := range patches {
		patchPointers = append(patchPointers, &p)
	}

	return patchPointers, nil
}

func (r *queryResolver) Task(ctx context.Context, taskID string) (*model.APITask, error) {
	task, err := task.FindOneId(taskID)
	if err != nil {
		return nil, errors.Wrap(err, "Error retreiving Task")
	}
	if task == nil {
		return nil, errors.Errorf("unable to find task %s", taskID)
	}
	apiTask := model.APITask{}
	err = apiTask.BuildFromService(task)
	if err != nil {
		return nil, errors.Wrap(err, "error converting task")
	}
	return &apiTask, nil
}

func (r *queryResolver) TaskTests(ctx context.Context, taskID string, sortCategory TaskSortCategory, sortDirection SortDirection, page int, limit int) ([]*model.APITest, error) {
	task, err := task.FindOneId(taskID)

	if err != nil {
		return nil, errors.Wrap(err, "Error retreiving Task")
	}

	sortBy := ""
	if sortCategory == TaskSortCategoryStatus {
		sortBy = "status"
	}
	if sortCategory == TaskSortCategoryDuration {
		sortBy = "duration"
	}
	if sortCategory == TaskSortCategoryTestName {
		sortBy = "test_name"
	}
	if sortDirection == SortDirectionDesc {
		sortBy = "-" + sortBy
	}
	sortOrder := []string{sortBy}
	tests, err := r.sc.FindTestsByTaskIdSortAndPaginate(taskID, sortOrder, page, limit, task.Execution)
	if err != nil {
		return nil, errors.Wrap(err, "Error retreiving test")
	}
	testPointers := []*model.APITest{}
	for _, t := range tests {
		fmt.Println(t.TestFile)
		apiTest := model.APITest{}
		err := apiTest.BuildFromService(&t)
		if err != nil {
			return nil, errors.Wrap(err, "error converting test")
		}
		testPointers = append(testPointers, &apiTest)
	}
	return testPointers, nil
}

// New injects resources into the resolvers, such as the data connector
func New() Config {
	return Config{
		Resolvers: &Resolver{
			sc: &data.DBConnector{},
		},
	}
}
