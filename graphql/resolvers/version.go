package resolvers

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	gqlError "github.com/evergreen-ci/evergreen/graphql/errors"
	"github.com/evergreen-ci/evergreen/graphql/generated"
	gqlModel "github.com/evergreen-ci/evergreen/graphql/model"
	"github.com/evergreen-ci/evergreen/graphql/resolvers/util"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	werrors "github.com/pkg/errors"
)

func (r *mutationResolver) RemoveItemFromCommitQueue(ctx context.Context, commitQueueID string, issue string) (*string, error) {
	result, err := data.CommitQueueRemoveItem(commitQueueID, issue, gimlet.GetUser(ctx).DisplayName())
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error removing item %s from commit queue %s: %s",
			issue, commitQueueID, err.Error()))
	}
	if result == nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("couldn't remove item %s from commit queue %s", issue, commitQueueID))
	}
	return &issue, nil
}

func (r *mutationResolver) RestartVersions(ctx context.Context, versionID string, abort bool, versionsToRestart []*model.VersionToRestart) ([]*restModel.APIVersion, error) {
	if len(versionsToRestart) == 0 {
		return nil, gqlError.InputValidationError.Send(ctx, "No versions provided. You must provide at least one version to restart")
	}
	modifications := model.VersionModification{
		Action:            evergreen.RestartAction,
		Abort:             abort,
		VersionsToRestart: versionsToRestart,
	}
	err := util.ModifyVersionHandler(ctx, versionID, modifications)
	if err != nil {
		return nil, err
	}
	versions := []*restModel.APIVersion{}
	for _, version := range versionsToRestart {
		if version.VersionId != nil {
			v, versionErr := model.VersionFindOneId(*version.VersionId)
			if versionErr != nil {
				return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error finding version by id %s: %s", *version.VersionId, versionErr.Error()))
			}
			if v == nil {
				return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find version with id %s", *version.VersionId))
			}
			apiVersion := restModel.APIVersion{}
			if err = apiVersion.BuildFromService(v); err != nil {
				return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error building APIVersion from service for `%s`: %s", *version.VersionId, err.Error()))
			}
			versions = append(versions, &apiVersion)
		}
	}
	return versions, nil
}

func (r *queryResolver) BuildVariantsForTaskName(ctx context.Context, projectID string, taskName string) ([]*task.BuildVariantTuple, error) {
	pid, err := model.GetIdForProject(projectID)
	if err != nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Could not find project with id: %s", projectID))
	}
	repo, err := model.FindRepository(pid)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error while getting repository for '%s': %s", projectID, err.Error()))
	}
	if repo == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("could not find repository '%s'", projectID))
	}
	taskBuildVariants, err := task.FindUniqueBuildVariantNamesByTask(pid, taskName, repo.RevisionOrderNumber)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error while getting build variant tasks for task '%s': %s", taskName, err.Error()))
	}
	if taskBuildVariants == nil {
		return nil, nil
	}
	return taskBuildVariants, nil
}

func (r *queryResolver) CommitQueue(ctx context.Context, id string) (*restModel.APICommitQueue, error) {
	commitQueue, err := data.FindCommitQueueForProject(id)
	if err != nil {
		if werrors.Cause(err) == err {
			return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("error finding commit queue for %s: %s", id, err.Error()))
		}
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error finding commit queue for %s: %s", id, err.Error()))
	}
	project, err := data.FindProjectById(id, true, true)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error finding project %s: %s", id, err.Error()))
	}
	if project.CommitQueue.Message != "" {
		commitQueue.Message = &project.CommitQueue.Message
	}
	commitQueue.Owner = &project.Owner
	commitQueue.Repo = &project.Repo

	for i, item := range commitQueue.Queue {
		patchId := ""
		if utility.FromStringPtr(item.Version) != "" {
			patchId = utility.FromStringPtr(item.Version)
		} else if utility.FromStringPtr(item.PatchId) != "" {
			patchId = utility.FromStringPtr(item.PatchId)
		}

		if patchId != "" {
			p, err := data.FindPatchById(patchId)
			if err != nil {
				return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("error finding patch: %s", err.Error()))
			}
			commitQueue.Queue[i].Patch = p
		}
	}
	return commitQueue, nil
}

