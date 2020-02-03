package graphql

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/rest/route"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
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
		return nil, errors.Errorf("could not find project '%s'", identifier)
	}

	usr := route.MustHaveUser(ctx)

	err = usr.AddFavoritedProject(identifier)
	if err != nil {
		return nil, errors.Wrap(err, "error adding project to user's favorites")
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

func (r *queryResolver) UserPatches(ctx context.Context, userID string) ([]*restModel.APIPatch, error) {
	patchPointers := []*restModel.APIPatch{}
	patches, err := r.sc.FindPatchesByUser(userID, time.Now(), 10)
	if err != nil {
		return patchPointers, errors.Wrap(err, "error retrieving patches")
	}

	for _, p := range patches {
		patchPointers = append(patchPointers, &p)
	}

	return patchPointers, nil
}

func (r *queryResolver) Task(ctx context.Context, taskID string) (*restModel.APITask, error) {
	task, err := task.FindOneId(taskID)
	if err != nil {
		return nil, errors.Wrap(err, "Error retreiving Task")
	}
	if task == nil {
		return nil, errors.Errorf("unable to find task %s", taskID)
	}
	apiTask := restModel.APITask{}
	err = apiTask.BuildFromService(task)
	if err != nil {
		return nil, errors.Wrap(err, "error converting task")
	}
	err = apiTask.BuildFromService(r.sc.GetURL())
	if err != nil {
		return nil, errors.Wrap(err, "error converting task")
	}
	return &apiTask, nil
}

func (r *queryResolver) Projects(ctx context.Context) (*Projects, error) {
	allProjs, err := model.FindAllTrackedProjectRefs()
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving projects")
	}

	groupsMap := make(map[string][]*restModel.UIProjectFields)

	usr := GetDBUserFromContext(ctx)

	favoriteIds := usr.FavoriteProjects
	favorites := []*restModel.UIProjectFields{}

	for _, p := range allProjs {
		groupName := strings.Join([]string{p.Owner, p.Repo}, "/")

		uiProj := restModel.UIProjectFields{
			DisplayName: p.DisplayName,
			Identifier:  p.Identifier,
			Repo:        p.Repo,
			Owner:       p.Owner,
		}

		if projs, ok := groupsMap[groupName]; ok {
			groupsMap[groupName] = append(projs, &uiProj)
		} else {
			groupsMap[groupName] = []*restModel.UIProjectFields{&uiProj}
		}

		// if proj ID is in favoriteIds then add proj to favorites
		if util.StringSliceContains(favoriteIds, p.Identifier) {
			favorites = append(favorites, &uiProj)
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
		Favorites:   favorites,
		AllProjects: groupsArr,
	}

	return &pjs, nil
}

func (r *queryResolver) TaskTests(ctx context.Context, taskID string, sortCategory *TaskSortCategory, sortDirection *SortDirection, page *int, limit *int, testName *string, status *string) ([]*restModel.APITest, error) {
	task, err := task.FindOneId(taskID)
	if err != nil {
		return nil, errors.Wrap(err, "Error retreiving Task")
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
		return nil, errors.Wrap(err, "Error retreiving test")
	}

	testPointers := []*restModel.APITest{}
	for _, t := range tests {
		apiTest := restModel.APITest{}
		err := apiTest.BuildFromService(&t)
		if err != nil {
			return nil, errors.Wrap(err, "error converting test")
		}
		testPointers = append(testPointers, &apiTest)
	}
	return testPointers, nil
}

// New injects resources into the resolvers, such as the data connector
func New(apiURL string) Config {
	return Config{
		Resolvers: &Resolver{
			sc: &data.DBConnector{URL: apiURL},
		},
	}
}
