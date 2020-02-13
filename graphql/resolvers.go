package graphql

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/evergreen-ci/gimlet"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/rest/route"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
	"github.com/vektah/gqlparser/gqlerror"
)

type Resolver struct {
	sc data.Connector
}

func (r *Resolver) Mutation() MutationResolver {
	return &mutationResolver{r}
}
func (r *Resolver) Query() QueryResolver {
	return &queryResolver{r}
}

type mutationResolver struct{ *Resolver }

func (r *mutationResolver) AddFavoriteProject(ctx context.Context, identifier string) (*restModel.UIProjectFields, error) {
	p, err := model.FindOneProjectRef(identifier)
	if err != nil || p == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("could not find project '%s'", identifier))
	}

	usr := route.MustHaveUser(ctx)

	err = usr.AddFavoritedProject(identifier)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}

	return &restModel.UIProjectFields{
		DisplayName: p.DisplayName,
		Identifier:  p.Identifier,
		Repo:        p.Repo,
		Owner:       p.Owner,
	}, nil
}

func (r *mutationResolver) RemoveFavoriteProject(ctx context.Context, identifier string) (*restModel.UIProjectFields, error) {
	p, err := model.FindOneProjectRef(identifier)
	if err != nil || p == nil {
		return nil, &gqlerror.Error{
			Message: fmt.Sprintln("Could not find proj", identifier),
			Extensions: map[string]interface{}{
				"code": "RESOURCE_NOT_FOUND",
			},
		}

	}

	usr := route.MustHaveUser(ctx)

	err = usr.RemoveFavoriteProject(identifier)
	if err != nil {
		return nil, &gqlerror.Error{
			Message: fmt.Sprintln("Error removing project", identifier),
			Extensions: map[string]interface{}{
				"code": "INTERNAL_SERVER_ERROR",
			},
		}
	}

	return &restModel.UIProjectFields{
		DisplayName: p.DisplayName,
		Identifier:  p.Identifier,
		Repo:        p.Repo,
		Owner:       p.Owner,
	}, nil
}

type queryResolver struct{ *Resolver }

type patchResolver struct{ *Resolver }

func (r *patchResolver) ID(ctx context.Context, obj *restModel.APIPatch) (string, error) {
	return *obj.Id, nil
}

func (r *queryResolver) Patch(ctx context.Context, id string) (*restModel.APIPatch, error) {
	patch, err := r.sc.FindPatchById(id)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	return patch, nil
}

func (r *queryResolver) UserPatches(ctx context.Context, userID string) ([]*restModel.APIPatch, error) {
	patchPointers := []*restModel.APIPatch{}
	patches, err := r.sc.FindPatchesByUser(userID, time.Now(), 10)
	if err != nil {
		return patchPointers, InternalServerError.Send(ctx, err.Error())
	}

	for _, p := range patches {
		patchPointers = append(patchPointers, &p)
	}

	return patchPointers, nil
}

func (r *queryResolver) Task(ctx context.Context, taskID string) (*restModel.APITask, error) {
	task, err := task.FindOneId(taskID)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}
	if task == nil {
		return nil, errors.Errorf("unable to find task %s", taskID)
	}
	apiTask := restModel.APITask{}
	err = apiTask.BuildFromService(task)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	err = apiTask.BuildFromService(r.sc.GetURL())
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	return &apiTask, nil
}

func (r *queryResolver) Projects(ctx context.Context) (*Projects, error) {
	allProjs, err := model.FindAllTrackedProjectRefs()
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}

	usr := route.MustHaveUser(ctx)
	groupsMap := make(map[string][]*restModel.UIProjectFields)
	favorites := []*restModel.UIProjectFields{}

	for _, p := range allProjs {
		groupName := strings.Join([]string{p.Owner, p.Repo}, "/")

		uiProj := restModel.UIProjectFields{
			DisplayName: p.DisplayName,
			Identifier:  p.Identifier,
			Repo:        p.Repo,
			Owner:       p.Owner,
		}

		// favorite projects are filtered out and appended to their own array
		if util.StringSliceContains(usr.FavoriteProjects, p.Identifier) {
			favorites = append(favorites, &uiProj)
			continue
		}
		if projs, ok := groupsMap[groupName]; ok {
			groupsMap[groupName] = append(projs, &uiProj)
		} else {
			groupsMap[groupName] = []*restModel.UIProjectFields{&uiProj}
		}
	}

	groupsArr := []*GroupedProjects{}

	for groupName, groupedProjects := range groupsMap {
		gp := GroupedProjects{
			Name:     groupName,
			Projects: groupedProjects,
		}
		groupsArr = append(groupsArr, &gp)
	}

	sort.SliceStable(groupsArr, func(i, j int) bool {
		return groupsArr[i].Name < groupsArr[j].Name
	})

	pjs := Projects{
		Favorites:     favorites,
		OtherProjects: groupsArr,
	}

	return &pjs, nil
}