func (r *queryResolver) HasVersion(ctx context.Context, id string) (bool, error) {
	v, err := model.VersionFindOne(model.VersionById(id))
	if err != nil {
		return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error finding version %s: %s", id, err.Error()))
	}
	if v != nil {
		return true, nil
	}

	if patch.IsValidId(id) {
		p, err := patch.FindOneId(id)
		if err != nil {
			return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error finding patch %s: %s", id, err.Error()))
		}
		if p != nil {
			return false, nil
		}
	}
	return false, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find patch or version %s", id))
}

func (r *queryResolver) MainlineCommits(ctx context.Context, options gqlModel.MainlineCommitsOptions, buildVariantOptions *gqlModel.BuildVariantOptions) (*gqlModel.MainlineCommits, error) {
	projectId, err := model.GetIdForProject(options.ProjectID)
	if err != nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Could not find project with id: %s", options.ProjectID))
	}
	limit := model.DefaultMainlineCommitVersionLimit
	if utility.FromIntPtr(options.Limit) != 0 {
		limit = utility.FromIntPtr(options.Limit)
	}
	requesters := options.Requesters
	if len(requesters) == 0 {
		requesters = evergreen.SystemVersionRequesterTypes
	}
	opts := model.MainlineCommitVersionOptions{
		Limit:           limit,
		SkipOrderNumber: utility.FromIntPtr(options.SkipOrderNumber),
		Requesters:      requesters,
	}

	versions, err := model.GetMainlineCommitVersionsWithOptions(projectId, opts)
	if err != nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Error getting activated versions: %s", err.Error()))
	}

	var mainlineCommits gqlModel.MainlineCommits
	matchingVersionCount := 0

	// We only want to return the PrevPageOrderNumber if a user is not on the first page
	if options.SkipOrderNumber != nil {
		prevPageCommit, err := model.GetPreviousPageCommitOrderNumber(projectId, utility.FromIntPtr(options.SkipOrderNumber), limit, requesters)

		if err != nil {
			// This shouldn't really happen, but if it does, we should return an error and log it
			grip.Warning(message.WrapError(err, message.Fields{
				"message":    "Error getting most recent version",
				"project_id": projectId,
			}))
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error getting most recent mainline commit: %s", err.Error()))
		}

		if prevPageCommit != nil {
			mainlineCommits.PrevPageOrderNumber = prevPageCommit
		}
	}

	index := 0
	versionsCheckedCount := 0

	// We will loop through each version returned from GetMainlineCommitVersionsWithOptions and see if there is a commit that matches the filter parameters (if any).
	// If there is a match, we will add it to the array of versions to be returned to the user.
	// If there are not enough matches to satisfy our limit, we will call GetMainlineCommitVersionsWithOptions again with the next order number to check and repeat the process.
	for matchingVersionCount < limit {
		// If we no longer have any more versions to check break out and return what we have.
		if len(versions) == 0 {
			break
		}
		// If we have checked more versions than the MaxMainlineCommitVersionLimit then break out and return what we have.
		if versionsCheckedCount >= model.MaxMainlineCommitVersionLimit {
			// Return an error if we did not find any versions that match.
			if matchingVersionCount == 0 {
				return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Matching version not found in %d most recent versions", model.MaxMainlineCommitVersionLimit))
			}
			break
		}
		versionsCheckedCount++
		v := versions[index]
		apiVersion := restModel.APIVersion{}
		err = apiVersion.BuildFromService(&v)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error building APIVersion from service: %s", err.Error()))
		}

		// If the version was created before we started caching activation status we must manually verify it and cache that value.
		if v.Activated == nil {
			err = util.SetVersionActivationStatus(&v)
			if err != nil {
				return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error setting version activation status: %s", err.Error()))
			}
		}
		mainlineCommitVersion := gqlModel.MainlineCommitVersion{}
		shouldCollapse := false
		if !utility.FromBoolPtr(v.Activated) {
			shouldCollapse = true
		} else if util.IsPopulated(buildVariantOptions) && utility.FromBoolPtr(options.ShouldCollapse) {
			opts := task.HasMatchingTasksOptions{
				TaskNames: buildVariantOptions.Tasks,
				Variants:  buildVariantOptions.Variants,
				Statuses:  util.GetValidTaskStatusesFilter(buildVariantOptions.Statuses),
			}
			hasTasks, err := task.HasMatchingTasks(v.Id, opts)
			if err != nil {
				return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error checking if version has tasks: %s", err.Error()))
			}
			if !hasTasks {
				shouldCollapse = true
			}
		}
		// If a version matches our filter criteria we append it directly to our returned list of mainlineCommits
		if !shouldCollapse {
			matchingVersionCount++
			mainlineCommits.NextPageOrderNumber = utility.ToIntPtr(v.RevisionOrderNumber)
			mainlineCommitVersion.Version = &apiVersion

		} else {
			// If a version does not match our filter criteria roll up all the unactivated versions that are sequentially near each other into a single MainlineCommitVersion,
			// and then append it to our returned list.
			// If we have any versions already we should check the most recent one first otherwise create a new one
			if len(mainlineCommits.Versions) > 0 {
				lastMainlineCommit := mainlineCommits.Versions[len(mainlineCommits.Versions)-1]

				// If the previous mainlineCommit contains rolled up unactivated versions append the latest RolledUp unactivated version
				if lastMainlineCommit.RolledUpVersions != nil {
					lastMainlineCommit.RolledUpVersions = append(lastMainlineCommit.RolledUpVersions, &apiVersion)
				} else {
					mainlineCommitVersion.RolledUpVersions = []*restModel.APIVersion{&apiVersion}
				}
			} else {
				mainlineCommitVersion.RolledUpVersions = []*restModel.APIVersion{&apiVersion}
			}
		}

		// Only add a mainlineCommit if a new one was added and it's not a modified existing RolledUpVersion
		if mainlineCommitVersion.Version != nil || mainlineCommitVersion.RolledUpVersions != nil {
			mainlineCommits.Versions = append(mainlineCommits.Versions, &mainlineCommitVersion)
		}
		index++
		// If we have exhausted all of our versions we should fetch some more.
		if index == len(versions) && matchingVersionCount < limit {
			skipOrderNumber := versions[len(versions)-1].RevisionOrderNumber
			opts := model.MainlineCommitVersionOptions{
				Limit:           limit,
				SkipOrderNumber: skipOrderNumber,
				Requesters:      requesters,
			}

			versions, err = model.GetMainlineCommitVersionsWithOptions(projectId, opts)
			if err != nil {
				return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Error getting activated versions: %s", err.Error()))
			}
			index = 0
		}
	}
	return &mainlineCommits, nil
}

