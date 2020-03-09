package graphql

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/rest/route"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
)

// GetGroupedFiles returns the files of a Task inside a GroupedFile struct
func GetGroupedFiles(ctx context.Context, name string, taskID string, execution int) (*GroupedFiles, error) {
	taskFiles, err := artifact.GetAllArtifacts([]artifact.TaskIDAndExecution{{TaskID: taskID, Execution: execution}})
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}
	hasUser := gimlet.GetUser(ctx) != nil
	strippedFiles, err := artifact.StripHiddenFiles(taskFiles, hasUser)
	if err != nil {
		return nil, err
	}

	apiFileList := []*restModel.APIFile{}
	for _, file := range strippedFiles {
		apiFile := restModel.APIFile{}
		err := apiFile.BuildFromService(file)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("error stripping hidden files"))
		}
		apiFileList = append(apiFileList, &apiFile)
	}
	return &GroupedFiles{TaskName: &name, Files: apiFileList}, nil
}

func SetScheduled(ctx context.Context, sc data.Connector, taskID string, isActive bool) (*restModel.APITask, error) {
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

// GetFormattedDate returns a time.Time type in the format "Dec 13, 2020, 11:58:04 pm"
func GetFormattedDate(t *time.Time, timezone string) (*string, error) {
	if t == nil {
		return nil, nil
	}

	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, err
	}

	timeInUserTimezone := t.In(loc)
	newTime := fmt.Sprintf("%s %d, %d, %s", timeInUserTimezone.Month(), timeInUserTimezone.Day(), timeInUserTimezone.Year(), timeInUserTimezone.Format(time.Kitchen))

	return &newTime, nil
}

// IsURL returns true if str is a url with scheme and domain name
func IsURL(str string) bool {
	u, err := url.ParseRequestURI(str)
	return err == nil && u.Scheme != "" && u.Host != ""
}

// BaseTaskStatuses represents the format {buildVariant: {displayName: status}} for base task statuses
type BaseTaskStatuses map[string]map[string]string

// GetBaseTaskStatusesFromPatchID gets the status of each base build associated with a task
func GetBaseTaskStatusesFromPatchID(r *queryResolver, patchID string) (BaseTaskStatuses, error) {
	version, err := r.sc.FindVersionById(patchID)
	if err != nil {
		return nil, fmt.Errorf("Error getting version %s: %s", patchID, err.Error())
	}
	if version == nil {
		return nil, fmt.Errorf("No version found for ID %s", patchID)
	}
	baseVersion, err := model.VersionFindOne(model.VersionBaseVersionFromPatch(version.Identifier, version.Revision))
	if err != nil {
		return nil, fmt.Errorf("Error getting base version from version %s: %s", version.Id, err.Error())
	}
	if baseVersion == nil {
		return nil, fmt.Errorf("No base version found from version %s", version.Id)
	}
	baseTasks, err := task.FindTasksFromVersions([]string{baseVersion.Id})
	if err != nil {
		return nil, fmt.Errorf("Error getting tasks from version %s: %s", baseVersion.Id, err.Error())
	}
	if baseTasks == nil {
		return nil, fmt.Errorf("No tasks found for version %s", baseVersion.Id)
	}

	baseTaskStatusesByDisplayNameByVariant := make(map[string]map[string]string)
	for _, task := range baseTasks {
		if _, ok := baseTaskStatusesByDisplayNameByVariant[task.BuildVariant]; !ok {
			baseTaskStatusesByDisplayNameByVariant[task.BuildVariant] = map[string]string{}
		}
		baseTaskStatusesByDisplayNameByVariant[task.BuildVariant][task.DisplayName] = task.Status
	}
	return baseTaskStatusesByDisplayNameByVariant, nil
}

// SchedulePatch schedules a patch and returns the scheduled patch model
func SchedulePatch(ctx context.Context, r *mutationResolver, patchID string) (*patch.Patch, error) {
	patch, err := patch.FindOne(patch.ById(patch.NewId(patchID)))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting patch %s: %s", patchID, err.Error()))
	}

	// Unmarshal the project config and set it in the project context
	project := &model.Project{}
	if _, err = model.LoadProjectInto([]byte(patch.PatchedConfig), patch.Project, project); err != nil {
		grip.Alertf("Error unmarshaling project config: %v", err)
	}

	tasks := model.TaskVariantPairs{}
	if len(patch.VariantsTasks) > 0 {
		tasks = model.VariantTasksToTVPairs(patch.VariantsTasks)
	} else {
		for _, v := range patch.BuildVariants {
			for _, t := range patch.Tasks {
				if project.FindTaskForVariant(t, v) != nil {
					tasks.ExecTasks = append(tasks.ExecTasks, model.TVPair{Variant: v, TaskName: t})
				}
			}
		}
	}

	tasks.ExecTasks = model.IncludePatchDependencies(project, tasks.ExecTasks)
	if err = model.ValidateTVPairs(project, tasks.ExecTasks); err != nil {
		return nil, BadRequest.Send(ctx, fmt.Sprintf("Error validating task variants: %s", err.Error()))
	}

	patch.Activated = true

	if patch.Version != "" {
		// This patch has already been finalized, just add the new builds and tasks
		version, err := r.sc.FindVersionById(patch.Version)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting version for patch: %s", err.Error()))
		}
		if version == nil {
			return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Version not found for patch %s: %s", patch.Version, err.Error()))
		}

		if err := model.AddNewBuildsForPatch(ctx, patch, version, project, tasks); err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error creating new builds for version `%s`", version.Id))
		}
	} else {
		env := evergreen.GetEnvironment()
		githubOauthToken, err := env.Settings().GetGithubOauthToken()
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting github oauth token: %s", err.Error()))
		}

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		requester := patch.GetRequester()
		_, err = model.FinalizePatch(ctx, patch, requester, githubOauthToken)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finalizing patch: %s", err.Error()))
		}

		if patch.IsGithubPRPatch() {
			job := units.NewGithubStatusUpdateJobForNewPatch(patch.Id.Hex())
			queue := env.RemoteQueue()
			if err := queue.Put(ctx, job); err != nil {
				return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error adding github status update job to queue: %s", err.Error()))
			}
		}
	}
	return patch, nil
}