func (r *queryResolver) TaskTests(ctx context.Context, taskID string, sortCategory *TaskSortCategory, sortDirection *SortDirection, page *int, limit *int, testName *string, status *string) ([]*restModel.APITest, error) {
	task, err := task.FindOneId(taskID)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}

	sortBy := ""
	if sortCategory != nil {
		switch *sortCategory {
		case TaskSortCategoryStatus:
			sortBy = testresult.StatusKey
			break
		case TaskSortCategoryDuration:
			sortBy = "duration"
			break
		case TaskSortCategoryTestName:
			sortBy = testresult.TestFileKey
		}
	}

	sortDir := 1
	if sortDirection != nil {
		switch *sortDirection {
		case SortDirectionDesc:
			sortDir = -1
			break
		}
	}

	if *sortDirection == SortDirectionDesc {
		sortDir = -1
	}

	testNameParam := ""
	if testName != nil {
		testNameParam = *testName
	}
	pageParam := 0
	if page != nil {
		pageParam = *page
	}
	limitParam := 0
	if limit != nil {
		limitParam = *limit
	}
	statusParam := ""
	if status != nil {
		statusParam = *status
	}
	tests, err := r.sc.FindTestsByTaskIdFilterSortPaginate(taskID, testNameParam, statusParam, sortBy, sortDir, pageParam, limitParam, task.Execution)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}

	testPointers := []*restModel.APITest{}
	for _, t := range tests {
		apiTest := restModel.APITest{}
		err := apiTest.BuildFromService(&t)
		if err != nil {
			return nil, InternalServerError.Send(ctx, err.Error())
		}
		testPointers = append(testPointers, &apiTest)
	}
	return testPointers, nil
}

func TestFiles(ctx context.Context, taskID string) ([]*GroupedFiles, error) {
	var err error
	t, err := task.FindOneId(taskID)
	if t.OldTaskId != "" {
		t, err = task.FindOneId(t.OldTaskId)
		if err != nil {
			return nil, ResourceNotFound.Send(ctx, err.Error())
		}
	}

	groupedFilesList := []*GroupedFiles{}

	if t.DisplayOnly {
		for _, execTaskID := range t.ExecutionTasks {
			var execTaskFiles []artifact.File
			execTaskFiles, err = artifact.GetAllArtifacts([]artifact.TaskIDAndExecution{{TaskID: execTaskID, Execution: t.Execution}})
			if err != nil {
				return nil, ResourceNotFound.Send(ctx, err.Error())
			}
			strippedFiles := artifact.StripHiddenFiles(execTaskFiles, gimlet.GetUser(ctx))

			var execTask *task.Task
			execTask, err = task.FindOne(task.ById(execTaskID))
			if err != nil {
				return nil, ResourceNotFound.Send(ctx, err.Error())
			}
			if execTask == nil {
				continue
			}
			apiFilesList := []*
			for _, f := range strippedFiles {
				apiFile := restModel.APIFile{}
				err := apiFile.BuildFromService(f)
				if err != nil {
					return nil, InternalServerError.Send(ctx, err.Error())
				}
				files = append(files, &apiFile)
			}
		}
	}
	modelFiles, err := artifact.GetAllArtifacts([]artifact.TaskIDAndExecution{{TaskID: taskID, Execution: t.Execution}})
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}

	return artifact.StripHiddenFiles(files, context.User), nil
}

func setScheduled(ctx context.Context, sc data.Connector, taskID string, isActive bool) (*restModel.APITask, error) {
	usr := route.MustHaveUser(ctx)
	if err := model.SetActiveState(taskID, usr.Username(), isActive); err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	task, err := task.FindOneId(taskID)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}
	if task == nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}
	apiTask := restModel.APITask{}
	err = apiTask.BuildFromService(task)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	err = apiTask.BuildFromService(sc.GetURL())
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	return &apiTask, nil
}

func (r *mutationResolver) ScheduleTask(ctx context.Context, taskID string) (*restModel.APITask, error) {
	task, err := setScheduled(ctx, r.sc, taskID, true)
	if err != nil {
		return nil, err
	}
	return task, nil
}

func (r *mutationResolver) UnscheduleTask(ctx context.Context, taskID string) (*restModel.APITask, error) {
	task, err := setScheduled(ctx, r.sc, taskID, false)
	if err != nil {
		return nil, err
	}
	return task, nil
}

// New injects resources into the resolvers, such as the data connector
func New(apiURL string) Config {
	return Config{
		Resolvers: &Resolver{
			sc: &data.DBConnector{URL: apiURL},
		},
	}
}