func (r *queryResolver) TaskNamesForBuildVariant(ctx context.Context, projectID string, buildVariant string) ([]string, error) {
	pid, err := model.GetIdForProject(projectID)
	if err != nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Could not find project with id: %s", projectID))
	}
	repo, err := model.FindRepository(pid)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error while getting repository for '%s': %s", projectID, err.Error()))
	}
	if repo == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("could not find repository '%s'", pid))
	}
	buildVariantTasks, err := task.FindTaskNamesByBuildVariant(pid, buildVariant, repo.RevisionOrderNumber)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error while getting tasks for '%s': %s", buildVariant, err.Error()))
	}
	if buildVariantTasks == nil {
		return []string{}, nil
	}
	return buildVariantTasks, nil
}

func (r *queryResolver) Version(ctx context.Context, id string) (*restModel.APIVersion, error) {
	v, err := model.VersionFindOneId(id)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error while finding version with id: `%s`: %s", id, err.Error()))
	}
	if v == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find version with id: `%s`", id))
	}
	apiVersion := restModel.APIVersion{}
	if err = apiVersion.BuildFromService(v); err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error building APIVersion from service for `%s`: %s", id, err.Error()))
	}
	return &apiVersion, nil
}

func (r *versionResolver) BaseTaskStatuses(ctx context.Context, obj *restModel.APIVersion) ([]string, error) {
	var baseVersion *model.Version
	var err error

	if evergreen.IsPatchRequester(utility.FromStringPtr(obj.Requester)) || utility.FromStringPtr(obj.Requester) == evergreen.AdHocRequester {
		// Get base commit if patch or periodic build.
		baseVersion, err = model.VersionFindOne(model.BaseVersionByProjectIdAndRevision(utility.FromStringPtr(obj.Project), utility.FromStringPtr(obj.Revision)))
	} else {
		// Get previous commit if mainline commit.
		baseVersion, err = model.VersionFindOne(model.VersionByProjectIdAndOrder(utility.FromStringPtr(obj.Project), obj.Order-1))
	}
	if baseVersion == nil || err != nil {
		return nil, nil
	}
	statuses, err := task.GetBaseStatusesForActivatedTasks(*obj.Id, baseVersion.Id)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("getting base version tasks: '%s'", err.Error()))
	}
	return statuses, nil
}

func (r *versionResolver) BaseVersion(ctx context.Context, obj *restModel.APIVersion) (*restModel.APIVersion, error) {
	baseVersion, err := model.VersionFindOne(model.BaseVersionByProjectIdAndRevision(*obj.Project, *obj.Revision))
	if baseVersion == nil || err != nil {
		return nil, nil
	}
	apiVersion := restModel.APIVersion{}
	if err = apiVersion.BuildFromService(baseVersion); err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error building APIVersion from service for `%s`: %s", baseVersion.Id, err.Error()))
	}
	return &apiVersion, nil
}

func (r *versionResolver) BuildVariants(ctx context.Context, obj *restModel.APIVersion, options *gqlModel.BuildVariantOptions) ([]*gqlModel.GroupedBuildVariant, error) {
	// If activated is nil in the db we should resolve it and cache it for subsequent queries. There is a very low likely hood of this field being hit
	if obj.Activated == nil {
		version, err := model.VersionFindOne(model.VersionById(*obj.Id))
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error fetching version: %s : %s", *obj.Id, err.Error()))
		}
		if err = util.SetVersionActivationStatus(version); err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error setting version activation status: %s", err.Error()))
		}
		obj.Activated = version.Activated
	}

	if !utility.FromBoolPtr(obj.Activated) {
		return nil, nil
	}
	groupedBuildVariants, err := util.GenerateBuildVariants(utility.FromStringPtr(obj.Id), *options)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error generating build variants for version %s : %s", *obj.Id, err.Error()))
	}
	return groupedBuildVariants, nil
}

func (r *versionResolver) BuildVariantStats(ctx context.Context, obj *restModel.APIVersion, options *gqlModel.BuildVariantOptions) ([]*task.GroupedTaskStatusCount, error) {
	opts := task.GetTasksByVersionOptions{
		TaskNames:                      options.Tasks,
		Variants:                       options.Variants,
		Statuses:                       options.Statuses,
		IncludeBuildVariantDisplayName: true,
	}
	stats, err := task.GetGroupedTaskStatsByVersion(utility.FromStringPtr(obj.Id), opts)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error getting version task stats: %s", err.Error()))
	}
	return stats, nil
}

func (r *versionResolver) ChildVersions(ctx context.Context, obj *restModel.APIVersion) ([]*restModel.APIVersion, error) {
	if !evergreen.IsPatchRequester(*obj.Requester) {
		return nil, nil
	}
	if err := data.ValidatePatchID(*obj.Id); err != nil {
		return nil, werrors.WithStack(err)
	}
	foundPatch, err := patch.FindOneId(*obj.Id)
	if err != nil {
		return nil, err
	}
	if foundPatch == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("patch with id %s not found", *obj.Id),
		}
	}
	childPatchIds := foundPatch.Triggers.ChildPatches
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Couldn't find a patch with id: `%s` %s", *obj.Id, err.Error()))
	}
	if len(childPatchIds) > 0 {
		childVersions := []*restModel.APIVersion{}
		for _, cp := range childPatchIds {
			// this calls the graphql Version query resolver
			cv, err := r.Query().Version(ctx, cp)
			if err != nil {
				//before erroring due to the version being nil or not found,
				// fetch the child patch to see if it's activated
				p, err := patch.FindOneId(cp)
				if err != nil {
					return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Encountered an error while fetching a child patch: %s", err.Error()))
				}
				if p == nil {
					return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to child patch %s", cp))
				}
				if p.Version != "" {
					//only return the error if the version is activated (and we therefore expect it to be there)
					return nil, gqlError.InternalServerError.Send(ctx, "An unexpected error occurred. Could not find a child version and expected one.")
				}
			}
			if cv != nil {
				childVersions = append(childVersions, cv)
			}
		}
		return childVersions, nil
	}
	return nil, nil
}

func (r *versionResolver) IsPatch(ctx context.Context, obj *restModel.APIVersion) (bool, error) {
	return evergreen.IsPatchRequester(*obj.Requester), nil
}

func (r *versionResolver) Manifest(ctx context.Context, obj *restModel.APIVersion) (*gqlModel.Manifest, error) {
	m, err := manifest.FindFromVersion(*obj.Id, *obj.Project, *obj.Revision, *obj.Requester)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error fetching manifest for version %s : %s", *obj.Id, err.Error()))
	}
	if m == nil {
		return nil, nil
	}
	versionManifest := gqlModel.Manifest{
		ID:              m.Id,
		Revision:        m.Revision,
		Project:         m.ProjectName,
		Branch:          m.Branch,
		IsBase:          m.IsBase,
		ModuleOverrides: m.ModuleOverrides,
	}
	modules := map[string]interface{}{}
	for key, module := range m.Modules {
		modules[key] = module
	}
	versionManifest.Modules = modules

	return &versionManifest, nil
}

func (r *versionResolver) Patch(ctx context.Context, obj *restModel.APIVersion) (*restModel.APIPatch, error) {
	if !evergreen.IsPatchRequester(*obj.Requester) {
		return nil, nil
	}
	apiPatch, err := data.FindPatchById(*obj.Id)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Couldn't find a patch with id: `%s` %s", *obj.Id, err.Error()))
	}
	return apiPatch, nil
}

func (r *versionResolver) PreviousVersion(ctx context.Context, obj *restModel.APIVersion) (*restModel.APIVersion, error) {
	if !evergreen.IsPatchRequester(utility.FromStringPtr(obj.Requester)) {
		previousVersion, err := model.VersionFindOne(model.VersionByProjectIdAndOrder(utility.FromStringPtr(obj.Project), obj.Order-1))
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error finding previous version for `%s`: %s", *obj.Id, err.Error()))
		}
		if previousVersion == nil {
			return nil, nil
		}
		apiVersion := restModel.APIVersion{}
		if err = apiVersion.BuildFromService(previousVersion); err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error building APIVersion from service for `%s`: %s", previousVersion.Id, err.Error()))
		}
		return &apiVersion, nil
	} else {
		return nil, nil
	}
}

func (r *versionResolver) ProjectMetadata(ctx context.Context, obj *restModel.APIVersion) (*restModel.APIProjectRef, error) {
	projectRef, err := model.FindMergedProjectRef(*obj.Project, *obj.Id, false)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("finding project ref for project `%s`: %s", *obj.Project, err.Error()))
	}
	if projectRef == nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("finding project ref for project `%s`: %s", *obj.Project, "Project not found"))
	}
	apiProjectRef := restModel.APIProjectRef{}
	if err = apiProjectRef.BuildFromService(projectRef); err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("building APIProjectRef from service for `%s`: %s", projectRef.Id, err.Error()))
	}
	return &apiProjectRef, nil
}

func (r *versionResolver) Status(ctx context.Context, obj *restModel.APIVersion) (string, error) {
	collectiveStatusArray, err := util.GetCollectiveStatusArray(*obj)
	if err != nil {
		return "", gqlError.InternalServerError.Send(ctx, fmt.Sprintf("getting collective status array: %s", err.Error()))
	}
	status := patch.GetCollectiveStatus(collectiveStatusArray)
	return status, nil
}

func (r *versionResolver) TaskCount(ctx context.Context, obj *restModel.APIVersion) (*int, error) {
	taskCount, err := task.Count(db.Query(task.DisplayTasksByVersion(*obj.Id)))
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error getting task count for version `%s`: %s", *obj.Id, err.Error()))
	}
	return &taskCount, nil
}

func (r *versionResolver) TaskStatusCounts(ctx context.Context, obj *restModel.APIVersion, options *gqlModel.BuildVariantOptions) ([]*task.StatusCount, error) {
	opts := task.GetTasksByVersionOptions{
		IncludeBaseTasks:      false,
		IncludeExecutionTasks: false,
		TaskNames:             options.Tasks,
		Variants:              options.Variants,
		Statuses:              util.GetValidTaskStatusesFilter(options.Statuses),
	}
	if len(options.Variants) != 0 {
		opts.IncludeBuildVariantDisplayName = true // we only need the buildVariantDisplayName if we plan on filtering on it.
	}
	stats, err := task.GetTaskStatsByVersion(*obj.Id, opts)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error getting version task stats: %s", err.Error()))
	}
	result := []*task.StatusCount{}
	for _, c := range stats.Counts {
		count := c
		result = append(result, &count)
	}
	return result, nil
}

func (r *versionResolver) TaskStatuses(ctx context.Context, obj *restModel.APIVersion) ([]string, error) {
	defaultSort := []task.TasksSortOrder{
		{Key: task.DisplayNameKey, Order: 1},
	}
	opts := task.GetTasksByVersionOptions{
		Sorts:                          defaultSort,
		IncludeBaseTasks:               false,
		FieldsToProject:                []string{task.DisplayStatusKey},
		IncludeBuildVariantDisplayName: false,
	}
	tasks, _, err := task.GetTasksByVersion(*obj.Id, opts)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error getting version tasks: %s", err.Error()))
	}
	return util.GetAllTaskStatuses(tasks), nil
}

func (r *versionResolver) TaskStatusStats(ctx context.Context, obj *restModel.APIVersion, options *gqlModel.BuildVariantOptions) (*task.TaskStats, error) {
	opts := task.GetTasksByVersionOptions{
		IncludeBaseTasks:      false,
		IncludeExecutionTasks: false,
		TaskNames:             options.Tasks,
		Variants:              options.Variants,
		Statuses:              util.GetValidTaskStatusesFilter(options.Statuses),
	}
	if len(options.Variants) != 0 {
		opts.IncludeBuildVariantDisplayName = true // we only need the buildVariantDisplayName if we plan on filtering on it.
	}
	stats, err := task.GetTaskStatsByVersion(*obj.Id, opts)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("getting version task status stats: %s", err.Error()))
	}
	return stats, nil
}

func (r *versionResolver) UpstreamProject(ctx context.Context, obj *restModel.APIVersion) (*gqlModel.UpstreamProject, error) {
	v, err := model.VersionFindOneId(utility.FromStringPtr(obj.Id))
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("finding version %s: '%s'", *obj.Id, err.Error()))
	}
	if v == nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Version %s not found", *obj.Id))
	}
	if v.TriggerID == "" || v.TriggerType == "" {
		return nil, nil
	}

	var projectID string
	var upstreamProject *gqlModel.UpstreamProject
	if v.TriggerType == model.ProjectTriggerLevelTask {
		upstreamTask, err := task.FindOneId(v.TriggerID)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("finding upstream task %s: '%s'", v.TriggerID, err.Error()))
		}
		if upstreamTask == nil {
			return nil, gqlError.ResourceNotFound.Send(ctx, "upstream task not found")
		}

		apiTask := restModel.APITask{}
		if err = apiTask.BuildFromArgs(upstreamTask, nil); err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error building APITask from service for `%s`: %s", upstreamTask.Id, err.Error()))
		}

		projectID = upstreamTask.Project
		upstreamProject = &gqlModel.UpstreamProject{
			Revision: upstreamTask.Revision,
			Task:     &apiTask,
		}
	} else if v.TriggerType == model.ProjectTriggerLevelBuild {
		upstreamBuild, err := build.FindOneId(v.TriggerID)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("finding upstream build %s: '%s'", v.TriggerID, err.Error()))
		}
		if upstreamBuild == nil {
			return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Upstream build %s not found", v.TriggerID))
		}

		upstreamVersion, err := model.VersionFindOneId(utility.FromStringPtr(&upstreamBuild.Version))
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("finding upstream version %s: '%s'", *obj.Id, err.Error()))
		}
		if v == nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("upstream version %s not found", *obj.Id))
		}

		apiVersion := restModel.APIVersion{}
		if err = apiVersion.BuildFromService(upstreamVersion); err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("building APIVersion from service for `%s`: %s", upstreamVersion.Id, err.Error()))
		}

		projectID = upstreamVersion.Identifier
		upstreamProject = &gqlModel.UpstreamProject{
			Revision: upstreamBuild.Revision,
			Version:  &apiVersion,
		}
	}
	upstreamProjectRef, err := model.FindBranchProjectRef(projectID)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("finding upstream project, project: %s, error: '%s'", projectID, err.Error()))
	}
	if upstreamProjectRef == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Upstream project %s not found", projectID))
	}

	upstreamProject.Owner = upstreamProjectRef.Owner
	upstreamProject.Repo = upstreamProjectRef.Repo
	upstreamProject.Project = upstreamProjectRef.Identifier
	upstreamProject.TriggerID = v.TriggerID
	upstreamProject.TriggerType = v.TriggerType
	return upstreamProject, nil
}

func (r *versionResolver) VersionTiming(ctx context.Context, obj *restModel.APIVersion) (*gqlModel.VersionTiming, error) {
	v, err := model.VersionFindOneId(*obj.Id)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error finding version `%s`: %s", *obj.Id, err.Error()))
	}
	if v == nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error finding version `%s`: %s", *obj.Id, "Version not found"))
	}
	timeTaken, makespan, err := v.GetTimeSpent()
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error getting timing for version `%s`: %s", *obj.Id, err.Error()))
	}
	// return nil if rounded timeTaken/makespan == 0s
	t := timeTaken.Round(time.Second)
	m := makespan.Round(time.Second)

	var apiTimeTaken restModel.APIDuration
	var apiMakespan restModel.APIDuration
	if t.Seconds() != 0 {
		apiTimeTaken = restModel.NewAPIDuration(t)
	}
	if m.Seconds() != 0 {
		apiMakespan = restModel.NewAPIDuration(m)
	}

	return &gqlModel.VersionTiming{
		TimeTaken: &apiTimeTaken,
		Makespan:  &apiMakespan,
	}, nil
}

// Version returns generated.VersionResolver implementation.
func (r *Resolver) Version() generated.VersionResolver { return &versionResolver{r} }

type versionResolver struct{ *Resolver }
